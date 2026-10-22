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
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/codegen"
	"rho/src/deps"
	"time"
)

// registryServer is a running registry built from the Rho sources.
type registryServer struct {
	base string
	dir  string
	// token is the secret the registry issues tokens with, and is empty on a
	// registry that issues them to anyone. It is not a credential a request
	// presents.
	token string
	// credential is a token this test holds and presents, which is what a
	// publish needs. It is separate from `token` because the two are different
	// things: one is how a caller gets in, the other is what the caller then
	// holds.
	credential string
	stop       func()
}

// startRegistry generates and builds the registry, then runs it on a loopback
// port with a private data directory.
func startRegistry(t *testing.T, token string) *registryServer {
	return startRegistryWithSettings(t, token, "")
}

// sqlComponentInstalled skips a test whose build needs the SQL component.
//
// The component turns a project's declared tables into the statements its
// generated code runs, so a registry cannot be built without it. That is a
// property of the toolchain a test is running against, and a machine that has
// not published the component has not got one — the same reason the sources and
// the Go toolchain are checked for above.
func sqlComponentInstalled(t *testing.T) {
	t.Helper()
	if _, err := componentBinary(SQLComponentName); err != nil {
		t.Skipf("the %s component is not installed or enabled: %v", SQLComponentName, err)
	}
	// The component generates the statements; the engine types them. Both are
	// modules a person turns on, so both are checked: a machine that has the
	// generator and not the engine would generate nothing and report it as a
	// generation failure, which is a much less useful thing to read.
	if deps.InstalledPluginRoot(SQLiteEnginePlugin) == "" {
		t.Skipf("the %s plugin is not installed or enabled; run: rho plugin enable %s",
			SQLiteEnginePlugin, SQLiteEnginePlugin)
	}
}

// resolveModules downloads what the generated project's go.mod names.
//
// A registry declared a database, so the generated project links the driver, and
// `go build` does not download what a go.mod names but has not verified. This is
// the step `rho build` takes before it builds; the harness builds the generated
// project itself, so it takes it here too.
func resolveModules(t *testing.T, dir string) {
	t.Helper()
	if !goModHasRequires(dir) {
		return
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOWORK=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("the registry's modules could not be resolved: %v\n%s", err, out)
	}
}

// startRegistryWithSettings runs the registry with a settings file, which is how
// a deployment configures it.
func startRegistryWithSettings(t *testing.T, token, settings string) *registryServer {
	t.Helper()
	// The registry routes through the standard library's ServeMux, which
	// carries the method-and-wildcard patterns its routes are declared with
	// from Go 1.22 — and the generated service it is built from declares the
	// Go this toolchain generates for.
	goToolchainAtLeast(t, codegen.GoModuleVersion)

	root := "../applications/registry"
	entry := filepath.Join(root, "src", "main.rho")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("registry sources are not present: %v", err)
	}
	// The registry keeps its records in the tables it declares, so building it
	// runs the generator that writes them. A machine without the component
	// installed cannot, and that is a fact about the machine rather than about
	// the registry — the same shape as the toolchain and the sources above.
	sqlComponentInstalled(t)

	dirs, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "go-server", "")
	if err != nil {
		t.Fatalf("generating the registry failed: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("expected the registry project to be generated")
	}
	serverDir := dirs[0].Dir

	binary := filepath.Join(serverDir, "registry-test")
	resolveModules(t, serverDir)
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = serverDir
	// The generated project is built on its own: it is a program of its own,
	// and the repository it was generated inside may have a Go workspace that
	// would otherwise claim it.
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated registry did not build: %v\n%s", err, out)
	}

	data := t.TempDir()
	addr := "127.0.0.1:18711"
	cmd := exec.Command(binary)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"RHO_HTTP_ADDR="+addr,
		"RHO_REGISTRY_TOKEN="+token,
		// Issuing is closed unless the caller is the administrator; the test
		// holds the secret the way a deployment's operator does.
		"RHO_REGISTRY_BOOTSTRAP_SECRET=test-bootstrap-secret",
	)
	if !strings.Contains(settings, "data_dir") {
		// An environment variable outranks the setting, so it is only set
		// when the test is not exercising the setting itself.
		cmd.Env = append(cmd.Env, "RHO_REGISTRY_DATA="+data)
	}
	if settings != "" {
		confPath := filepath.Join(t.TempDir(), "registry.conf")
		if err := os.WriteFile(confPath, []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd.Env = append(cmd.Env, "RHO_REGISTRY_CONFIG="+confPath)
	}
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
	// A publish needs a credential on every registry, because the registry
	// records who owns the module and an anonymous publish has nobody to record.
	// Minting one here rather than in each test keeps that a property of the
	// harness instead of something every test has to remember.
	server.credential = mintTokenAt(t, server.base, "harness")
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

// request performs a request against the registry, presenting a token when one
// is given.
func (s *registryServer) request(t *testing.T, method, path string, body []byte, token string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, s.base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

// publishModule publishes a package the way the module client does.
//
// It goes through the same request the client makes — the manifest fields as
// query parameters and a credential in the header — rather than a shortened
// form, so what the tests exercise is the contract a user meets. A publish with
// no author is refused by the registry, and a publish with no credential is
// refused on any registry, so a helper that left either out would test a path
// nobody can take.
func (s *registryServer) publishModule(t *testing.T, name, version, kind, author string, payload []byte, token string) (int, string) {
	t.Helper()
	// The manifest fields are encoded the way the client encodes them, because
	// an author's name is as likely to contain a space as a username is to
	// contain a dot, and an unencoded space truncates the value the registry
	// reads rather than failing visibly.
	fields := url.Values{}
	fields.Set("version", version)
	fields.Set("type", kind)
	fields.Set("main", "src/main.rho")
	fields.Set("author", author)
	path := fmt.Sprintf("/modules/%s/%s/rho-pkg?%s", name, version, fields.Encode())
	return s.request(t, "POST", path, payload, token)
}

// publish publishes as the registry's own author with the given token, which is
// what most of these tests need: they are testing storage, not ownership.
func (s *registryServer) publish(t *testing.T, name, version string, payload []byte) (int, string) {
	t.Helper()
	return s.publishModule(t, name, version, "module", "Aurel Miron", payload, s.credential)
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

	status, body := server.publishModule(t, "math", "0.1.0", "module", "Aurel Miron", payload, server.credential)
	if status != 201 {
		t.Fatalf("publish: got %d %s, want 201", status, body)
	}
	if !strings.Contains(body, `"name":"math"`) || !strings.Contains(body, `"version":"0.1.0"`) {
		t.Errorf("publish body should name the module and version, got %q", body)
	}

	status, body = server.request(t, "GET", "/modules/math/0.1.0/rho-pkg", nil, "")
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

	if status, body := server.publishModule(t, "math", "0.1.0", "module", "Aurel Miron", []byte("a"), server.credential); status != 201 {
		t.Fatalf("publish 0.1.0: %d %s", status, body)
	}
	if status, body := server.publishModule(t, "math", "0.2.0", "module", "Aurel Miron", []byte("b"), server.credential); status != 201 {
		t.Fatalf("publish 0.2.0: %d %s", status, body)
	}

	status, body := server.request(t, "GET", "/modules/math/0.2.0/rho-pkg", nil, "")
	if status != 200 || body != "b" {
		t.Errorf("download 0.2.0: got %d %q, want 200 \"b\"", status, body)
	}
	status, body = server.request(t, "GET", "/api/modules/math", nil, "")
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
		if status, body := server.publishModule(t, module, "1.0.0", "module",
			"Aurel Miron", []byte(module), server.credential); status != 201 {
			t.Fatalf("publish %s: %d %s", module, status, body)
		}
	}

	status, body := server.request(t, "GET", "/api/search?q=math", nil, "")
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
	status, body = server.request(t, "GET", "/api/search?q=", nil, "")
	if status != 200 || !strings.Contains(body, `"name":"core"`) {
		t.Errorf("an empty query should list everything, got %d %q", status, body)
	}
}

// The search result has to satisfy the client's ModuleInfo shape.
func TestRegistrySearchResponseShape(t *testing.T) {
	server := startRegistry(t, "")
	if status, _ := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("x"), server.credential); status != 201 {
		t.Fatalf("publish failed with %d", status)
	}
	_, body := server.request(t, "GET", "/api/search?q=math", nil, "")
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
	server := startRegistryWithSettings(t, "secret", "data_dir = "+filepath.Join(t.TempDir(), "d")+"\naccess = public\n")
	issued := server.mintToken(t, "miraur")

	status, body := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("x"), "")
	if status != 401 {
		t.Fatalf("publishing without a token: got %d %s, want 401", status, body)
	}
	if !strings.Contains(body, "token") {
		t.Errorf("expected the error to mention a token, got %q", body)
	}

	status, body = server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("x"), issued)
	if status != 201 {
		t.Fatalf("publishing with a token: got %d %s, want 201", status, body)
	}
}

// Reading on a registry needs no token, so a client installs without
// credentials; `access` governs publishing, not reading.
func TestRegistryPublicReadsWithoutToken(t *testing.T) {
	server := startRegistry(t, "")
	if status, _ := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("x"), server.credential); status != 201 {
		t.Fatalf("seed publish failed")
	}
	if status, _ := server.request(t, "GET", "/modules/math/1.0.0/rho-pkg", nil, ""); status != 200 {
		t.Errorf("download without a token: got %d, want 200", status)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Failure Modes                                                        */
/* -------------------------------------------------------------------------- */

func TestRegistryRejectsDuplicateVersion(t *testing.T) {
	server := startRegistry(t, "")
	if status, _ := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("a"), server.credential); status != 201 {
		t.Fatalf("first publish failed")
	}
	status, body := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("b"), server.credential)
	if status != 409 {
		t.Fatalf("duplicate publish: got %d %s, want 409", status, body)
	}
}

func TestRegistryRejectsEmptyPackage(t *testing.T) {
	server := startRegistry(t, "")
	issued := server.mintToken(t, "miraur")
	// An empty package is refused on its own terms, so the publish carries
	// everything else a real one would and only the body is missing.
	status, body := server.publishModule(t, "math", "1.0.0", "module",
		"Aurel Miron", []byte{}, issued)
	if status != 400 {
		t.Fatalf("empty publish: got %d %s, want 400", status, body)
	}
}

func TestRegistryUnknownModuleIsNotFound(t *testing.T) {
	server := startRegistry(t, "")
	status, body := server.request(t, "GET", "/modules/nope/1.0.0/rho-pkg", nil, "")
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
	cmd.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr, "RHO_REGISTRY_DATA="+data, "RHO_REGISTRY_BOOTSTRAP_SECRET=test-bootstrap-secret")
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
		status, body := server.request(t, "POST", path, []byte("x"), "")
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
		// The derived tree the documentation is generated from, and the
		// database the records are kept in. The index is not a file any more:
		// what a module holds is a table, not a document beside its packages.
		case "modules", "docs", "registry.db":
		default:
			t.Errorf("unexpected entry in the data directory: %s", entry.Name())
		}
	}
}

// The health endpoint is what a load balancer polls.
func TestRegistryHealth(t *testing.T) {
	server := startRegistry(t, "")
	status, body := server.request(t, "GET", "/healthz", nil, "")
	if status != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("healthz: got %d %q", status, body)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Deployment Shape                                                    */
/* -------------------------------------------------------------------------- */

// The service listens on the address it is given and honours its data
// directory, which is what the systemd and container units depend on.
// A public registry serves its modules to anyone, and a publish still needs a
// credential, because a module published without one belongs to nobody and is
// then a module nobody can add a version to or withdraw.
//
// The two halves are separate on purpose: `access` decides who may *publish*,
// and reading needs no credential at all. A registry is a registry anyone can
// install from; it is not one anyone can fill.
func TestRegistryPublicAccessAllowsReadingAndPublishingWithACredential(t *testing.T) {
	server := startRegistryWithSettings(t, "secret", "data_dir = "+filepath.Join(t.TempDir(), "d")+"\naccess = public\n")
	issued := server.mintToken(t, "miraur")

	// A publish with no credential is refused: the registry has to record who
	// owns the module, and an anonymous publish has nobody to record.
	status, body := server.request(t, "POST",
		"/modules/math/1.0.0/rho-pkg?version=1.0.0&type=module&main=src/main.rho&author=Aurel",
		[]byte("x"), "")
	if status != 401 {
		t.Errorf("publishing with no credential: got %d %s, want 401", status, body)
	}

	// With one it is accepted, and the owner is the identity in the token.
	status, body = server.publishModule(t, "math", "1.0.0", "module",
		"Aurel Miron", []byte("x"), issued)
	if status != 201 {
		t.Errorf("publishing with a credential: got %d %s, want 201", status, body)
	}

	// Reading stays public, which is what `access = public` means.
	status, body = server.request(t, "GET", "/modules/math/1.0.0/rho-pkg", nil, "")
	if status != 200 {
		t.Errorf("downloading with no credential: got %d %s, want 200", status, body)
	}
	if !strings.Contains(body, "x") {
		t.Errorf("the download should carry the package, got %q", body)
	}
}

// A publish that names no author is refused, because a registry that records an
// owner has to be able to show who wrote the module as well as who published it.
func TestRegistryRefusesAPublishWithNoAuthor(t *testing.T) {
	server := startRegistry(t, "")
	issued := server.mintToken(t, "miraur")
	status, body := server.request(t, "POST",
		"/modules/math/1.0.0/rho-pkg?version=1.0.0&type=module&main=src/main.rho",
		[]byte("x"), issued)
	if status != 400 {
		t.Fatalf("publish with no author: got %d %s, want 400", status, body)
	}
	if !strings.Contains(body, "author") {
		t.Errorf("the error should say what is missing, got %q", body)
	}
}

// The data directory comes from the settings, so an install cannot erase it.
func TestRegistryHonoursDataDirSetting(t *testing.T) {
	data := filepath.Join(t.TempDir(), "elsewhere")
	server := startRegistryWithSettings(t, "", "data_dir = "+data+"\n")
	issued := server.mintToken(t, "miraur")
	if status, _ := server.publishModule(t, "math", "1.0.0", "module", "Aurel Miron", []byte("x"), issued); status != 201 {
		t.Fatalf("publish failed with %d", status)
	}
	// The package is a row rather than a file, so what a publish produced is
	// read back from the database the data directory is beside.
	if !registryHasVersionAt(t, filepath.Join(data, "registry.db"), "math", "1.0.0") {
		t.Error("the published version is not in the database beside the data_dir")
	}
}

// A token may never outlive the ceiling the settings allow.
func TestRegistryTokenLifetimeHonoursSettings(t *testing.T) {
	server := startRegistryWithSettings(t, "", "data_dir = "+filepath.Join(t.TempDir(), "d")+"\ntoken_days = 5\ntoken_max_days = 7\n")

	asked := func(days string) int64 {
		// Issuing is closed unless the caller is the administrator, so the
		// request carries the administrator secret.
		request, err := http.NewRequest("POST",
			server.base+"/auth/token?user=x&host=y&days="+days, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer test-bootstrap-secret")
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		var answer struct {
			IssuedAt  int64 `json:"iat"`
			ExpiresAt int64 `json:"exp"`
		}
		if err := json.Unmarshal(payload, &answer); err != nil {
			t.Fatalf("cannot read the token response: %s", payload)
		}
		return (answer.ExpiresAt - answer.IssuedAt) / 86400
	}

	if got := asked("3"); got != 3 {
		t.Errorf("a 3 day request should be honoured, got %d days", got)
	}
	if got := asked("999"); got != 7 {
		t.Errorf("a 999 day request should be clamped to the 7 day ceiling, got %d", got)
	}
	if got := asked("0"); got != 5 {
		t.Errorf("no request should use the 5 day default, got %d", got)
	}
}

func TestRegistryHonoursAddressAndDataDir(t *testing.T) {
	// The registry routes through the standard library's ServeMux, which needs
	// Go 1.22 for the patterns its routes are declared with, and the generated
	// service declares the Go this toolchain generates for.
	goToolchainAtLeast(t, codegen.GoModuleVersion)
	root := "../applications/registry"
	if _, err := os.Stat(filepath.Join(root, "src", "main.rho")); err != nil {
		t.Skip("registry sources are not present")
	}
	// The registry's records are the tables it declares, so its build runs the
	// generator that writes them. See sqlComponentInstalled.
	sqlComponentInstalled(t)

	dirs, err := GenerateAllTargetsDetailed(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil, "go-server", "")
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
	resolveModules(t, serverDir)
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = serverDir
	// The generated project is built on its own: it is a program of its own,
	// and the repository it was generated inside may have a Go workspace that
	// would otherwise claim it.
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	cmd := exec.Command(binary)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr, "RHO_REGISTRY_DATA="+data, "RHO_REGISTRY_BOOTSTRAP_SECRET=test-bootstrap-secret")
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
	// The harness below builds its own server, so it presents a credential of
	// its own rather than borrowing the one the shared helper would mint: what
	// this test is about is the address and the data directory, not who may
	// publish.
	fields := url.Values{}
	fields.Set("version", "1.0.0")
	fields.Set("type", "module")
	fields.Set("author", "Aurel Miron")
	probe := fmt.Sprintf("http://%s/modules/probe/1.0.0/rho-pkg?%s", addr, fields.Encode())
	issued := mintTokenAt(t, "http://"+addr, "miraur")
	if status, body := clientPost(probe, "x", issued); status != 201 {
		t.Fatalf("publish: %d %s", status, body)
	}
	if !registryHasVersionAt(t, filepath.Join(data, "registry.db"), "probe", "1.0.0") {
		t.Error("the published version is not in the database beside the configured data directory")
	}
}

// registryHasVersionAt reports whether a version was published into a database.
//
// The package is a row rather than a file, so a test that wants to know a
// publish happened reads the table the publish wrote rather than looking for a
// file beside it.
func registryHasVersionAt(t *testing.T, database, name, version string) bool {
	t.Helper()
	// The database is read as a file rather than through a driver: this test
	// binary links no driver, and whether a row is in the file is a question
	// about the publish rather than about the Go that asks.
	data, err := os.ReadFile(database)
	if err != nil {
		t.Fatalf("reading %s: %v", database, err)
	}
	if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		t.Fatalf("%s is not a database, it begins %q", database, data[:16])
	}
	// The table's own text is in the file, so a version that was published is
	// in it and one that was not is not. This is a smoke test that the publish
	// reached the database; the service's own tests cover what it contains.
	return bytes.Contains(data, []byte(name)) && bytes.Contains(data, []byte(version))
}

// clientPost posts a package, presenting a credential when one is given.
func clientPost(url, body, token string) (int, string) {
	request, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	request.Header.Set("Content-Type", "application/gzip")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(payload)
}

/* -------------------------------------------------------------------------- */
/*  Test: Pages and Settings                                                  */
/* -------------------------------------------------------------------------- */

// registryPost publishes a package, optionally with a credential.

// seedRegistry publishes a few versions so the pages have something to show.
// It mints a token first, so it works whatever the access mode is.
func seedRegistry(t *testing.T, server *registryServer) {
	t.Helper()
	token := server.mintToken(t, "seed")
	for _, module := range []struct{ name, version string }{
		{"math", "0.1.0"},
		{"math", "0.2.0"},
		{"rho-lsp", "0.1.0"},
	} {
		body := "package-" + module.name + "-" + module.version
		if status, text := server.publishModule(t, module.name, module.version, "module",
			"Aurel Miron", []byte(body), token); status != 201 {
			t.Fatalf("publish %s@%s: %d %s", module.name, module.version, status, text)
		}
	}
}

// mintToken asks the registry for a token, the way a client would.
func (s *registryServer) mintToken(t *testing.T, user string) string {
	t.Helper()
	return mintTokenAt(t, s.base, user)
}

// mintTokenAt is mintToken for a server this test started itself, which is why
// it takes the address rather than a registry: a test with its own harness
// should not have to build a registryServer just to be given a credential.
func mintTokenAt(t *testing.T, base, user string) string {
	t.Helper()
	// Issuing is closed unless the caller is the administrator, so the test
	// presents the administrator secret the way a deployment's operator would.
	t.Setenv("RHO_REGISTRY_BOOTSTRAP_SECRET", "test-bootstrap-secret")
	request, err := http.NewRequest("POST",
		base+"/auth/token?user="+user+"&host=test&days=1&scopes=pull%20push", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-bootstrap-secret")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	var answer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &answer); err != nil || answer.Token == "" {
		t.Fatalf("no token issued: %s", payload)
	}
	return answer.Token
}

// getPage fetches a page and returns its status and body.
func getPage(t *testing.T, base, path string) (int, string) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(payload)
}

// The index lists each module once, badged with its newest version.
func TestRegistryIndexBadgesNewestVersion(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	status, body := getPage(t, server.base, "/")
	if status != 200 {
		t.Fatalf("index: got %d", status)
	}
	if strings.Count(body, `>math</span>`) != 1 {
		t.Errorf("the index should list math once, got:\n%s", body)
	}
	// The badge is the newest version, and it links to that version's page.
	if !strings.Contains(body, `>0.2.0</a>`) {
		t.Errorf("the badge should be the newest version 0.2.0, got:\n%s", body)
	}
	// Scoped to math: another module may legitimately hold an older version.
	if strings.Contains(body, `href="/modules/math/0.1.0"`) {
		t.Errorf("the index should not link an older version of math, got:\n%s", body)
	}
}

// A module page lists every version, newest first.
func TestRegistryModulePageListsVersionsNewestFirst(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	status, body := getPage(t, server.base, "/modules/math")
	if status != 200 {
		t.Fatalf("module page: got %d", status)
	}
	newest := strings.Index(body, ">0.2.0</span>")
	oldest := strings.Index(body, ">0.1.0</span>")
	if newest < 0 || oldest < 0 {
		t.Fatalf("expected both versions, got:\n%s", body)
	}
	if newest > oldest {
		t.Errorf("versions should be newest first, got:\n%s", body)
	}
}

// A version page shows the public metadata for that version.
func TestRegistryVersionPageShowsMetadata(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	status, body := getPage(t, server.base, "/modules/math/0.1.0")
	if status != 200 {
		t.Fatalf("version page: got %d", status)
	}
	for _, want := range []string{"math", "0.1.0", "rho-pkg"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q on the version page, got:\n%s", want, body)
		}
	}
}

func TestRegistryVersionPageUnknownVersion(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	if status, _ := getPage(t, server.base, "/modules/math/9.9.9"); status != 404 {
		t.Errorf("an unknown version should be 404, got %d", status)
	}
	if status, _ := getPage(t, server.base, "/modules/nope"); status != 404 {
		t.Errorf("an unknown module should be 404, got %d", status)
	}
}

// Every page carries the footer: the registry version, the year, and a link.
func TestRegistryFooterOnEveryPage(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	pages := []string{"/", "/search", "/search?q=math", "/modules/math", "/modules/math/0.2.0"}
	for _, path := range pages {
		status, body := getPage(t, server.base, path)
		if status != 200 {
			t.Errorf("%s: got %d", path, status)
			continue
		}
		for _, want := range []string{"<footer>", "Rho programming language", "rho-lang.org", "v0.1.0"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: expected %q in the footer, got:\n%s", path, want, body)
			}
		}
	}
}

// The client surface stays JSON while the pages stay HTML.
func TestRegistryAPIIsJSON(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)

	status, body := getPage(t, server.base, "/api/modules/math")
	if status != 200 || !strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Errorf("the versions endpoint should be JSON, got %d %q", status, body)
	}
	status, body = getPage(t, server.base, "/api/search?q=math")
	if status != 200 || !strings.HasPrefix(strings.TrimSpace(body), "[") {
		t.Errorf("the search endpoint should be JSON, got %d %q", status, body)
	}
}

// A public registry (`access = public`, the default) serves reads to anyone:
// search, a download, and a module's documentation need no credential.
//
// This is a separate test from the secured one because the harness gives each
// registry a fixed loopback port, so a test may only run one at a time.
func TestRegistryPublicServesReadsToAnyone(t *testing.T) {
	public := startRegistry(t, "")
	seedRegistry(t, public)
	if status, body := getPage(t, public.base, "/api/search?q=math"); status != 200 {
		t.Errorf("a public registry should serve search, got %d %q", status, body)
	}
	if status, _ := getPage(t, public.base, "/modules/math/0.1.0/rho-pkg"); status != 200 {
		t.Errorf("a public registry should serve a download, got %d", status)
	}
}

// A secured registry (`access = secured`) keeps reads public and restricts who
// may publish: an identity the administrator minted a token for still cannot
// publish unless the allowlist names it.
//
// It is a separate test from the public one because the harness gives each
// registry a fixed loopback port, so a test may only run one at a time.
func TestRegistrySecuredRestrictsPublishingNotReading(t *testing.T) {
	// The identities the harness publishes with are named, so seeding works; a
	// stranger is deliberately not.
	t.Setenv("RHO_REGISTRY_ALLOWED_USERS", "harness,seed")
	secured := startRegistryWithSettings(t, "secret", "data_dir = "+filepath.Join(t.TempDir(), "d")+"\naccess = secured\n")
	seedRegistry(t, secured)

	// Reading is public on a secured registry.
	if status, body := getPage(t, secured.base, "/api/search?q=math"); status != 200 {
		t.Errorf("search should stay public on a secured registry, got %d %q", status, body)
	}
	if status, _ := getPage(t, secured.base, "/modules/math/0.1.0/rho-pkg"); status != 200 {
		t.Errorf("a download should stay public on a secured registry, got %d", status)
	}
	if status, _ := getPage(t, secured.base, "/modules/math/0.1.0/docs/"); status == 401 {
		t.Errorf("a documentation page should not require a token on a secured registry")
	}

	// A token holder the allowlist does not name may not publish. The token is
	// minted with the administrator secret, so this tests the publish policy and
	// not whether a token could be obtained.
	stranger := secured.mintToken(t, "stranger")
	if status, body := secured.publishModule(t, "other", "1.0.0", "module", "X", []byte("x"), stranger); status != 403 {
		t.Errorf("a publisher outside the allowlist: got %d %s, want 403", status, body)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Page Layout                                                         */
/* -------------------------------------------------------------------------- */

// A short page must not overflow the viewport, or the footer ends up below the
// fold behind a scrollbar nobody asked for.
//
// The cause is subtle: min-height: 100vh with the default content-box sizing
// adds the body's padding on top of the viewport height, so the page is always
// taller than the window. The stylesheet therefore has to include the padding
// in the height, and the body has to fill the viewport so the flex layout can
// place the footer at the bottom.
func TestRegistryLayoutDoesNotOverflow(t *testing.T) {
	server := startRegistry(t, "")

	_, body := getPage(t, server.base, "/")
	if !strings.Contains(body, "No modules published yet.") {
		t.Fatalf("expected the empty state, got:\n%s", body)
	}

	for _, rule := range []string{
		// Padding counted inside the declared height, not added to it.
		"box-sizing: border-box",
		// The body fills the viewport, so the flex layout can place the footer.
		"html, body { height: 100%; }",
		"min-height: 100vh",
		// The content column grows to fill the space above the footer.
		"main { flex: 1 0 auto; }",
	} {
		if !strings.Contains(body, rule) {
			t.Errorf("the stylesheet is missing %q, so a short page will overflow", rule)
		}
	}

	// The footer must be present even when there is nothing to list, since that
	// is the case the overflow was reported against.
	if !strings.Contains(body, "<footer>") || !strings.Contains(body, "</footer>") {
		t.Error("the footer should be present on the empty page")
	}
}

// The footer stays at the bottom on every page, not only the empty one.
func TestRegistryFooterOnTheEmptyPage(t *testing.T) {
	server := startRegistry(t, "")
	_, body := getPage(t, server.base, "/")
	if !strings.Contains(body, "Rho programming language") {
		t.Errorf("expected the copyright in the footer, got:\n%s", body)
	}
}

// goToolchainAtLeast reports whether the go on PATH is new enough for a
// generated project to build.
//
// A generated project declares the Go it is for in its own go.mod, and the
// toolchain that generated it is the one that knows. Saying so is better than
// letting a build fail on a symbol the toolchain has never heard of, which
// reads as a fault in the generator rather than as a toolchain that is behind
// the project.
func goToolchainAtLeast(t *testing.T, want string) {
	t.Helper()
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		t.Skip("go toolchain not available")
	}
	// "go version go1.22.4 linux/amd64" — the release is the field after the
	// program name.
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		t.Skipf("cannot read the go version from %q", strings.TrimSpace(string(out)))
	}
	have := strings.TrimPrefix(fields[2], "go")
	if compareGoReleases(have, want) < 0 {
		t.Skipf("the generated project needs go %s or newer; this toolchain is go %s", want, have)
	}
}

// compareGoReleases compares two Go release strings, which are dotted numbers
// with an optional suffix that does not affect the order of the numbers.
func compareGoReleases(left, right string) int {
	fields := func(value string) []int {
		value = strings.TrimSuffix(value, "\n")
		// A release candidate or a development build sorts before its release.
		fields := strings.Split(value, ".")
		out := []int{}
		for _, field := range fields {
			number := 0
			for _, digit := range field {
				if digit < '0' || digit > '9' {
					break
				}
				number = number*10 + int(digit-'0')
			}
			out = append(out, number)
		}
		return out
	}
	leftFields, rightFields := fields(left), fields(right)
	for i := 0; i < len(leftFields) || i < len(rightFields); i++ {
		a, b := 0, 0
		if i < len(leftFields) {
			a = leftFields[i]
		}
		if i < len(rightFields) {
			b = rightFields[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

// A write from another page is refused: the registry is reachable from a browser
// a person is using, and a page that can post to it could otherwise publish, or
// withdraw, on that person's behalf.
func TestRegistryRefusesCrossSiteWrites(t *testing.T) {
	server := startRegistry(t, "")
	request, err := http.NewRequest("POST", server.base+"/modules/x/1.0.0/rho-pkg", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "http://evil.example")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-site write should be refused, got %d", response.StatusCode)
	}
}

// A published version carries the digest of the package it holds, and a client
// reads it from the version list. It is the digest of the bytes that were
// uploaded, so a package altered where it is stored no longer agrees with it.
func TestRegistryRecordsThePackageDigest(t *testing.T) {
	server := startRegistry(t, "")
	payload := []byte{0x1f, 0x8b, 0x08, 0x00, 'd', 'i', 'g', 'e', 's', 't', 0x00}
	status, body := server.publish(t, "digest", "1.0.0", payload)
	if status != 201 {
		t.Fatalf("publish: got %d %s", status, body)
	}

	status, body = server.request(t, "GET", "/api/modules/digest", nil, "")
	if status != 200 {
		t.Fatalf("versions: got %d %s", status, body)
	}
	var document struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		t.Fatalf("cannot read the version document: %s", body)
	}
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	if document.SHA256["1.0.0"] != want {
		t.Errorf("recorded digest = %q, want %q", document.SHA256["1.0.0"], want)
	}
}

// A publisher's signature travels with the publish and comes back beside the
// digest it is over, so a client can check one against the other. The registry
// stores it as sent and does not judge it.
func TestRegistryServesThePublishersSignature(t *testing.T) {
	server := startRegistry(t, "")
	payload := []byte{0x1f, 0x8b, 0x08, 0x00, 's', 'i', 'g', 0x00}

	fields := url.Values{}
	fields.Set("version", "1.0.0")
	fields.Set("type", "module")
	fields.Set("main", "src/main.rho")
	fields.Set("author", "Aurel Miron")
	fields.Set("signer", strings.Repeat("ab", 32))
	fields.Set("signature", strings.Repeat("cd", 64))
	status, body := server.request(t, "POST", "/modules/signed/1.0.0/rho-pkg?"+fields.Encode(), payload, server.credential)
	if status != 201 {
		t.Fatalf("publish: got %d %s", status, body)
	}
	// An unsigned module alongside it must not gain one.
	if status, body = server.publish(t, "unsigned", "1.0.0", payload); status != 201 {
		t.Fatalf("publish unsigned: got %d %s", status, body)
	}

	read := func(name string) map[string]map[string]string {
		status, body := server.request(t, "GET", "/api/modules/"+name, nil, "")
		if status != 200 {
			t.Fatalf("versions of %s: got %d %s", name, status, body)
		}
		var document struct {
			Signatures map[string]map[string]string `json:"signatures"`
		}
		if err := json.Unmarshal([]byte(body), &document); err != nil {
			t.Fatalf("cannot read the version document: %s", body)
		}
		return document.Signatures
	}

	got := read("signed")["1.0.0"]
	if got["signer"] != strings.Repeat("ab", 32) || got["signature"] != strings.Repeat("cd", 64) {
		t.Errorf("the signature came back as %v", got)
	}
	if _, found := read("unsigned")["1.0.0"]; found {
		t.Error("an unsigned module was served with a signature")
	}
}

// A page path that tries to leave the documentation is refused. The router
// normalizes a literal "../" but not a percent-encoded one, so a request can
// arrive with ".." in the page name — and the store, not the router, is what
// must confine it.
func TestRegistryRefusesAnEncodedDocsTraversal(t *testing.T) {
	server := startRegistry(t, "")
	seedRegistry(t, server)
	status, body := getPage(t, server.base,
		"/modules/math/0.1.0/docs/%2e%2e%2f%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd")
	if status != 404 {
		t.Errorf("a traversal page should not be served, got %d %q", status, body)
	}
	if strings.Contains(body, "root:") {
		t.Fatalf("the docs route served a file outside the documentation")
	}
}
