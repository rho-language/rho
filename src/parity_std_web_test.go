//
// @file parity_std_web_test.go
// @brief std.web.http and std.web.template, on the backend they are for.
//
// Both modules are for the Go server target (`@backends go-server`): a Response
// is the type the server backend generates, and the template renderer is the
// server's. So they are tested there — a generated service, built, started and
// asked over HTTP — and their refusal everywhere else is in backends_test.go.
//
// The service carries no template files. It still renders and escapes: a
// handler may fill markup it built itself, and a server generated without
// template files used to have no `templateFill` or `escapeHtml` at all.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const webManifest = `module:
  name: webstd
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: go
  targets:
    - go-server
`

const webSource = `module webstd

import std.web.http
import std.web.template

route GET "/text"         => plain
route GET "/json"         => created
route GET "/page"         => page
route GET "/escape"       => escaped
route GET "/redirect"     => moved
route GET "/redirect-bad" => moved_badly
route GET "/headers"      => headed
route GET "/codes"        => codes
route GET "/stream"       => stream
route GET "/events"       => events

fn plain(req: Request) Response {
    return text(status_ok(), "hello")
}

fn created(req: Request) Response {
    return json(status_created(), "{\"a\":1}")
}

fn page(req: Request) Response {
    let markup: string = "<p>{{ name }}</p>{{{ raw }}}{# gone #}{{#if flag}}[on]{{else}}[off]{{/if}}{{ missing ?? fallback }}"
    return html(status_ok(), fill(markup, vars("name", "a&b") + vars("raw", "<b>x</b>") + vars("flag", "1")))
}

fn escaped(req: Request) Response {
    return text(status_ok(), escape("<x>&\"'"))
}

fn moved(req: Request) Response {
    return redirect("/target")
}

fn moved_badly(req: Request) Response {
    // A line break would add a header of its own; the location becomes "/".
    return redirect("/x\nSet-Cookie: stolen=1")
}

fn headed(req: Request) Response {
    return with_headers(text(status_ok(), "h"), "X-Rho: yes")
}

fn codes(req: Request) Response {
    let all: string = int_to_str(status_ok()) + " " + int_to_str(status_created()) + " " + int_to_str(status_no_content()) + " " + int_to_str(status_bad_request()) + " " + int_to_str(status_unauthorized()) + " " + int_to_str(status_forbidden()) + " " + int_to_str(status_not_found()) + " " + int_to_str(status_conflict()) + " " + int_to_str(status_payload_too_large()) + " " + int_to_str(status_internal()) + " " + int_to_str(status_found())
    return text(status_ok(), all)
}

fn stream(req: Request) Response {
    return with_headers(sse_file(getenv("WEB_STREAM_FILE"), 20), "Access-Control-Allow-Origin: http://127.0.0.1:8478")
}

fn events(req: Request) Response {
    return sse_events(getenv("WEB_EVENTS_FILE"), getenv("WEB_SNAPSHOT_FILE"), 20)
}

fn main() void {
    print("webstd")
}
`

// freeAddress is a loopback address nothing is listening on.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

func TestStdWebOnTheGoServer(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(webManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(webSource), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "webstd")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated server did not build: %v\n%s", err, out)
	}

	work := t.TempDir()
	// The progress file does not exist yet: an action that starts after the page
	// connects writes it, and a file that appears while the stream waits is read
	// from its first line (one already there is followed from its end).
	streamFile := filepath.Join(work, "progress.log")
	snapshot := filepath.Join(work, "snapshot.jsonl")
	if err := os.WriteFile(snapshot, []byte(`{"event":"status","data":"<b>ready</b>"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventsFile := filepath.Join(work, "events.jsonl")
	if err := os.WriteFile(eventsFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	addr := freeAddress(t)
	server := exec.Command(binary)
	server.Dir = work
	server.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr, "WEB_STREAM_FILE="+streamFile,
		"WEB_EVENTS_FILE="+eventsFile, "WEB_SNAPSHOT_FILE="+snapshot)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	})
	client := &http.Client{
		Timeout: 5 * time.Second,
		// A redirect is what is being tested, so it is not followed.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	base := "http://" + addr
	if !waitForServer(base, client) {
		t.Fatal("the server did not become ready")
	}

	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := client.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	for _, c := range []struct {
		path, body, contentType string
		status                  int
		header, value           string
	}{
		{path: "/text", status: 200, body: "hello", contentType: "text/plain; charset=utf-8"},
		{path: "/json", status: 201, body: `{"a":1}`, contentType: "application/json"},
		{path: "/page", status: 200, body: "<p>a&amp;b</p><b>x</b>[on]fallback", contentType: "text/html; charset=utf-8"},
		{path: "/escape", status: 200, body: "&lt;x&gt;&amp;&quot;&#39;"},
		{path: "/redirect", status: 302, header: "Location", value: "/target"},
		{path: "/redirect-bad", status: 302, header: "Location", value: "/"},
		{path: "/headers", status: 200, body: "h", header: "X-Rho", value: "yes"},
		{path: "/codes", status: 200, body: "200 201 204 400 401 403 404 409 413 500 302"},
	} {
		resp, body := get(c.path)
		if resp.StatusCode != c.status {
			t.Errorf("GET %s: status %d, want %d", c.path, resp.StatusCode, c.status)
		}
		if c.body != "" && body != c.body {
			t.Errorf("GET %s: body %q, want %q", c.path, body, c.body)
		}
		if c.contentType != "" && resp.Header.Get("Content-Type") != c.contentType {
			t.Errorf("GET %s: content type %q, want %q", c.path, resp.Header.Get("Content-Type"), c.contentType)
		}
		if c.header != "" && resp.Header.Get(c.header) != c.value {
			t.Errorf("GET %s: %s is %q, want %q", c.path, c.header, resp.Header.Get(c.header), c.value)
		}
	}
	if resp, _ := get("/redirect-bad"); resp.Header.Get("Set-Cookie") != "" {
		t.Errorf("a location with a line break added a header: %q", resp.Header.Get("Set-Cookie"))
	}

	// sse_file sends each line as an event and closes on __DONE__.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(streamFile, []byte("step one\nstep two\n__DONE__:0\n"), 0o644)
	}()
	resp, body := get("/stream")
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("sse_file content type %q", resp.Header.Get("Content-Type"))
	}
	// A stream carries the headers its handler added (a CORS answer for a page
	// on another port).
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:8478" {
		t.Errorf("sse_file dropped the handler's header: Access-Control-Allow-Origin is %q", got)
	}
	for _, want := range []string{"data: step one", "data: step two", "event: done", "data: 0"} {
		if !strings.Contains(body, want) {
			t.Errorf("sse_file stream lacks %q:\n%s", want, body)
		}
	}

	// sse_events sends the snapshot first, as named events, and then follows.
	streamResp, err := client.Get(base + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer streamResp.Body.Close()
	reader := bufio.NewReader(streamResp.Body)
	seen := ""
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(seen, "data: <b>ready</b>") {
		line, err := reader.ReadString('\n')
		seen += line
		if err != nil {
			break
		}
	}
	if !strings.Contains(seen, "event: status") || !strings.Contains(seen, "data: <b>ready</b>") {
		t.Errorf("sse_events did not send the snapshot as a named event:\n%s", seen)
	}
}
