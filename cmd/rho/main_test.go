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

func TestTestCLIRejectsTooManyArguments(t *testing.T) {
    if code := testCLI([]string{"one", "two"}); code != 2 {
        t.Fatalf("testCLI returned %d, want 2", code)
    }
}

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
    outBin := outC + "_bin"

    defer os.Remove(in)
    defer os.Remove(outC)
    defer os.Remove(outBin)
    defer os.Remove("rho.mod")
    defer os.Remove("rho.lock")
    defer os.RemoveAll(".rho-deps")

    buildCLI([]string{in})

    if _, err := os.Stat(outC); err != nil {
        t.Fatalf("expected %s to exist", outC)
    }

    if _, err := os.Stat(outBin); err != nil {
        t.Fatalf("expected binary %s to exist", outBin)
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
    outBin := outC + "_bin"

    defer os.Remove(in)
    defer os.Remove(outC)
    defer os.Remove(outBin)
    defer os.Remove("rho.mod")
    defer os.Remove("rho.lock")
    defer os.RemoveAll(".rho-deps")

    buildCLI([]string{"-o", outC, in})

    if _, err := os.Stat(outC); err != nil {
        t.Fatalf("expected custom output %s to exist", outC)
    }

    if _, err := os.Stat(outBin); err != nil {
        t.Fatalf("expected binary %s to exist", outBin)
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
    outBin := outC + "_bin"

    defer os.Remove(outC)
    defer os.Remove(outBin)
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

    if _, err := os.Stat(outBin); err != nil {
        t.Fatalf("expected %s to exist", outBin)
    }

    /* ---------------------------------------------------------------------- */
    /* 7. Run binary and check output                                          */
    /* ---------------------------------------------------------------------- */

    cmd := exec.Command("./" + outBin)
    outBytes, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("binary failed: %v", err)
    }

    out := string(outBytes)
    if !strings.Contains(out, "Result: 5") {
        t.Fatalf("unexpected output: %s", out)
    }
}
