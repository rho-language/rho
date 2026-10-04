//
// @file main_test.go
// @brief The run, from a command line to an exit code.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// project writes a project with a manifest, a build artifact and a budget, which
// is the smallest thing a run can be asked about.
func project(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(root, "build", "generated", "c", "build", "test-board")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifacts(t, build)
	return root
}

// writeArtifacts puts a linker map in a build tree, which is what a run reads.
func writeArtifacts(t *testing.T, build string) {
	t.Helper()
	contents := `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr
RAM              0x0000000020000000 0x0000000000010000 rw

Linker script and memory map

.text           0x0000000008000000      0x400
 .text.big
                0x0000000008000000      0x200 main-dep0/libfoo.a(foo.c.obj)
                0x0000000008000000              big

.bss            0x0000000020000000      0x100
`
	if err := os.WriteFile(filepath.Join(build, "test-board.map"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

const withinBudget = `module:
  name: firmware
  type: module
  version: 0.1.0

main: src/main.rho

build:
  language: c
  toolchain: test-board

mem:
  regions:
    FLASH: { max_bytes: 4096, warn_threshold: "85%" }
    RAM: { max_bytes: 4096 }
  enforce:
    fail_on_overflow: true
`

// A report prints and exits zero whatever it found: a person runs it to read it,
// and a report that exited non-zero would be a check.
func TestAReportPrintsAndDoesNotFail(t *testing.T) {
	root := project(t, withinBudget)
	code := analyse([]string{root, "--top", "3"}, false)
	if code != exitClean {
		t.Fatalf("expected a report to exit clean, got %d", code)
	}
}

// `check` is the same analysis with the outcome enforced, which is the whole
// difference between the two commands.
func TestACheckFailsAProgramOverItsBudget(t *testing.T) {
	over := strings.Replace(withinBudget, "FLASH: { max_bytes: 4096", "FLASH: { max_bytes: 512", 1)
	root := project(t, over)
	code := analyse([]string{root}, true)
	if code != exitOverBudget {
		t.Fatalf("expected the check to fail, got %d", code)
	}
}

func TestACheckPassesAProgramInsideItsBudget(t *testing.T) {
	root := project(t, withinBudget)
	if code := analyse([]string{root}, true); code != exitClean {
		t.Fatalf("expected the check to pass, got %d", code)
	}
}

// Nothing to read is not the same as nothing over budget, and a pipeline told the
// first as the second has its gate switched off.
func TestARunWithNothingToAnalyseSaysSoAndDoesNotPass(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(withinBudget), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := analyse([]string{root}, true); code == exitClean {
		t.Fatal("expected a run with nothing to read not to pass")
	}
}

// `--write-baseline` records the figures, and a later run compares against them.
func TestABaselineIsRecordedAndThenCompared(t *testing.T) {
	root := project(t, withinBudget)
	if code := analyse([]string{root, "--write-baseline"}, false); code != exitClean {
		t.Fatalf("recording a baseline failed: %d", code)
	}
	baseline := filepath.Join(root, "mem.baseline.json")
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("expected a baseline at %s: %v", baseline, err)
	}
	if code := analyse([]string{root}, false); code != exitClean {
		t.Fatalf("the run against the baseline failed: %d", code)
	}
}

// Both spellings of a flag are the flag. An editor's launch configuration writes
// `--flag=value` and a person writes `-flag value`, and a run that refused either
// would fail on configurations that are perfectly ordinary.
func TestBothSpellingsOfAFlagAreTheFlag(t *testing.T) {
	cases := [][]string{
		{"--top", "7"},
		{"-top", "7"},
		{"--top=7"},
		{"-top=7"},
	}
	for _, args := range cases {
		options := options{}
		rest := parseArgs(args, &options)
		if options.top != 7 || !options.topSet {
			t.Errorf("%v: expected top=7, got %+v", args, options.top)
		}
		if len(rest) != 0 {
			t.Errorf("%v: expected no positional arguments, got %v", args, rest)
		}
	}
}

// The project directory is the one argument that is not a flag, so `rho mem .`
// works.
func TestAProjectDirectoryIsNotAFlag(t *testing.T) {
	options := options{}
	rest := parseArgs([]string{".", "--bytes"}, &options)
	if len(rest) != 1 || rest[0] != "." {
		t.Fatalf("expected the project directory, got %v", rest)
	}
	if !options.bytes {
		t.Error("expected --bytes to be set")
	}
}
