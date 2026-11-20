//
// @file supervision_test.go
// @brief The generated server under a supervisor: it stops at once, and it
// tells systemd it is ready and alive.
//
// A supervisor restarts a service by sending SIGTERM, and a page's live-update
// stream is a request that never ends by itself: the server ends its streams as
// shutdown begins, so a restart does not wait out the grace period. Under
// systemd the server also speaks the notify protocol — READY=1 when it listens,
// WATCHDOG=1 while its own /healthz answers, STOPPING=1 as it stops — and
// carries the marker a supervisor looks for before asking it to. Each of these
// is checked on a generated service, built and run, with a datagram socket of
// the test's own standing in for systemd's.
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
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const supervisedSource = `module supervised

import std.web.http

route GET "/stream" => stream
route GET "/later"  => later

fn stream(req: Request) Response {
    return sse_file(getenv("SUP_STREAM_FILE"), 20)
}

fn later(req: Request) Response {
    return sse_file(getenv("SUP_MISSING_FILE"), 20)
}

fn main() void {
    print("supervised")
}
`

const unhealthySource = `module supervised

import std.web.http

route GET "/healthz" => sick

fn sick(req: Request) Response {
    return text(status_internal(), "not well")
}

fn main() void {
    print("supervised")
}
`

// buildSupervised generates a Go server from one Rho source and builds it.
func buildSupervised(t *testing.T, source string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: supervised\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: go\n  targets:\n    - go-server\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "supervised")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated server did not build: %v\n%s", err, out)
	}
	return binary
}

// startSupervised runs a built server with extra environment and stops it when
// the test ends.
func startSupervised(t *testing.T, binary string, env ...string) (*exec.Cmd, string) {
	t.Helper()
	addr := freeAddress(t)
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append(withoutNotify(os.Environ()), append([]string{"RHO_HTTP_ADDR=" + addr}, env...)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd, "http://" + addr
}

// notifySocket is a stand-in for systemd's notify socket: every datagram the
// server sends arrives on the returned channel.
func notifySocket(t *testing.T, name string) <-chan string {
	t.Helper()
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: name, Net: "unixgram"})
	if err != nil {
		t.Fatalf("listen on %q: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	messages := make(chan string, 64)
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				close(messages)
				return
			}
			messages <- string(buffer[:n])
		}
	}()
	return messages
}

// awaitMessage waits for one datagram equal to want, and fails the test when it
// does not come in time.
func awaitMessage(t *testing.T, messages <-chan string, want string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case got, ok := <-messages:
			if !ok {
				t.Fatalf("the notify socket closed before %s arrived", want)
			}
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("no %s within %s", want, within)
		}
	}
}

// A server with live-update streams open stops within seconds of SIGTERM,
// rather than waiting for streams that never end by themselves. Two streams are
// open: one following a file, one still waiting for its file to appear.
func TestServerEndsItsStreamsAtShutdown(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	work := t.TempDir()
	streamFile := filepath.Join(work, "progress.log")
	if err := os.WriteFile(streamFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, base := startSupervised(t, binary,
		"SUP_STREAM_FILE="+streamFile, "SUP_MISSING_FILE="+filepath.Join(work, "never.log"))
	client := &http.Client{Timeout: 5 * time.Second}
	if !waitForServer(base, client) {
		t.Fatal("the server did not become ready")
	}

	// A stream has no timeout of its own: it is open until the server ends it.
	streamClient := &http.Client{}
	for _, path := range []string{"/stream", "/later"} {
		resp, err := streamClient.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("GET %s: status %d, content type %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	if err := os.WriteFile(streamFile, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("the server did not exit cleanly: %v", err)
		}
		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Fatalf("the server took %s to stop", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the server was still running 5 s after SIGTERM, with two streams open")
	}
}

// Under systemd the server says READY=1 once it listens, WATCHDOG=1 while its
// own /healthz answers, and STOPPING=1 when it is told to stop.
func TestServerNotifiesSystemd(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	socket := filepath.Join(t.TempDir(), "notify")
	messages := notifySocket(t, socket)
	cmd, _ := startSupervised(t, binary, "NOTIFY_SOCKET="+socket, "WATCHDOG_USEC=1000000")

	awaitMessage(t, messages, "READY=1", 10*time.Second)
	awaitMessage(t, messages, "WATCHDOG=1", 2*time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	awaitMessage(t, messages, "STOPPING=1", 5*time.Second)
}

// A watchdog meant for another process is not fed by this one.
func TestServerWatchdogIsForItsOwnPid(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	socket := filepath.Join(t.TempDir(), "notify")
	messages := notifySocket(t, socket)
	startSupervised(t, binary, "NOTIFY_SOCKET="+socket, "WATCHDOG_USEC=1000000", "WATCHDOG_PID=1")

	awaitMessage(t, messages, "READY=1", 10*time.Second)
	assertNoWatchdog(t, messages, 2500*time.Millisecond)
}

// A server whose /healthz fails is alive but cannot serve: it says it is ready,
// and then nothing, so systemd's watchdog restarts it. The socket is in the
// abstract namespace, which systemd names with a leading @.
func TestServerWithoutHealthSendsNoWatchdog(t *testing.T) {
	binary := buildSupervised(t, unhealthySource)
	socket := fmt.Sprintf("@rho-notify-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	messages := notifySocket(t, socket)
	_, base := startSupervised(t, binary, "NOTIFY_SOCKET="+socket, "WATCHDOG_USEC=1000000")

	awaitMessage(t, messages, "READY=1", 10*time.Second)
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("/healthz answered 200; the test needs it to fail")
	}
	assertNoWatchdog(t, messages, 2500*time.Millisecond)
}

// assertNoWatchdog fails the test when a WATCHDOG=1 arrives within the window.
func assertNoWatchdog(t *testing.T, messages <-chan string, window time.Duration) {
	t.Helper()
	deadline := time.After(window)
	for {
		select {
		case got := <-messages:
			if got == "WATCHDOG=1" {
				t.Fatal("WATCHDOG=1 was sent")
			}
		case <-deadline:
			return
		}
	}
}

// The binary carries the marker a supervisor reads to know it may ask for
// notification and a watchdog.
func TestServerBinaryCarriesTheNotifyMarker(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("rho-sd-notify-v1")) {
		t.Fatal("the generated server binary does not contain rho-sd-notify-v1")
	}
}

// Without NOTIFY_SOCKET the server says nothing about notification at all: a
// run by hand, or in CI, is what it was.
func TestServerWithoutNotifySocketIsQuiet(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	addr := freeAddress(t)
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append(withoutNotify(os.Environ()), "RHO_HTTP_ADDR="+addr, "WATCHDOG_USEC=1000000")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	if !waitForServer("http://"+addr, &http.Client{Timeout: 5 * time.Second}) {
		t.Fatal("the server did not become ready")
	}
	time.Sleep(1200 * time.Millisecond)
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Wait()
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "rho-sd-notify-v1") {
			t.Fatalf("a server with no notify socket spoke of it: %s", scanner.Text())
		}
	}
}

// withoutNotify drops the notify variables a test run under systemd inherits.
func withoutNotify(env []string) []string {
	out := env[:0:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, "NOTIFY_SOCKET=") || strings.HasPrefix(entry, "WATCHDOG_") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// The watchdog's own probes of /healthz are not written to the log; a request
// to /healthz from anyone else still is.
func TestServerWatchdogProbesAreNotLogged(t *testing.T) {
	binary := buildSupervised(t, supervisedSource)
	socket := filepath.Join(t.TempDir(), "notify")
	messages := notifySocket(t, socket)
	addr := freeAddress(t)
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append(withoutNotify(os.Environ()), "RHO_HTTP_ADDR="+addr, "NOTIFY_SOCKET="+socket, "WATCHDOG_USEC=1000000")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	awaitMessage(t, messages, "READY=1", 10*time.Second)
	awaitMessage(t, messages, "WATCHDOG=1", 2*time.Second)
	awaitMessage(t, messages, "WATCHDOG=1", 2*time.Second)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_, _ = cmd.Process.Wait()
	if got := strings.Count(output.String(), "GET /healthz"); got != 1 {
		t.Errorf("the log has %d lines for /healthz, want 1 (the request made by hand, none for the watchdog's probes):\n%s", got, output.String())
	}
}
