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
	in := "test_compile_v3.rho"
	out := "test_compile_v3.c"

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
	in := "print_string.rho"
	out := "print_string.c"
	bin := "print_string_bin"

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	binPath := "./" + bin
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
	in := "yaml_print.rho"
	out := "yaml_print.c"
	bin := "yaml_print_bin"

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	binPath := "./" + bin
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
	in := "concat_print.rho"
	out := "concat_print.c"
	bin := "concat_print_bin"

	defer os.Remove(in)
	defer os.Remove(out)
	defer os.Remove(bin)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := CompileFile(in); err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}

	cmd := exec.Command("gcc", "-std=c11", "-o", "./"+bin, out)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, out)
	}

	cmd = exec.Command("./" + bin)
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
	in := "cli_default.rho"
	out := "cli_default.c"

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
	in := "cli_custom.rho"
	out := "custom_output.c"

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
