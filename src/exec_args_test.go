package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// rho_exec_args passes each line as exactly one argument, on either backend: a
// value with quotes, spaces or shell syntax in it reaches the program unchanged,
// and never becomes a second argument or a command.
func TestExecArgsPassesValuesVerbatimOnBothBackends(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not on the path")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	os.MkdirAll(filepath.Dir(entry), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644)
	os.WriteFile(entry, []byte(`module app

extern fn rho_exec_args(program: string, arguments: string, output_path: string) u32
extern fn rho_read_text(path: string) string

fn main() void {
    let code: u32 = rho_exec_args("printf", "[%s]\na \"b\" c\n$(id); x\n\n", "out.txt")
    print(int_to_str(code))
    print(rho_read_text("out.txt"))
}
`), 0o644)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "args-c")
	if out, err := exec.Command("gcc", "-std=c11", "-D_POSIX_C_SOURCE=200809L", "-o", cBinary,
		filepath.Join(dirs[0], "main.c"), filepath.Join(dirs[0], "app.c"), filepath.Join(dirs[0], "runtime.c"), "-I", dirs[0]).CombinedOutput(); err != nil {
		t.Fatalf("C build: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "args-go")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = dirs[1]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go build: %v\n%s", err, out)
	}
	// printf repeats its format for every argument: three arguments, the last an
	// empty line, and none of them split or run.
	want := `0[a "b" c][$(id); x][]`
	for name, binary := range map[string]string{"c": cBinary, "go": goBinary} {
		run := exec.Command(binary)
		run.Dir = t.TempDir()
		out, err := run.Output()
		if err != nil || string(out) != want {
			t.Errorf("%s: output %q, %v; want %q", name, out, err, want)
		}
	}
}

// rho_sleep_ms waits at least as long as it was asked, on either backend.
func TestSleepMsWaitsOnBothBackends(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not on the path")
	}
	c, g := runBothBackends(t, `module app

extern fn rho_sleep_ms(milliseconds: u32) void

fn main() void {
    rho_sleep_ms(120)
    print("slept")
}
`)
	for name, o := range map[string]outcome{"c": c, "go": g} {
		if o.failed != "" || o.stdout != "slept" {
			t.Errorf("%s: %q %s", name, o.stdout, o.failed)
		}
	}
}
