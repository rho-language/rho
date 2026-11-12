package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two imported modules that declare the same name each keep their own: a
// module's call to its own init is its own, and a qualified call reaches the
// module it names. Before, the first declaration won across the program, so
// module b's init() ran module a's.
func TestImportedModulesKeepTheirOwnNames(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("module.yml", "module:\n  name: scope\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n")
	write("src/app/a.rho", "module app.a\nfn init() string { return \"a\" }\nfn hello_a() string { return init() }\n")
	write("src/app/b.rho", "module app.b\nfn init() string { return \"b\" }\nfn hello_b() string { return init() }\n")
	write("src/main.rho", "module main\nimport app.a\nimport app.b\nfn main() void {\n    print(hello_a() + hello_b() + \"|\" + a.hello_a() + b.init())\n}\n")

	dirs, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "scope")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "ab|ab" {
		t.Errorf("output = %q, want %q", out, "ab|ab")
	}
}

// A qualified call names an imported module and a function it declares; anything
// else is refused with what is wrong.
func TestAQualifiedCallMustNameAModuleAndItsFunction(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "app"), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: scope\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "app", "a.rho"), []byte("module app.a\nfn hello() string { return \"a\" }\n"), 0o644)
	for call, want := range map[string]string{
		"a.nothing()": `module app.a has no function "nothing"`,
		"c.hello()":   `"c" is not a value in scope or an imported module`,
	} {
		os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte("module main\nimport app.a\nfn main() void {\n    print("+call+")\n}\n"), 0o644)
		_, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to say %s", call, err, want)
		}
	}
}
