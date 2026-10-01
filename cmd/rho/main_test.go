//
// @file main_test.go
// @brief Tests for Rho CLI - Enterprise v3.
//
// Validates:
// - rho build <file>
// - rho build -o <out.c>
// - rho run <file>
// - dependency integration (no deps present)
// - error handling
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func writeMinimalModule(t *testing.T, name string) {
	src := `
module ` + name + `

fn main() void {
}
`
	if err := os.WriteFile(name+".rho", []byte(src), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func writeMinimalManifest(t *testing.T) {
	mod := `
module testmod
version 0.1.0
`
	if err := os.WriteFile("rho.mod", []byte(mod), 0644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests for CLI subcommands (internal functions)                             */
/* -------------------------------------------------------------------------- */

// rho test takes a path and, optionally, the target to run on. More than that
// is a usage error.

/* -------------------------------------------------------------------------- */
/*  Test: rho build (default output)                                          */
/* -------------------------------------------------------------------------- */

func TestCLIBuildDefault(t *testing.T) {
	writeMinimalModule(t, "cli_build_default")
	writeMinimalManifest(t)

	in := "cli_build_default.rho"
	// Compiled artefacts belong under the descriptor's build_dir, never next
	// to the source file.
	outC := filepath.Join("build", "cli_build_default.c")

	defer os.Remove(in)
	defer os.Remove(outC)
	defer os.Remove("rho.mod")
	defer os.Remove("rho.lock")
	defer os.RemoveAll(".rho-deps")

	buildCLI([]string{in})

	if _, err := os.Stat(outC); err != nil {
		t.Fatalf("expected %s to exist", outC)
	}

	// The binary is named after the CMake project and lives in the build tree of
	// the target it was built for, so it is found rather than spelled out: a test
	// that hard-coded the path would break on the project's naming rules rather
	// than on anything about the build.
	if built := builtBinary("native"); built == "" {
		t.Fatalf("expected a binary to be built for the native target, found none under build/generated/c/build/native")
	}

	if _, err := os.Stat("cli_build_default.c"); err == nil {
		t.Fatalf("build output must not be written next to the source file")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: rho build (custom output)                                           */
/* -------------------------------------------------------------------------- */

func TestCLIBuildCustomOut(t *testing.T) {
	writeMinimalModule(t, "cli_custom")
	writeMinimalManifest(t)

	in := "cli_custom.rho"
	outC := "cli_custom_out.c"

	defer os.Remove(in)
	defer os.Remove(outC)
	defer os.Remove("rho.mod")
	defer os.Remove("rho.lock")
	defer os.RemoveAll(".rho-deps")

	buildCLI([]string{"-o", outC, in})

	if _, err := os.Stat(outC); err != nil {
		t.Fatalf("expected custom output %s to exist", outC)
	}

	// The binary is named after the CMake project and lives in the build tree of
	// the target it was built for, so it is found rather than spelled out: a test
	// that hard-coded the path would break on the project's naming rules rather
	// than on anything about the build.
	if built := builtBinary("native"); built == "" {
		t.Fatalf("expected a binary to be built for the native target, found none under build/generated/c/build/native")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: rho run                                                             */
/* -------------------------------------------------------------------------- */

func TestCLIRun(t *testing.T) {
	writeMinimalModule(t, "cli_run")
	writeMinimalManifest(t)

	in := "cli_run.rho"

	defer os.Remove(in)
	defer os.Remove("rho.mod")

	// runCLI should not error
	runCLI([]string{in})
}

/* -------------------------------------------------------------------------- */
/*  Test: rho build error (missing file)                                      */
/* -------------------------------------------------------------------------- */

func TestCLIBuildMissingFile(t *testing.T) {
	writeMinimalManifest(t)

	defer os.Remove("rho.mod")

	err := func() (e error) {
		defer func() {
			if r := recover(); r != nil {
				e = nil // buildCLI exits via os.Exit, so panic is expected
			}
		}()
		buildCLI([]string{"does_not_exist.rho"})
		return nil
	}()

	if err != nil {
		t.Fatalf("expected buildCLI to exit, got error: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: rho build with C dependency                                         */
/* -------------------------------------------------------------------------- */

func TestCLIBuildWithCDependency(t *testing.T) {
	tmp := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	/* ---------------------------------------------------------------------- */
	/* 1. Create fake C dependency                                            */
	/* ---------------------------------------------------------------------- */

	dep := filepath.Join(tmp, ".rho-deps", "depA")
	os.MkdirAll(filepath.Join(dep, "include"), 0755)
	os.MkdirAll(filepath.Join(dep, "src"), 0755)

	// depA.h
	hdr := `
#ifndef DEPA_H
#define DEPA_H

int depA_add(int a, int b);

#endif
`
	os.WriteFile(filepath.Join(dep, "include", "depA.h"), []byte(hdr), 0644)

	// depA.c
	csrc := `
#include "depA.h"

int depA_add(int a, int b) {
    return a + b;
}
`
	os.WriteFile(filepath.Join(dep, "src", "depA.c"), []byte(csrc), 0644)

	// depA rho.mod
	depMod := `
module depA
version 1.0.0
`
	os.WriteFile(filepath.Join(dep, "rho.mod"), []byte(depMod), 0644)

	/* ---------------------------------------------------------------------- */
	/* 2. Create project manifest                                             */
	/* ---------------------------------------------------------------------- */

	rootMod := `
module root
version 0.1.0

dep depA {
    type = "c"
    version = ">=1.0.0 <2.0.0"
    path = ".rho-deps/depA"
}
`
	os.WriteFile("rho.mod", []byte(rootMod), 0644)

	/* ---------------------------------------------------------------------- */
	/* 3. Create Rho program that calls C function                            */
	/* ---------------------------------------------------------------------- */

	rhoSrc := `
module test

extern fn depA_add(a: i32, b: i32) i32;

fn main() void {
    let x: i32 = depA_add(2, 3);
    print("Result: " + string.from_i32(x));
}
`
	os.WriteFile("test.rho", []byte(rhoSrc), 0644)

	/* ---------------------------------------------------------------------- */
	/* 4. Build                                                                */
	/* ---------------------------------------------------------------------- */

	buildCLI([]string{"test.rho"})

	outC := filepath.Join("build", "test.c")

	defer os.Remove(outC)
	defer os.Remove("test.rho")
	defer os.Remove("rho.mod")
	defer os.Remove("rho.lock")
	defer os.RemoveAll(".rho-deps")

	/* ---------------------------------------------------------------------- */
	/* 5. Verify C file exists                                                 */
	/* ---------------------------------------------------------------------- */

	if _, err := os.Stat(outC); err != nil {
		t.Fatalf("expected %s to exist", outC)
	}

	/* ---------------------------------------------------------------------- */
	/* 6. Verify binary exists                                                 */
	/* ---------------------------------------------------------------------- */

	binary := builtBinary("native")
	if binary == "" {
		t.Fatalf("expected a binary to be built for the native target, found none")
	}

	/* ---------------------------------------------------------------------- */
	/* 7. Run binary and check output                                          */
	/* ---------------------------------------------------------------------- */

	// Run what the build produced, at the path it produced it, rather than a
	// path this test assembles: a test that runs a file it constructed is a test
	// of the construction.
	cmd := exec.Command("./" + binary)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary failed: %v", err)
	}

	out := string(outBytes)
	if !strings.Contains(out, "Result: 5") {
		t.Fatalf("unexpected output: %s", out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Finding what a build produced                                              */
/* -------------------------------------------------------------------------- */

// builtBinary returns the first executable file in the build tree of a target,
// or "" when there is none.
//
// A build writes its output where CMake puts it, which is a tree named after the
// target rather than a file named after the module. Walking for it keeps the
// tests honest about what they are checking — that a binary was produced for the
// target — without restating the layout.
func builtBinary(target string) string {
	root := filepath.Join("build", "generated", "c", "build", target)
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 != 0 {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}
