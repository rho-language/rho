//
// @file emit_go_sse_test.go
// @brief Tests for the Server-Sent Events helpers the Go backend emits.
//
// The streamer is emitted as source, so the behaviour is checked by building
// that source with a small harness. The test exercises the part the Rho side
// cannot: named events, multi-line HTML data, and a snapshot sent before the
// live file is followed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNamedEventHelpersAreEmitted checks the names an extern sse_events call
// resolves to and the response content type that selects the named stream.
func TestNamedEventHelpersAreEmitted(t *testing.T) {
	rendered := goResponseType()
	for _, want := range []string{
		"func sseEvents(path string, snapshot string, intervalMs uint32) Response",
		"func stdWebHttpSseEvents(path string, snapshot string, intervalMs uint32) Response",
		"func stdWebHttpResponseHeader(resp Response, headers string) Response",
		"writeNamedEventStream(ctx, w, resp.Body)",
		`"text/event-stream; events"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the emitted source does not contain %q", want)
		}
	}
}

const sseScaffold = `package main

import (
	"bufio"
	"context"
	jsonenc "encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Response struct {
	Status      uint32
	Body        string
	contentType string
	Headers     string
}

`

const sseTests = `package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeWriter struct {
	mu     sync.Mutex
	header http.Header
	body   strings.Builder
}

func (w *safeWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *safeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}

func (w *safeWriter) WriteHeader(int) {}

func (w *safeWriter) Flush() {}

func (w *safeWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func waitFor(t *testing.T, w *safeWriter, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in:\n%s", want, w.String())
}

func TestResponseHeaders(t *testing.T) {
	resp := stdWebHttpResponseHeader(Response{Status: 200, Body: "ok"}, "HX-Trigger: changed")
	if resp.Headers != "HX-Trigger: changed" {
		t.Fatalf("first header: %q", resp.Headers)
	}
	resp = stdWebHttpResponseHeader(resp, "HX-Redirect: /")
	if resp.Headers != "HX-Trigger: changed\nHX-Redirect: /" {
		t.Fatalf("second header: %q", resp.Headers)
	}
	if resp.Body != "ok" || resp.Status != 200 {
		t.Fatalf("the response changed: %#v", resp)
	}
}

func TestNamedEventStream(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.jsonl")
	snapshot := filepath.Join(dir, "snapshot.jsonl")
	if err := os.WriteFile(snapshot, []byte(
		"{\"event\":\"state\",\"data\":\"<b>a</b>\\n<i>b</i>\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, nil, 0644); err != nil {
		t.Fatal(err)
	}

	writer := &safeWriter{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		writeNamedEventStream(ctx, writer, live+sseSeparator+snapshot+sseSeparator+"10")
	}()

	waitFor(t, writer, "event: state")
	waitFor(t, writer, "data: <b>a</b>")
	waitFor(t, writer, "data: <i>b</i>")

	file, err := os.OpenFile(live, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(
		"{\"event\":\"widget:1\",\"data\":\"<div class=\\\"card\\\">x</div>\"}\n"); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	waitFor(t, writer, "event: widget:1")
	waitFor(t, writer, "data: <div class=\"card\">x</div>")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream did not stop after its context was cancelled")
	}
}
`

// TestNamedEventStream builds the emitted streamer and follows a snapshot and
// a live record, so the framing a browser receives is the framing tested.
func TestNamedEventStream(t *testing.T) {
	requireGoToolchain(t)
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":      "module sse\n\ngo 1.21\n",
		"sse.go":      sseScaffold + goResponseHelpers() + goSSEHelpers(),
		"sse_test.go": sseTests,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the emitted event stream failed its tests: %v\n%s", err, out)
	}
}
