package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandUsesTheEnvironmentAndDefaults(t *testing.T) {
	t.Setenv("RHO_WORKSPACE", "")
	t.Setenv("FOO_DIR", "/x")
	t.Setenv("UNSET_ONE", "")
	for in, want := range map[string]string{
		"plain/path":            "plain/path",
		"${FOO_DIR}/a":          "/x/a",
		"${UNSET_ONE:-/d}/b":    "/d/b",
		"${FOO_DIR}/${FOO_DIR}": "/x//x",
	} {
		got, err := Expand(in, "")
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := Expand("${UNSET_ONE}/a", ""); err == nil {
		t.Error("an unset variable with no default must be an error")
	}
	if _, err := Expand("${oops", ""); err == nil {
		t.Error("an unterminated reference must be an error")
	}
}

func TestPlatformWorkspaceFallsBackToTheWorkspace(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "examples", "a")
	os.MkdirAll(sub, 0o755)
	t.Setenv("RHO_WORKSPACE", "")
	t.Setenv("ROBOHUB_WORKSPACE", "")
	got, err := Expand("${ROBOHUB_WORKSPACE}/robohub-common", sub)
	want := filepath.Join(root, "robohub-common")
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
	t.Setenv("RHO_WORKSPACE", "/ws")
	if got, _ := Expand("${ROBOHUB_WORKSPACE}/x", sub); got != "/ws/x" {
		t.Errorf("RHO_WORKSPACE not honoured: %q", got)
	}
	t.Setenv("ROBOHUB_WORKSPACE", "/rh")
	if got, _ := Expand("${ROBOHUB_WORKSPACE}/x", sub); got != "/rh/x" {
		t.Errorf("platform variable must win: %q", got)
	}
}

func TestRhoHomeResolvesWithoutBeingSet(t *testing.T) {
	t.Setenv("RHO_HOME", "")
	t.Setenv("HOME", "/h")
	got, err := Expand("${RHO_HOME}/modules", "")
	if err != nil || got != "/h/.rho/modules" {
		t.Fatalf("got %q, %v", got, err)
	}
}
