//
// @file monitor_test.go
// @brief Reading a target's output: the transports, the choice between them, and
//        what is added to what the target printed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serve starts a TCP listener that answers every connection with `body`, once,
// and returns its port.
func serve(t *testing.T, body string) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				io := []byte(body)
				// Written in pieces, because a target prints in pieces and a
				// reader that only works when the whole line arrives at once is a
				// reader that works on a test and not on a board.
				for start := 0; start < len(io); start += 8 {
					end := start + 8
					if end > len(io) {
						end = len(io)
					}
					connection.Write(io[start:end])
					time.Sleep(time.Millisecond)
				}
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

/* -------------------------------------------------------------------------- */
/*  RTT                                                                       */
/* -------------------------------------------------------------------------- */

// RTT is a socket, so it is testable against a real listener — which is the point
// of it being a socket: the client opens no probe, so nothing about reading a
// target's output costs a debugger the board.
func TestRTTReadsWhatTheServerSends(t *testing.T) {
	port := serve(t, "[1 s] hello from the board\n")
	source := NewRTTSource("127.0.0.1", port, 0, time.Second)
	if err := source.Available(); err != nil {
		t.Fatalf("expected the server to be found: %v", err)
	}
	stream, err := source.Open()
	if err != nil {
		t.Fatal(err)
	}
	said, err := ReadAll(stream, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if said != "[1 s] hello from the board\n" {
		t.Fatalf("unexpected output: %q", said)
	}
}

// Nothing listening is said, with the address, rather than as a failure to find
// something at all — which is the difference between a person who knows where to
// look and one who does not.
func TestRTTWithNoServerSaysWhere(t *testing.T) {
	source := NewRTTSource("127.0.0.1", 1, 0, 200*time.Millisecond)
	err := source.Available()
	if err == nil {
		t.Fatal("expected no server to be found")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("expected the address named, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Detection                                                                 */
/* -------------------------------------------------------------------------- */

// The order is the design: the two transports that cost a debugger nothing come
// first, and the one that contends with a debugger comes last and is never chosen
// over a working alternative.
func TestTheOrderCostsADebuggerNothingUntilItMust(t *testing.T) {
	want := []Kind{KindRTT, KindUART, KindSWO}
	if len(AutoOrder) != len(want) {
		t.Fatalf("expected %v, got %v", want, AutoOrder)
	}
	for index, kind := range want {
		if AutoOrder[index] != kind {
			t.Fatalf("expected %v, got %v", want, AutoOrder)
		}
	}
}

// `auto` finds a working transport, and a live RTT server is the one it finds
// first.
func TestAutoFindsRTTWhenAServerIsThere(t *testing.T) {
	port := serve(t, "ready\n")
	config := DefaultConfig()
	config.RTT.Host, config.RTT.Port = "127.0.0.1", port
	config.SWO.File = filepath.Join(t.TempDir(), "nothing.swo")

	source, err := Detect(config, nil)
	if err != nil {
		t.Fatalf("expected a source: %v", err)
	}
	if source.Kind() != KindRTT {
		t.Fatalf("expected RTT first, got %s", source.Kind())
	}
}

// "Nothing to read" is the one answer a person cannot act on. Every transport is
// tried and every failure is reported, because which of the three was close is
// what turns it into something they can fix.
func TestDetectionSaysWhatItTriedWhenItFindsNothing(t *testing.T) {
	config := DefaultConfig()
	config.RTT.Host, config.RTT.Port = "127.0.0.1", 1
	config.UART.Port = filepath.Join(t.TempDir(), "no-such-console")
	config.Timeout = 200 * time.Millisecond
	config.SWO.File = filepath.Join(t.TempDir(), "no-such-capture.swo")

	_, err := Detect(config, nil)
	if err == nil {
		t.Fatal("expected nothing to be found")
	}
	said := err.Error()
	for _, kind := range Backends() {
		if !strings.Contains(said, string(kind)) {
			t.Errorf("expected %s to be reported as tried, got:\n%s", kind, said)
		}
	}
}

// A transport that was named is the only one tried, whatever else is available:
// somebody who asked for SWO is asking for it because nothing else works.
func TestANamedBackendSkipsTheRest(t *testing.T) {
	port := serve(t, "ready\n")
	config := DefaultConfig()
	config.Backend = KindSWO
	config.RTT.Host, config.RTT.Port = "127.0.0.1", port

	if _, err := Detect(config, nil); err == nil {
		t.Fatal("expected the named backend to be the only one tried")
	}
}

/* -------------------------------------------------------------------------- */
/*  The SWO decoder                                                           */
/* -------------------------------------------------------------------------- */

// An ITM packet is one, two or four bytes, and its low bit says whether another
// follows. That is the whole of the framing, and getting it wrong turns a trace
// into rubbish.
func TestSWODecodesConsolePackets(t *testing.T) {
	decoder := NewSWODecoder(nil)
	// Port 0, the software console, one character per packet. `h` is 0x68 and `i`
	// is 0x69, so the second needs the `0x03` header: a character's own bit 0 set
	// is exactly what the extra header bit is for.
	trace := []byte{0x01, 'h', 0x03, 'i'}
	if got := decoder.Feed(trace); got != "hi" {
		t.Fatalf("expected %q, got %q", "hi", got)
	}
	if emitted, _ := decoder.Counts(); emitted != 2 {
		t.Errorf("expected two console packets, got %d", emitted)
	}
}

// A packet split across two reads has to be joined before it can be read, because
// its length is only known once its last byte arrives. This is why the decoder has
// state at all.
func TestASWOPacketSplitAcrossReadsIsJoined(t *testing.T) {
	decoder := NewSWODecoder(nil)
	// The header arrived; the character has not. A console packet is two bytes,
	// so half of one is not a packet and there is nothing to print yet.
	if got := decoder.Feed([]byte{0x01}); got != "" {
		t.Fatalf("expected nothing before the packet is complete, got %q", got)
	}
	if decoder.Pending() != 1 {
		t.Fatalf("expected the partial packet to be held, got %d bytes", decoder.Pending())
	}
	if got := decoder.Feed([]byte{'a'}); got != "a" {
		t.Fatalf("expected the joined packet, got %q", got)
	}
}

// The header range carries the core's own instrumentation and no console output.
// Printed as text it would be the trace's rubbish in the middle of the firmware's
// words.
func TestSWODropsTheHeaderRange(t *testing.T) {
	decoder := NewSWODecoder(nil)
	// The header range is one byte each, which is what the core emits for an
	// overhead packet and a trace-format announcement.
	trace := []byte{0x00, 0x01, 'o', 0x01, 'k'}
	got := decoder.Feed(trace)
	if strings.ContainsAny(got, "\x00\x01\x02\x03") {
		t.Fatalf("a header packet was printed: %q", got)
	}
	if got != "ok" {
		t.Fatalf("expected the console output to survive, got %q", got)
	}
	if _, dropped := decoder.Counts(); dropped != 1 {
		t.Errorf("expected the header packet dropped, got %d", dropped)
	}
}

// A port this decoder was not asked for is not output. A firmware that traces its
// own counters on a data port would otherwise fill the console with its own
// instrumentation as binary.
func TestSWODropsPortsItWasNotAskedFor(t *testing.T) {
	decoder := NewSWODecoder([]int{ITMPortSoftware})
	// Port 1 is the hardware console, which this decoder was not asked for.
	if got := decoder.Feed([]byte{0x81, 'x'}); got != "" {
		t.Fatalf("expected an unasked port to be dropped, got %q", got)
	}
	// And the asked port survives, including a character whose bit 0 is set, which
	// is the case the framing has to get right or the console loses most of its
	// output.
	if got := decoder.Feed([]byte{0x03, 'y'}); got != "y" {
		t.Fatalf("expected the asked port to survive, got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Formatting                                                                */
/* -------------------------------------------------------------------------- */

// A level is the one word in a line that says how bad it was, and every log format
// spells it its own way. Matching only one spelling would colour half of what a
// monitor watches.
func TestALevelIsReadFromTheLine(t *testing.T) {
	cases := map[string]Level{
		"ERROR: bus fault":            LevelError,
		"[ERR] something went wrong":  LevelError,
		"warning: clock drifted":      LevelWarn,
		"WARN  near the limit":        LevelWarn,
		"info: started":               LevelInfo,
		"DEBUG entering loop":         LevelDebug,
		"[5 s] hello from the board":  LevelTrace,
		"value = 42":                  LevelTrace,
		"the error rate was measured": LevelTrace,
	}
	for line, want := range cases {
		if got := LevelOf(line); got != want {
			t.Errorf("%q: expected %v, got %v", line, want, got)
		}
	}
}

// A firmware that prints values with no level at all is printing values. Calling
// that a warning would be a monitor inventing a problem.
func TestAPlainLineIsOrdinaryOutput(t *testing.T) {
	if got := LevelOf("[412 s] counter = 7"); got != LevelTrace {
		t.Errorf("expected ordinary output, got %v", got)
	}
}

// The terminal is the only place colour belongs: a log file full of escape
// sequences is a file `grep` cannot read.
func TestColourIsOnlyWrittenWhenAsked(t *testing.T) {
	line := Line{Text: "ERROR: bus fault", Level: LevelError, At: time.Unix(0, 0)}
	plain := &bytes.Buffer{}
	NewFormatter(plain, Formatting{Timestamps: true, ColorLevels: true, Color: false}).Write(line)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("colour was written without being asked for: %q", plain.String())
	}

	colored := &bytes.Buffer{}
	NewFormatter(colored, Formatting{Timestamps: true, ColorLevels: true, Color: true}).Write(line)
	if !strings.Contains(colored.String(), "\x1b[31m") {
		t.Errorf("expected the error colour, got %q", colored.String())
	}
}

// A timestamp is when the line arrived, which is this machine's clock and not the
// target's — the target has no way to know when its bytes reached a terminal.
func TestATimestampIsWhenTheLineArrived(t *testing.T) {
	out := &bytes.Buffer{}
	formatter := NewFormatter(out, Formatting{Timestamps: true})
	if err := formatter.Write(Line{
		Text: "ready",
		At:   time.Date(2026, 10, 4, 15, 4, 5, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "15:04:05.000") {
		t.Errorf("expected the arrival time, got %q", out.String())
	}
}

// The log file gets the plain line: no escape sequences, and appended to rather
// than truncated, because a run that truncated it would destroy the run before it.
func TestTheLogFileIsAppendedAndPlain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "target.log")
	line := Line{Text: "ERROR: bus fault", Level: LevelError, At: time.Now()}

	for _, options := range []Formatting{
		{Timestamps: true, ColorLevels: true, Color: true},
		{Timestamps: true, ColorLevels: true, Color: false},
	} {
		formatter := NewFormatter(&bytes.Buffer{}, options)
		if err := formatter.AttachLog(path); err != nil {
			t.Fatal(err)
		}
		if err := formatter.Write(line); err != nil {
			t.Fatal(err)
		}
		formatter.Close()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\x1b[") {
		t.Errorf("the log file carries escape sequences: %q", string(data))
	}
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != 2 {
		t.Errorf("expected both runs recorded, got %d lines: %q", lines, string(data))
	}
}

// A firmware that prints without a newline produces a stream of partial lines, and
// printing each fragment as a line would double the output. The tail waits for the
// stream rather than assuming a newline is coming.
func TestAPartialLineIsPrintedOnceWhenTheStreamEnds(t *testing.T) {
	stream := &fakeStream{body: "no newline here"}
	out := &bytes.Buffer{}
	formatter := NewFormatter(out, Formatting{})
	formatter.SetClock(time.Now)
	if err := formatter.Tail(stream, make(chan struct{})); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "no newline here\n" {
		t.Fatalf("expected the held text printed once, got %q", got)
	}
}

// The stop has to work even when nothing is arriving: `--duration` on a silent
// board would otherwise wait for output that is never coming.
func TestTailStopsWhileWaitingForASilentTarget(t *testing.T) {
	stream := &blockingStream{}
	out := &bytes.Buffer{}
	formatter := NewFormatter(out, Formatting{})
	done := make(chan struct{})
	close(done)

	finished := make(chan error, 1)
	go func() { finished <- formatter.Tail(stream, done) }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("expected a stopped run to end cleanly, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Tail did not return after being told to stop")
	}
}

/* -------------------------------------------------------------------------- */
/*  Configuration                                                             */
/* -------------------------------------------------------------------------- */

// A project's block declares what is wrong with the defaults, and what it does not
// say keeps working.
func TestAMonitorBlockFillsInWhatItDoesNotSay(t *testing.T) {
	root := t.TempDir()
	manifest := `module:
  name: firmware
  type: module
  version: 0.1.0

main: src/main.rho

monitor:
  uart: { baud_rate: 230400 }
`
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	config := ReadConfig(root)
	if config.UART.BaudRate != 230400 {
		t.Errorf("expected the declared baud rate, got %d", config.UART.BaudRate)
	}
	// Everything else keeps its default: a block declaring one thing is not a
	// block declaring everything else to be nothing.
	if config.Backend != Kind("auto") {
		t.Errorf("expected auto, got %s", config.Backend)
	}
	if config.RTT.Port != 9090 || config.RTT.Host != "localhost" {
		t.Errorf("expected the default RTT endpoint, got %s", config.RTT.Endpoint())
	}
	if !config.Formatting.Timestamps {
		t.Error("expected timestamps on by default")
	}
}

// Naming a console is a decision not to guess, so it turns the scan off — a
// project that says which console it has must not be overridden by whichever node
// appears first.
func TestNamingAConsultTurnsTheScanOff(t *testing.T) {
	root := t.TempDir()
	manifest := `module:
  name: firmware
  type: module
  version: 0.1.0

main: src/main.rho

monitor:
  uart:
    port: /dev/ttyUSB3
`
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	config := ReadConfig(root)
	if config.UART.Port != "/dev/ttyUSB3" {
		t.Errorf("expected the declared console, got %q", config.UART.Port)
	}
	if config.UART.AutoDetect {
		t.Error("expected the scan off once a console is named")
	}
}

/* -------------------------------------------------------------------------- */
/*  Doubles for the streams                                                  */
/* -------------------------------------------------------------------------- */

// fakeStream is a stream with a body and an end.
type fakeStream struct {
	body   string
	closed bool
}

func (s *fakeStream) Describe() string { return "a test stream" }
func (s *fakeStream) Read(p []byte) (int, error) {
	if s.body == "" {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, s.body)
	s.body = s.body[n:]
	if s.body == "" {
		return n, fmt.Errorf("EOF")
	}
	return n, nil
}
func (s *fakeStream) Close() error { s.closed = true; return nil }

// blockingStream never returns, which is what a silent board looks like.
type blockingStream struct{ closed bool }

func (s *blockingStream) Describe() string { return "a silent board" }
func (s *blockingStream) Read(p []byte) (int, error) {
	select {}
}
func (s *blockingStream) Close() error { s.closed = true; return nil }
