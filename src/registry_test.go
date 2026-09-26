//
// @file registry_test.go
// @brief Integration test for the Rho module registry.
//
// The Rho tests in applications/registry/tests cover the request validation
// and the response shapes. What they cannot reach is the storage port and the
// HTTP protocol, because those only exist in the generated Go service. This
// test builds that service and exercises it over real HTTP.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rho

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// registryServer is a running registry built from the Rho sources.
type registryServer struct {
	base string
	dir  string
	// token is the publish token, empty when the registry is open.
	token string
	stop  func()
}

// startRegistry generates and builds the registry, then runs it on a loopback
// port with a private data directory.
func startRegistry(t *testing.T, token string) *registryServer {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	root := "../applications/registry"
	entry := filepath.Join(root, "src", "main.rho")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("registry sources are not present: %v", err)
	}

	dirs, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "go-server")
	if err != nil {
		t.Fatalf("generating the registry failed: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("expected the registry project to be generated")
	}
	serverDir := dirs[0].Dir

	binary := filepath.Join(serverDir, "registry-test")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = serverDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated registry did not build: %v\n%s", err, out)
	}

	data := t.TempDir()
	addr := "127.0.0.1:18711"
	cmd := exec.Command(binary)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"RHO_HTTP_ADDR="+addr,
		"RHO_REGISTRY_DATA="+data,
		"RHO_REGISTRY_TOKEN="+token,
	)
	logFile, err := os.CreateTemp(t.TempDir(), "registry-*.log")
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the registry failed: %v", err)
	}

	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	t.Cleanup(stop)

	server := &registryServer{base: "http://" + addr, dir: serverDir, token: token, stop: stop}
	client := &http.Client{Timeout: 5 * time.Second}
	if !waitForRegistry(server.base, client) {
		logged, _ := os.ReadFile(logFile.Name())
		t.Fatalf("the registry did not become ready\n%s", logged)
	}
	return server
}

func waitForRegistry(base string, client *http.Client) bool {
	for i := 0; i < 100; i++ {
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// request performs a request against the registry.
func (s *registryServer) request(t *testing.T, method, path string, body []byte, auth bool) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, s.base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if auth && s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(payload)
}

/* -------------------------------------------------------------------------- */
/*  Test: Publish and Download                                                 */
/* -------------------------------------------------------------------------- */

// The core round trip the Rho module client depends on: publish a package, then
// download the identical bytes.
func TestRegistryPublishesAndDownloads(t *testing.T) {
	server := startRegistry(t, "")

	// A gzip-like payload, including bytes that are not printable text.
	payload := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 'r', 'h', 'o', 0xff, 0xfe, 0x00}

	status, body := server.request(t, "POST", "/modules/math/0.1.0/rho-pkg", payload, false)
	if status != 201 {
		t.Fatalf("publish: got %d %s, want 201", status, body)
	}
	if !strings.Contains(body, `"name":"math"`) || !strings.Contains(body, `"version":"0.1.0"`) {
		t.Errorf("publish body should name the module and version, got %q", body)
	}

	status, body = server.request(t, "GET", "/modules/math/0.1.0/rho-pkg", nil, false)
	if status != 200 {
		t.Fatalf("download: got %d %s, want 200", status, body)
	}
	if body != string(payload) {
		t.Errorf("downloaded bytes differ:\n got %v\nwant %v", []byte(body), payload)
	}
}

// A second version of the same module is a separate entry.
func TestRegistryKeepsVersionsApart(t *testing.T) {
	server := startRegistry(t, "")

	if status, body := server.request(t, "POST", "/modules/math/0.1.0/rho-pkg", []byte("a"), false); status != 201 {
		t.Fatalf("publish 0.1.0: %d %s", status, body)
	}
	if status, body := server.request(t, "POST", "/modules/math/0.2.0/rho-pkg", []byte("b"), false); status != 201 {
		t.Fatalf("publish 0.2.0: %d %s", status, body)
	}

	status, body := server.request(t, "GET", "/modules/math/0.2.0/rho-pkg", nil, false)
	if status != 200 || body != "b" {
		t.Errorf("download 0.2.0: got %d %q, want 200 \"b\"", status, body)
	}
	status, body = server.request(t, "GET", "/modules/math", nil, false)
	if status != 200 {
		t.Fatalf("versions: %d %s", status, body)
	}
	for _, want := range []string{`"0.1.0"`, `"0.2.0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("versions should list %s, got %q", want, body)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Search                                                               */
/* -------------------------------------------------------------------------- */

func TestRegistrySearch(t *testing.T) {
	server := startRegistry(t, "")
	for _, module := range []string{"math", "math-linalg", "core"} {
		if status, body := server.request(t, "POST", "/modules/"+module+"/1.0.0/rho-pkg", []byte(module), false); status != 201 {
			t.Fatalf("publish %s: %d %s", module, status, body)
		}
	}

	status, body := server.request(t, "GET", "/search?q=math", nil, false)
	if status != 200 {
		t.Fatalf("search: %d %s", status, body)
	}
	if !strings.Contains(body, `"name":"math"`) || !strings.Contains(body, `"name":"math-linalg"`) {
		t.Errorf("search for math should match both, got %q", body)
	}
	if strings.Contains(body, `"name":"core"`) {
		t.Errorf("search for math should not match core, got %q", body)
	}

	// An empty query lists everything.
	status, body = server.request(t, "GET", "/search?q=", nil, false)
	if status != 200 || !strings.Contains(body, `"name":"core"`) {
		t.Errorf("an empty query should list everything, got %d %q", status, body)
	}
}

// The search result has to satisfy the client's ModuleInfo shape.
func TestRegistrySearchResponseShape(t *testing.T) {
	server := startRegistry(t, "")
	if status, _ := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("x"), false); status != 201 {
		t.Fatalf("publish failed with %d", status)
	}
	_, body := server.request(t, "GET", "/search?q=math", nil, false)
	for _, field := range []string{`"name"`, `"version"`, `"type"`, `"description"`, `"main"`, `"command"`, `"features"`, `"deps"`} {
		if !strings.Contains(body, field) {
			t.Errorf("search result is missing %s: %q", field, body)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Authentication                                                       */
/* -------------------------------------------------------------------------- */

func TestRegistryRequiresTokenToPublish(t *testing.T) {
	server := startRegistry(t, "secret")

	status, body := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("x"), false)
	if status != 401 {
		t.Fatalf("publishing without a token: got %d %s, want 401", status, body)
	}
	if !strings.Contains(body, "unauthorized") {
		t.Errorf("expected an unauthorized error, got %q", body)
	}

	status, body = server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("x"), true)
	if status != 201 {
		t.Fatalf("publishing with a token: got %d %s, want 201", status, body)
	}
}

// Reading does not require a token, so a client can install without credentials.
func TestRegistryReadsWithoutToken(t *testing.T) {
	server := startRegistry(t, "secret")
	if status, _ := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("x"), true); status != 201 {
		t.Fatalf("seed publish failed")
	}
	if status, _ := server.request(t, "GET", "/modules/math/1.0.0/rho-pkg", nil, false); status != 200 {
		t.Errorf("download without a token: got %d, want 200", status)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Failure Modes                                                        */
/* -------------------------------------------------------------------------- */

func TestRegistryRejectsDuplicateVersion(t *testing.T) {
	server := startRegistry(t, "")
	if status, _ := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("a"), false); status != 201 {
		t.Fatalf("first publish failed")
	}
	status, body := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte("b"), false)
	if status != 409 {
		t.Fatalf("duplicate publish: got %d %s, want 409", status, body)
	}
}

func TestRegistryRejectsEmptyPackage(t *testing.T) {
	server := startRegistry(t, "")
	status, body := server.request(t, "POST", "/modules/math/1.0.0/rho-pkg", []byte{}, false)
	if status != 400 {
		t.Fatalf("empty publish: got %d %s, want 400", status, body)
	}
}

func TestRegistryUnknownModuleIsNotFound(t *testing.T) {
	server := startRegistry(t, "")
	status, body := server.request(t, "GET", "/modules/nope/1.0.0/rho-pkg", nil, false)
	if status != 404 {
		t.Fatalf("missing module: got %d %s, want 404", status, body)
	}
}

// A module name that would escape the data directory must be refused, and
// nothing may be written outside it.
func TestRegistryRejectsPathTraversal(t *testing.T) {
	server := startRegistry(t, "")

	// Put the data directory inside a private parent, so anything written
	// outside it is visible.
	parent := t.TempDir()
	data := filepath.Join(parent, "data")
	if err := os.MkdirAll(data, 0755); err != nil {
		t.Fatal(err)
	}

	// Restart the service against the private data directory.
	server.stop()
	addr := strings.TrimPrefix(server.base, "http://")
	binary := filepath.Join(server.dir, "registry-test")
	cmd := exec.Command(binary)
	cmd.Dir = server.dir
	cmd.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr, "RHO_REGISTRY_DATA="+data)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	if !waitForRegistry(server.base, client) {
		t.Fatal("the registry did not restart")
	}

	// The URL path is normalised by net/http, so a traversal is attempted
	// through the encoded form and through a raw separator.
	attempts := []string{
		"/modules/..%2f..%2fescape/rho-pkg",
		"/modules/../../escape/1.0.0/rho-pkg",
		"/modules/..%5c..%5cescape/1.0.0/rho-pkg",
		"/modules/%2e%2e%2fescape/1.0.0/rho-pkg",
	}
	for _, path := range attempts {
		status, body := server.request(t, "POST", path, []byte("x"), false)
		if status == 201 {
			t.Errorf("traversal %q was accepted: %s", path, body)
		}
	}

	// Nothing may appear beside the data directory.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "data" {
			t.Errorf("traversal wrote %q outside the data directory", entry.Name())
		}
	}
	// And nothing unexpected inside it either.
	inner, err := os.ReadDir(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range inner {
		switch entry.Name() {
		case "modules", "index":
		default:
			t.Errorf("unexpected entry in the data directory: %s", entry.Name())
		}
	}
}

// The health endpoint is what a load balancer polls.
func TestRegistryHealth(t *testing.T) {
	server := startRegistry(t, "")
	status, body := server.request(t, "GET", "/healthz", nil, false)
	if status != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("healthz: got %d %q", status, body)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Deployment Shape                                                    */
/* -------------------------------------------------------------------------- */

// The service listens on the address it is given and honours its data
// directory, which is what the systemd and container units depend on.
func TestRegistryHonoursAddressAndDataDir(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := "../applications/registry"
	if _, err := os.Stat(filepath.Join(root, "src", "main.rho")); err != nil {
		t.Skip("registry sources are not present")
	}

	dirs, err := GenerateAllTargetsDetailed(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil, "go-server")
	if err != nil {
		t.Fatalf("generating the registry failed: %v", err)
	}
	serverDir := dirs[0].Dir
	if _, err := os.Stat(filepath.Join(serverDir, "ports.go")); err != nil {
		t.Errorf("the generated project should carry the storage port: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serverDir, "static")); err != nil {
		t.Errorf("the descriptor should create the static folder: %v", err)
	}

	// The service writes only under the data directory it is given.
	data := t.TempDir()
	addr := "127.0.0.1:18811"
	binary := filepath.Join(serverDir, "registry-addr-test")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = serverDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	cmd := exec.Command(binary)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr, "RHO_REGISTRY_DATA="+data)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	if !waitForRegistry("http://"+addr, client) {
		t.Fatalf("the registry did not listen on %s", addr)
	}
	if status, body := clientPost(fmt.Sprintf("http://%s/modules/probe/1.0.0/rho-pkg", addr), "x"); status != 201 {
		t.Fatalf("publish: %d %s", status, body)
	}
	if _, err := os.Stat(filepath.Join(data, "modules", "probe", "1.0.0", "package.rho-pkg")); err != nil {
		t.Errorf("the package should be under the configured data directory: %v", err)
	}
}

func clientPost(url, body string) (int, string) {
	resp, err := http.Post(url, "application/gzip", strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(payload)
}
