//
// @file engine_gdb.go
// @brief The GDB engine: the debugger this component ships with.
//
// It is one implementation of the Backend contract and not the only one — that is
// the point of the contract — but it is the one that needs no plugin, because
// every cross toolchain has a GDB and every GDB speaks the Machine Interface.
//
// The translation an editor's requests become is the whole content of this file,
// and it is written as a table rather than spread through a session because it is
// the part worth reading: a breakpoint is `-break-insert`, threads are
// `-thread-info`, a frame is `-stack-list-frames`, a step is `-exec-next`,
// `-exec-step` or `-exec-finish` depending which way it goes. None of that is
// negotiable, and none of it should need reading a session to find out.
//
// What *is* negotiable is which side of the connection starts the program. A
// program running on this machine is started by the debugger, with `-exec-run`. A
// program on a board was started by whatever flashed it, so the debugger connects
// to a probe, writes the program if the session says to, and continues to the
// entry point. A session that gets this backwards hangs on the first Continue, so
// the distinction is in the engine rather than in the caller.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// How long one command may take before the session says the debugger did not
// answer.
//
// Generous, because the first command of a session on a board includes halting a
// core that was running, and a timeout that fires while the probe is negotiating
// reports a session failure for a session that was working.
const commandTimeout = 60 * time.Second

// The GDB engine.
type GDB struct {
	// program is the built program this session is about.
	program string
	// remote is the `host:port` a board is reached at. Empty means the program runs
	// on this machine and there is nothing to connect to.
	remote string
	// load says whether the program is written to the board when the session
	// starts.
	load bool
	// arguments is what the editor said it was launching, kept because the entry
	// point it named is only needed once the breakpoints are in: the order is
	// connect, write, set breakpoints, run, and the run is where a stop-at-entry
	// becomes a breakpoint rather than a hope.
	arguments LaunchArguments

	mu    sync.Mutex
	token int

	// The debugger process and its pipes. MI is spoken on standard input and
	// standard output, which is the only way a GDB speaks it.
	process *debugger

	// incoming carries the records that are answers. Notifications never come
	// through here: they are acted on where they arrive, because a stop can happen
	// while nothing is outstanding at all.
	incoming chan Record

	// handles maps the number an editor holds to the name GDB gave a value or a
	// frame. The editor holds numbers from two sources and cannot tell them apart,
	// so every value this engine makes gets a number of its own.
	handles map[int]string
	// frames are the frames of a stack, by the number an editor holds. A frame
	// reference in the protocol is a number and GDB's is a level, so the mapping
	// lives here rather than being worked out per request.
	frames map[int]frameAddress
	// nextReference hands out the numbers an editor holds.
	nextReference int
	// values is what each scope holds, by the reference the editor holds it under.
	// Held rather than fetched on demand because by the time the editor asks, the
	// frame may have moved on.
	values map[int][]Variable
	// breakpoints are the ones actually set, by the handle the debugger gave, so
	// that replacing the breakpoints in a file can delete the ones that were in it.
	breakpoints map[string]Breakpoint

	notifications chan Notification
	readOnce      sync.Once
	stopped       chan struct{}
	stopOnce      sync.Once
	// lines is what the debugger printed on its own stream — warnings, the reason
	// a symbol is missing, the error from a command that failed — which is not in
	// any answer and is often the whole explanation.
	lines *lineBuffer
	// trace is the whole conversation with the debugger, when one was asked for.
	//
	// Off unless `RHO_DBG_MI_TRACE` names a file, because a session on a board is
	// the one situation where the protocol itself is worth reading, and it is a
	// debugging tool's business to let a person read it rather than to add another
	// program in the middle.
	trace *os.File
}

// frameAddress is where a frame is, as GDB names it: a thread and a level. Neither
// is the frame number an editor holds.
type frameAddress struct {
	thread int
	level  int
}

// debugger is a debugger process and the pipes to speak MI through.
type debugger struct {
	name     string
	program  string
	command  *exec.Cmd
	commands io.WriteCloser
	records  io.ReadCloser
	errors   io.ReadCloser
	once     sync.Once
}

// traceFor is the file a session traces its protocol to, or nil.
//
// Opened once and appended to, so that two sessions on the same machine leave one
// file in the order they happened.
func traceFor(name string) *os.File {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return file
}

// GDBArgs are the arguments a GDB is started with.
//
// `--interpreter=mi2` is what makes the process speak MI at all; the rest turn
// off behaviour that only gets in the way of a session driven by a program rather
// than by a person. Confirmation prompts would hang the session on a write to a
// board that already has that program on it, and paging would hold records in a
// buffer this session never reads.
func GDBArgs() []string {
	return []string{
		"--interpreter=mi2",
		"--nx",
		"--quiet",
		"-ex", "set confirm off",
		"-ex", "set pagination off",
		"-ex", "set height 0",
		"-ex", "set width 0",
		"-ex", "set breakpoint pending on",
		"-ex", "set print frame-arguments all",
		"-ex", "set logging enabled off",
		"-ex", "set backtrace past-main on",
	}
}

// GDBOptions is how a debugger is started for a session.
type GDBOptions struct {
	// Remote is the probe's address, or empty for a program on this machine.
	Remote string
	// Load says whether the program is written to the board at the start.
	Load bool
	// Malloc is the allocator the program is run with, which a cross build needs:
	// GDB's own is the host's, and the host's allocator in a cross program is a
	// fault on the board rather than on this machine.
	Malloc string
}

// StartGDB starts a debugger and brings it into the state a session needs.
//
// Four things happen before anything works: asynchronous reporting is turned on,
// so a stop arrives as a notification rather than as the answer to whatever was
// asked; the program's symbols are loaded, so a file and a line resolve; the
// board is connected to, if there is one; and the program is written to it, if the
// session asked for that rather than attaching to what is already there.
func StartGDB(debuggerName string, program string, options GDBOptions) (*GDB, error) {
	resolved, err := ResolveDebugger(debuggerName)
	if err != nil {
		return nil, fmt.Errorf("a debugging session needs a debugger: %w", err)
	}
	// The executable comes first because that is what a process is: the flags
	// describe it, they are not it.
	argv := append([]string{resolved}, GDBArgs()...)
	// Named by where it was found rather than by what was asked for: a machine that
	// has gdb-multiarch and not arm-none-eabi-gdb should say so in its session's
	// first line, because that is the first thing to check when it behaves
	// differently from somebody else's machine.
	process, err := startDebugger(resolved, append(argv, program)...)
	if err != nil {
		return nil, err
	}

	engine := &GDB{
		program:       program,
		remote:        options.Remote,
		load:          options.Load,
		process:       process,
		incoming:      make(chan Record, 64),
		handles:       map[int]string{},
		frames:        map[int]frameAddress{},
		values:        map[int][]Variable{},
		breakpoints:   map[string]Breakpoint{},
		nextReference: firstReference,
		notifications: make(chan Notification, 64),
		stopped:       make(chan struct{}),
		lines:         &lineBuffer{},
		trace:         traceFor(os.Getenv("RHO_DBG_MI_TRACE")),
	}
	engine.readLoop()

	// `on`, not `true`: gdb's `-gdb-set` takes on/off for a boolean setting and
	// answers `true` with an error — and with asynchronous reporting off it prints
	// a prompt after every command, so the whole session reads as though the
	// debugger were not understanding anything.
	if _, err := engine.Send(Of("-gdb-set").Arg("mi-async").Arg("on")); err != nil {
		engine.Close()
		return nil, err
	}
	if options.Malloc != "" {
		// Through the console interpreter, because `set environment` is a command
		// rather than an MI one and the only way to run one is to ask for it as such.
		if _, err := engine.Send(Of("-interpreter-exec").Arg("console").
			Arg(`set environment MALLOC_PRELOAD=` + options.Malloc)); err != nil {
			engine.Close()
			return nil, err
		}
	}
	if program != "" {
		// The program alone: gdb reads the path as the argument, and answers a
		// `filename` keyword in front of it with "filename: No such file or
		// directory", which is the error for a file it cannot find and the file it
		// was given is the one after it.
		if _, err := engine.Send(With("-file-exec-and-symbols", program)); err != nil {
			engine.Close()
			return nil, fmt.Errorf("load the program: %w", err)
		}
	}
	if options.Remote != "" {
		// `-target-select remote <address>`: two arguments, the kind of connection
		// and where to make it. Sent as one quoted argument it is looked up as the
		// *name* of a target, and gdb answers "Undefined target command", which reads
		// as though the address were wrong rather than as though the kind was
		// missing.
		if _, err := engine.Send(Of("-target-select").Word("remote").Word(options.Remote)); err != nil {
			engine.Close()
			return nil, fmt.Errorf("reach %s: %w", options.Remote, err)
		}
		if options.Load {
			// `load`, not `-exec-run`: on a board the program is written by the
			// debugger and started by continuing to the entry point. `-exec-run` is
			// for a program this machine starts, and a remote target refuses it.
			// Through the console interpreter, because `load` is a command and not
			// an MI one: gdb answers a bare `load` with "Undefined MI command".
			// It writes the program already loaded for its symbols.
			if _, err := engine.Send(Of("-interpreter-exec").Arg("console").Arg("load")); err != nil {
				engine.Close()
				return nil, fmt.Errorf("write the program to the board: %w", err)
			}
		}
	}
	return engine, nil
}

// teardownTimeout is how long a command sent while the session is ending may take.
//
// Short, and deliberately so: everything sent from here is a request to wind up,
// and a request that winds up slowly is the one thing a person pressing stop is
// waiting on. What is left is closed regardless — the process, and the probe
// behind it — so a debugger that does not answer costs a moment rather than a
// session that never ends.
const teardownTimeout = 3 * time.Second

// defaultEntryPoint is where a session that was told to stop at the entry point
// stops when it was not told where that is.
//
// `main` because that is the name every C program has, and this is what a person
// pressing F5 on an embedded program means by "at the start of my code" — the
// reset vector and the startup assembly above it belong to the vendor.
const defaultEntryPoint = "main"

// firstReference is the first number an editor is handed.
//
// High enough not to be a frame level, and low enough to stay readable in a log:
// GDB numbers levels from zero, so anything above two digits is unmistakably one
// of ours.
const firstReference = 1000

// startDebugger runs a debugger with its pipes.
func startDebugger(name string, arguments ...string) (*debugger, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("start %s: nothing to run", name)
	}
	command := exec.Command(arguments[0], arguments[1:]...)
	commands, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	records, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	errors, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	// Its own process group: a debugger started through a shell would leave the
	// shell behind when the session ended, holding the port the next session wants.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	return &debugger{
		name:     name,
		program:  arguments[0],
		command:  command,
		commands: commands,
		records:  records,
		errors:   errors,
	}, nil
}

// Close ends the debugger, and everything it started.
func (d *debugger) Close() error {
	var failure error
	d.once.Do(func() {
		_ = d.commands.Close()
		if d.command.Process != nil {
			if err := d.command.Process.Signal(syscall.SIGTERM); err != nil {
				failure = err
			}
		}
		done := make(chan struct{})
		go func() {
			_ = d.command.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			if d.command.Process != nil {
				_ = d.command.Process.Kill()
			}
			<-done
		}
	})
	return failure
}

/* -------------------------------------------------------------------------- */
/*  Sending                                                                    */
/* -------------------------------------------------------------------------- */

// Send runs one MI command and answers with the record that ended it.
//
// Notifications met on the way are acted on rather than discarded. A stop that
// arrives while a command is outstanding is the stop the editor is about to be
// told about, and dropping it because a command was being answered is how a
// session stops reporting where it is.
func (g *GDB) Send(command Command) (Record, error) {
	return g.SendWithin(commandTimeout, command)
}

// SendWithin runs one MI command, and says how long it may take.
//
// The bound is a parameter because a session that is *ending* cannot wait as long
// as one that is starting: a command sent while the program is still running —
// and `-gdb-exit` is one, because gdb detaches the target before it answers —
// waits for a core that will not stop on its own, and a session whose end waits on
// that is a session whose editor waits with it.
func (g *GDB) SendWithin(within time.Duration, command Command) (Record, error) {
	g.mu.Lock()
	g.token++
	token := strconv.Itoa(g.token)
	g.mu.Unlock()

	line := command.String(token)
	g.tracef("->", line)
	if _, err := io.WriteString(g.process.commands, line+"\n"); err != nil {
		return Record{}, fmt.Errorf("%s: the debugger stopped reading commands: %w", g.process.name, err)
	}

	deadline := time.Now().Add(within)
	for {
		select {
		case record, more := <-g.incoming:
			if !more {
				return Record{}, fmt.Errorf("%s ended before answering %s", g.process.name, command.name)
			}
			g.tracef("<-", renderRecord(record))
			if record.Token != token {
				continue
			}
			// An answer that says it failed *is* a failure. Read here rather than
			// where the record arrives, because a command's answer is the one record
			// that is about the command rather than about the program: a debugger
			// that answers `-var-create` for an expression that is not there with
			// `^error` is telling this session what to tell the editor, and a session
			// that passed that on as an empty value shows a person a blank where their
			// variable is.
			if record.Kind == KindResult && record.Class == "error" {
				return Record{}, fmt.Errorf("%s", errorOf(record))
			}
			return record, nil
		case <-time.After(time.Until(deadline)):
			return Record{}, fmt.Errorf("%s did not answer %s within %s",
				g.process.name, command.name, within)
		case <-g.stopped:
			return Record{}, fmt.Errorf("%s is no longer running", g.process.name)
		}
	}
}

// tracef is one line of the protocol, when one is being traced.
func (g *GDB) tracef(direction string, line string) {
	if g.trace == nil {
		return
	}
	fmt.Fprintf(g.trace, "%-2s %s\n", direction, strings.TrimRight(line, "\r\n"))
}

// renderRecord is a record as it would be written, for the trace.
func renderRecord(record Record) string {
	out := &strings.Builder{}
	if record.Token != "" {
		out.WriteString(record.Token)
	}
	out.WriteString(string(record.Kind))
	out.WriteString(record.Class)
	for name, value := range sortedResults(record.Results) {
		fmt.Fprintf(out, ",%s=%s", name, render(value))
	}
	return out.String()
}

func sortedResults(results Result) map[string]Value {
	// Sorted, because a trace that reorders itself between runs cannot be compared
	// with an earlier one.
	out := map[string]Value{}
	for _, name := range sortedNames(results) {
		out[name] = results[name]
	}
	return out
}

func sortedNames(results Result) []string {
	names := make([]string, 0, len(results))
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// readLoop reads records until the debugger's output ends.
//
// It never stops on a line it cannot parse. A debugger's target stream carries
// what the program under test prints, and a program that prints something
// beginning with a bracket has not broken anything — so an unparsable line becomes
// output rather than the end of the read. This is the single most common way a
// debugger integration dies on an ordinary program.
func (g *GDB) readLoop() {
	g.readOnce.Do(func() {
		go func() {
			defer close(g.incoming)
			reader := bufio.NewReader(g.process.records)
			for {
				line, err := reader.ReadString('\n')
				if strings.TrimSpace(line) != "" {
					g.deliver(line)
				}
				if err != nil {
					if err != io.EOF {
						g.notify(Notification{Kind: NotifyError, Err: err})
					}
					g.stopOnce.Do(func() { close(g.stopped) })
					g.notify(Notification{Kind: NotifyExited, ExitCode: g.exitCode()})
					return
				}
			}
		}()
		// The debugger's own stream is read on its own goroutine and kept, because
		// it is where the warning about a missing symbol goes, and that warning is
		// usually the reason a breakpoint did not verify.
		go func() {
			reader := bufio.NewReader(g.process.errors)
			for {
				line, err := reader.ReadString('\n')
				if strings.TrimSpace(line) != "" {
					text := strings.TrimRight(line, "\r\n")
					g.lines.Write(text)
					g.notify(Notification{Kind: NotifyOutput, Category: OutputStderr, Text: text})
				}
				if err != nil {
					return
				}
			}
		}()
	})
}

func (g *GDB) exitCode() int {
	if g.process.command.ProcessState == nil {
		return 0
	}
	return g.process.command.ProcessState.ExitCode()
}

// deliver parses a line and puts it where it belongs.
func (g *GDB) deliver(line string) {
	if IsPrompt(line) {
		// The debugger's prompt is neither a record nor the program's output, and
		// showing it puts a line in the console for every command.
		return
	}
	record, err := Parse(line)
	if err != nil {
		g.tracef("<x", strings.TrimRight(line, "\r\n"))
		// Not a record. Almost certainly the program's own output, which is
		// forwarded: a person debugging a board is watching for the program's
		// prints as much as for the debugger's answers.
		g.notify(Notification{
			Kind: NotifyOutput, Category: OutputStdout, Text: strings.TrimRight(line, "\r\n"),
		})
		return
	}
	switch record.Kind {
	case KindResult, KindStatus:
		if record.Token == "" {
			// A result with no token answers a command sent outside this session —
			// one of the start-up `-ex` commands. It is not asked for, so it is
			// shown rather than waited for.
			g.report(record)
			return
		}
		select {
		case g.incoming <- record:
		case <-g.stopped:
		}
	default:
		g.report(record)
	}
}

// report acts on a record that is not an answer to an outstanding command.
func (g *GDB) report(record Record) {
	switch record.Kind {
	case KindAsyncStop:
		g.notify(Notification{
			Kind: NotifyStopped,
			Stop: Stop{Kind: stopKindOf(record.Results.String("reason")), Thread: 1},
			Text: record.Results.String("text"),
		})
	case KindNotify:
		switch record.Class {
		case "thread-group-exited":
			g.notify(Notification{Kind: NotifyExited})
		case "thread-created":
			g.notify(Notification{Kind: NotifyContinued})
		}
	case KindTargetOut, KindTargetErr:
		g.notify(Notification{Kind: NotifyOutput, Category: OutputStdout, Text: record.Results.String("output")})
	case KindLogOut:
		g.notify(Notification{Kind: NotifyOutput, Category: OutputConsole, Text: record.Results.String("output")})
	}
	if record.Class == "error" {
		g.notify(Notification{
			Kind: NotifyError,
			Err:  fmt.Errorf("%s: %s", g.process.name, record.Results.String("msg")),
		})
	}
}

// stopKindOf is an MI stop reason as an editor's reason.
//
// The one that matters here is `signal-received`. A Cortex-M fault arrives as a
// signal, and an adapter that reports it as a plain stop sends a person to the
// wrong pane to look for a breakpoint that was never hit.
func stopKindOf(reason string) StopKind {
	switch reason {
	case "breakpoint-hit", "watchpoint-trigger", "hw-watchpoint-trigger", "read-watchpoint-trigger":
		return StopBreakpoint
	case "end-stepping-range", "function-finished", "location-reached":
		return StopStep
	case "entry-point-hit":
		return StopEntry
	case "signal-received":
		return StopException
	default:
		return StopPause
	}
}

// notify passes something the debugger volunteered on.
func (g *GDB) notify(notification Notification) {
	select {
	case g.notifications <- notification:
	case <-g.stopped:
	default:
		// The buffer is full because nobody is reading. Dropped rather than
		// blocking: the read that fills it is the debugger's, and blocking it
		// would stop the debugger answering at all.
	}
}

// Notifications is what the debugger volunteers.
func (g *GDB) Notifications() <-chan Notification { return g.notifications }

// Describe is this engine as a diagnostic shows it.
func (g *GDB) Describe() Description {
	description := Description{
		Name:    g.process.name,
		Kind:    KindDriver,
		Program: g.process.program,
		Source:  "this component",
	}
	if g.remote != "" {
		description.Port = g.remote
	}
	return description
}

// Said is what the debugger printed on its own stream.
func (g *GDB) Said() string { return g.lines.String() }

/* -------------------------------------------------------------------------- */
/*  The translation                                                            */
/* -------------------------------------------------------------------------- */

// Initialize is the handshake, and what this session can do.
//
// Whether breakpoints may be set after the program has started is the one answer
// that differs between a board and a host, and it is a fact about the connection
// rather than about the debugger: on a board the program is already running
// before anybody connects, so a breakpoint can only be set afterwards; on a host
// this component starts the program itself and waits for the editor to say it has
// sent its breakpoints, which is the mode the protocol has for that.
func (g *GDB) Initialize(_ InitializeArguments) (Capabilities, error) {
	return Capabilities{
		SupportsConfigurationDone:         g.remote != "",
		SupportsConditionalBreakpoints:    true,
		SupportsHitConditionalBreakpoints: true,
		SupportsEvaluateForHovers:         true,
		SupportsTerminateRequest:          true,
		SupportTerminateDebuggee:          g.remote == "",
		SupportsSetVariable:               true,
		SupportsMemoryReferences:          true,
	}, nil
}

// Launch says what kind of session this is, and starts nothing.
//
// The starting is `configurationDone` on both sides of the connection, and it is
// there because the order is the contract: connect, write the program, set the
// breakpoints, run. A launch that ran before the breakpoints were in would run past
// every one of them, and the session would look like a debugger whose breakpoints
// do not work.
func (g *GDB) Launch(arguments LaunchArguments) error {
	g.arguments = arguments
	// Nothing is started and nothing is interrupted.
	//
	// The `load` above leaves the core halted at its reset vector, and a session
	// that interrupts a core which is already stopped gets a `SIGINT` reported as a
	// stop of its own — so the editor shows a pause that nothing did, at a line
	// nobody was at, before a single request has been answered. A board that was
	// left running by something else is halted by the probe's own reset on connect,
	// which is where a launch's halt belongs.
	//
	// A host program is started by configurationDone, once the breakpoints are in.
	return nil
}

// run is the half of the session that starts the program, whichever side of the
// connection it is on. Kept as one place so the ordering has one description: the
// difference is `-exec-run` for a program this machine starts and
// `-exec-continue` for one the flash command already put on a core.

// Attach attaches to a program that is already running.
//
// Interrupted, which is the one place a halt is what was asked for: a person
// attaching has said where they are, continuing would step over whatever they
// attached to see, and interrupting a core that is already stopped is a no-op
// rather than the spurious stop a launch would have caused.
func (g *GDB) Attach(_ AttachArguments) error {
	_, err := g.Send(Of("-exec-interrupt"))
	return err
}

// ConfigurationDone is the editor saying it has sent every breakpoint it means to.
//
// On this machine that is when the program starts. Not before: a program started
// before the breakpoints are in runs past all of them, which is the failure a
// launch appears to have when it worked perfectly.
func (g *GDB) ConfigurationDone() error {
	if g.remote != "" {
		// Stopping before the program's own first line is a temporary breakpoint,
		// not a flag: gdb is told where to stop, and the breakpoint goes away by
		// itself when it is hit, which is what "stop at the entry point" means.
		entry := g.arguments.RunToEntryPoint
		if entry == "" && g.arguments.StopAtEntry {
			entry = defaultEntryPoint
		}
		if entry != "" {
			// Reported as unverified rather than refused: a program whose entry point
			// is named differently is a program this session can still debug, and a
			// refusal would stop the session over a name.
			if _, err := g.Send(Of("-tbreak").Word(entry)); err != nil {
				g.notify(Notification{Kind: NotifyOutput, Category: OutputConsole,
					Text: fmt.Sprintf("no entry point named %q, so the program will run to its first breakpoint\n", entry)})
			}
		}
		_, err := g.Send(Of("-exec-continue"))
		return err
	}
	// On this machine nothing has started the program, so this is where it starts —
	// after the editor has said it has sent every breakpoint it means to, which is
	// the whole reason that request exists.
	_, err := g.Send(Of("-exec-run"))
	return err
}

// SetBreakpoints replaces the breakpoints in one file.
//
// `-break-insert` with a location is a breakpoint: the file and line as the
// editor gave them, with the condition and the ignore count beside them when the
// editor sent them. Pending breakpoints are on, so a file that is not in the
// program's symbols yet still becomes one — and is answered as unverified rather
// than refused, which is what lets an editor grey it out instead of showing an
// error.
func (g *GDB) SetBreakpoints(file string, wanted []Wanted) ([]Breakpoint, error) {
	// Deleted first: the protocol says this request replaces every breakpoint in
	// the source, so accumulating would keep breakpoints a person has removed, and
	// would leave two breakpoints on one line where the editor shows one.
	for handle, set := range g.breakpoints {
		if set.Source != file {
			continue
		}
		if _, err := g.Send(With("-break-delete", handle)); err != nil {
			return nil, err
		}
		delete(g.breakpoints, handle)
	}

	set := make([]Breakpoint, 0, len(wanted))
	for index, one := range wanted {
		answer, err := g.insert(file, one)
		if err != nil {
			// One that could not be set is answered unverified with the reason. The
			// rest are still set: refusing the whole request would leave a
			// breakpoint a person asked for unset, and say nothing about which of
			// them worked.
			set = append(set, Breakpoint{
				ID: index + 1, Verified: false, Message: err.Error(),
				Source: file, Line: one.Line,
			})
			continue
		}
		set = append(set, answer)
	}
	return set, nil
}

// insert sets one breakpoint.
func (g *GDB) insert(file string, one Wanted) (Breakpoint, error) {
	// Options first, then the location, in the one form gdb reads: `file:line`
	// rather than the `-source`/`-line` pair, which gdb has no options for. The
	// order is not a detail — gdb answers a command whose options follow its
	// location with "Garbage following <location>", which is a sentence that means
	// nothing about what was wrong with it.
	command := Of("-break-insert")
	if one.Condition != "" {
		command = command.Opt("-c", one.Condition)
	}
	if one.HitCount > 0 {
		command = command.Opt("-i", strconv.Itoa(one.HitCount))
	}
	command = command.Word(Location(file, one.Line))
	answer, err := g.Send(command)
	if err != nil {
		return Breakpoint{}, err
	}

	// The number is inside the breakpoint the answer reports, not beside it: a
	// debugger that has set one says `^done,bkpt={number="1",...}`. It is read from
	// the tuple first and from the top level second, because a pending breakpoint —
	// one waiting for a program that has not loaded it — has no number at all.
	located, _ := answer.Results.Tuple("bkpt")
	number, hasNumber := 0, false
	if located != nil {
		number, hasNumber = located.Int("number")
	}
	if !hasNumber {
		number, hasNumber = answer.Results.Int("number")
	}
	set := Breakpoint{ID: number, Verified: hasNumber && number > 0, Source: file, Line: one.Line}
	if located != nil {
		if line, found := located.Number("line"); found {
			set.Line = line
		}
		if full := located.Text("fullname"); full != "" {
			set.Source = full
		} else if name := located.Text("file"); name != "" {
			set.Source = name
		}
		if column, found := located.Number("column"); found {
			set.Column = column
		}
	}
	if handle := answer.Results.String("bkpt"); handle != "" {
		g.breakpoints[handle] = set
		if !set.Verified {
			set.Message = "waiting for the program to load it"
		}
	} else {
		set.Verified = false
		set.Message = "the debugger did not report a breakpoint for it"
	}
	return set, nil
}

// Threads are the threads the program has.
//
// `-thread-info` is the one that reports the current thread as well, which
// `-thread-list-ids` does not; an editor's thread picker wants the current one.
func (g *GDB) Threads() ([]Thread, error) {
	if _, err := g.Send(Of("-thread-select").Num(1)); err != nil {
		// Not fatal: a program with one thread does not need it, and refusing to
		// list the threads of a program that has not started would be worse.
		_ = err
	}
	answer, err := g.Send(Of("-thread-info"))
	if err != nil {
		return nil, err
	}
	listed, _ := answer.Results.List("threads")
	threads := make([]Thread, 0, len(listed))
	for _, entry := range listed {
		tuple, isTuple := entry.(Tuple)
		if !isTuple {
			continue
		}
		id, _ := tuple.Number("id")
		name := tuple.Text("name")
		if name == "" {
			name = fmt.Sprintf("thread %d", id)
		}
		threads = append(threads, Thread{ID: id, Name: name})
	}
	if len(threads) == 0 {
		// A program that has not started has no threads to list, which is not the
		// same as one with none. Answering with the one the debugger will use when
		// it does start gives an editor something to ask about.
		threads = append(threads, Thread{ID: 1, Name: "thread 1"})
	}
	return threads, nil
}

// StackTrace are the frames of a thread, innermost first.
//
// `-stack-list-frames` numbers the frame it is in as zero, so the level of each
// frame is worked out from its position rather than taken from the answer. The
// numbers an editor holds are this component's own: the editor asks about a frame
// by a number and a level is not one, and a frame number that changed meaning
// between two requests would make a stale view show a different function.
func (g *GDB) StackTrace(thread int, levels Range) ([]Frame, error) {
	// `-stack-list-frames` takes two numbers and nothing else: no `thread`, and no
	// `low-frame=`/`high-frame=` pairs. Which thread it is is chosen with
	// `-thread-select`, and the range is positional.
	command := Of("-stack-list-frames")
	if levels.Skip > 0 || levels.Frames > 0 {
		command = command.Num(levels.Skip)
		if levels.Frames > 0 {
			command = command.Num(levels.Skip + levels.Frames)
		}
	}
	answer, err := g.Send(command)
	if err != nil {
		return nil, err
	}
	listed, _ := answer.Results.List("stack")
	frames := make([]Frame, 0, len(listed))
	for index, entry := range listed {
		tuple, isTuple := entry.(Tuple)
		if !isTuple {
			continue
		}
		// A frame comes back as a named entry — `frame={...}` — rather than as a
		// bare tuple, because a stack list may hold other kinds of entry too. Read
		// through the name, falling back to the entry itself for a debugger that
		// sends it bare.
		inner, hasInner := tuple.Nested("frame")
		if !hasInner {
			inner = tuple
		}
		line, _ := inner.Number("line")
		file := inner.Text("file")
		if full := inner.Text("fullname"); full != "" {
			file = full
		}
		frame := Frame{
			ID:     g.reference(),
			Name:   inner.Text("func"),
			Line:   line,
			Source: file,
		}
		g.frames[frame.ID] = frameAddress{thread: threadOf(thread), level: levels.Skip + index}
		frames = append(frames, frame)
	}
	return frames, nil
}

// Scopes are what can be read in a frame: registers, arguments, locals.
//
// Three, and not one, because an editor shows them in three panes and a session
// answering with a single "variables" scope makes a person hunt through a flat
// list for a core register. Each is fetched with the command that fetches it —
// `-data-list-register-names` with `-data-list-register-values` for the
// registers, `-stack-list-arguments` and `-stack-list-locals` for the rest — and
// each gets a reference of its own so they can be expanded independently.
func (g *GDB) Scopes(frame int) ([]Scope, error) {
	// The thread is selected rather than named: `-stack-list-locals` has no
	// `thread` option, and gdb answers one it does not have with a usage message.
	if address, known := g.frames[frame]; known && address.thread > 0 {
		_, _ = g.Send(Of("-thread-select").Num(address.thread))
	}

	registers, err := g.registers()
	if err != nil {
		return nil, err
	}
	arguments, err := g.variablesOf(With("-stack-list-arguments", "0"), frame)
	if err != nil {
		// A frame with no arguments is a frame that has not been set up yet. The
		// scope is still answered, with nothing in it.
		arguments = nil
	}
	locals, err := g.variablesOf(With("-stack-list-locals", "0"), frame)
	if err != nil {
		locals = nil
	}

	return []Scope{
		g.scope(registers, "Registers", "registers"),
		g.scope(arguments, "Arguments", "arguments"),
		g.scope(locals, "Locals", "locals"),
	}, nil
}

// registers is the register file, fetched once per frame and read as a whole.
func (g *GDB) registers() ([]Variable, error) {
	answer, err := g.Send(Of("-data-list-register-names"))
	if err != nil {
		return nil, err
	}
	// The format is a required argument, and `x` is hexadecimal — which is what a
	// register view reads. Sent without it, gdb answers with a usage message and the
	// scope comes back empty.
	values, err := g.Send(With("-data-list-register-values", "x"))
	if err != nil {
		return nil, err
	}
	names, _ := answer.Results.List("register-names")
	contents, _ := values.Results.List("register-values")
	variables := make([]Variable, 0, len(names))
	for index, entry := range names {
		name, _ := entry.(string)
		// GDB names a register by number when it has none of its own, and a row of
		// unnamed numbers is not something a person can read a core register out
		// of, so those are left out.
		if name == "" || isNumber(name) {
			continue
		}
		value := ""
		if index < len(contents) {
			if tuple, isTuple := contents[index].(Tuple); isTuple {
				value = tuple.Text("value")
			}
		}
		variables = append(variables, Variable{
			Name: name, Value: value, PresentationHint: "data",
		})
	}
	return variables, nil
}

// variablesOf is the names of a frame's variables, which is what a scope's count
// needs and what its children are made from.
//
// The names are fetched without their values on purpose: a scope that reported
// every value when it was opened would read the whole of a deep stack's locals on
// every click of a frame, which on a board is a slow operation over a debug
// connection.
func (g *GDB) variablesOf(command Command, frame int) ([]Variable, error) {
	_ = frame
	answer, err := g.Send(command)
	if err != nil {
		return nil, err
	}
	listed, _ := answer.Results.List("variables")
	variables := make([]Variable, 0, len(listed))
	for _, entry := range listed {
		tuple, isTuple := entry.(Tuple)
		if !isTuple {
			continue
		}
		variables = append(variables, Variable{
			Name:            tuple.Text("name"),
			Type:            tuple.Text("type"),
			Value:           tuple.Text("value"),
			MemoryReference: addressOf(tuple),
		})
	}
	return variables, nil
}

// scope is a scope holding what was fetched for it.
//
// The values are held here rather than fetched when the editor asks for them: by
// the time it asks, the frame may have moved, and a list of locals read from a
// frame that is no longer there is a list of the wrong locals.
func (g *GDB) scope(variables []Variable, name string, hint string) Scope {
	reference := g.reference()
	g.handles[reference] = ""
	g.values[reference] = variables
	return Scope{
		Name:               name,
		PresentationHint:   hint,
		VariablesReference: reference,
		NamedVariables:     len(variables),
	}
}

// Variables are what a scope or a structure holds.
func (g *GDB) Variables(reference int) ([]Variable, error) {
	variables, found := g.values[reference]
	if !found {
		return nil, fmt.Errorf("nothing is held under reference %d", reference)
	}
	return variables, nil
}

// Evaluate is the value of an expression in a frame.
//
// `-var-evaluate-expression` rather than `-data-evaluate-expression`, because the
// first gives back a handle as well as a value, and a handle is what a watch
// window and a hover need: both ask again later, and a value read by the second
// form has to be evaluated again in a frame that has since changed.
func (g *GDB) Evaluate(expression string, frame int, reference int) (Evaluation, error) {
	// `-var-create NAME FRAME EXPRESSION`, in that order and with no keywords: gdb
	// answers `-var-create: Usage: NAME FRAME EXPRESSION.` when given anything else.
	// The frame is a number when the editor named one and `*` — the current frame —
	// when it did not.
	name := fmt.Sprintf("rho%d", reference)
	frameOf := "*"
	if address, known := g.frames[frame]; known {
		frameOf = strconv.Itoa(address.level)
		if address.thread > 0 {
			_, _ = g.Send(Of("-thread-select").Num(address.thread))
		}
	}
	command := Of("-var-create").Arg(name).Arg(frameOf).Arg(expression)
	answer, err := g.Send(command)
	if err != nil {
		return Evaluation{}, err
	}
	handle := answer.Results.String("name")
	evaluation := Evaluation{
		Value:        answer.Results.String("value"),
		Type:         answer.Results.String("type"),
		EvaluateName: expression,
	}
	if children, _ := answer.Results.Int("numchild"); children > 0 {
		evaluation.VariablesReference = g.reference()
		g.handles[evaluation.VariablesReference] = handle
	}
	return evaluation, nil
}

// Resume lets a thread carry on.
func (g *GDB) Resume(thread int) error {
	// The thread is selected rather than named: `-exec-continue` has no `thread`
	// option, and answers one it does not have with a usage message.
	if _, err := g.Send(Of("-thread-select").Num(threadOf(thread))); err != nil {
		return err
	}
	_, err := g.Send(Of("-exec-continue"))
	return err
}

// Step advances a thread.
//
// Three commands for three directions, and the mapping is the one thing about
// stepping that is not interchangeable: `-exec-next` steps over a call,
// `-exec-step` steps into one, and `-exec-finish` runs until the frame it is in
// returns.
func (g *GDB) Step(thread int, kind StepKind) error {
	if _, err := g.Send(Of("-thread-select").Num(threadOf(thread))); err != nil {
		return err
	}
	var command Command
	switch kind {
	case StepInto:
		command = Of("-exec-step")
	case StepOut:
		command = Of("-exec-finish")
	default:
		command = Of("-exec-next")
	}
	_, err := g.Send(command)
	return err
}

// Pause interrupts a thread that is running.
func (g *GDB) Pause(thread int) error {
	if _, err := g.Send(Of("-thread-select").Num(threadOf(thread))); err != nil {
		return err
	}
	_, err := g.Send(Of("-exec-interrupt"))
	return err
}

// ReadMemory is what is at an address, as hexadecimal bytes.
func (g *GDB) ReadMemory(address uint64, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("no length was given to read")
	}
	// Positional: an address and a length, in that order.
	answer, err := g.Send(Of("-data-read-memory-bytes").
		Arg(fmt.Sprintf("0x%x", address)).Num(length))
	if err != nil {
		return "", err
	}
	// The answer is a list of blocks, one per region the read crossed. They are
	// joined rather than the first taken: a read across a boundary comes back in
	// more than one, and showing half a register is worse than showing it on two
	// lines.
	listed, _ := answer.Results.List("memory")
	blocks := []string{}
	for _, entry := range listed {
		tuple, isTuple := entry.(Tuple)
		if !isTuple {
			continue
		}
		if contents := tuple.Text("contents"); contents != "" {
			blocks = append(blocks, contents)
		}
	}
	if len(blocks) == 0 {
		return "", fmt.Errorf("nothing could be read at 0x%x", address)
	}
	return strings.Join(blocks, ""), nil
}

// Disconnect ends the session.
//
// What happens to the program depends on which side of the connection it is on. A
// program on a board was started by whatever flashed it and outlives the session;
// a program on this machine was started here, and leaving it running with nobody
// watching is a process nobody will ever look at again.
func (g *GDB) Disconnect(terminate bool) error {
	// Halted first, so that the debugger can finish what it has to do.
	//
	// `-gdb-exit` detaches the target before it answers, and a core that is still
	// running does not detach on its own: the answer arrives when the program stops,
	// and a session whose end waits for that is a session a person cannot end. A
	// halt here is free — the session is over either way, and what the board does
	// afterwards is the board's own business, which is what `terminate` is about.
	_, _ = g.SendWithin(teardownTimeout, Of("-exec-interrupt"))

	command := Of("-gdb-exit")
	if terminate && g.remote == "" {
		// A program on a board was started by whatever flashed it and outlives the
		// session; a program on this machine was started here and is killed rather
		// than left running with nobody watching.
		command = Of("-exec-abort")
	}
	if _, err := g.SendWithin(teardownTimeout, command); err != nil {
		// Ending the session must not fail because the debugger was already gone, or
		// slow about it. The process is closed either way, and that is what ends
		// this.
		return nil
	}
	return nil
}

// Close releases the debugger.
func (g *GDB) Close() error {
	if g.process == nil {
		return nil
	}
	return g.process.Close()
}

/* -------------------------------------------------------------------------- */
/*  Numbers                                                                    */
/* -------------------------------------------------------------------------- */

// reference hands out the number an editor holds for a frame, a scope or a value.
//
// High and increasing. GDB numbers its frames from zero, and the editor holds
// numbers from two sources — its own frame numbers and these — with no way to tell
// them apart, so a number above two digits is deliberately unmistakable.
func (g *GDB) reference() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextReference++
	return g.nextReference
}

func threadOf(thread int) int {
	if thread > 0 {
		return thread
	}
	return 1
}

// addressOf is a variable's address, where it has one. Zero for one the debugger
// kept in a register bank rather than in memory.
func addressOf(value Tuple) uint64 {
	if address, found := value.Int("addr"); found && address > 0 {
		return uint64(address)
	}
	return 0
}

func isNumber(value string) bool {
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return value != ""
}

// errorOf is what a `^error` record says went wrong.
//
// gdb puts the reason in `msg` when it has one, and the record's class is what it
// is when it does not — so both are read rather than only the first, because a
// failure with no message is a failure nobody can act on.
func errorOf(record Record) string {
	if message := strings.TrimSpace(record.Results.String("msg")); message != "" {
		return message
	}
	if class := record.Class; class != "" {
		return class
	}
	return "the debugger reported an error"
}
