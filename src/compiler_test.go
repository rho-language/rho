//
// @file compiler_test.go
// @brief Rho Compiler - Orchestrator v3 tests.
//
// Tests the full compiler pipeline and the CLI wrapper.
// Validates lexing, parsing, typechecking, IR generation, C emission,
// hosted main() generation, YAML loading, string concat,
// and custom output path handling.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Full Pipeline (CompileFile)                                         */
/* -------------------------------------------------------------------------- */

func TestCompileFile(t *testing.T) {
	src := `
module lighting

struct Light {
    pin: u8
}

fn init() void {
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "test_compile_v3.rho")
	out := filepath.Join(dir, "test_compile_v3.c")

	defer os.Remove(in)
	defer os.Remove(out)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read generated C file: %v", err)
	}

	outC := string(data)

	if !strings.Contains(outC, "module lighting") {
		t.Fatalf("expected module name in C output")
	}
	if !strings.Contains(outC, "typedef struct") {
		t.Fatalf("expected struct definition in C output")
	}
	if !strings.Contains(outC, "uint8_t pin;") {
		t.Fatalf("expected struct field pin in C output")
	}
	if !strings.Contains(outC, "void init(") {
		t.Fatalf("expected init() function in C output")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: print("Hello from Rho")                                             */
/* -------------------------------------------------------------------------- */

func TestRhoPrintString(t *testing.T) {
	src := `
module hello

fn main() void {
    print("Hello from Rho");
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "print_string.rho")
	out := filepath.Join(dir, "print_string.c")
	bin := filepath.Join(dir, "print_string_bin")

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, out)
	}

	cmd = exec.Command(binPath)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}

	// `print` writes exactly what it is given: a newline is in the output only
	// if the program put one there. Appending one would make what a program
	// prints depend on the language rather than on the program.
	if got := string(data); got != "Hello from Rho" {
		t.Fatalf("unexpected output: got %q, want %q", got, "Hello from Rho")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: YAML load + print                                                   */
/* -------------------------------------------------------------------------- */

func TestRhoYAMLPrint(t *testing.T) {
	cfg := "user:\n  name: Alice\n"
	if err := os.WriteFile("config_test.yml", []byte(cfg), 0644); err != nil {
		t.Fatalf("write YAML config failed: %v", err)
	}
	defer os.Remove("config_test.yml")

	src := `
module hello

fn main() void {
    let name: string = yaml.load("config_test.yml");
    print(name);
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "yaml_print.rho")
	out := filepath.Join(dir, "yaml_print.c")
	bin := filepath.Join(dir, "yaml_print_bin")

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, out)
	}

	cmd = exec.Command(binPath)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}

	if got := string(data); got != "Alice" {
		t.Fatalf("unexpected output: got %q, want %q", got, "Alice")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: String concat + YAML load                                           */
/* -------------------------------------------------------------------------- */

func TestRhoStringConcat(t *testing.T) {
	cfg := "user:\n  name: Alice\n"
	if err := os.WriteFile("concat_test.yml", []byte(cfg), 0644); err != nil {
		t.Fatalf("write YAML config failed: %v", err)
	}
	defer os.Remove("concat_test.yml")

	src := `
module hello

fn main() void {
    let name: string = yaml.load("concat_test.yml");
    print("Hello " + name);
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "concat_print.rho")
	out := filepath.Join(dir, "concat_print.c")
	bin := filepath.Join(dir, "concat_print_bin")

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	cmd := exec.Command("gcc", "-std=c11", "-o", bin, out)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, out)
	}

	cmd = exec.Command(bin)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}

	if got := string(data); got != "Hello Alice" {
		t.Fatalf("unexpected output: got %q, want %q", got, "Hello Alice")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: CLI (RunCLI) default output                                         */
/* -------------------------------------------------------------------------- */

func TestRunCLIDefaultOutput(t *testing.T) {
	src := `
module cli_test

fn main() void {
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "cli_default.rho")
	out := filepath.Join(dir, "cli_default.c")

	defer os.Remove(in)
	defer os.Remove(out)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	code := RunCLI([]string{in})
	if code != 0 {
		t.Fatalf("RunCLI returned non-zero exit code: %d", code)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output file %s to exist", out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: CLI (RunCLI) custom -o output                                       */
/* -------------------------------------------------------------------------- */

func TestRunCLICustomOutput(t *testing.T) {
	src := `
module cli_custom

fn main() void {
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "cli_custom.rho")
	out := filepath.Join(dir, "custom_output.c")

	defer os.Remove(in)
	defer os.Remove(out)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	code := RunCLI([]string{"-o", out, in})
	if code != 0 {
		t.Fatalf("RunCLI returned non-zero exit code: %d", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected custom output file %s to exist", out)
	}

	outC := string(data)

	if !strings.Contains(outC, "module cli_custom") {
		t.Fatalf("expected module name in custom C output")
	}
	if !strings.Contains(outC, "int main(") {
		t.Fatalf("expected hosted C main() function in custom C output")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: continue                                                            */
/* -------------------------------------------------------------------------- */

// A loop that skips what it has already done is ordinary to write, and the
// statement that says so is a statement of the language.
//
// The cases that matter are the ones where a continue could leave a loop
// running for ever: the last thing the loop does, and a loop whose own step
// comes after the continue. A continue in a `for` leaves the loop through its
// step, so the loop still ends; a continue in a `while` leaves it through the
// condition, and the condition is what the loop is.
func TestRhoContinue(t *testing.T) {
	src := `module cont

fn skipped(n: u32) u32 {
    var total: u32 = 0
    var i: u32 = 0
    while i < n {
        i = i + 1
        if i % 2 == 0 {
            continue
        }
        total = total + i
    }
    return total
}

fn last_is_skipped(n: u32) u32 {
    var count: u32 = 0
    var i: u32 = 0
    while i < n {
        i = i + 1
        count = count + 1
        continue
    }
    return count
}

fn stepped() u32 {
    let values: u32[4] = [1, 2, 3, 4]
    var total: u32 = 0
    for value in values {
        if value == 2 {
            continue
        }
        total = total + value
    }
    return total
}

fn main() void {
    print(int_to_str(skipped(10)));
    print("|");
    print(int_to_str(last_is_skipped(5)));
    print("|");
    print(int_to_str(stepped()));
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "continue_test.rho")
	out := filepath.Join(dir, "continue_test.c")
	bin := filepath.Join(dir, "continue_test_bin")

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, output)
	}
	cmd = exec.Command(binPath)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}
	// The odd numbers up to ten, then a loop that continues every time and
	// still ends, then a loop over an array that skips the second element.
	if got := string(data); got != "25|5|8" {
		t.Fatalf("unexpected output: got %q, want %q", got, "25|5|8")
	}
}

// A continue outside a loop is a mistake, and it is said so rather than
// compiled: a program that continues out of nothing has no meaning.
func TestRhoContinueOutsideALoopIsRefused(t *testing.T) {
	src := `module cont

fn wrong() u32 {
    continue
    return 0
}

fn main() void {
    print(int_to_str(wrong()));
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "continue_outside.rho")
	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
	err := CompileFile(in)
	if err == nil {
		t.Fatal("expected a continue outside a loop to be refused")
	}
	if !strings.Contains(err.Error(), "continue outside loop") {
		t.Fatalf("expected the reason to be named, got %q", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: files                                                               */
/* -------------------------------------------------------------------------- */

// A program that reads a file and writes one: the primitives a registry is
// managed with.
func TestRhoFileReadAndWrite(t *testing.T) {
	src := `module files

extern fn rho_write_text(path: string, text: string) i32
extern fn rho_read_text(path: string) string

fn main() void {
    let path: string = "rho_files_test.txt"
    let written: i32 = rho_write_text(path, "hello from rho")
    if written != 0 {
        let text: string = rho_read_text(path)
        print(text)
    }
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "files_test.rho")
	out := filepath.Join(dir, "files_test.c")
	bin := filepath.Join(dir, "files_test_bin")
	data := "rho_files_test.txt"

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)
	defer os.Remove(data)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, output)
	}
	cmd = exec.Command(binPath)
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}
	if string(got) != "hello from rho" {
		t.Fatalf("unexpected output: got %q, want %q", string(got), "hello from rho")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: the C runtime's YAML and its process runner                          */
/* -------------------------------------------------------------------------- */

// The runtime's answers are the same on either backend: a program that means
// one thing on a host and another elsewhere is a program nobody can reason
// about.
//
// The YAML is asked for by a path, told what it is, and asked what it holds;
// and a program can run another program and read what it said.
func TestRhoRuntimeYAMLAndExec(t *testing.T) {
	src := `module runtime

extern fn rho_write_text(path: string, text: string) i32
extern fn rho_read_text(path: string) string
extern fn rho_yaml_value(path: string, key: string) string
extern fn rho_yaml_kind(path: string, key: string) string
extern fn rho_yaml_keys(path: string, key: string) string
extern fn rho_exec(program: string, arguments: string, output_path: string) u32

fn main() void {
    let path: string = "rho_runtime_test.yml"
    let written: u32 = rho_write_text(path, "name: a-module\nboard:\n  connectors:\n    - name: one\n    - name: two\n")
    if written == 0 {
        print("could not write")
        return
    }
    print(rho_yaml_value(path, "name"))
    print("|")
    print(rho_yaml_value(path, "board.connectors"))
    print("|")
    print(rho_yaml_kind(path, "board.connectors"))
    print("|")
    print(rho_yaml_keys(path, "board"))
    print("|")
    let code: u32 = rho_exec("echo", "it ran", "rho_runtime_test_out.txt")
    print(int_to_str(code))
    print("|")
    print(rho_read_text("rho_runtime_test_out.txt"))
}
`
	in := "runtime_test.rho"
	out := "runtime_test.c"
	bin := "runtime_test_bin"
	data := "rho_runtime_test.yml"
	report := "rho_runtime_test_out.txt"

	dir := t.TempDir()
	in = filepath.Join(dir, in)
	out = filepath.Join(dir, out)
	bin = filepath.Join(dir, bin)
	data = filepath.Join(dir, data)
	report = filepath.Join(dir, report)

	// The paths in the program are relative, so the run happens in the
	// directory the files are in.
	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)
	defer os.Remove(data)
	defer os.Remove(report)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, output)
	}
	cmd = exec.Command(binPath)
	cmd.Dir = dir
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}
	// The value of a path, the kind of it, the keys under it, and what a
	// program that ran said.
	//
	// A value that is a block has no value of its own — it is a list, and what
	// is in it is asked for by name — and the same is true on either backend.
	if want := "a-module||list|connectors|0|it ran\n"; string(got) != want {
		t.Fatalf("unexpected output: got %q, want %q", string(got), want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: getenv                                                              */
/* -------------------------------------------------------------------------- */

// A program that reads its environment: the value is there when it is set, and
// empty when it is not, on either backend.
func TestRhoGetenv(t *testing.T) {
	src := `module getenv

extern fn getenv(name: string) string

fn main() void {
    let value: string = getenv("RHO_TEST_VALUE")
    print(value)
}
`
	in := "getenv_test.rho"
	out := "getenv_test.c"
	bin := "getenv_test_bin"

	dir := t.TempDir()
	in = filepath.Join(dir, in)
	out = filepath.Join(dir, out)
	bin = filepath.Join(dir, bin)

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	binPath := bin
	cmd := exec.Command("gcc", "-std=c11", "-o", binPath, out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, output)
	}
	// Set the variable for the run.
	cmd = exec.Command(binPath)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "RHO_TEST_VALUE=hello from the environment")
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}
	if string(got) != "hello from the environment" {
		t.Fatalf("unexpected output: got %q, want %q", string(got), "hello from the environment")
	}
}
