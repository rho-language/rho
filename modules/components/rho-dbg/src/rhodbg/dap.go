//
// @file dap.go
// @brief The Debug Adapter Protocol: the messages an editor sends, and the ones
// it is answered with.
//
// The Debug Adapter Protocol is JSON-RPC with no envelope worth the name: a
// sequence number, a type, and a body whose shape depends on the request. What
// this file is for is the framing, which is the part an implementer gets wrong
// first — every message is preceded by a `Content-Length` header counting its own
// bytes, and a reader that assumes one JSON object per line desynchronises on the
// first message containing a newline and then fails in a way that looks like the
// editor sending nonsense.
//
// Line-delimited JSON is accepted as well, because some clients send it and
// refusing them buys nothing: a reader that handles both reads either.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// ProtocolVersion is the version of the Debug Adapter Protocol this implements.
//
// The editor is told this in its `initialize` answer and may refuse the session
// if it is one it does not speak, which is the right outcome: a mismatch that
// shows up as a session that hangs is much harder to read than one that is
// refused.
const ProtocolVersion = "1.0.0"

/* -------------------------------------------------------------------------- */
/*  Messages                                                                   */
/* -------------------------------------------------------------------------- */

// Message is one message in either direction: a request from the editor, a
// response to one, or an event with nothing asked for.
//
// The body is left as raw JSON and decoded by the caller, because what a message
// means depends entirely on which command it carries, and a struct with forty
// optional fields is not a better answer than that.
type Message struct {
	// Seq is the message's own number. A response carries the request's, so the
	// editor can tell which request it is answering.
	Seq int `json:"seq"`
	// Type is request, response or event.
	Type string `json:"type"`
	// Command is the request's name, or an event's name. Absent on a response,
	// which repeats the request's.
	Command string `json:"command,omitempty"`
	// Arguments is a request's body or an event's body.
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// RequestSeq is the sequence number of the request a response answers.
	RequestSeq int `json:"request_seq,omitempty"`
	// Success says whether a response is an answer or a refusal.
	Success bool `json:"success,omitempty"`
	// Message is what a refusal says. Empty on a successful response.
	Message string `json:"message,omitempty"`
	// Body is a response's body.
	Body json.RawMessage `json:"body,omitempty"`
}

// The message types and the success flag, as the protocol spells them.
const (
	MessageRequest  = "request"
	MessageResponse = "response"
	MessageEvent    = "event"

	successTrue  = true
	successFalse = false
)

// The requests this adapter answers. They are named here rather than inline
// because a handler that misses one is a request the editor will wait on until it
// times out, so the set is stated in one place and read by both ends of it.
const (
	RequestInitialize    = "initialize"
	RequestLaunch        = "launch"
	RequestAttach        = "attach"
	RequestConfiguration = "configurationDone"
	RequestSetBreakpoint = "setBreakpoints"
	RequestSetException  = "setExceptionBreakpoints"
	RequestThreads       = "threads"
	RequestStackTrace    = "stackTrace"
	RequestScopes        = "scopes"
	RequestVariables     = "variables"
	RequestSetVariable   = "setVariable"
	RequestEvaluate      = "evaluate"
	RequestContinue      = "continue"
	RequestNext          = "next"
	RequestStepIn        = "stepIn"
	RequestStepOut       = "stepOut"
	RequestPause         = "pause"
	RequestDisconnected  = "disconnect"
	RequestTerminate     = "terminate"
)

// The events this adapter sends without being asked.
const (
	EventInitialized   = "initialized"
	EventStopped       = "stopped"
	EventContinued     = "continued"
	EventExited        = "exited"
	EventTerminated    = "terminated"
	EventThreadStarted = "threadStarted"
	EventOutput        = "output"
	EventBreakpoint    = "breakpoint"
	EventInvalidated   = "invalidated"
)

// Why a session stopped, which is what the editor puts in its status bar.
const (
	ReasonBreakpoint = "breakpoint"
	ReasonStep       = "step"
	ReasonPause      = "pause"
	ReasonEntry      = "entry point"
	ReasonException  = "exception"
	ReasonFunction   = "function breakpoint"
)

// Output categories, which decide where a line of a debugger's own output goes in
// the editor's console.
const (
	OutputConsole = "console"
	OutputStdout  = "stdout"
	OutputStderr  = "stderr"
)

/* -------------------------------------------------------------------------- */
/*  Framing                                                                    */
/* -------------------------------------------------------------------------- */

// Codec reads and writes protocol messages on a pair of streams.
//
// Safe for one reader and one writer at a time, which is what a session is: the
// editor writes requests on one stream and this adapter writes answers on the
// other. The write side is locked because the debugger's own output is forwarded
// to the editor as it arrives, which is a different goroutine from the one
// answering requests.
type Codec struct {
	reader *bufio.Reader
	writer io.Writer
	// seq numbers the messages this side sends. An editor does not require the
	// numbers to mean anything in particular, but they must not repeat.
	seq  int
	lock sync.Mutex
}

// NewCodec is a codec over a reader and a writer.
func NewCodec(reader io.Reader, writer io.Writer) *Codec {
	return &Codec{reader: bufio.NewReader(reader), writer: writer}
}

// Next returns the sequence number the next message sent will carry.
func (c *Codec) Next() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.seq++
	return c.seq
}

// Read reads the next message, or reports that the stream has ended.
func (c *Codec) Read() (Message, error) {
	return readMessage(c.reader)
}

// readMessage reads one framed message.
//
// A header block is a run of `name: value` lines ended by a blank line, and the
// body is exactly as many bytes as `Content-Length` said. Anything before the
// first header line that is not a header is a line-delimited message, which is
// how a client that skips the framing is still understood.
func readMessage(reader *bufio.Reader) (Message, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return Message{}, io.EOF
			}
			return Message{}, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			// The blank line ends the header block. A body of zero length is a
			// message with nothing in it, which is a message that was not sent.
			if length < 0 {
				continue
			}
			break
		}
		// A line that opens a JSON object is a body, not a header. Decided on that
		// rather than on the absence of a colon, because a JSON message is full of
		// them: a client that sends one object per line would otherwise have the
		// first line of its message read as a header, and nothing after it would
		// ever be read at all.
		if strings.HasPrefix(strings.TrimSpace(trimmed), "{") {
			return parseBody([]byte(trimmed))
		}
		if !strings.Contains(trimmed, ":") {
			return parseBody([]byte(trimmed))
		}
		name, value, _ := strings.Cut(trimmed, ":")
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, parseErr := strconv.Atoi(strings.TrimSpace(value))
			if parseErr != nil {
				return Message{}, fmt.Errorf("Content-Length is not a number: %q", value)
			}
			length = parsed
		}
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return Message{}, fmt.Errorf("read a message of %d bytes: %w", length, err)
	}
	return parseBody(body)
}

func parseBody(body []byte) (Message, error) {
	message := Message{}
	if err := json.Unmarshal(body, &message); err != nil {
		return Message{}, fmt.Errorf("a message is not a message: %w", err)
	}
	return message, nil
}

// Write sends a message, framed.
//
// The header is written with the byte count in it rather than a count guessed
// from the body: a body holding a non-ASCII string has more bytes than it has
// characters, and a count that is short truncates the message in a way the reader
// cannot tell from a truncated file.
func (c *Codec) Write(message Message) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("render a message: %w", err)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, err := fmt.Fprintf(c.writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.writer.Write(body)
	return err
}

// Response is a successful answer to a request.
func (c *Codec) Response(request Message, body any) error {
	encoded, err := encode(body)
	if err != nil {
		return err
	}
	return c.Write(Message{
		Seq:        c.Next(),
		Type:       MessageResponse,
		Command:    request.Command,
		RequestSeq: request.Seq,
		Success:    successTrue,
		Body:       encoded,
	})
}

// Failure is a refusal of a request.
//
// A refusal always says something. A client that receives `success: false` with
// no message has nothing to show the person who pressed the button, and shows
// them nothing — which is indistinguishable from the editor being broken.
func (c *Codec) Failure(request Message, reason error) error {
	reason_ := "the request failed"
	if reason != nil && strings.TrimSpace(reason.Error()) != "" {
		reason_ = reason.Error()
	}
	return c.Write(Message{
		Seq:        c.Next(),
		Type:       MessageResponse,
		Command:    request.Command,
		RequestSeq: request.Seq,
		Success:    successFalse,
		Message:    reason_,
	})
}

// Event sends something the editor was not waiting for.
func (c *Codec) Event(name string, body any) error {
	encoded, err := encode(body)
	if err != nil {
		return err
	}
	return c.Write(Message{
		Seq:     c.Next(),
		Type:    MessageEvent,
		Command: name,
		Body:    encoded,
	})
}

// Output tells the editor about a line of text that came from somewhere else: the
// debugger's own console stream, the probe's log, the program's serial output.
//
// Sent unframed through the same codec rather than to the terminal, because a
// person debugging a board is watching the editor's console and nothing else.
func (c *Codec) Output(category, text string) error {
	return c.Event(EventOutput, map[string]any{
		"category": category,
		"output":   text,
	})
}

// unmarshal decodes a message body into a typed value.
//
// Errors are ignored rather than reported: it is used on bodies that arrived from
// another process, where a field that is not what was expected means the field is
// absent rather than that the session cannot go on. A session that stopped over it
// would stop for a field nobody asked for.
func unmarshal(body []byte, into any) error {
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, into)
}

// encodeBody renders a request body to send.
func encodeBody(body any) (json.RawMessage, error) { return encode(body) }

func encode(body any) (json.RawMessage, error) {
	if body == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("render a response body: %w", err)
	}
	return encoded, nil
}

/* -------------------------------------------------------------------------- */
/*  Request bodies                                                             */
/* -------------------------------------------------------------------------- */

// InitializeArguments is what an editor says when a session starts.
//
// Nothing in it is required and nothing in it is trusted: an editor that says
// nothing at all is still answered with the full set of capabilities, because the
// cost of refusing a session over a missing field is a person who cannot debug and
// has no way to find out why.
type InitializeArguments struct {
	ClientID                  string `json:"clientID,omitempty"`
	ClientName                string `json:"clientName,omitempty"`
	AdapterID                 string `json:"adapterID,omitempty"`
	PathFormat                string `json:"pathFormat,omitempty"`
	LinesStartAt1             bool   `json:"linesStartAt1,omitempty"`
	ColumnsStartAt1           bool   `json:"columnsStartAt1,omitempty"`
	SupportsVariableType      bool   `json:"supportsVariableType,omitempty"`
	SupportsVariablePaging    bool   `json:"supportsVariablePaging,omitempty"`
	SupportsRunInTerminal     bool   `json:"supportsRunInTerminal,omitempty"`
	SupportsMemoryRequests    bool   `json:"supportsMemoryRequests,omitempty"`
	SupportsProgressReporting bool   `json:"supportsProgressReporting,omitempty"`
	SupportsInvalidatedEvents bool   `json:"supportsInvalidatedEvents,omitempty"`
}

// LaunchArguments is what an editor says when a session starts a program.
//
// `program` is the built binary, `target` the board's name — the two a launch
// configuration written by `rho ide` carries — and everything else is a fact
// about how to reach the board that a person may need to override for the one
// board on their desk that is wired differently.
type LaunchArguments struct {
	// Program is the built program to load, as an absolute path or one relative
	// to the workspace. A path relative to here is resolved against the working
	// directory, which is what an editor starts an adapter with.
	Program string `json:"program,omitempty"`
	// Target is the board, as a rho-target spec names it. Read through
	// rho-target, so a board a plugin contributes is a board this session can be
	// run on.
	Target string `json:"target,omitempty"`
	// StopAtEntry is whether the session stops before the program's first
	// instruction rather than running to the first breakpoint.
	StopAtEntry bool `json:"stopAtEntry,omitempty"`
	// NoDebug is whether to start the program without a debugger attached to it,
	// which is what an editor sends for a plain run.
	NoDebug bool `json:"noDebug,omitempty"`
	// Malloc is the memory allocator to preload, which for a cross build is the
	// one the target's C library provides.
	Malloc string `json:"Malloc,omitempty"`
	// Cwd is the working directory the program runs in.
	Cwd string `json:"cwd,omitempty"`
	// Arguments and Environment are the program's own, rather than the
	// debugger's.
	Arguments   []string          `json:"args,omitempty"`
	Environment map[string]string `json:"env,omitempty"`

	// ServerPath is the probe server, when the target needs one and this machine
	// has it somewhere else than where the target spec says.
	ServerPath string `json:"serverpath,omitempty"`
	// ServerArgs are the probe server's own arguments, replacing the ones the
	// target spec declares.
	ServerArgs []string `json:"serverArgs,omitempty"`
	// GDBPath is the debugger program, for the same reason.
	GDBPath string `json:"gdbPath,omitempty"`
	// Servertype is what the debugger connects to — `gdb`, `pyocd`, `jlink` —
	// which is what tells one probe server from another at the far end.
	Servertype string `json:"servertype,omitempty"`
	// Device is the probe, as the vendor names it.
	Device string `json:"device,omitempty"`
	// Interface is the probe, as the server names it.
	Interface string `json:"interface,omitempty"`
	// ConfigFiles are the probe server's configuration files.
	ConfigFiles []string `json:"configFiles,omitempty"`
	// SVDFile is where the chip's register map is, for a project that ships one
	// the target spec's own path does not point at.
	SVDFile string `json:"svdFile,omitempty"`
	// Port is the TCP port the probe server listens on.
	Port string `json:"port,omitempty"`
	// Load says whether the program is written to the board at the start of the
	// session. A launch that flashes, and a relaunch that does not, are different
	// sessions and the difference is here rather than in a setting.
	Load *bool `json:"load,omitempty"`
	// Reset says how the board is reset before the session starts.
	Reset string `json:"reset,omitempty"`
	// RunToEntryPoint is a function to stop at rather than the program's entry.
	RunToEntryPoint string `json:"runToEntryPoint,omitempty"`
}

// AttachArguments is what an editor says when a session attaches to a program
// that is already running.
type AttachArguments struct {
	Program     string            `json:"program,omitempty"`
	Target      string            `json:"target,omitempty"`
	Port        string            `json:"port,omitempty"`
	ServerPath  string            `json:"serverpath,omitempty"`
	GDBPath     string            `json:"gdbPath,omitempty"`
	Servertype  string            `json:"servertype,omitempty"`
	Device      string            `json:"device,omitempty"`
	Interface   string            `json:"interface,omitempty"`
	ConfigFiles []string          `json:"configFiles,omitempty"`
	SVDFile     string            `json:"svdFile,omitempty"`
	ProcessID   int               `json:"processId,omitempty"`
	Environment map[string]string `json:"env,omitempty"`
	StopAtEntry bool              `json:"stopAtEntry,omitempty"`
}

// Source is a file, as the editor names one.
//
// The path is kept as the editor wrote it and only resolved when it is needed,
// because the editor's `${workspaceFolder}` is substituted on its side and a path
// resolved before that substitution has happened is a path to nothing.
type Source struct {
	Name             string `json:"name,omitempty"`
	Path             string `json:"path,omitempty"`
	SourceReference  int    `json:"sourceReference,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
}

// SourceBreakpoint is one breakpoint as the editor asks for it.
//
// A breakpoint with no line is one the editor has because the source has not been
// read yet, and it is dropped rather than refused: the editor resends it once the
// file is there, and a refusal would put an error in its console for something
// that has not gone wrong.
type SourceBreakpoint struct {
	Line       int     `json:"line"`
	Column     int     `json:"column,omitempty"`
	Condition  string  `json:"condition,omitempty"`
	HitCount   int     `json:"hitCondition,omitempty"`
	LogMessage string  `json:"logMessage,omitempty"`
	Source     *Source `json:"source,omitempty"`
}

// SetBreakpointsArguments replaces every breakpoint in one source.
type SetBreakpointsArguments struct {
	Source         SourceBreakpointsSource `json:"source"`
	Breakpoints    []SourceBreakpoint      `json:"breakpoints"`
	Lines          []int                   `json:"lines,omitempty"`
	SourceModified bool                    `json:"sourceModified,omitempty"`
}

// SourceBreakpointsSource is the file a set of breakpoints belongs to.
type SourceBreakpointsSource struct {
	Path            string `json:"path,omitempty"`
	Name            string `json:"name,omitempty"`
	SourceReference int    `json:"sourceReference,omitempty"`
}

// StackTraceArguments asks for the frames of one thread.
type StackTraceArguments struct {
	ThreadID   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

// ScopesArguments asks what can be read in one frame.
type ScopesArguments struct {
	FrameID int `json:"frameId"`
}

// VariablesArguments asks what is in one scope or structure.
type VariablesArguments struct {
	VariablesReference int    `json:"variablesReference"`
	Start              int    `json:"start,omitempty"`
	Count              int    `json:"count,omitempty"`
	Filter             string `json:"filter,omitempty"`
}

// EvaluateArguments asks for the value of an expression.
type EvaluateArguments struct {
	Expression string `json:"expression"`
	FrameID    int    `json:"frameId,omitempty"`
	Context    string `json:"context,omitempty"`
}

// ContinueArguments asks a thread to carry on.
type ContinueArguments struct {
	ThreadID int `json:"threadId"`
}

// StepArguments asks a thread to step.
type StepArguments struct {
	ThreadID int `json:"threadId"`
}

// DisconnectArguments asks a session to end.
type DisconnectArguments struct {
	TerminateDebuggee bool `json:"terminateDebuggee,omitempty"`
	Restart           bool `json:"restart,omitempty"`
	SuspendDebuggee   bool `json:"suspendDebuggee,omitempty"`
}
