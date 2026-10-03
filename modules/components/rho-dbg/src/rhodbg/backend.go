//
// @file backend.go
// @brief What a debugging session runs on: the contract an engine fills.
//
// GDB is not the debugger. It is the debugger this component ships with, which is
// the one every cross toolchain has and the one that speaks the Machine
// Interface. A board that is debugged with LLDB, with a probe that speaks
// CMSIS-DAP, or with anything else is a session on a different engine, and the
// session above it should not know or care which.
//
// So the session talks to this interface and never to a debugger. That is what
// makes "add another debugger" a plugin rather than a fork: an engine answers the
// same four kinds of question — what is stopped, where, what can be read, and
// what should happen next — and the editor's protocol is served identically.
//
// There are two ways for an engine to exist, and they differ only in where it
// runs:
//
//   a driver    in this process, implementing Backend directly. The GDB/MI engine
//               is one of these.
//   an adapter  another program, in its own process, speaking the Debug Adapter
//               Protocol itself. These are what plugins contribute, because a
//               debugger that already has an adapter — lldb-dap, probe-rs — needs
//               no wrapper of ours, only to be run and relayed to.
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
	"strings"
)

// HookEngine is the hook a plugin answers with the debuggers it provides.
const HookEngine = "engine"

// HookRegisterMap is the hook a plugin answers with where a chip's registers are
// described.
const HookRegisterMap = "register-map"

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-dbg"

/* -------------------------------------------------------------------------- */
/*  The contract                                                               */
/* -------------------------------------------------------------------------- */

// Backend is what a session runs on: the thing that is stopped, and the thing that
// decides what happens when it is told to continue.
//
// Everything an editor asks for is one of these calls. Anything a session needs
// that is not here — the program's stdout, a probe's log — arrives as a
// Notification on the channel, because an editor asks for it and a debugger
// volunteers it, and a design where a debugger has to be asked for its own output
// is a design where that output goes missing.
type Backend interface {
	// Describe says what this backend is, for a diagnostic a person reads when a
	// session does not start.
	Describe() Description

	// Initialize answers the editor's handshake with what this session can do.
	Initialize(InitializeArguments) (Capabilities, error)

	// Launch starts a program and stops where it was asked to.
	Launch(LaunchArguments) error
	// Attach attaches to one that is already running.
	Attach(AttachArguments) error
	// ConfigurationDone says the editor has sent every breakpoint it means to, so
	// the program may be run. A session that started the program before this is a
	// session that ran straight past the breakpoints.
	ConfigurationDone() error

	// SetBreakpoints replaces the breakpoints in one source, and answers with the
	// ones that are actually set — a breakpoint on a line with no code on it is
	// answered as unverified rather than refused, so the editor can grey it out.
	SetBreakpoints(file string, wanted []Wanted) ([]Breakpoint, error)

	// Threads are the threads the program has, including the ones it will create.
	Threads() ([]Thread, error)

	// StackTrace are the frames of one thread, most recent first, which is the
	// order a stack is read in and the order an editor shows them in.
	StackTrace(thread int, levels Range) ([]Frame, error)

	// Scopes are what can be read in one frame: the registers, the arguments, the
	// locals. An engine that cannot separate them says so rather than answering
	// with one scope called "variables".
	Scopes(frame int) ([]Scope, error)

	// Variables are what is in a scope or a structure, read through the handle the
	// scopes and the structures gave.
	Variables(reference int) ([]Variable, error)

	// Evaluate is the value of an expression in a frame.
	//
	// The reference is the number the editor will hold for it, given in so that an
	// engine whose values are structures can attach children to the number the
	// editor is about to be told about. Without it, every expression is a leaf and
	// a struct in a watch window is a wall of text.
	Evaluate(expression string, frame int, reference int) (Evaluation, error)

	// Resume lets a thread carry on.
	Resume(thread int) error
	// Step advances a thread one line, one call, or one return.
	Step(thread int, kind StepKind) error
	// Pause interrupts a thread that is running.
	Pause(thread int) error

	// ReadMemory is the value at an address, as a hexadecimal string.
	//
	// It is here rather than in the session because reading memory is the
	// debugger's privilege and not the editor's: an editor showing a register has
	// no business reaching around the debugger to do it.
	ReadMemory(address uint64, length int) (string, error)

	// Disconnect ends the session. Terminate says what happens to the program:
	// killed on a board, left running on a host.
	Disconnect(terminate bool) error

	// Notifications are what the debugger volunteers: a stop, an exit, a line of
	// its own output. Closed when the session is over.
	Notifications() <-chan Notification

	// Close releases everything this backend holds, including the processes it
	// started. It is called once the editor has been answered, so that a session
	// that is ending does not take the probe down underneath a message still on
	// its way to the editor.
	Close() error
}

// Description is what a backend is, as a diagnostic shows it.
type Description struct {
	// Name is the debugger, such as `arm-none-eabi-gdb` or `lldb-dap`.
	Name string `json:"name"`
	// Kind is how it runs: `driver` in this process, `adapter` as a program of its
	// own.
	Kind string `json:"kind"`
	// Program is the executable, when there is one.
	Program string `json:"program,omitempty"`
	// Arguments are what it was started with.
	Arguments []string `json:"arguments,omitempty"`
	// Port is the TCP port it is listening on, for an engine that is.
	Port string `json:"port,omitempty"`
	// Source says where it came from: this component, or a plugin.
	Source string `json:"source,omitempty"`
}

// The two kinds of engine.
const (
	// KindDriver is an engine in this process.
	KindDriver = "driver"
	// KindAdapter is another program speaking the Debug Adapter Protocol, relayed
	// to.
	KindAdapter = "adapter"
)

// String is a description as one line, for a log.
func (d Description) String() string {
	if d.Program == "" {
		return d.Name
	}
	return d.Name + " (" + strings.Join(append([]string{d.Program}, d.Arguments...), " ") + ")"
}

/* -------------------------------------------------------------------------- */
/*  What a session reads                                                       */
/* -------------------------------------------------------------------------- */

// Capabilities is what a session can do, which the editor is told at handshake and
// then relies on for the rest of the session.
//
// Every one of these is answered from what the engine actually supports rather
// than from what the protocol has. Advertising a capability that then fails is
// worse than not advertising it: the editor shows the feature, the person uses it,
// and the failure arrives as an error in a place they were not looking.
type Capabilities struct {
	// SupportsConfigurationDone is whether breakpoints may be set after the program
	// has started. True for a board, because the program on a board is already
	// running before anybody connects; false for a host, where this component
	// starts it.
	SupportsConfigurationDone         bool `json:"supportsConfigurationDone"`
	SupportsFunctionBreakpoints       bool `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints    bool `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsHitConditionalBreakpoints bool `json:"supportsHitConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers         bool `json:"supportsEvaluateForHovers,omitempty"`
	SupportsSetVariable               bool `json:"supportsSetVariable,omitempty"`
	SupportsTerminateRequest          bool `json:"supportsTerminateRequest"`
	SupportsDelayedStackTraceLoading  bool `json:"supportsDelayedStackTraceLoading,omitempty"`
	SupportTerminateDebuggee          bool `json:"supportTerminateDebuggee,omitempty"`
	SupportsRestartRequest            bool `json:"supportsRestartRequest,omitempty"`
	SupportsMemoryReferences          bool `json:"supportsMemoryReferences,omitempty"`
	SupportsProgressReporting         bool `json:"supportsProgressReporting,omitempty"`
	SupportsInvalidatedEvents         bool `json:"supportsInvalidatedEvents,omitempty"`
	SupportsExceptionInfoRequest      bool `json:"supportsExceptionInfoRequest,omitempty"`
	// ExceptionBreakpointFilters are the exception kinds this session can stop on.
	ExceptionBreakpointFilters []ExceptionFilter `json:"exceptionBreakpointFilters,omitempty"`
}

// ExceptionFilter is one kind of stop a session can offer.
type ExceptionFilter struct {
	Filter  string `json:"filter"`
	Label   string `json:"label,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// Thread is one thread of the program.
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Frame is one entry of a stack, and what an editor shows for it.
type Frame struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Line             int    `json:"line"`
	Column           int    `json:"column,omitempty"`
	Source           string `json:"source,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
}

// Range is how much of a stack is wanted, and what to skip to get there.
type Range struct {
	// Frames is how many. Zero means as many as there are.
	Frames int
	// Skip is how many to leave off the front — which is how an editor pages
	// through a deep stack without asking for all of it every time.
	Skip int
}

// Scope is one thing that can be read in a frame.
type Scope struct {
	Name string `json:"name"`
	// PresentationHint is how an editor labels it: `locals`, `arguments`,
	// `registers`, `globals`.
	PresentationHint string `json:"presentationHint,omitempty"`
	// VariablesReference is zero when there is nothing in it. A zero reference is
	// what tells an editor not to offer to expand it, so a scope with nothing in
	// it is a scope with a zero reference rather than an empty scope.
	VariablesReference int `json:"variablesReference"`
	// NamedVariables and IndexedVariables are counts, which let an editor show
	// "3 of 12" before asking for any of them.
	NamedVariables   int  `json:"namedVariables,omitempty"`
	IndexedVariables int  `json:"indexedVariables,omitempty"`
	Expensive        bool `json:"expensive,omitempty"`
}

// Variable is one value a session can show.
type Variable struct {
	// Name is what the editor lists it as.
	Name string `json:"name"`
	// Value is its value, rendered as text.
	Value string `json:"value"`
	// Type is what it is, where the debugger knows. Separate from the value
	// because a person deciding whether to expand something is deciding from the
	// type, and the value alone often says nothing about it.
	Type string `json:"type,omitempty"`
	// VariablesReference is zero when the value is not a structure.
	VariablesReference int `json:"variablesReference"`
	// EvaluateName is how the value can be asked for again, which is what a
	// watch window and a hover both do.
	EvaluateName     string `json:"evaluateName,omitempty"`
	NamedVariables   int    `json:"namedVariables,omitempty"`
	IndexedVariables int    `json:"indexedVariables,omitempty"`
	// MemoryReference is the address, for a value in memory. Zero for a register
	// or a local the debugger kept in a register bank.
	MemoryReference  uint64 `json:"memoryReference,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
}

// Breakpoint is one breakpoint, as it is actually set.
type Breakpoint struct {
	ID       int  `json:"id"`
	Verified bool `json:"verified"`
	// Message says why an unverified breakpoint is unverified. An unverified
	// breakpoint with no explanation is one a person cannot act on.
	Message string `json:"message,omitempty"`
	// Source is the file it is in, which is not always the file it was asked for:
	// a line with no code on it moves to the nearest line that has some.
	Source string `json:"source,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// Wanted is one breakpoint as the editor asked for it, before any debugger has
// had a say about where it actually is.
//
// It is not a Breakpoint because the two answer different questions: this one says
// what a person wanted, and the other says what the debugger did about it. Folding
// them into one type is how a condition ends up being reported as a verification
// failure.
type Wanted struct {
	// Line is where the editor put it. One-based, because every editor counts from
	// one and translating here would put the conversion in two places.
	Line int
	// Condition is the expression that has to be true for it to stop.
	Condition string
	// HitCount is how many times to let it pass before it stops.
	HitCount int
	// LogMessage is what the editor would print instead of stopping.
	LogMessage string
}

// Evaluation is what an expression came to.
type Evaluation struct {
	Value string
	Type  string
	// VariablesReference is zero when the value is not something that can be
	// expanded.
	VariablesReference int
	// EvaluateName is how the value can be asked for again.
	EvaluateName string
}

// StepKind is which way a step goes.
type StepKind string

// The three ways to move through a program.
const (
	StepOver StepKind = "over"
	StepInto StepKind = "into"
	StepOut  StepKind = "out"
)

// StopKind is why a session stopped, which is what the editor's status bar says.
type StopKind string

// The reasons a session stops. Each is one an engine can distinguish, because
// telling a person they stopped at a breakpoint when they stopped because the
// program faulted sends them looking in the wrong place.
const (
	StopBreakpoint StopKind = ReasonBreakpoint
	StopStep       StopKind = ReasonStep
	StopEntry      StopKind = ReasonEntry
	StopPause      StopKind = ReasonPause
	StopException  StopKind = ReasonException
	StopFunction   StopKind = ReasonFunction
)

// Notification is something the debugger volunteers.
type Notification struct {
	// Kind is what happened: a stop, an exit, a line of output, or an error the
	// debugger reported.
	Kind NotificationKind
	// Stop says where, when Kind is a stop.
	Stop Stop
	// ExitCode is how the program ended, when Kind is an exit.
	ExitCode int
	// Category and Text are a line of output, when Kind is output.
	Category string
	Text     string
	// Text is what an error was, when Kind is an error.
	Err error
}

// NotificationKind is what a notification is about.
type NotificationKind string

// The things a debugger volunteers.
const (
	// NotifyStopped is a stop: the reason, the thread, and where it is.
	NotifyStopped NotificationKind = "stopped"
	// NotifyContinued is a thread that was told to carry on.
	NotifyContinued NotificationKind = "continued"
	// NotifyExited is a program that ended, with its exit code.
	NotifyExited NotificationKind = "exited"
	// NotifyTerminated is a session that ended for any other reason.
	NotifyTerminated NotificationKind = "terminated"
	// NotifyOutput is a line from the debugger or the program's own console.
	NotifyOutput NotificationKind = "output"
	// NotifyError is something the debugger reported that is not a stop.
	NotifyError NotificationKind = "error"
)

// Stop is where a session is stopped, and why.
type Stop struct {
	Kind   StopKind
	Thread int
	// Reason is the text behind a stop that is not a plain one: a signal, a fault,
	// an exception name. An editor shows it in place of the reason, which is what
	// makes a Cortex-M fault legible — "program received signal SIGSEGV" says
	// nothing, the faulting address says something.
	Text string
}

/* -------------------------------------------------------------------------- */
/*  Engines as plugins                                                         */
/* -------------------------------------------------------------------------- */

// EngineRequest is what a plugin is asked: what this session needs a debugger
// for.
type EngineRequest struct {
	// Target is the board, as its spec names it. Empty when the session is on a
	// program running on this machine.
	Target string `json:"target"`
	// Program is the built binary, which is what an adapter that has to be told
	// what to load is told.
	Program string `json:"program,omitempty"`
	// Request is `launch` or `attach`, because an adapter that only does one of
	// them has to say which.
	Request string `json:"request,omitempty"`
	// Builtin are the engines this component already has, so a plugin need not
	// provide something that is here.
	Builtin []string `json:"builtin,omitempty"`
}

// EngineSpec is one debugger a plugin provides.
//
// It describes a program rather than an implementation, because a debugger this
// component has never heard of already has its own adapter — that is what
// lldb-dap is, and what probe-rs ships. Wrapping one in a protocol of ours would
// be a second implementation of something that works, and the one that would rot
// first.
type EngineSpec struct {
	// Name is how the engine is chosen, and what a diagnostic calls it.
	Name string `json:"name"`
	// Kind is `adapter` for a program speaking the Debug Adapter Protocol. It is
	// the only kind a plugin provides: an in-process driver would have to be
	// compiled into this component, which is the thing being avoided.
	Kind string `json:"kind"`
	// Targets are the boards this engine is for. Empty means it is a fallback, and
	// loses to any engine that names the board.
	Targets []string `json:"targets,omitempty"`
	// Program is the executable. `${target}` and `${program}` in it are filled in
	// from the session, which is what lets one spec serve two boards.
	Program string `json:"program"`
	// Args are its arguments, with the same substitution.
	Args []string `json:"args,omitempty"`
	// Comment is what this engine is for, shown by `rho dbg engines`.
	Comment string `json:"comment,omitempty"`
}

// Serves reports whether this engine is the one for a target.
//
// A named board wins, and a fallback loses to one that names it — otherwise
// enabling a plugin for a board would change nothing, which is the outcome a
// plugin nobody enabled has anyway.
func (s EngineSpec) Serves(target string) bool {
	for _, named := range s.Targets {
		if named == target {
			return true
		}
	}
	return false
}

// Expansion is the program an engine spec turns into for a session.
func (s EngineSpec) Expansion(target, program string) (string, []string) {
	replace := func(value string) string {
		value = strings.ReplaceAll(value, "${target}", target)
		return strings.ReplaceAll(value, "${program}", program)
	}
	arguments := make([]string, 0, len(s.Args))
	for _, argument := range s.Args {
		arguments = append(arguments, replace(argument))
	}
	return replace(s.Program), arguments
}

// Validate is a spec that is usable.
func (s EngineSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("an engine names nothing")
	}
	if strings.TrimSpace(s.Program) == "" {
		return fmt.Errorf("the engine %q names no program to run", s.Name)
	}
	if s.Kind != KindAdapter && s.Kind != "" {
		return fmt.Errorf("the engine %q is a %q, and only an %q can be provided by a plugin",
			s.Name, s.Kind, KindAdapter)
	}
	return nil
}

// RegisterMapRequest is what a plugin is asked about a chip's registers.
type RegisterMapRequest struct {
	Target string `json:"target"`
	SVD    string `json:"svd,omitempty"`
}

// RegisterMapResponse is what a plugin answers with: where the registers are
// described.
type RegisterMapResponse struct {
	// SVD is a CMSIS-SVD file, which is the form a vendor ships and the form this
	// component reads. A plugin that describes registers some other way would have
	// to describe them in this file anyway.
	SVD string `json:"svd,omitempty"`
	// Comment is what the map came from, shown when it is used.
	Comment string `json:"comment,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Plugin answers                                                             */
/* -------------------------------------------------------------------------- */

// pluginEngines reads the engines out of what a plugin answered.
func pluginEngines(payload []byte) ([]EngineSpec, error) {
	answer := struct {
		Engines []EngineSpec `json:"engines"`
	}{}
	if err := json.Unmarshal(payload, &answer); err != nil {
		return nil, err
	}
	return answer.Engines, nil
}
