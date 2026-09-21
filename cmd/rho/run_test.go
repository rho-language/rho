package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRunFile(t *testing.T) {
	source := `
module hello

fn main() void {
    print("Hello CLI");
}
`
	input := "run_test_hello.rho"
	cOutput := "run_test_hello.c"
	executable := "run_test_hello_bin"

	defer os.Remove(input)
	defer os.Remove(cOutput)
	defer os.Remove(executable)

	if err := os.WriteFile(input, []byte(source), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := RunFile(input); err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}

	if _, err := os.Stat(executable); err == nil {
		t.Fatalf("expected executable %s to be cleaned up", executable)
	}

	command := exec.Command("gcc", "-std=c11", "-o", executable, cOutput)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("gcc failed: %v\n%s", err, output)
	}

	command = exec.Command("./" + executable)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("running program failed: %v", err)
	}

	if string(output) != "Hello CLI\n" {
		t.Fatalf("unexpected output: got %q, want %q", string(output), "Hello CLI\n")
	}
}

func TestRunFileError(t *testing.T) {
	input := "run_test_invalid.rho"
	defer os.Remove(input)

	if err := os.WriteFile(input, []byte("not rho code"), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	err := RunFile(input)
	if err == nil {
		t.Fatalf("expected RunFile to fail")
	}
	if !strings.Contains(err.Error(), "rho compile failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
