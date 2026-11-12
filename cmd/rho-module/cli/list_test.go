package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The listing for a program is one installed version per line, with where it is
// installed, so a reader such as an orchestrator finds what is installed without
// knowing how the tree is laid out.
func TestListAsTSVNamesWhereEachModuleIs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	root := filepath.Join(home, "modules", "installed", "module", "greeter@0.2.0")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: greeter\n  type: rho\n  version: 0.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	read, write, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = write
	err := List([]string{"--format", "tsv"})
	write.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(read)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(fields) != 5 || fields[0] != "module" || fields[1] != "greeter" || fields[2] != "0.2.0" || fields[3] != root {
		t.Fatalf("listing = %q", out)
	}
	if err := List([]string{"--format", "json"}); err == nil {
		t.Error("an unknown format was accepted")
	}
}
