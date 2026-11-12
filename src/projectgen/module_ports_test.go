package projectgen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A program's ports come from its modules: what a module lists under ports,
// and the built-in port that implements a module's foreign functions.
func TestAProgramGetsThePortsItsModulesAskFor(t *testing.T) {
	project := t.TempDir()
	logger := t.TempDir()
	os.WriteFile(filepath.Join(project, "module.yml"), []byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nports:\n  - docstore\n"), 0o644)
	os.WriteFile(filepath.Join(logger, "module.yml"), []byte("module:\n  name: rho-log\n  type: component\n  version: 0.1.2\n  import_path: rho.log\nmain: rho-log\n"), 0o644)
	got := modulePorts(Context{ProjectRoot: project, ModuleRoots: []string{project, logger}})
	if want := []string{"docstore", "logemit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ports = %v, want %v", got, want)
	}
	if got := modulePorts(Context{ProjectRoot: t.TempDir()}); len(got) != 0 {
		t.Errorf("a program with no modules asks for %v", got)
	}
}
