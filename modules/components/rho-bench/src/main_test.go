//
// @file main_test.go
// @brief The run, from a command line to a table or a document.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rho/rho-bench/src/rhobench"
)

// Both spellings of a flag are the flag. An editor's launch configuration writes
// `--flag=value` and a person writes `-flag value`, and a run that refused either
// would fail on configurations that are perfectly ordinary.
func TestBothSpellingsOfAFlagAreTheFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--binary", "./bench"},
		{"-binary", "./bench"},
		{"--binary=./bench"},
		{"-binary=./bench"},
	} {
		opts := parseArgs(args)
		if opts.binary != "./bench" {
			t.Errorf("%v: expected the binary to be read, got %q", args, opts.binary)
		}
	}
}

// The project directory is the one argument that is not a flag.
func TestAProjectDirectoryIsNotAFlag(t *testing.T) {
	opts := parseArgs([]string{".", "--no-color"})
	if !filepath.IsAbs(opts.projectRoot) {
		t.Errorf("expected the project directory to be resolved, got %q", opts.projectRoot)
	}
	if opts.color {
		t.Error("expected --no-color to have been recorded")
	}
	if !opts.footprint {
		t.Error("expected the static figure to be on by default")
	}
}

// A benchmark program is looked for in a small list of conventional places rather
// than by scanning `build/`: a build tree holds object files, a CMake cache and one
// intermediate directory per board, and a scan finds whichever was built last.
func TestTheConventionalBinaryIsFoundAndTheTreeIsNotScanned(t *testing.T) {
	root := t.TempDir()
	if got := conventionalBinary(root); got != "" {
		t.Fatalf("expected nothing in an empty project, got %q", got)
	}
	built := filepath.Join(root, "build", "bench", "bench")
	if err := os.MkdirAll(filepath.Dir(built), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(built, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := conventionalBinary(root); got != built {
		t.Fatalf("expected %q, got %q", built, got)
	}
}

// A run with nothing to measure says so and does not pass: a pipeline told "nothing
// to measure" as "nothing regressed" has a gate that was never armed.
func TestARunWithNothingToMeasureDoesNotPass(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: x\n  type: module\n  version: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := measure([]string{root, "--binary", filepath.Join(root, "no-such-bench")}, true); code == exitClean {
		t.Fatal("expected a run with nothing to run not to pass")
	}
}

// `--diff` asks for another branch's baseline, and a ref that does not have one is
// that branch's problem and not this run's.
func TestADiffRefIsCarried(t *testing.T) {
	opts := parseArgs([]string{"--diff", "origin/main"})
	if opts.diffRef != "origin/main" {
		t.Errorf("expected the ref to be read, got %q", opts.diffRef)
	}
}

// Colour is a property of where the output is going, not of what the project said.
func TestColourIsOffUnlessAsked(t *testing.T) {
	if !parseArgs(nil).color {
		t.Error("expected colour on by default, for a terminal")
	}
	opts := parseArgs([]string{"--no-color"})
	if opts.color {
		t.Error("expected --no-color to turn it off")
	}
}

// A benchmark that writes escape sequences into a log is a file `grep` cannot read,
// so the reset sequence must not be written literally either — a report with colour
// off once ended a line with the reset code and nothing else.
func TestNoColourMeansNoEscapeSequences(t *testing.T) {
	summary := rhobench.Summary{
		Target:  "host",
		Results: []rhobench.Result{},
	}
	plain := &bytes.Buffer{}
	rhobench.WriteReport(plain, summary, rhobench.Options{Target: "host", Color: false})
	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("the report carried escape sequences with colour off: %q", plain.String())
	}
	colored := &bytes.Buffer{}
	rhobench.WriteReport(colored, summary, rhobench.Options{Target: "host", Color: true})
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Error("the report carried no escape sequences with colour on")
	}
}
