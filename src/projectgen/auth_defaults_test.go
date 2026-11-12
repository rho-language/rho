package projectgen

import (
	"os"
	"path/filepath"
	"testing"
)

// An application's auth wrapper takes its audience from the service it deploys
// as, its key from where rho auth keygen puts it, and its login route from the
// audience; what the descriptor writes stays.
func TestTheAuthWrapperFollowsFromTheApplication(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: mesh\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndeploy:\n  service: mesh\n  port: 8478\n"), 0o644)
	target := withAuthDefaults(Target{Middleware: []Middleware{
		{Name: "cors"},
		{Name: "auth", Options: map[string]string{"issuer": "keeper", "skip_paths": "/healthz"}},
	}}, root)
	options := target.Middleware[1].Options
	want := map[string]string{"issuer": "keeper", "skip_paths": "/healthz", "audience": "mesh",
		"public_key": DefaultAuthPublicKey, "login_url": "/auth/login?audience=mesh"}
	for key, value := range want {
		if options[key] != value {
			t.Errorf("%s = %q, want %q", key, options[key], value)
		}
	}
	written := withAuthDefaults(Target{Middleware: []Middleware{{Name: "auth", Options: map[string]string{"audience": "other", "login_url": "/in"}}}}, root)
	if written.Middleware[0].Options["audience"] != "other" || written.Middleware[0].Options["login_url"] != "/in" {
		t.Errorf("a written setting was replaced: %v", written.Middleware[0].Options)
	}
}
