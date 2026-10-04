//
// @file session_test.go
// @brief Tests for a session's side of the protocol.
//
// A session is the editor's state machine, and its orderings are what a person
// notices when they are wrong: a program that runs past its own breakpoints, a
// register tree that shows where the program used to be, a refusal with nothing in
// it. So the session is driven here with a recording engine — one that answers
// everything and remembers what it was asked — and what is checked is both what the
// editor was told and what the engine was asked.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is an engine that answers everything and remembers what it was asked.
//
// It stands in for a debugger so that a session can be tested without one: the
// translation from the editor's protocol to an engine's contract is the part that
// is written here, and testing it against a real debugger would test the debugger.
type recorder struct {
	mu sync.Mutex
	// asked is every call, in order, as `method(args)`.
	asked []string

	capabilities Capabilities
	breakpoints  []Breakpoint
	threads      []Thread
	frames       []Frame
	scopes       []Scope
	variables    map[int][]Variable
	values       map[uint64]string

	launched LaunchArguments
	attached AttachArguments
	loaded   []Wanted
	stepped  []string
	resumed  []int
	paused   []int
	failSet  error
	failEval error
	closed   bool
	updates  chan Notification
}

func newRecorder() *recorder {
	return &recorder{
		capabilities: Capabilities{SupportsConfigurationDone: true, SupportsTerminateRequest: true},
		breakpoints:  []Breakpoint{{ID: 1, Verified: true, Source: "main.c", Line: 42}},
		threads:      []Thread{{ID: 1, Name: "main"}},
		frames:       []Frame{{ID: 1001, Name: "main", Line: 42, Source: "main.c"}},
		scopes: []Scope{
			{Name: "Registers", PresentationHint: "registers", VariablesReference: 1002, NamedVariables: 2},
			{Name: "Locals", PresentationHint: "locals", VariablesReference: 1003, NamedVariables: 1},
		},
		variables: map[int][]Variable{
			1002: {{Name: "pc", Value: "0x400abc", Type: "uint32"}},
			1003: {{Name: "counter", Value: "3", Type: "int"}},
		},
		values: map[uint64]string{
			0x40000000: "48000f20",
			0x40000004: "00000000",
		},
	}
}

func (r *recorder) note(format string, values ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, fmt.Sprintf(format, values...))
}

func (r *recorder) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked)
}

func (r *recorder) saw(call string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, one := range r.asked {
		if one == call {
			return true
		}
	}
	return false
}

func (r *recorder) said() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.asked, "\n")
}

func (r *recorder) Describe() Description { return Description{Name: "recorder", Kind: KindDriver} }

func (r *recorder) Initialize(InitializeArguments) (Capabilities, error) {
	r.note("initialize")
	return r.capabilities, nil
}

func (r *recorder) Launch(arguments LaunchArguments) error {
	r.note("launch target=%s program=%s", arguments.Target, arguments.Program)
	r.launched = arguments
	return nil
}

func (r *recorder) Attach(arguments AttachArguments) error {
	r.note("attach target=%s", arguments.Target)
	r.attached = arguments
	return nil
}

func (r *recorder) ConfigurationDone() error {
	r.note("configurationDone")
	return nil
}

func (r *recorder) SetBreakpoints(file string, wanted []Wanted) ([]Breakpoint, error) {
	r.note("setBreakpoints %s", file)
	for _, one := range wanted {
		r.note("  line=%d condition=%q hits=%d", one.Line, one.Condition, one.HitCount)
	}
	r.loaded = wanted
	return r.breakpoints, r.failSet
}

func (r *recorder) Threads() ([]Thread, error) {
	r.note("threads")
	return r.threads, nil
}

func (r *recorder) StackTrace(thread int, levels Range) ([]Frame, error) {
	r.note("stackTrace thread=%d skip=%d frames=%d", thread, levels.Skip, levels.Frames)
	return r.frames, nil
}

func (r *recorder) Scopes(frame int) ([]Scope, error) {
	r.note("scopes frame=%d", frame)
	return r.scopes, nil
}

func (r *recorder) Variables(reference int) ([]Variable, error) {
	r.note("variables reference=%d", reference)
	variables, found := r.variables[reference]
	if !found {
		return nil, fmt.Errorf("nothing under reference %d", reference)
	}
	return variables, nil
}

func (r *recorder) Evaluate(expression string, frame int, reference int) (Evaluation, error) {
	r.note("evaluate %q frame=%d reference=%d", expression, frame, reference)
	if r.failEval != nil {
		return Evaluation{}, r.failEval
	}
	return Evaluation{Value: "7", Type: "int", EvaluateName: expression}, nil
}

func (r *recorder) Resume(thread int) error {
	r.note("resume thread=%d", thread)
	r.resumed = append(r.resumed, thread)
	return nil
}

func (r *recorder) Step(thread int, kind StepKind) error {
	r.note("step %s thread=%d", kind, thread)
	r.stepped = append(r.stepped, string(kind))
	return nil
}

func (r *recorder) Pause(thread int) error {
	r.note("pause thread=%d", thread)
	r.paused = append(r.paused, thread)
	return nil
}

func (r *recorder) ReadMemory(address uint64, length int) (string, error) {
	r.note("readMemory 0x%x length=%d", address, length)
	value, found := r.values[address]
	if !found {
		return "", fmt.Errorf("nothing at 0x%x", address)
	}
	return value, nil
}

func (r *recorder) Disconnect(terminate bool) error {
	r.note("disconnect terminate=%v", terminate)
	return nil
}

func (r *recorder) Notifications() <-chan Notification { return r.updates }

func (r *recorder) Close() error {
	r.closed = true
	return nil
}

// volunteer is the debugger saying something, which a session turns into an event.
func (r *recorder) volunteer(notification Notification) {
	r.updates <- notification
}

/* -------------------------------------------------------------------------- */
/*  Driving a session                                                          */
/* -------------------------------------------------------------------------- */

// conversation is a session being talked to, with everything it has said.
//
// Over a pipe rather than a buffer, because the session writes from its own
// goroutine while the test reads: a buffer would need a lock, and a lock in a test
// harness hides the ordering bugs the test is looking for.
type conversation struct {
	t       *testing.T
	writer  *Codec
	session *Session
	// said is everything the session has sent, drained by a goroutine.
	said chan Message
	// pending is the answer read past the events, held so it is not read twice.
	pending *Message
}

// quiet is how long a test waits for the session to say nothing more before
// concluding it has finished. Short enough not to slow the suite, long enough that a
// session which is answering has answered.
const quiet = 200 * time.Millisecond

// next is the next thing the session said, or nothing within `quiet`.
func (c *conversation) next() (Message, bool) {
	c.t.Helper()
	select {
	case message := <-c.said:
		return message, true
	case <-time.After(quiet):
		return Message{}, false
	}
}

// ask sends one request the way an editor would, and reads its answer.
func (c *conversation) ask(command string, arguments any) Message {
	c.t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.writer.Write(Message{
		Seq: c.writer.Next(), Type: MessageRequest, Command: command, Arguments: encoded,
	}); err != nil {
		c.t.Fatal(err)
	}
	// An answer may not be the next thing said: a session sends events as well, and
	// a harness that assumed otherwise would read an event as a refusal.
	for {
		message, said := c.next()
		if !said {
			c.t.Fatalf("%s: the session said nothing", command)
		}
		if message.Type != MessageEvent {
			if message.Command != command {
				c.t.Fatalf("expected an answer to %s, got a %s (%q)", command, message.Type, message.Command)
			}
			return message
		}
	}
}

// writeRegisterMap gives the session a chip's register map, the way a session is
// given one when it is opened.
func (c *conversation) writeRegisterMap(load *SVD) error {
	c.t.Helper()
	c.session.WithRegisterMap(load, "chip.svd", "")
	return nil
}

// events are what the session said that was not an answer, up to the next answer.
func (c *conversation) events(limit int) []Message {
	c.t.Helper()
	out := []Message{}
	for len(out) < limit && c.pending == nil {
		message, said := c.next()
		if !said {
			return out
		}
		if message.Type != MessageEvent {
			answer := message
			c.pending = &answer
			return out
		}
		out = append(out, message)
	}
	return out
}

// sessionOf is a session on an engine, ready to be talked to.
//
// The editor's side is drained by its own goroutine, because the two ends are
// synchronous: a session that says two things in a row writes the second one only
// once the first has been read, and a test that reads one message at a time would
// deadlock against its own harness rather than against the code.
func sessionOf(t *testing.T, engine Backend) *conversation {
	t.Helper()
	sessionIn, editorOut := net.Pipe()
	sessionOut, editorIn := net.Pipe()

	session := NewSession(NewCodec(sessionIn, sessionOut), engine, nil)
	go session.Serve()
	t.Cleanup(func() {
		_ = editorOut.Close()
		_ = sessionOut.Close()
	})

	said := make(chan Message, 64)
	go func() {
		reader := NewCodec(editorIn, nil)
		for {
			message, err := reader.Read()
			if err != nil {
				close(said)
				return
			}
			said <- message
		}
	}()

	return &conversation{
		t:       t,
		writer:  NewCodec(nil, editorOut),
		said:    said,
		session: session,
	}
}

func bodyOf(t *testing.T, message Message, into any) {
	t.Helper()
	if len(message.Body) == 0 {
		t.Fatalf("%s: no body", message.Command)
	}
	if err := json.Unmarshal(message.Body, into); err != nil {
		t.Fatalf("%s: %v", message.Body, err)
	}
}

/* -------------------------------------------------------------------------- */
/*  The handshake                                                              */
/* -------------------------------------------------------------------------- */

// The handshake is the one place the editor is told what this session can do, and
// what it is told has to be what the engine can do: a capability the engine does
// not have produces a feature that fails when somebody uses it.
func TestTheHandshakeAnswersWithTheEnginesOwnCapabilities(t *testing.T) {
	engine := newRecorder()
	engine.capabilities = Capabilities{
		SupportsConfigurationDone:      true,
		SupportsConditionalBreakpoints: true,
	}
	talk := sessionOf(t, engine)

	answer := talk.ask(RequestInitialize, InitializeArguments{ClientName: "vscode"})
	if !answer.Success {
		t.Fatalf("expected the handshake to succeed, got %q", answer.Message)
	}
	capabilities := Capabilities{}
	bodyOf(t, answer, &capabilities)
	if !capabilities.SupportsConfigurationDone || !capabilities.SupportsConditionalBreakpoints {
		t.Errorf("expected the engine's own capabilities, got %+v", capabilities)
	}
	if !engine.saw("initialize") {
		t.Error("expected the engine to be asked")
	}

	// And the editor is told the session is ready, or it waits for a session that
	// is waiting for the editor.
	found := false
	for _, event := range talk.events(2) {
		if event.Command == EventInitialized {
			found = true
		}
	}
	if !found {
		t.Error("expected an initialized event")
	}
}

// The whole conversation, in the order the protocol requires it. This is the test
// that a launch works at all: a program started before the breakpoints are in runs
// past all of them.
func TestASessionInTheOrderTheProtocolRequires(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)

	talk.ask(RequestInitialize, InitializeArguments{})
	talk.ask(RequestLaunch, LaunchArguments{Program: "build/output/blinky", Target: "pico"})
	answer := talk.ask(RequestSetBreakpoint, SetBreakpointsArguments{
		Source:      SourceBreakpointsSource{Path: "main.c"},
		Breakpoints: []SourceBreakpoint{{Line: 42}, {Line: 43}},
	})
	if !answer.Success {
		t.Fatalf("expected the breakpoints to be set, got %q", answer.Message)
	}
	set := struct {
		Breakpoints []Breakpoint `json:"breakpoints"`
	}{}
	bodyOf(t, answer, &set)
	if len(set.Breakpoints) != 1 || !set.Breakpoints[0].Verified {
		t.Errorf("expected the verified breakpoint the engine reported, got %+v", set.Breakpoints)
	}
	talk.ask(RequestConfiguration, map[string]any{})

	// The engine saw the launch, both lines, and only then the configuration being
	// done — in that order.
	said := engine.said()
	launch := strings.Index(said, "launch")
	firstLine := strings.Index(said, "line=42")
	done := strings.Index(said, "configurationDone")
	if launch < 0 || firstLine < 0 || done < 0 {
		t.Fatalf("expected a launch, the breakpoints and a configuration, got:\n%s", said)
	}
	if !(launch < firstLine && firstLine < done) {
		t.Errorf("expected the breakpoints between the launch and the configuration, got:\n%s", said)
	}
}

// An editor may send either the breakpoints or the older list of line numbers, and
// refusing the older one would make this adapter look like it does not work at all.
func TestBreakpointsAreAcceptedInBothForms(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)
	talk.ask(RequestSetBreakpoint, SetBreakpointsArguments{
		Source:      SourceBreakpointsSource{Path: "main.c"},
		Breakpoints: []SourceBreakpoint{{Line: 0}, {Line: 42}},
	})
	if len(engine.loaded) != 1 || engine.loaded[0].Line != 42 {
		t.Errorf("expected the breakpoint with a line, got %+v", engine.loaded)
	}

	engine2 := newRecorder()
	talk2 := sessionOf(t, engine2)
	talk2.ask(RequestSetBreakpoint, SetBreakpointsArguments{
		Source: SourceBreakpointsSource{Path: "main.c"},
		Lines:  []int{7, 8, 0},
	})
	if len(engine2.loaded) != 2 {
		t.Errorf("expected both lines, got %+v", engine2.loaded)
	}
}

// A condition and an ignore count are what make a breakpoint worth having, and
// both are on the request rather than on the file — so they have to be carried
// across, or a conditional breakpoint is silently unconditional.
func TestAConditionalBreakpointIsCarriedAcross(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)
	talk.ask(RequestSetBreakpoint, SetBreakpointsArguments{
		Source:      SourceBreakpointsSource{Path: "main.c"},
		Breakpoints: []SourceBreakpoint{{Line: 42, Condition: "counter == 3", HitCount: 2}},
	})
	if len(engine.loaded) != 1 {
		t.Fatalf("expected one breakpoint, got %+v", engine.loaded)
	}
	if engine.loaded[0].Condition != "counter == 3" || engine.loaded[0].HitCount != 2 {
		t.Errorf("expected the condition and the count to be carried, got %+v", engine.loaded[0])
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading a stopped program                                                  */
/* -------------------------------------------------------------------------- */

func TestAStoppedProgramIsReadThroughToItsLocals(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)

	threads := struct {
		Threads []Thread `json:"threads"`
	}{}
	bodyOf(t, talk.ask(RequestThreads, map[string]any{}), &threads)
	if len(threads.Threads) != 1 || threads.Threads[0].Name != "main" {
		t.Fatalf("expected the engine's thread, got %+v", threads.Threads)
	}

	stack := struct {
		StackFrames []Frame `json:"stackFrames"`
		TotalFrames int     `json:"totalFrames"`
	}{}
	bodyOf(t, talk.ask(RequestStackTrace, StackTraceArguments{ThreadID: 1, StartFrame: 2, Levels: 5}), &stack)
	if len(stack.StackFrames) != 1 || stack.StackFrames[0].Name != "main" {
		t.Errorf("expected the frame, got %+v", stack.StackFrames)
	}
	if !engine.saw("stackTrace thread=1 skip=2 frames=5") {
		t.Errorf("expected the paging to be passed on, got:\n%s", engine.said())
	}

	scopes := struct {
		Scopes []Scope `json:"scopes"`
	}{}
	bodyOf(t, talk.ask(RequestScopes, ScopesArguments{FrameID: 1001}), &scopes)
	if len(scopes.Scopes) < 2 {
		t.Fatalf("expected the engine's scopes, got %+v", scopes.Scopes)
	}
	// The register map's own scope is appended, and it is last: it is not part of the
	// frame, and showing it among the locals would make every other scope a lie.
	last := scopes.Scopes[len(scopes.Scopes)-1]
	if last.Name != "Peripherals" {
		t.Errorf("expected the register map's scope, got %+v", last)
	}

	variables := struct {
		Variables []Variable `json:"variables"`
	}{}
	bodyOf(t, talk.ask(RequestVariables, VariablesArguments{VariablesReference: 1003}), &variables)
	if len(variables.Variables) != 1 || variables.Variables[0].Name != "counter" {
		t.Errorf("expected the local, got %+v", variables.Variables)
	}

	// What an expression came to, with the reference the editor will hold.
	evaluated := struct {
		Result       string `json:"result"`
		Type         string `json:"type"`
		EvaluateName string `json:"evaluateName"`
	}{}
	bodyOf(t, talk.ask(RequestEvaluate, EvaluateArguments{Expression: "counter + 1", FrameID: 1001}), &evaluated)
	if evaluated.Result != "7" || evaluated.EvaluateName != "counter + 1" {
		t.Errorf("expected the value and how to ask again, got %+v", evaluated)
	}
}

func TestResumingAndSteppingReachTheEngineInTheRightDirection(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)

	talk.ask(RequestContinue, ContinueArguments{ThreadID: 1})
	talk.ask(RequestNext, StepArguments{ThreadID: 1})
	talk.ask(RequestStepIn, StepArguments{ThreadID: 1})
	talk.ask(RequestStepOut, StepArguments{ThreadID: 1})
	talk.ask(RequestPause, StepArguments{ThreadID: 1})

	// Three directions, three different things to ask for, and a resume and a pause
	// that are not a direction at all.
	if strings.Join(engine.stepped, ",") != "over,into,out" {
		t.Errorf("expected over, into and out in that order, got %v", engine.stepped)
	}
	if len(engine.resumed) != 1 || engine.resumed[0] != 1 {
		t.Errorf("expected one resume of thread 1, got %v", engine.resumed)
	}
	if len(engine.paused) != 1 {
		t.Errorf("expected one pause, got %v", engine.paused)
	}
}

/* -------------------------------------------------------------------------- */
/*  Failing                                                                    */
/* -------------------------------------------------------------------------- */

// Every request is answered, always: an editor waits for a response to every one
// it sends, and a session that exits instead leaves a person looking at a spinner
// with nothing to act on.
func TestARequestTheEngineCannotAnswerIsRefusedRatherThanDropped(t *testing.T) {
	engine := newRecorder()
	engine.failEval = fmt.Errorf("no symbol named nope")
	talk := sessionOf(t, engine)

	answer := talk.ask(RequestEvaluate, EvaluateArguments{Expression: "nope"})
	if answer.Success {
		t.Fatal("expected a refusal")
	}
	if answer.Message != "no symbol named nope" {
		t.Errorf("expected the reason to be passed on, got %q", answer.Message)
	}

	// And the session carries on: a later request is still answered.
	if !talk.ask(RequestThreads, map[string]any{}).Success {
		t.Error("expected the session to carry on after a refusal")
	}
}

// An editor may ask for a request this adapter has never heard of — a protocol that
// grows, or one this component does not implement. Refusing by name lets the editor
// fall back to its own default; dropping it makes the editor wait.
func TestAnUnknownRequestIsRefusedByName(t *testing.T) {
	talk := sessionOf(t, newRecorder())
	answer := talk.ask("goto", map[string]any{})
	if answer.Success {
		t.Error("expected a refusal")
	}
	if !strings.Contains(answer.Message, "goto") {
		t.Errorf("expected the request to be named in the refusal, got %q", answer.Message)
	}
}

func TestDisconnectingEndsTheSession(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)
	answer := talk.ask(RequestDisconnected, DisconnectArguments{TerminateDebuggee: true})
	if !answer.Success {
		t.Errorf("expected the disconnect to succeed, got %q", answer.Message)
	}
	// The editor is told the session ended, or it waits for a session that has gone.
	// And the teardown is checked after that, because the editor is answered before
	// the session tears itself down — a person pressing stop should not wait for
	// the probe to close.
	found := false
	for _, event := range talk.events(3) {
		if event.Command == EventTerminated {
			found = true
		}
	}
	if !found {
		t.Error("expected a terminated event")
	}
	if !engine.saw("disconnect terminate=true") {
		t.Errorf("expected the terminate flag to be passed on, got:\n%s", engine.said())
	}
}
