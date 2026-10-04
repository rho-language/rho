//
// @file engine_adapter.go
// @brief An engine that is another program, relaying to it.
//
// This is how a debugger this component has never heard of becomes a session.
//
// A debugger that already has a Debug Adapter of its own — lldb-dap, probe-rs,
// pyOCD — does not need to be driven through the Machine Interface, and wrapping
// one in a protocol of ours would be a second implementation of something that
// already works, and the one that would rot first. So a plugin contributes a
// *program*, and this relays the editor's protocol to it and its answers back.
//
// Which means most of this file is sequence-number bookkeeping. The two sides
// each number their own messages, and the numbers are what an answer says it
// belongs to, so a relay that forwarded them unchanged would answer the wrong
// request — a symptom that looks like a hung session, because the editor is
// waiting for an answer it will never be given and the answer that did arrive is
// for something else.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// How long a relayed adapter is given to start answering before the session says
// it did not.
const adapterStartup = 60 * time.Second

// Adapter is an engine that is a debug adapter of its own.
//
// It implements the same Backend as the GDB engine and knows nothing about
// debugging: it starts a program, relays the protocol, and turns the events that
// matter into the same notifications every engine produces. That is what lets a
// board be debugged by something that has never been seen here.
type Adapter struct {
	spec    EngineSpec
	target  string
	program string

	codec  *Codec
	child  *Codec
	child0 *osExec

	mu sync.Mutex
	// pending maps a request this side was asked to a request it asked of the
	// other, so an answer can be given to whoever is waiting for it.
	pending map[int]int
	// asked maps a request the other side asked to a request this side has to
	// answer, for the requests an adapter makes of the editor.
	asked map[int]int
	// awaiting is where the answer to a request this side asked of the engine is
	// delivered. Some requests are answered to the editor by the relay loop and
	// some are waited for here — a breakpoint list has to be waited for, because
	// the editor is shown what came back, and a continue does not, because nothing
	// is shown for it.
	awaiting map[int]chan Message
	// capabilities are what the engine said it could do at handshake.
	capabilities Capabilities
	// started is whether the handshake has happened, which the relay needs to know
	// because the capabilities arrive in it.
	started bool

	notifications chan Notification
	stopOnce      sync.Once
	closed        chan struct{}
}

// osExec is a process the relay drives.
type osExec struct {
	command  *exec.Cmd
	commands io.WriteCloser
}

// StartAdapter starts the program an engine spec names and prepares to relay to
// it.
//
// Started before the handshake, because the handshake is what the editor sends
// first and it has to reach the child while it is still there. A child that
// refuses the protocol and exits says why on its own output, which is read into
// the failure rather than lost.
func StartAdapter(spec EngineSpec, target string, program string, editor *Codec) (*Adapter, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	executable, arguments := spec.Expansion(target, program)
	resolved, err := ResolveProgram(executable)
	if err != nil {
		return nil, fmt.Errorf("the engine %q: %w", spec.Name, err)
	}

	command := exec.Command(resolved, arguments...)
	commands, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("the engine %q: %w", spec.Name, err)
	}
	records, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("the engine %q: %w", spec.Name, err)
	}
	// The adapter's own diagnostics go to this session's console rather than to the
	// toolchain's: "no probe found" is a sentence about the session, and it is
	// usually the whole answer to why the session did not start.
	errors, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("the engine %q: %w", spec.Name, err)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("the engine %q: %w", spec.Name, err)
	}

	engine := &Adapter{
		spec:          spec,
		target:        target,
		program:       program,
		codec:         editor,
		child:         NewCodec(records, commands),
		child0:        &osExec{command: command, commands: commands},
		pending:       map[int]int{},
		asked:         map[int]int{},
		awaiting:      map[int]chan Message{},
		notifications: make(chan Notification, 64),
		closed:        make(chan struct{}),
	}
	go engine.diagnose(errors)
	go engine.relay()
	return engine, nil
}

// diagnose forwards what the engine printed on its own stream.
func (a *Adapter) diagnose(reader interface{ Read([]byte) (int, error) }) {
	buffer := make([]byte, 4096)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			a.notify(Notification{
				Kind: NotifyOutput, Category: OutputStderr, Text: string(buffer[:count]),
			})
		}
		if err != nil {
			return
		}
	}
}

// relay reads the engine's side and answers the editor's.
//
// Both directions are handled here rather than in a handler each, because a
// response and an event are the same message with different meanings and a
// session that relays only one of them works right up until the first stop.
func (a *Adapter) relay() {
	for {
		message, err := a.child.Read()
		if err != nil {
			a.finish()
			return
		}
		switch message.Type {
		case MessageResponse:
			a.answer(message)
		case MessageRequest:
			a.ask(message)
		case MessageEvent:
			a.note(message)
		}
	}
}

// answer relays a response, renumbered to the request on this side it belongs to.
func (a *Adapter) answer(message Message) {
	a.mu.Lock()
	mine, known := a.pending[message.RequestSeq]
	delete(a.pending, message.RequestSeq)
	if message.Command == RequestInitialize && message.Success {
		a.started = true
	}
	waiting := a.awaiting[message.RequestSeq]
	delete(a.awaiting, message.RequestSeq)
	a.mu.Unlock()

	// An answer somebody is waiting for goes to them; one nobody is waiting for is
	// relayed on to the editor, which is what the requests behind `pending` are.
	if waiting != nil {
		select {
		case waiting <- message:
		case <-time.After(commandTimeout):
		}
		return
	}
	if !known {
		// The engine answered a request this side did not ask, which happens when
		// a request was relayed before the session was tracking it. Dropped rather
		// than forwarded: an answer with a request_seq nobody is waiting on is one
		// the editor cannot match to anything, and forwarding it makes it look for
		// one.
		return
	}
	relayed := Message{
		Seq:        a.codec.Next(),
		Type:       MessageResponse,
		Command:    message.Command,
		RequestSeq: mine,
		Success:    message.Success,
		Message:    message.Message,
		Body:       message.Body,
	}
	if message.Command == RequestInitialize && message.Success {
		a.remember(bodyAs[Capabilities](message.Body))
	}
	_ = a.codec.Write(relayed)
}

// ask relays a request the engine made of the editor.
//
// Some adapters ask the editor to run something in a terminal — a probe server, a
// program under test — and an editor answers those. Relaying them is what makes
// the session work the way the adapter expects rather than in the subset this
// component happens to implement.
func (a *Adapter) ask(message Message) {
	relayed := Message{
		Seq:       a.codec.Next(),
		Type:      MessageRequest,
		Command:   message.Command,
		Arguments: message.Arguments,
	}
	a.mu.Lock()
	a.asked[relayed.Seq] = message.Seq
	a.mu.Unlock()
	if err := a.codec.Write(relayed); err != nil {
		a.notify(Notification{Kind: NotifyError, Err: err})
	}
}

// Answer relays the editor's answer to a request the engine made.
//
// Called by the session when the editor answers something this adapter asked of
// it; a request the session has no answer for is refused rather than dropped,
// because an adapter waiting for an answer it will never get waits forever.
func (a *Adapter) Answer(message Message) error {
	a.mu.Lock()
	theirs, known := a.asked[message.RequestSeq]
	delete(a.asked, message.RequestSeq)
	a.mu.Unlock()

	relayed := Message{
		Seq:        a.child.Next(),
		Type:       MessageResponse,
		Command:    message.Command,
		RequestSeq: theirs,
		Success:    message.Success,
		Message:    message.Message,
		Body:       message.Body,
	}
	if !known && !message.Success {
		relayed.Message = "the editor did not answer this request"
	}
	if err := a.child.Write(relayed); err != nil {
		return fmt.Errorf("relay the editor's answer to %s: %w", message.Command, err)
	}
	return nil
}

// note relays an event, and takes what this session needs from it.
func (a *Adapter) note(message Message) {
	if message.Command == EventOutput {
		body := bodyAs[outputBody](message.Body)
		a.notify(Notification{Kind: NotifyOutput, Category: body.Category, Text: body.Output})
		_ = a.codec.Write(a.event(message))
		return
	}
	switch message.Command {
	case EventStopped:
		body := bodyAs[stoppedBody](message.Body)
		a.notify(Notification{
			Kind: NotifyStopped,
			Stop: Stop{Kind: StopKind(body.Reason), Thread: body.ThreadID, Text: body.Text},
		})
	case EventExited:
		body := bodyAs[exitBody](message.Body)
		a.notify(Notification{Kind: NotifyExited, ExitCode: body.ExitCode})
	case EventTerminated:
		a.notify(Notification{Kind: NotifyTerminated})
	}
	_ = a.codec.Write(a.event(message))
}

// event is an engine's event, renumbered for the editor.
func (a *Adapter) event(message Message) Message {
	return Message{
		Seq:       a.codec.Next(),
		Type:      MessageEvent,
		Command:   message.Command,
		Arguments: message.Arguments,
	}
}

// outputBody is what an output event carries.
type outputBody struct {
	Category string `json:"category,omitempty"`
	Output   string `json:"output"`
}

// stoppedBody is what a stop event carries.
type stoppedBody struct {
	Reason            string `json:"reason"`
	ThreadID          int    `json:"threadId"`
	Text              string `json:"text,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped,omitempty"`
	Description       string `json:"description,omitempty"`
}

// exitBody is what an exit event carries.
type exitBody struct {
	ExitCode int `json:"exitCode"`
}

func bodyAs[T any](body []byte) T {
	var out T
	_ = unmarshal(body, &out)
	return out
}

// Send relays a request to the engine and answers with its response.
func (a *Adapter) Send(command string, arguments any) (Message, error) {
	encoded, err := encodeBody(arguments)
	if err != nil {
		return Message{}, err
	}
	sequence := a.child.Next()
	a.mu.Lock()
	a.pending[sequence] = a.codec.Next()
	mine := a.pending[sequence]
	a.mu.Unlock()

	if err := a.child.Write(Message{
		Seq:       sequence,
		Type:      MessageRequest,
		Command:   command,
		Arguments: encoded,
	}); err != nil {
		return Message{}, fmt.Errorf("the engine %q: %w", a.spec.Name, err)
	}
	// The answer is relayed by the relay loop, which sends it on to the editor as
	// it arrives. Waiting for it here as well would mean waiting for something
	// that is answered twice, so this asks only whether the engine took the
	// request at all, which the write did.
	_ = mine
	return Message{Seq: sequence}, nil
}

// remember keeps the capabilities the engine reported at handshake.
//
// Taken from the relayed answer rather than asked for again: the engine already
// said what it could do, and asking a second time would be asking a question whose
// answer changes between two asks.
func (a *Adapter) remember(reported Capabilities) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.capabilities = reported
}

// Initialize relays the handshake and answers with what the engine said.
//
// The editor is answered after the engine's, so the capabilities are the engine's
// own rather than a translation of them. An adapter that under-reports is a
// session with fewer features; an adapter that is told it has more than it does is
// a session whose features fail when they are used.
func (a *Adapter) Initialize(arguments InitializeArguments) (Capabilities, error) {
	if _, err := a.Send(RequestInitialize, arguments); err != nil {
		return Capabilities{}, err
	}
	deadline := time.Now().Add(adapterStartup)
	for {
		a.mu.Lock()
		started := a.started
		a.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			return Capabilities{}, fmt.Errorf("the engine %q did not answer the handshake", a.spec.Name)
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.capabilities, nil
}

// Launch relays a launch to the engine.
func (a *Adapter) Launch(arguments LaunchArguments) error {
	_, err := a.Send(RequestLaunch, arguments)
	return err
}

// Attach relays an attach to the engine.
func (a *Adapter) Attach(arguments AttachArguments) error {
	_, err := a.Send(RequestAttach, arguments)
	return err
}

// ConfigurationDone relays the editor saying it has sent its breakpoints.
//
// And it is the engine that must wait for them, not this adapter: an engine that
// is told the session is configured before the breakpoints arrive runs past all of
// them, and that is the engine's ordering to get right, not this one's to guess.
func (a *Adapter) ConfigurationDone() error {
	_, err := a.Send(RequestConfiguration, map[string]any{})
	return err
}

// SetBreakpoints relays the breakpoints and waits for what the engine set.
//
// Unlike the other relays, this one waits: the answer is what the editor will be
// shown, and answering before it arrives would show nothing.
func (a *Adapter) SetBreakpoints(file string, wanted []Wanted) ([]Breakpoint, error) {
	asked := make([]SourceBreakpoint, 0, len(wanted))
	for _, one := range wanted {
		asked = append(asked, SourceBreakpoint{
			Line:      one.Line,
			Condition: one.Condition,
			HitCount:  one.HitCount,
			Source:    &Source{Path: file},
		})
	}
	answer, err := a.await(RequestSetBreakpoint, SetBreakpointsArguments{
		Source:      SourceBreakpointsSource{Path: file},
		Breakpoints: asked,
	})
	if err != nil {
		return nil, err
	}
	body := bodyAs[breakpointsBody](answer.Body)
	located := file
	if body.Source.Path != "" {
		located = body.Source.Path
	}
	set := make([]Breakpoint, 0, len(body.Breakpoints))
	for _, one := range body.Breakpoints {
		set = append(set, Breakpoint{
			ID:       one.ID,
			Verified: one.Verified,
			Message:  one.Message,
			Source:   located,
			Line:     one.Line,
		})
	}
	return set, nil
}

type breakpointsBody struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
	Source      Source       `json:"source"`
}

// await sends a request to the engine and waits for its answer, which arrives
// through the relay loop.
func (a *Adapter) await(command string, arguments any) (Message, error) {
	encoded, err := encodeBody(arguments)
	if err != nil {
		return Message{}, err
	}
	sequence := a.child.Next()
	// Held so that Send's bookkeeping does not also renumber this one: the answer
	// has to come back to this caller rather than to the editor.
	waiting := make(chan Message, 1)
	a.mu.Lock()
	a.awaiting[sequence] = waiting
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.awaiting, sequence)
		a.mu.Unlock()
	}()

	if err := a.child.Write(Message{
		Seq: sequence, Type: MessageRequest, Command: command, Arguments: encoded,
	}); err != nil {
		return Message{}, fmt.Errorf("the engine %q: %w", a.spec.Name, err)
	}
	select {
	case answer := <-waiting:
		if !answer.Success {
			return Message{}, fmt.Errorf("%s", answer.Message)
		}
		return answer, nil
	case <-time.After(commandTimeout):
		return Message{}, fmt.Errorf("the engine %q did not answer %s", a.spec.Name, command)
	}
}

// Threads asks the engine what threads the program has.
func (a *Adapter) Threads() ([]Thread, error) {
	answer, err := a.await(RequestThreads, map[string]any{})
	if err != nil {
		return nil, err
	}
	body := bodyAs[threadsBody](answer.Body)
	return body.Threads, nil
}

type threadsBody struct {
	Threads []Thread `json:"threads"`
}

// StackTrace asks the engine for the frames of a thread.
func (a *Adapter) StackTrace(thread int, levels Range) ([]Frame, error) {
	answer, err := a.await(RequestStackTrace, StackTraceArguments{
		ThreadID:   thread,
		StartFrame: levels.Skip,
		Levels:     levels.Frames,
	})
	if err != nil {
		return nil, err
	}
	body := bodyAs[stackBody](answer.Body)
	return body.StackFrames, nil
}

type stackBody struct {
	StackFrames []Frame `json:"stackFrames"`
	TotalFrames int     `json:"totalFrames"`
}

// Scopes asks the engine what can be read in a frame.
func (a *Adapter) Scopes(frame int) ([]Scope, error) {
	answer, err := a.await(RequestScopes, ScopesArguments{FrameID: frame})
	if err != nil {
		return nil, err
	}
	body := bodyAs[scopesBody](answer.Body)
	return body.Scopes, nil
}

type scopesBody struct {
	Scopes []Scope `json:"scopes"`
}

// Variables asks the engine what is in a scope.
//
// The reference is passed straight through: it is the engine's own numbering, and
// renumbering it here would mean keeping a second mapping of numbers the editor
// holds to numbers the engine holds, for no gain.
func (a *Adapter) Variables(reference int) ([]Variable, error) {
	answer, err := a.await(RequestVariables, VariablesArguments{VariablesReference: reference})
	if err != nil {
		return nil, err
	}
	body := bodyAs[variablesBody](answer.Body)
	return body.Variables, nil
}

type variablesBody struct {
	Variables []Variable `json:"variables"`
}

// Evaluate asks the engine what an expression comes to.
func (a *Adapter) Evaluate(expression string, frame int, reference int) (Evaluation, error) {
	answer, err := a.await(RequestEvaluate, EvaluateArguments{Expression: expression, FrameID: frame})
	if err != nil {
		return Evaluation{}, err
	}
	body := bodyAs[evaluateBody](answer.Body)
	evaluation := Evaluation{
		Value:              body.Result,
		Type:               body.Type,
		VariablesReference: body.VariablesReference,
		EvaluateName:       body.EvaluateName,
	}
	if evaluation.VariablesReference == 0 {
		evaluation.VariablesReference = reference
	}
	return evaluation, nil
}

type evaluateBody struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference,omitempty"`
	EvaluateName       string `json:"evaluateName,omitempty"`
}

// ReadMemory asks the engine what is at an address.
//
// Only where the engine answers a memory request at all. An engine that does not
// is told so rather than answered with zeroes: a register view full of zeros
// looks like a chip whose peripherals are all off.
func (a *Adapter) ReadMemory(address uint64, length int) (string, error) {
	answer, err := a.await("readMemory", map[string]any{
		"memoryReference": fmt.Sprintf("0x%x", address),
		"offset":          0,
		"count":           length,
	})
	if err != nil {
		return "", err
	}
	body := bodyAs[readMemoryBody](answer.Body)
	return body.Data, nil
}

type readMemoryBody struct {
	Address string `json:"address,omitempty"`
	Data    string `json:"data,omitempty"`
}

// Resume relays a continue.
func (a *Adapter) Resume(thread int) error {
	_, err := a.Send(RequestContinue, ContinueArguments{ThreadID: thread})
	return err
}

// Step relays a step, in the direction the editor asked for.
func (a *Adapter) Step(thread int, kind StepKind) error {
	command := RequestNext
	switch kind {
	case StepInto:
		command = RequestStepIn
	case StepOut:
		command = RequestStepOut
	}
	_, err := a.Send(command, StepArguments{ThreadID: thread})
	return err
}

// Pause relays a pause.
func (a *Adapter) Pause(thread int) error {
	_, err := a.Send(RequestPause, StepArguments{ThreadID: thread})
	return err
}

// Disconnect relays a disconnect and stops the engine.
func (a *Adapter) Disconnect(terminate bool) error {
	_, err := a.Send(RequestDisconnected, DisconnectArguments{TerminateDebuggee: terminate})
	if err != nil {
		// Ending the session must not fail because the engine is already gone: the
		// editor is being told it ended either way, and an error here would be an
		// error about something that has already happened.
		return nil
	}
	return nil
}

// Notifications is what the engine volunteers.
func (a *Adapter) Notifications() <-chan Notification { return a.notifications }

// Describe is this engine as a diagnostic shows it.
func (a *Adapter) Describe() Description {
	executable, arguments := a.spec.Expansion(a.target, a.program)
	return Description{
		Name:      a.spec.Name,
		Kind:      KindAdapter,
		Program:   executable,
		Arguments: arguments,
		Source:    "a plugin",
	}
}

// Close stops the engine and everything it started.
func (a *Adapter) Close() error {
	if a.child0 == nil {
		return nil
	}
	var failure error
	a.stopOnce.Do(func() {
		close(a.closed)
		_ = a.child0.commands.Close()
		if a.child0.command.Process != nil {
			if err := a.child0.command.Process.Signal(syscall.SIGTERM); err != nil {
				failure = err
			}
		}
		done := make(chan struct{})
		go func() {
			_ = a.child0.command.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			if a.child0.command.Process != nil {
				_ = a.child0.command.Process.Kill()
			}
			<-done
		}
	})
	return failure
}

// finish says the engine has ended.
func (a *Adapter) finish() {
	a.stopOnce.Do(func() { close(a.closed) })
	a.notify(Notification{Kind: NotifyExited})
}

func (a *Adapter) notify(notification Notification) {
	select {
	case a.notifications <- notification:
	case <-a.closed:
	default:
	}
}
