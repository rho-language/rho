package rho

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A call into an included module's unlisted function is reported even from a
// file that is not the one including the module; an exported one, and one the
// calling module defines itself, are not.
func TestACallPastAModulesExportsIsReported(t *testing.T) {
	dir := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(dir, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lib/module.yml", "module:\n  name: shapes\n  type: module\n  version: 0.1.0\nmain: src/lib.rho\nprovides:\n  import: acme.shapes\n  exports:\n    - area\n")
	write("lib/src/lib.rho", "module shapes\nfn area(w: u32) u32 { return lib_square(w) }\nfn lib_square(w: u32) u32 { return w * w }\nfn helper() u32 { return 1 }\n")
	write("app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	write("app/src/main.rho", "module main\ninclude \"../../lib/src/lib.rho\"\ninclude \"other.rho\"\nfn main() void { print(int_to_str(area(2) + other())) }\n")
	write("app/src/other.rho", "module main.other\nfn helper() u32 { return 2 }\nfn other() u32 { return lib_square(3) + helper() }\n")

	read, writeEnd, _ := os.Pipe()
	saved := os.Stderr
	os.Stderr = writeEnd
	_, err := loadModuleGraph(filepath.Join(dir, "app", "src", "main.rho"))
	os.Stderr = saved
	writeEnd.Close()
	out, _ := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "calls lib_square, which the shapes module does not export") || !strings.Contains(text, "other.rho") {
		t.Fatalf("the call from other.rho is not reported:\n%s", text)
	}
	if strings.Contains(text, "calls area") || strings.Contains(text, "calls helper") {
		t.Fatalf("an exported or an own function is reported:\n%s", text)
	}
}
