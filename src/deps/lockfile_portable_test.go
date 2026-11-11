package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockfileRecordsWorkspacePathsPortably(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("RHO_WORKSPACE", workspace)
	project := filepath.Join(workspace, "app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(project, "rho.lock")
	outside := "/opt/elsewhere/lib"
	lock := &Lockfile{Packages: []LockedPackage{
		{Name: "inside", Path: filepath.Join(workspace, "lib", "inside"), Rev: "abc"},
		{Name: "outside", Path: outside},
	}}
	if err := lock.Save(lockPath); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(lockPath)
	if strings.Contains(string(raw), workspace) {
		t.Fatalf("the lockfile names this machine's workspace: %s", raw)
	}
	if !strings.Contains(string(raw), "${RHO_WORKSPACE}/lib/inside") {
		t.Fatalf("the workspace path was not written portably: %s", raw)
	}
	if lock.Packages[0].Path != filepath.Join(workspace, "lib", "inside") {
		t.Fatal("saving changed the lockfile in memory")
	}

	// Read on another machine, the same record points into that machine's workspace.
	other := t.TempDir()
	t.Setenv("RHO_WORKSPACE", other)
	loaded, err := LoadLockfile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Packages[0].Path != filepath.Join(other, "lib", "inside") {
		t.Errorf("inside = %q", loaded.Packages[0].Path)
	}
	if loaded.Packages[1].Path != outside {
		t.Errorf("a path outside the workspace must be kept as it was, got %q", loaded.Packages[1].Path)
	}
}
