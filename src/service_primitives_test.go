package rho

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// A generated Go program proves who it is with the credential its environment
// names, calls another service with the token it got, and follows that
// service's event stream into a file sse_events can serve.
func TestServicePrimitivesCarryAnIdentity(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	var issued atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/service-token" || r.FormValue("client_id") != "aquarium" ||
			r.FormValue("client_secret") != "s3cret" || r.FormValue("audience") != "mesh" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		issued.Add(1)
		fmt.Fprint(w, `{"token":"T1","expires_in":600}`)
	}))
	defer issuer.Close()
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T1" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/data":
			fmt.Fprint(w, `{"value":42}`)
		case "/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": keep-alive\n\nevent: telemetry\nid: 7\ndata: {\"node\":1}\n\ndata: plain\n\n")
		}
	}))
	defer resource.Close()

	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	os.MkdirAll(filepath.Dir(entry), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644)
	os.WriteFile(entry, []byte(fmt.Sprintf(`module app

extern fn rho_http_request(method: string, url: string, body: string, content_type: string, audience: string, scopes: string, output_path: string) u32
extern fn rho_sse_follow(url: string, audience: string, scopes: string, path: string) bool
extern fn rho_read_text(path: string) string
extern fn rho_exec(program: string, arguments: string, output_path: string) u32

fn main() void {
    let status: u32 = rho_http_request("GET", "%[1]s/data", "", "", "mesh", "mesh:home", "out.txt")
    print(int_to_str(status))
    print(rho_read_text("out.txt"))
    let denied: u32 = rho_http_request("GET", "%[1]s/data", "", "", "", "", "denied.txt")
    print(int_to_str(denied))
    let started: bool = rho_sse_follow("%[1]s/stream", "mesh", "mesh:home", "feed.jsonl")
    var waited: u32 = 0
    while rho_read_text("feed.jsonl") == "" && waited < 50 {
        waited = waited + 1 + rho_exec("sleep", "0.1", "sleep.txt")
    }
    waited = waited + rho_exec("sleep", "0.3", "sleep.txt")
    print("|")
    print(rho_read_text("feed.jsonl"))
}
`, resource.URL)), 0o644)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "svc")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[1]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go build: %v\n%s", err, out)
	}
	work := t.TempDir()
	secret := filepath.Join(work, "client.secret")
	os.WriteFile(secret, []byte("s3cret\n"), 0o600)
	run := exec.Command(binary)
	run.Dir = work
	run.Env = append(os.Environ(), "RHO_CLIENT_ID=aquarium", "RHO_CLIENT_SECRET_FILE="+secret, "RHO_AUTH_URL="+issuer.URL)
	out, err := run.Output()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		`200{"value":42}`, // the token was fetched and carried
		"401",             // without an audience, no token is sent
		`{"data":"{\"node\":1}","event":"telemetry"}`, // a named event, as sse_events reads it
		`{"data":"plain","event":"message"}`,          // an unnamed one is a message
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
	if n := issued.Load(); n != 1 {
		t.Errorf("the issuer was asked %d times; a held token is reused until it is due for renewal", n)
	}
	if info, err := os.Stat(filepath.Join(work, "feed.jsonl")); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("the followed feed is %o, want 600", info.Mode().Perm())
	}
}
