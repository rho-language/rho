//
// @file request_query_test.go
// @brief A handler reads the request's raw query string.
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

// request_query is the raw query, without its "?", so a handler that sends a
// request on elsewhere (a redirect) keeps it whole.
func TestRequestQueryIsTheRawQuery(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: rawquery\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: go\n  targets:\n    - go-server\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `module rawquery

route GET "/q" => q

fn q(req: Request) Response {
    return Response{ 200, request_query(req), "text/plain; charset=utf-8", "" }
}

fn main() void {
    print("rawquery")
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
	addr := "127.0.0.1:18518"
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
	resp, err := client.Get("http://" + addr + "/q?y=1&z=a%20b")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "y=1&z=a%20b" {
		t.Fatalf("request_query = %q, want the raw query y=1&z=a%%20b", body)
	}
}
