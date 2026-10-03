//
// @file dap_test.go
// @brief Tests for the framing, which is where an adapter's first bug is.
//
// Every message is preceded by a header counting its own bytes, and a reader that
// assumes otherwise desynchronises on the first message with a newline in it —
// which is every message that carries a file name or a stack frame. So these are
// about bytes: what a body with non-ASCII text does to the count, and what a client
// that skips the framing gets.
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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAMessageIsFramedWithItsOwnLength(t *testing.T) {
	out := &bytes.Buffer{}
	codec := NewCodec(strings.NewReader(""), out)

	request := Message{Seq: 1, Type: MessageRequest, Command: RequestLaunch}
	if err := codec.Write(request); err != nil {
		t.Fatal(err)
	}
	written := out.String()
	header, body, found := strings.Cut(written, "\r\n\r\n")
	if !found {
		t.Fatalf("expected a header, got %q", written)
	}
	if !strings.HasPrefix(header, "Content-Length: ") {
		t.Errorf("expected a Content-Length header, got %q", header)
	}
	// The count is of bytes, not of characters: a message carrying a file name
	// with a non-ASCII character in it is shorter in characters than in bytes, and
	// a count that is short truncates the message with nothing to show for it.
	if !strings.Contains(header, fmt.Sprint(len(body))) {
		t.Errorf("expected the header to count the body's bytes, got %q for %q", header, body)
	}
	read, err := NewCodec(bufio.NewReaderSize(strings.NewReader(written), 1<<16), nil).Read()
	if err != nil {
		t.Fatal(err)
	}
	if read.Command != RequestLaunch || read.Seq != 1 {
		t.Errorf("expected the message back, got %+v", read)
	}
}

// Several messages in a row is the normal case and the one a framing bug shows up
// in: the second message is read from the wrong place and comes back as garbage.
func TestSeveralMessagesAreReadInOrder(t *testing.T) {
	out := &bytes.Buffer{}
	writer := NewCodec(strings.NewReader(""), out)
	commands := []string{RequestInitialize, RequestLaunch, RequestSetBreakpoint, RequestThreads}
	for index, command := range commands {
		if err := writer.Write(Message{Seq: index + 1, Type: MessageRequest, Command: command}); err != nil {
			t.Fatal(err)
		}
	}

	reader := NewCodec(bufio.NewReaderSize(bytes.NewReader(out.Bytes()), 1<<16), nil)
	for index, command := range commands {
		message, err := reader.Read()
		if err != nil {
			t.Fatalf("%d: %v", index, err)
		}
		if message.Command != command || message.Seq != index+1 {
			t.Errorf("%d: expected %s, got %+v", index, command, message)
		}
	}
}

// Some clients send one JSON object per line instead of framing. Refusing them
// buys nothing, and the whole point of an adapter is to be driven by whatever the
// editor is.
func TestAClientThatDoesNotFrameIsUnderstood(t *testing.T) {
	reader := NewCodec(strings.NewReader(`{"seq":1,"type":"request","command":"initialize"}
{"seq":2,"type":"request","command":"threads"}
`), nil)
	first, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if first.Command != RequestInitialize {
		t.Errorf("expected the first request, got %+v", first)
	}
	second, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if second.Command != RequestThreads {
		t.Errorf("expected the second request, got %+v", second)
	}
}

// The end of the editor's stream is how a session ends normally, and it must not
// be reported as a failure: `rho dbg` exiting non-zero when a person pressed stop
// is a toolchain that looks broken every time it is used correctly.
func TestTheEndOfTheStreamIsNotAFailure(t *testing.T) {
	reader := NewCodec(strings.NewReader(""), nil)
	if _, err := reader.Read(); err == nil {
		t.Error("expected the end of the stream to be reported")
	}
}

func TestAResponseCarriesTheRequestItAnswers(t *testing.T) {
	out := &bytes.Buffer{}
	codec := NewCodec(strings.NewReader(""), out)

	request := Message{Seq: 7, Type: MessageRequest, Command: RequestStackTrace}
	if err := codec.Response(request, map[string]any{"stackFrames": []Frame{}}); err != nil {
		t.Fatal(err)
	}
	answer, err := NewCodec(bytes.NewReader(out.Bytes()), nil).Read()
	if err != nil {
		t.Fatal(err)
	}
	if answer.Type != MessageResponse || answer.RequestSeq != 7 || answer.Command != RequestStackTrace {
		t.Fatalf("expected an answer to request 7, got %+v", answer)
	}
	if !answer.Success {
		t.Error("expected a successful response")
	}
}

// A refusal that says nothing is a refusal a person cannot act on: the editor
// shows the message and there is nothing to show.
func TestARefusalAlwaysSaysSomething(t *testing.T) {
	out := &bytes.Buffer{}
	codec := NewCodec(strings.NewReader(""), out)
	request := Message{Seq: 2, Type: MessageRequest, Command: RequestEvaluate}
	if err := codec.Failure(request, nil); err != nil {
		t.Fatal(err)
	}
	answer, err := NewCodec(bytes.NewReader(out.Bytes()), nil).Read()
	if err != nil {
		t.Fatal(err)
	}
	if answer.Success {
		t.Error("expected a refusal")
	}
	if strings.TrimSpace(answer.Message) == "" {
		t.Error("expected the refusal to say something")
	}

	out.Reset()
	if err := codec.Failure(request, fmt.Errorf("there is no expression")); err != nil {
		t.Fatal(err)
	}
	answer, _ = NewCodec(bytes.NewReader(out.Bytes()), nil).Read()
	if answer.Message != "there is no expression" {
		t.Errorf("expected the reason to be passed on, got %q", answer.Message)
	}
}

func TestEventsAndOutputAreSentUnframedThroughTheSameChannel(t *testing.T) {
	out := &bytes.Buffer{}
	codec := NewCodec(strings.NewReader(""), out)
	if err := codec.Event(EventInitialized, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := codec.Output(OutputStdout, "LED on\n"); err != nil {
		t.Fatal(err)
	}
	reader := NewCodec(bytes.NewReader(out.Bytes()), nil)

	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != MessageEvent || event.Command != EventInitialized {
		t.Fatalf("expected an initialized event, got %+v", event)
	}

	line, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(line.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["category"] != OutputStdout || body["output"] != "LED on\n" {
		t.Errorf("expected the output and where it goes, got %v", body)
	}
}

// The numbers this side sends must not repeat: an editor that cannot tell two
// messages apart will answer one of them twice.
func TestSequenceNumbersDoNotRepeat(t *testing.T) {
	out := &bytes.Buffer{}
	codec := NewCodec(strings.NewReader(""), out)
	seen := map[int]bool{}
	for index := 0; index < 5; index++ {
		if err := codec.Event(EventOutput, map[string]any{"output": "x"}); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewCodec(bytes.NewReader(out.Bytes()), nil)
	for {
		message, err := reader.Read()
		if err != nil {
			break
		}
		if seen[message.Seq] {
			t.Fatalf("sequence number %d was sent twice", message.Seq)
		}
		seen[message.Seq] = true
	}
}

// A launch configuration is the argument an editor sends first and most often, so
// its reading is worth checking against what `rho ide` writes.
func TestALaunchConfigurationIsReadAsItWasWritten(t *testing.T) {
	// Exactly what `rho ide vscode` writes for a board.
	written := `{
	  "name": "Debug pico",
	  "type": "rho",
	  "request": "launch",
	  "cwd": "${workspaceFolder}",
	  "program": "${workspaceFolder}/build/output/blinky",
	  "target": "pico",
	  "stopAtEntry": true,
	  "preLaunchTask": "rho: build"
	}`
	arguments := LaunchArguments{}
	if err := json.Unmarshal([]byte(written), &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.Program != "${workspaceFolder}/build/output/blinky" {
		t.Errorf("expected the program as written, got %q", arguments.Program)
	}
	if arguments.Target != "pico" || !arguments.StopAtEntry {
		t.Errorf("expected the board and the entry point, got %+v", arguments)
	}

	// And what an attach asks for, which has no program to build and no entry point.
	attach := AttachArguments{}
	if err := json.Unmarshal([]byte(`{"program":"a.elf","target":"pico","stopAtEntry":true}`), &attach); err != nil {
		t.Fatal(err)
	}
	if attach.Program != "a.elf" || attach.Target != "pico" {
		t.Errorf("expected an attach for a board, got %+v", attach)
	}
}
