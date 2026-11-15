//
// @file cli_args_test.go
// @brief Flags after the path, a program's own arguments, and each command's help.
//
// @project Rho Programming Language
// @date 2026-11-15
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// inDir runs a test from a directory of its own and comes back.
func inDir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

// captureStdout is what a function printed on standard output, including what a
// program it ran printed there.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = writer
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	defer func() { os.Stdout = saved }()
	run()
	writer.Close()
	os.Stdout = saved
	return <-done
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// `rho new <file> --description "..."` is how the documentation writes it, and
// the description is the file's @brief.
func TestNewReadsAFlagAfterThePath(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)
	writeFile(t, "module.yml", "module:\n  name: acme\n  type: rho\n  version: 0.1.0\n  description: x\nmain: src/main.rho\nbuild:\n  language: c\n  targets: [c]\n")
	if code := newCLI([]string{"src/cache.rho", "--description", "What the cache keeps.", "-date", "2026-01-02"}); code != 0 {
		t.Fatalf("rho new exited %d", code)
	}
	data, err := os.ReadFile(filepath.Join("src", "cache.rho"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "@brief What the cache keeps.") || !strings.Contains(text, "@date 2026-01-02") {
		t.Fatalf("the flags after the path were not read:\n%s", text)
	}
}

// A test file is not told to include main.rho, whose main would clash with its
// own; it is told the module's other sources.
func TestANewTestFileSuggestsTheModulesOwnFiles(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)
	writeFile(t, "module.yml", "module:\n  name: acme\n  type: rho\n  version: 0.1.0\n  description: x\nmain: src/main.rho\nbuild:\n  language: c\n  targets: [c]\n")
	writeFile(t, "src/main.rho", "module acme\nfn main() void {}\n")
	writeFile(t, "src/csv.rho", "module acme.csv\n")
	if code := newCLI([]string{"tests/csv_test.rho"}); code != 0 {
		t.Fatalf("rho new exited %d", code)
	}
	data, _ := os.ReadFile(filepath.Join("tests", "csv_test.rho"))
	text := string(data)
	if strings.Contains(text, `include "../src/main.rho"`) {
		t.Fatalf("a test file was told to include main.rho:\n%s", text)
	}
	if !strings.Contains(text, `include "../src/csv.rho"`) {
		t.Fatalf("a test file was not told the module's own file:\n%s", text)
	}
}

// `rho init <dir> --target c,go` reads the flag after the directory, takes the
// language from the first target, and puts the description in the @brief.
func TestInitReadsFlagsAfterTheDirectory(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)
	out := captureStdout(t, func() {
		if code := initCLI([]string{"csv-report", "--target", "c,go", "--description", "Summarises a CSV file."}); code != 0 {
			t.Fatalf("rho init exited %d", code)
		}
	})
	data, err := os.ReadFile(filepath.Join("csv-report", "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{"language: c\n", "    - c\n    - go\n", "@brief csv_report — Summarises a CSV file.", "name: csv_report"} {
		if !strings.Contains(manifest, want) {
			t.Errorf("module.yml lacks %q:\n%s", want, manifest)
		}
	}
	if !strings.Contains(out, `module name: csv_report (from "csv-report"`) {
		t.Errorf("rho init did not say the name was changed:\n%s", out)
	}
	main, _ := os.ReadFile(filepath.Join("csv-report", "src", "main.rho"))
	if !strings.Contains(string(main), "@brief Summarises a CSV file.") || !strings.Contains(string(main), "\nmodule csv_report\n") {
		t.Errorf("main.rho does not start with its header:\n%s", main)
	}
}

// Asking a command for its help is not a usage error.
func TestHelpIsNotAnError(t *testing.T) {
	quiet := func(run func() int) int {
		saved := os.Stderr
		null, _ := os.Open(os.DevNull)
		os.Stderr = null
		defer func() { os.Stderr = saved; null.Close() }()
		var code int
		captureStdout(t, func() { code = run() })
		return code
	}
	for name, run := range map[string]func() int{
		"init":     func() int { return initCLI([]string{"--help"}) },
		"new":      func() int { return newCLI([]string{"-h"}) },
		"install":  func() int { return installCLI([]string{"--help"}) },
		"env":      func() int { return envCLI([]string{"--help"}) },
		"protocol": func() int { return protocolCLI([]string{"--help"}) },
		"ide":      func() int { return ideCLI([]string{"--help"}) },
	} {
		if code := quiet(run); code != 0 {
			t.Errorf("rho %s --help exited %d", name, code)
		}
	}
}

// --emit-c writes the translation and nothing else: no generated project, no
// binary, so a test program can never take the place of the application's.
// The flag is read after the path.
func TestEmitCWritesOnlyTheTranslation(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler")
	}
	dir := t.TempDir()
	inDir(t, dir)
	writeFile(t, "module.yml", "module:\n  name: emitc\n  type: rho\n  version: 0.1.0\n  description: x\nmain: src/main.rho\nbuild:\n  language: c\n  targets: [c, go]\n")
	writeFile(t, "src/main.rho", "module emitc\n\nfn main() void {\n    print(\"hi\\n\")\n}\n")
	out := filepath.Join(dir, "scratch", "t.c")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	var code int
	captureStdout(t, func() { code = buildCLI([]string{"src/main.rho", "--emit-c", "-o", out}) })
	if code != 0 {
		t.Fatalf("rho build --emit-c exited %d", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the translation was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join("build", "generated")); err == nil {
		t.Fatal("--emit-c generated a project")
	}
	if _, err := os.Stat(filepath.Join("build", "emitc")); err == nil {
		t.Fatal("--emit-c built a binary")
	}
}

// `rho run <dir> args...` gives the program its arguments, in the directory rho
// was run from.
func TestRunPassesTheArgumentsToTheProgram(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no Go toolchain")
	}
	dir := t.TempDir()
	project := filepath.Join(dir, "argsapp")
	writeFile(t, filepath.Join(project, "module.yml"), "module:\n  name: argsapp\n  type: rho\n  version: 0.1.0\n  description: x\nmain: src/main.rho\nbuild:\n  language: go\n  targets: [go]\n")
	writeFile(t, filepath.Join(project, "src", "main.rho"), "module argsapp\n\nextern fn rho_arg(index: i32) string\n\nfn main() void {\n    print(\"[\" + rho_arg(0) + \"][\" + rho_arg(1) + \"][\" + get_args() + \"]\\n\")\n}\n")
	inDir(t, dir)
	for _, args := range [][]string{
		{"argsapp", "a b", "-v"},
		{"argsapp", "--", "a b", "-v"},
	} {
		got := captureStdout(t, func() { runCLI(args) })
		if !strings.Contains(got, "[a b][-v][a b -v]") {
			t.Errorf("rho run %q printed %q", args, got)
		}
	}
}
