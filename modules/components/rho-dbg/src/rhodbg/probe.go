//
// @file probe.go
// @brief Starting and stopping the things a session runs.
//
// A debugging session is three processes, and only one of them is a debugger:
// something has to hold the connection to the board, and that something is not
// the debugger. OpenOCD, pyOCD and probe-rs each do it differently, and a
// session that does not start the right one connects to a port nothing is
// listening on and reports that as a connection failure — which is true, and
// names the wrong thing.
//
// So the probe is started here, from what the target spec declares, and waited
// for rather than assumed: a port that has been opened is not a port that is
// serving, and starting to talk to one too early is a failure whose message is a
// refused connection rather than "the probe is still starting".
//
// Everything this file starts is stopped again in the same file, including when
// the session ended because a person pressed stop. A probe left running holds the
// board, and the next session's failure to connect is then this one's failure to
// clean up.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"rho/src/deps"
	"rho/src/sysdep"
)

// How long a probe is given to start listening before the session gives up on it.
//
// Generous, because a probe that is starting on a board that is also being
// flashed can take several seconds, and impatient is worse than slow: the session
// fails either way, but an impatient one fails while the probe is about to work.
const probeStartup = 20 * time.Second

// How often the port is checked while waiting.
const probePoll = 100 * time.Millisecond

// Process is something a session started, and will stop.
type Process struct {
	// Name is what a diagnostic calls it.
	Name string
	// Command is the program that was run.
	Command string
	// Arguments are what it was run with.
	Arguments []string

	command *exec.Cmd
	// log is what it printed, kept so a failure can show it. A probe that says
	// "unable to find a CMSIS-DAP device" on its own output is the whole answer to
	// why a session did not start, and it is not in any error this code raises.
	log     *lineBuffer
	stopped sync.Once
	// ended is closed once the process has been waited for, so that stopping it and
	// noticing it has ended are the same thing rather than two calls to Wait — the
	// second of which is an error about the first.
	ended chan struct{}
}

// lineBuffer is the tail of a process's output, bounded.
//
// Bounded because a probe left running for a day would otherwise grow a buffer
// until the machine runs out of memory, and the last hundred lines are all that is
// ever read.
type lineBuffer struct {
	mu    sync.Mutex
	lines []string
}

const logLines = 200

// Write appends a line, dropping the oldest when there are too many.
func (b *lineBuffer) Write(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, text)
	if len(b.lines) > logLines {
		b.lines = b.lines[len(b.lines)-logLines:]
	}
}

// Lines is what has been said, oldest first.
func (b *lineBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.lines...)
}

// String is what the process said, as one string.
func (b *lineBuffer) String() string {
	return strings.Join(b.Lines(), "\n")
}

// ExitCode is how the process ended, or nil while it is running or if it was
// stopped by a signal.
func (p *Process) ExitCode() *int {
	if p.command == nil || p.command.ProcessState == nil {
		return nil
	}
	code := p.command.ProcessState.ExitCode()
	return &code
}

// HasEnded reports whether the process is gone, which is what tells a waiter that
// the thing it was waiting for will never happen.
func (p *Process) HasEnded() bool {
	if p.ended == nil {
		return true
	}
	select {
	case <-p.ended:
		return true
	default:
		return false
	}
}

// Stop ends the process, and says what it printed as it went.
//
// Asked to stop rather than killed outright first, because a probe that is told
// to quit closes the connection to the board and the debugger's session ends
// cleanly; one that is killed leaves the board halted with the core in whatever
// state it was last in, which is a fault the next session inherits.
//
// The wait is bounded. A probe that ignores being asked to quit is killed, because
// a session that will not end is worse than one that ends with the port still
// briefly occupied.
func (p *Process) Stop() (string, error) {
	if p.command == nil || p.command.Process == nil {
		return "", nil
	}
	var said string
	p.stopped.Do(func() {
		// Its whole process group, not just itself: a probe started through a
		// script leaves the server it launched behind, and that one holds the board.
		group := -p.command.Process.Pid
		_ = syscall.Kill(group, syscall.SIGTERM)
		select {
		case <-p.ended:
		case <-time.After(3 * time.Second):
			// A probe that ignores being asked to quit is killed. A session that
			// will not end is worse than one that ends with the port briefly busy.
			_ = syscall.Kill(group, syscall.SIGKILL)
			<-p.ended
		}
		said = p.log.String()
	})
	return said, nil
}

// Start runs a program, keeping what it says.
//
// Output is captured rather than connected to this process's own streams because
// the editor is what a person is watching: a probe's startup log belongs in the
// session's console, and this component's stderr is the toolchain's, which a
// person running `rho dbg` by hand is watching for errors of its own.
func Start(name string, arguments ...string) (*Process, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("start %s: nothing to run", name)
	}
	// Resolved through the same reader as everything else, so a missing program is
	// reported by name rather than as an exit status nobody can read.
	resolved, err := ResolveProgram(arguments[0])
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	command := exec.Command(resolved, arguments[1:]...)
	log := &lineBuffer{}
	command.Stdout = logWriter{log}
	command.Stderr = logWriter{log}
	// Its own process group, so that stopping it stops the children a tool may
	// have started: probe-rs launches a cargo subcommand, and openocd forks.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	process := &Process{
		Name:      name,
		Command:   resolved,
		Arguments: arguments[1:],
		command:   command,
		log:       log,
		ended:     make(chan struct{}),
	}
	// Waited for exactly once, here. Anything else that wants to know whether the
	// process has ended waits on this rather than calling Wait itself, because a
	// second Wait is an error rather than an answer.
	go func() {
		_ = command.Wait()
		close(process.ended)
	}()
	return process, nil
}

// logWriter is what a process's output is read into, one line at a time.
type logWriter struct{ log *lineBuffer }

func (w logWriter) Write(data []byte) (int, error) {
	w.log.Write(strings.TrimRight(string(data), "\r\n"))
	return len(data), nil
}

/* -------------------------------------------------------------------------- */
/*  The probe                                                                  */
/* -------------------------------------------------------------------------- */

// Probe is the program that holds the connection to the board: OpenOCD, pyOCD,
// probe-rs, a vendor's gdbserver.
//
// It is started before the debugger connects and stopped when the session ends,
// so it exists for the length of a session and no longer.
type Probe struct {
	// Port is the TCP port the debugger connects to.
	Port    string
	process *Process
	stopped sync.Once
}

// ProbeSpec is what a target says about reaching its board for a session.
type ProbeSpec struct {
	// Program is what to run. Empty means the board is debugged in place and there
	// is no probe to start — a program on a host, or one already running on a
	// board that a debugger reaches directly.
	Program string
	// Arguments are its arguments, before the port.
	Arguments []string
	// Port is the port to listen on. Empty means 3333, which is what every server
	// in this family defaults to.
	Port string
}

// StartProbe starts the probe a session needs, and waits for it to listen.
//
// A spec with no program is answered with a probe that is not there: the debugger
// connects to the board itself in that case, which is a real mode rather than a
// missing feature, and it is how a program on this machine is debugged.
func StartProbe(spec ProbeSpec) (*Probe, error) {
	port := strings.TrimSpace(spec.Port)
	if port == "" {
		port = DefaultPort
	}
	if strings.TrimSpace(spec.Program) == "" {
		return &Probe{Port: port}, nil
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("the target says to listen on %q, which is not a port", port)
	}

	arguments := append(append([]string{}, spec.Arguments...), "-s", port)
	process, err := Start("the probe server", append([]string{spec.Program}, arguments...)...)
	if err != nil {
		return nil, fmt.Errorf("a session on this board needs a probe server: %w", err)
	}

	probe := &Probe{Port: port, process: process}
	if err := probe.waitUntilListening(); err != nil {
		said, _ := process.Stop()
		return nil, &noProbeError{reason: err.Error(), said: said}
	}
	return probe, nil
}

// DefaultPort is the port a debugger connects to when nothing says otherwise. It
// is GDB's own default, so a session that says nothing lands where a debugger run
// by hand would have connected.
const DefaultPort = "3333"

// waitUntilListening waits for the port to accept, or for the process to die
// trying.
//
// The two failures are told apart, because they are different problems: a probe
// that exited said why on its own output, and a probe that is running and not
// listening is either still starting or connected to a board that is not there.
func (p *Probe) waitUntilListening() error {
	deadline := time.Now().Add(probeStartup)
	address := net.JoinHostPort("127.0.0.1", p.Port)
	for time.Now().Before(deadline) {
		// The probe dying first is the common case, and waiting out the whole
		// timeout on a process that has already exited turns a two-second failure
		// into a twenty-second one.
		if p.process.HasEnded() {
			if code := p.process.ExitCode(); code != nil {
				return fmt.Errorf("the probe server exited with code %d", *code)
			}
			return fmt.Errorf("the probe server ended before it listened on %s", address)
		}
		connection, err := net.DialTimeout("tcp", address, probePoll)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		time.Sleep(probePoll)
	}
	return fmt.Errorf("the probe server did not listen on %s within %s", address, probeStartup)
}

// noProbeError is a probe that would not start, with what it said about why.
type noProbeError struct {
	reason string
	said   string
}

func (e *noProbeError) Error() string {
	if strings.TrimSpace(e.said) == "" {
		return e.reason
	}
	return e.reason + ": " + lastLine(e.said)
}

// ErrNoProbe is a session on a board that cannot be reached at all, said as one
// value so a caller can tell it from a debugger that failed.
//
// A caller that cannot distinguish the two ends up saying "gdb failed" about a
// board that is unplugged, which sends a person to install a debugger they
// already have.
var ErrNoProbe = errors.New("no probe")

// IsNoProbe reports whether a failure is a board that could not be reached.
func IsNoProbe(err error) bool {
	var missing *noProbeError
	return errors.As(err, &missing)
}

// LastSaid is what a process printed as it stopped, for a diagnostic.
func (p *Probe) LastSaid() string {
	if p.process == nil {
		return ""
	}
	return p.process.log.String()
}

// Stop ends the probe.
//
// Idempotent, and nil when there was none: the caller stops what it started
// without having to remember whether what it started was a process or a fact.
func (p *Probe) Stop() error {
	if p == nil || p.process == nil {
		return nil
	}
	var failure error
	p.stopped.Do(func() {
		said, err := p.process.Stop()
		if err != nil {
			failure = fmt.Errorf("stop the probe server: %w", err)
		} else if exit := p.process.ExitCode(); exit != nil && *exit != 0 && strings.TrimSpace(said) != "" {
			// Not a failure to have stopped it: a probe that is unhappy while
			// being told to quit is normal, and saying so here would put a line in
			// the console of every session that ends cleanly.
			_ = said
		}
	})
	return failure
}

// Describe is the probe as a diagnostic shows it.
func (p *Probe) Describe() string {
	if p == nil || p.process == nil {
		return "none"
	}
	return fmt.Sprintf("%s %s", p.process.Command, strings.Join(p.process.Arguments, " "))
}

// lastLine is the last line of what a process said, which is the one that says
// why.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if trimmed := strings.TrimSpace(lines[index]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Where a session runs                                                       */
/* -------------------------------------------------------------------------- */

// ProgramEnvironment is the environment a started program gets.
//
// Built rather than inherited, because a debugger started by an editor inherits
// that editor's whole environment, which on a developer machine includes whatever
// the shell had exported. The variables listed here are what a cross build
// actually needs; a session that has to be launched from a particular shell to
// find a toolchain is a session that does not work for anybody who is not using
// that shell.
func ProgramEnvironment(paths []string) []string {
	environment := []string{
		"PATH=" + strings.Join(paths, string(os.PathListSeparator)),
		"TERM=dumb",
	}
	return environment
}

// DebuggerFor is the debugger program a session runs, given the one a target names
// or derives, and the path it was found at.
//
// The first that exists wins, and the rest are fallbacks rather than preferences: a
// target says `arm-none-eabi-gdb`, and a Debian-family machine has no such program
// because its distribution ships one debugger for several architectures under
// another name. Refusing the session there would leave a machine with a perfectly
// good cross debugger unable to debug a board its own toolchain builds for.
//
// Ordered from most specific to least, and never the other way round: a machine
// with both the cross debugger its toolchain ships and the multiarch one uses the
// cross debugger, because that is the one whose own include paths match the
// program it is about.
func ResolveDebugger(name string, extraPaths ...string) (string, error) {
	candidates := []string{name}
	if name != "gdb" {
		// The distribution's one, under its own name rather than the prefixed one:
		// a machine that ships one debugger for several architectures does not call
		// it after each of them.
		candidates = append(candidates, "gdb-multiarch", "gdb")
	}
	// What was fetched into the Rho home, looked in *before* the fallback rather
	// than after it: a toolchain fetched from the vendor is the one the module asked
	// for, while gdb-multiarch is a thing the machine happens to have. The other way
	// round would report a satisfied requirement and then debug with something else,
	// which is the exact mismatch this whole block exists to catch.
	paths := append([]string{}, sysdep.FetchBinDirs(sysdep.ToolchainRoot(deps.RhoHome()))...)
	paths = append(paths, extraPaths...)
	for _, candidate := range candidates {
		if resolved, err := ResolveProgram(candidate, paths...); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("none of these is installed: %s", strings.Join(candidates, ", "))
}

// ResolveProgram finds a program on the path, and says which path it is at.
//
// Named and reported rather than executed directly, so that a session's failure
// can say "arm-none-eabi-gdb is not installed" instead of exit status 127.
func ResolveProgram(name string, extraPaths ...string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("no program to run")
	}
	if containsSlash(name) {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name, nil
		}
		return "", fmt.Errorf("%s is not there", name)
	}
	path := os.Getenv("PATH")
	for _, directory := range append(extraPaths, strings.Split(path, string(os.PathListSeparator))...) {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		candidate := directory + string(os.PathSeparator) + name
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s is not installed", name)
}

func containsSlash(name string) bool {
	return strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator)
}
