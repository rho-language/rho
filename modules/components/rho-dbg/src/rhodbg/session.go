//
// @file session.go
// @brief A debugging session: the editor's protocol, answered by an engine.
//
// Everything protocol-specific that is not the engine's business is here: the
// handshake, the references the editor holds, the events that are sent without
// being asked for, and the ordering a session is required to have. The engine
// answers questions about the program; this answers the editor.
//
// The ordering is the part worth stating. The protocol has a required sequence —
// initialize, then launch or attach, then breakpoints, then configurationDone —
// and the order matters: a program started before the editor has sent its
// breakpoints runs past all of them, and a session that reported success while
// doing so would look like a session with a breakpoint that did not work.
//
// The peripheral register map is answered from here too, rather than by every
// engine, because it is the same tree for every debugger: what a chip's registers
// *are* comes from the chip's own file, and only reading them needs a debugger.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"rho/src/targetspec"
)

// Session is one debugging session.
//
// One per adapter process: the editor starts the adapter, the adapter is the
// session, and the session ends when the adapter does.
type Session struct {
	codec  *Codec
	engine Backend
	probe  *Probe

	mu sync.Mutex
	// configuration is what the editor said it is about to do, kept so the
	// configurationDone that comes at the end of it can be used to start the
	// program.
	configuration json.RawMessage
	launched      bool
	attached      bool
	// map is the register map a session exposes under the peripherals scope. Nil
	// when the session has none, which is reported rather than shown as an empty
	// tree: a chip with no register map file is a project that has not vendored
	// one.
	registerMap *SVD
	// registerMapName is where the map was read from, for a diagnostic.
	registerMapName string
	// registerMapDiagnostics is what was wrong with it, for a session that says so.
	registerMapDiagnostics string
	// peripheralRoot is the reference the list of peripherals is held under.
	peripheralRoot int
	// peripheralQueue holds each expandable peripheral by the reference it was
	// given. Its registers are read the first time somebody expands it rather than
	// at handshake: reading a chip's registers means reading memory, and doing that
	// for every peripheral nobody looked at would be a slow operation over a debug
	// connection.
	peripheralQueue map[int]Peripheral
	// fields holds the registers whose bit fields are worth expanding, by the
	// reference the register was given.
	fields map[int]Register
	// registerValues is what a register read, by address, so that expanding the
	// same peripheral twice reads the board once.
	registerValues map[uint64]cachedRegister
	// nextReference hands out the numbers the editor holds. High, so that a
	// reference is visibly one and cannot be confused with a frame level.
	nextReference int
	// stopped is the reason of the last stop, for a resume that follows one.
	stopped Stop
	// opening is what the session said about itself as it started.
	opening []string
	// registerMapAnnounced is what was said about the register map, so it is said
	// once rather than once per request that touches it.
	registerMapAnnounced bool
}

// reportRegisterMap says what was wrong with a chip's register map.
//
// Said once, in the session's console, rather than refused: a session that stops
// over a file nobody vendored is a session that cannot debug anything, and a
// session that runs with the registers missing is a session that debugs
// everything but one view of it.
func (s *Session) reportRegisterMap(reason string) {
	s.mu.Lock()
	already := s.registerMapAnnounced
	s.registerMapAnnounced = true
	s.registerMapDiagnostics = reason
	s.mu.Unlock()
	if !already {
		_ = s.codec.Output(OutputStderr, "rho dbg: "+reason+"\n")
	}
}

// cachedRegister is a register's value as it was read.
//
// The number is kept as well as the text, because a field is decoded from it and
// a field cannot be decoded from the hexadecimal string without parsing it again
// for every field of every register.
type cachedRegister struct {
	value   string
	number  uint64
	numeric bool
}

// numeric reports whether the value could be read as a number, which is what
// decides whether the fields under it can be shown at all.
func (c cachedRegister) parsed() bool { return c.numeric }

// FirstReference is the first number an editor is handed.
//
// GDB numbers its frames from zero and an editor holds numbers from two sources
// with no way to tell them apart, so every number this session invents is above a
// thousand: unmistakable in a log, and never a level.
const FirstReference = firstReference

// NewSession is a session on an engine.
func NewSession(codec *Codec, engine Backend, probe *Probe) *Session {
	return &Session{
		codec:           codec,
		engine:          engine,
		probe:           probe,
		peripheralQueue: map[int]Peripheral{},
		fields:          map[int]Register{},
		registerValues:  map[uint64]cachedRegister{},
		nextReference:   firstReference,
		peripheralRoot:  firstReference - 1,
	}
}

// WithRegisterMap gives the session a chip's register map.
//
// Given rather than found, because where a map comes from is a question with more
// than one right answer — the target spec's own path, a file the project vendors,
// a plugin that knows where this vendor keeps them — and the caller is what knows
// which of those applies.
func (s *Session) WithRegisterMap(registerMap *SVD, name string, diagnostics string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerMap = registerMap
	s.registerMapName = name
	s.registerMapDiagnostics = diagnostics
	return s
}

// Serve answers the editor until it stops asking.
//
// Sequential, deliberately: the protocol is a conversation, and handling two
// requests at once means a launch and a setBreakpoints in flight together — which
// is what produces a program that starts before its breakpoints are in. One
// request at a time, with the engine's notifications read on their own goroutine,
// because those arrive whether or not anything has been asked.
func (s *Session) Serve() error {
	go s.forward()

	for {
		message, err := s.codec.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if message.Type == MessageRequest {
			s.handle(message)
			continue
		}
		if message.Type == MessageResponse {
			// The editor answering something this session asked of it, which
			// happens with an adapter that runs something in a terminal.
			s.answer(message)
		}
	}
}

// answer passes the editor's answer to the engine that asked.
func (s *Session) answer(message Message) {
	relay, isRelay := s.engine.(*Adapter)
	if !isRelay {
		return
	}
	if err := relay.Answer(message); err != nil {
		s.report(err)
	}
}

// forward turns what the engine volunteers into the editor's events.
func (s *Session) forward() {
	for notification := range s.engine.Notifications() {
		switch notification.Kind {
		case NotifyStopped:
			s.mu.Lock()
			s.stopped = notification.Stop
			s.mu.Unlock()
			s.ForgetRegisterValues()
			_ = s.codec.Event(EventStopped, map[string]any{
				"reason":            string(notification.Stop.Kind),
				"threadId":          notification.Stop.Thread,
				"allThreadsStopped": true,
				"text":              notification.Text,
			})
			// A stop makes everything a person was looking at stale: the stack, the
			// registers, the locals. Told once rather than per scope, because the
			// editor asks for what it needs when it needs it.
			_ = s.codec.Event(EventInvalidated, map[string]any{
				"areas": []any{"stacks", "variables", "registers"},
			})
		case NotifyContinued:
			_ = s.codec.Event(EventContinued, map[string]any{
				"threadId":            s.stopped.Thread,
				"allThreadsContinued": true,
			})
		case NotifyExited:
			_ = s.codec.Event(EventExited, map[string]any{"exitCode": notification.ExitCode})
			_ = s.codec.Event(EventTerminated, map[string]any{})
		case NotifyTerminated:
			_ = s.codec.Event(EventTerminated, map[string]any{})
		case NotifyOutput:
			category := notification.Category
			if category == "" {
				category = OutputConsole
			}
			_ = s.codec.Output(category, notification.Text)
		case NotifyError:
			// An error the engine reported, shown rather than raised: an error
			// raised here would end the session over something the session can carry
			// on through, which is what a warning from a debugger is.
			if notification.Err != nil {
				_ = s.codec.Output(OutputStderr, notification.Err.Error()+"\n")
			}
		}
	}
}

// report puts something in the editor's console.
func (s *Session) report(err error) {
	if err == nil {
		return
	}
	_ = s.codec.Output(OutputStderr, err.Error()+"\n")
}

/* -------------------------------------------------------------------------- */
/*  Requests                                                                   */
/* -------------------------------------------------------------------------- */

// handle answers one request.
//
// Every failure is answered rather than raised. An editor waits for a response to
// every request it sends, and a session that exits instead of answering leaves a
// person looking at a spinner with no explanation and no way to find out why.
func (s *Session) handle(message Message) {
	var failure error
	switch message.Command {
	case RequestInitialize:
		failure = s.initialize(message)
	case RequestLaunch:
		failure = s.launch(message)
	case RequestAttach:
		failure = s.attach(message)
	case RequestConfiguration:
		failure = s.configured(message)
	case RequestSetBreakpoint:
		failure = s.breakpoints(message)
	case RequestThreads:
		failure = s.threads(message)
	case RequestStackTrace:
		failure = s.stack(message)
	case RequestScopes:
		failure = s.scopes(message)
	case RequestVariables:
		failure = s.variables(message)
	case RequestEvaluate:
		failure = s.evaluate(message)
	case RequestContinue, RequestNext, RequestStepIn, RequestStepOut, RequestPause:
		failure = s.resume(message)
	case RequestDisconnected, RequestTerminate:
		failure = s.disconnect(message)
	case RequestSetException, RequestSetVariable:
		// Accepted and not acted on. Answered successfully rather than refused:
		// an editor that asked can then go on to what it wanted, instead of
		// treating the whole session as broken over a feature it merely offered.
		failure = s.codec.Response(message, map[string]any{})
	default:
		// An unknown request is refused by name. It may be one this component has
		// never heard of, and saying so is what lets the editor fall back to its own
		// default rather than waiting for an answer that is not coming.
		failure = s.codec.Failure(message, fmt.Errorf("this adapter does not answer %s", message.Command))
	}
	if failure != nil {
		s.report(failure)
	}
}

func (s *Session) initialize(message Message) error {
	arguments := InitializeArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	capabilities, err := s.engine.Initialize(arguments)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	if err := s.codec.Response(message, capabilities); err != nil {
		return err
	}
	// The editor is told the session is ready before anything else is asked of it.
	// Sending it later produces an editor that waits for a session that is waiting
	// for the editor.
	return s.codec.Event(EventInitialized, map[string]any{})
}

func (s *Session) launch(message Message) error {
	arguments := LaunchArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	s.mu.Lock()
	s.configuration = message.Arguments
	s.mu.Unlock()

	if err := s.engine.Launch(arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	s.mu.Lock()
	s.launched = true
	s.mu.Unlock()

	if arguments.NoDebug {
		// A run rather than a debug session: there is nothing to wait for, so the
		// editor is told the configuration is done and the session carries on.
		return s.codec.Response(message, map[string]any{})
	}
	return s.codec.Response(message, map[string]any{})
}

func (s *Session) attach(message Message) error {
	arguments := AttachArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	if err := s.engine.Attach(arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	s.mu.Lock()
	s.attached = true
	s.mu.Unlock()
	return s.codec.Response(message, map[string]any{})
}

// configured is the editor saying it has sent every breakpoint it means to.
func (s *Session) configured(message Message) error {
	if err := s.engine.ConfigurationDone(); err != nil {
		return s.codec.Failure(message, err)
	}
	return s.codec.Response(message, map[string]any{})
}

func (s *Session) breakpoints(message Message) error {
	arguments := SetBreakpointsArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	file := arguments.Source.Path
	if file == "" {
		return s.codec.Failure(message, fmt.Errorf("the breakpoints name no file"))
	}

	// The editor's breakpoints are asked for in one form and answered in another.
	// Converted here rather than in the engine, so that an engine implements one
	// reading of a breakpoint and does not have to know what an editor calls one.
	wanted := make([]Wanted, 0, len(arguments.Breakpoints)+len(arguments.Lines))
	for _, one := range arguments.Breakpoints {
		if one.Line <= 0 {
			// A breakpoint with no line is one the editor has because the file has
			// not been read yet. Dropped rather than refused: the editor sends it
			// again when it has the file, and a refusal would put an error in its
			// console for something that has not gone wrong.
			continue
		}
		wanted = append(wanted, Wanted{
			Line:       one.Line,
			Condition:  one.Condition,
			HitCount:   one.HitCount,
			LogMessage: one.LogMessage,
		})
	}
	// `lines` is the older form, in which a breakpoint is a line number. Still
	// answered, because an editor may send either and refusing the older one would
	// make this adapter look like it does not work at all.
	for _, line := range arguments.Lines {
		if line > 0 {
			wanted = append(wanted, Wanted{Line: line})
		}
	}

	set, err := s.engine.SetBreakpoints(file, wanted)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	return s.codec.Response(message, map[string]any{"breakpoints": set})
}

func (s *Session) threads(message Message) error {
	threads, err := s.engine.Threads()
	if err != nil {
		return s.codec.Failure(message, err)
	}
	if threads == nil {
		threads = []Thread{}
	}
	return s.codec.Response(message, map[string]any{"threads": threads})
}

func (s *Session) stack(message Message) error {
	arguments := StackTraceArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	levels := Range{Skip: arguments.StartFrame, Frames: arguments.Levels}
	frames, err := s.engine.StackTrace(arguments.ThreadID, levels)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	if frames == nil {
		frames = []Frame{}
	}
	return s.codec.Response(message, map[string]any{
		"stackFrames": frames,
		"totalFrames": len(frames) + arguments.StartFrame,
	})
}

func (s *Session) scopes(message Message) error {
	arguments := ScopesArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	scopes, err := s.engine.Scopes(arguments.FrameID)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	// The register map is a scope of its own, appended rather than woven in: it is
	// not part of the frame, it is a description of the chip, and an editor that
	// showed it among the locals would make every other frame's scope a lie.
	scopes = append(scopes, s.peripheralScope())
	return s.codec.Response(message, map[string]any{"scopes": scopes})
}

func (s *Session) variables(message Message) error {
	arguments := VariablesArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	variables, err := s.variablesOf(arguments.VariablesReference)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	if variables == nil {
		variables = []Variable{}
	}
	return s.codec.Response(message, map[string]any{"variables": variables})
}

func (s *Session) evaluate(message Message) error {
	arguments := EvaluateArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return s.codec.Failure(message, err)
	}
	if strings.TrimSpace(arguments.Expression) == "" {
		return s.codec.Failure(message, fmt.Errorf("there is nothing to evaluate"))
	}
	reference := s.reference()
	answer, err := s.engine.Evaluate(arguments.Expression, arguments.FrameID, reference)
	if err != nil {
		return s.codec.Failure(message, err)
	}
	body := map[string]any{"result": answer.Value}
	if answer.Type != "" {
		body["type"] = answer.Type
	}
	if answer.VariablesReference != 0 {
		body["variablesReference"] = answer.VariablesReference
	}
	if answer.EvaluateName != "" {
		body["evaluateName"] = answer.EvaluateName
	}
	return s.codec.Response(message, body)
}

func (s *Session) resume(message Message) error {
	thread := 0
	arguments := ContinueArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return err
	}
	thread = arguments.ThreadID
	stepArguments := StepArguments{}
	if err := unmarshal(message.Arguments, &stepArguments); err != nil {
		return err
	}
	if stepArguments.ThreadID != 0 {
		thread = stepArguments.ThreadID
	}

	// A register value read before the program moved is a value of where it used to
	// be, so what has been read is dropped before anything is resumed.
	s.ForgetRegisterValues()

	var err error
	switch message.Command {
	case RequestContinue:
		err = s.engine.Resume(thread)
	case RequestNext:
		err = s.engine.Step(thread, StepOver)
	case RequestStepIn:
		err = s.engine.Step(thread, StepInto)
	case RequestStepOut:
		err = s.engine.Step(thread, StepOut)
	case RequestPause:
		err = s.engine.Pause(thread)
	}
	if err != nil {
		return s.codec.Failure(message, err)
	}
	body := map[string]any{}
	if message.Command == RequestContinue {
		body["allThreadsContinued"] = true
	}
	return s.codec.Response(message, body)
}

func (s *Session) disconnect(message Message) error {
	arguments := DisconnectArguments{}
	if err := unmarshal(message.Arguments, &arguments); err != nil {
		return err
	}
	err := s.codec.Response(message, map[string]any{})
	if stopErr := s.engine.Disconnect(arguments.TerminateDebuggee); stopErr != nil {
		s.report(stopErr)
	}
	_ = s.codec.Event(EventTerminated, map[string]any{})
	return err
}

/* -------------------------------------------------------------------------- */
/*  The register map                                                           */
/* -------------------------------------------------------------------------- */

// peripheralScope is the scope the chip's registers appear under.
//
// Reported as absent when there is no register map, rather than as a scope with
// nothing in it: an empty peripheral tree is read as a chip that has no
// peripherals, which is a different claim from one that has none available.
func (s *Session) peripheralScope() Scope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registerMap == nil {
		return Scope{
			Name:               "Peripherals",
			PresentationHint:   "registers",
			VariablesReference: 0,
			Expensive:          true,
		}
	}
	return Scope{
		Name:               "Peripherals",
		PresentationHint:   "registers",
		VariablesReference: s.peripheralRoot,
		NamedVariables:     len(s.registerMap.Peripherals),
	}
}

// variablesOf is what a reference holds, from whichever side holds it.
func (s *Session) variablesOf(reference int) ([]Variable, error) {
	s.mu.Lock()
	if reference == s.peripheralRoot {
		s.mu.Unlock()
		return s.peripherals()
	}
	peripheral, isPeripheral := s.peripheralQueue[reference]
	register, isRegister := s.fields[reference]
	s.mu.Unlock()
	switch {
	case isPeripheral:
		return s.registersOf(peripheral), nil
	case isRegister:
		return s.fieldsOf(register), nil
	}
	return s.engine.Variables(reference)
}

// peripherals are the chip's interfaces, as a list an editor can expand.
func (s *Session) peripherals() ([]Variable, error) {
	s.mu.Lock()
	registerMap := s.registerMap
	s.mu.Unlock()
	if registerMap == nil {
		return nil, fmt.Errorf("this session has no register map")
	}
	out := make([]Variable, 0, len(registerMap.Peripherals))
	for _, peripheral := range registerMap.Peripherals {
		base, parsed := peripheral.Address()
		entry := Variable{
			Name:  peripheral.Name,
			Type:  "peripheral",
			Value: peripheral.BaseAddress,
		}
		if peripheral.Description != "" {
			entry.EvaluateName = peripheral.Description
		}
		// Only an interface with a base address and a register is expandable. One
		// with neither is a cluster description that names no memory of its own,
		// and offering to expand it would show an empty list where the registers
		// are somewhere further down.
		if parsed && len(peripheral.AllRegisters()) > 0 {
			entry.VariablesReference = s.reference()
			s.mu.Lock()
			s.peripheralQueue[entry.VariablesReference] = peripheral
			s.mu.Unlock()
			entry.MemoryReference = base
		}
		out = append(out, entry)
	}
	return out, nil
}

// registersOf are a peripheral's registers, with their values read from the board.
//
// The values are read here rather than held: a register's value is a fact about
// right now, and a session that read them when the peripheral was first listed
// would show the state of the program at handshake for as long as the session
// lasted — which is the opposite of what a person stopped at a register to see.
func (s *Session) registersOf(peripheral Peripheral) []Variable {
	base, parsed := peripheral.Address()
	if !parsed {
		return nil
	}
	out := make([]Variable, 0, 8)
	for _, register := range peripheral.AllRegisters() {
		address, addressed := register.Address(base)
		entry := Variable{
			Name: register.Name,
			Type: registerType(register),
		}
		if !register.Readable() {
			// A write-only register reads back as whatever the hardware drives,
			// which is not what was written. Shown as what it is rather than as a
			// value nobody believes.
			entry.Value = "write-only"
			out = append(out, entry)
			continue
		}
		if !addressed {
			entry.Value = "no address"
			out = append(out, entry)
			continue
		}
		entry.Value = s.readRegister(address, register.Width())
		entry.MemoryReference = address
		entry.EvaluateName = register.Name
		if len(register.Fields) > 0 {
			entry.VariablesReference = s.reference()
			s.mu.Lock()
			s.fields[entry.VariablesReference] = register
			s.mu.Unlock()
		}
		out = append(out, entry)
	}
	return out
}

// registerType is a register described as its width, which is what tells a person
// reading the tree that `CTRL` is 32 bits and `STATUS` is 16.
func registerType(register Register) string {
	return fmt.Sprintf("uint%d", register.SizeBits())
}

// readRegister is what is at an address, formatted for a register view.
//
// Cached for the life of a stop rather than forever: expanding the same peripheral
// twice should not read the board twice, and reading it again after the program
// has moved on should not show the old value.
func (s *Session) readRegister(address uint64, width int) string {
	if cached, found := s.registerValues[address]; found {
		return cached.value
	}
	contents, err := s.engine.ReadMemory(address, width)
	if err != nil {
		// Cached like a value that was read: a register that cannot be read is
		// usually one that cannot be read at all, and retrying it for every
		// expansion of every peripheral is a slow operation over a debug link for
		// an answer that will be the same.
		s.mu.Lock()
		s.registerValues[address] = cachedRegister{value: "unreadable"}
		s.mu.Unlock()
		return "unreadable"
	}
	cached := cachedRegister{value: contents}
	if number, err := strconv.ParseUint(strings.TrimPrefix(contents, "0x"), 16, 64); err == nil {
		cached.number = number
		cached.numeric = true
	}
	s.mu.Lock()
	s.registerValues[address] = cached
	s.mu.Unlock()
	return contents
}

// ForgetRegisterValues drops what has been read, so the next expansion reads the
// board again.
//
// Called when the program moves, which is the only moment a value can be out of
// date: everything about the register file is still true, and everything about the
// value in it is now wrong.
func (s *Session) ForgetRegisterValues() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerValues = map[uint64]cachedRegister{}
}

// fieldsOf are a register's named fields, with the bits each one is holding.
func (s *Session) fieldsOf(register Register) []Variable {
	address, _ := register.Address(0)
	read, haveValue := s.registerValues[address]
	out := make([]Variable, 0, len(register.Fields))
	for _, field := range register.Fields {
		offset, width, described := field.Bounds()
		if !described {
			// A field this cannot place. Left out rather than guessed at: a shift by
			// a width nobody stated is a plausible number that means nothing, and
			// somebody reading it would believe it.
			continue
		}
		entry := Variable{
			Name: field.Name,
			Type: fmt.Sprintf("bit %d..%d", offset, offset+width-1),
		}
		if haveValue {
			if bits, ok := DecodeField(field, read.number, register.Width()); ok {
				entry.Value = fmt.Sprintf("0x%x", bits)
			}
		}
		out = append(out, entry)
	}
	return out
}

// Close ends the session: the engine, the probe, and nothing else.
//
// In that order, and deliberately. The probe holds the board, and a session that
// stopped the probe before the engine had finished would take the connection down
// under a debugger that is still writing to it — which produces a spurious error
// in the console of a session that ended perfectly well.
func (s *Session) Close() error {
	var failure error
	if err := s.engine.Close(); err != nil {
		failure = err
	}
	if err := s.probe.Stop(); err != nil && failure == nil {
		failure = err
	}
	return failure
}

// reference hands out a number the editor holds.
func (s *Session) reference() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextReference++
	return s.nextReference
}

// RegisterMapPath is where a session looks for a chip's register map.
//
// `roots` are the vendor trees of the platforms the project depends on, which is
// where a register map a platform carries once for every program built against it
// will be. Everything else is said by the reader in this package, which is where a
// session's facts are read.
func RegisterMapPath(declared string, projectRoot string, program string, roots []string) (string, bool) {
	return targetspec.FindFile(declared, projectRoot, program, roots)
}
