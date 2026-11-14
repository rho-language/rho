//
// @file request_remote_test.go
// @brief A handler reads the address the connection came from.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// request_remote is the TCP peer's address, and a client cannot change it with
// a header: a rate limit kept per address has to be keyed on something the
// client does not write.
func TestRequestRemoteIsTheConnectionsPeer(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: remote\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: go\n  targets:\n    - go-server\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `module remote

route GET "/who" => who

fn who(req: Request) Response {
    return Response{ 200, request_remote(req), "text/plain; charset=utf-8", "" }
}

fn main() void {
    print("remote")
}
`
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	serverDir := dirs[0]
	binary := filepath.Join(serverDir, "server")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = serverDir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated server did not build: %v\n%s", err, out)
	}
	addr := "127.0.0.1:18517"
	cmd := exec.Command(binary)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(), "RHO_HTTP_ADDR="+addr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	client := &http.Client{Timeout: 5 * time.Second}
	if !waitForServer("http://"+addr, client) {
		t.Fatal("the server did not become ready")
	}
	request, err := http.NewRequest("GET", "http://"+addr+"/who", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	request.Header.Set("X-Real-IP", "203.0.113.9")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "127.0.0.1" {
		t.Fatalf("request_remote = %q, want the loopback peer 127.0.0.1", body)
	}
}
