//
// @file shell_test.go
// @brief Tests for Rho Install - Shell utilities.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package util

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  DetectShell                                                                */
/* -------------------------------------------------------------------------- */

func TestDetectShell(t *testing.T) {
    os.Setenv("SHELL", "/bin/bash")
    if DetectShell() != "bash" {
        t.Fatalf("DetectShell failed for bash")
    }

    os.Setenv("SHELL", "/usr/bin/zsh")
    if DetectShell() != "zsh" {
        t.Fatalf("DetectShell failed for zsh")
    }

    os.Setenv("SHELL", "/usr/bin/fish")
    if DetectShell() != "fish" {
        t.Fatalf("DetectShell failed for fish")
    }
}

/* -------------------------------------------------------------------------- */
/*  GetShellRC                                                                 */
/* -------------------------------------------------------------------------- */

func TestGetShellRC(t *testing.T) {
    tmp := t.TempDir()
    os.Setenv("HOME", tmp)

    os.Setenv("SHELL", "/bin/bash")
    rc, err := GetShellRC()
    if err != nil {
        t.Fatalf("GetShellRC failed: %v", err)
    }
    if filepath.Base(rc) != ".bashrc" {
        t.Fatalf("unexpected RC file: %s", rc)
    }
}

/* -------------------------------------------------------------------------- */
/*  EnsureRC                                                                   */
/* -------------------------------------------------------------------------- */

func TestEnsureRC(t *testing.T) {
    tmp := t.TempDir()
    os.Setenv("HOME", tmp)
    os.Setenv("SHELL", "/bin/bash")

    rc, err := EnsureRC()
    if err != nil {
        t.Fatalf("EnsureRC failed: %v", err)
    }

    if _, err := os.Stat(rc); os.IsNotExist(err) {
        t.Fatalf("RC file missing: %s", rc)
    }
}

/* -------------------------------------------------------------------------- */
/*  RCContains / AppendToRC                                                    */
/* -------------------------------------------------------------------------- */

func TestRCContainsAndAppend(t *testing.T) {
    tmp := t.TempDir()
    os.Setenv("HOME", tmp)
    os.Setenv("SHELL", "/bin/bash")

    rc, _ := EnsureRC()

    // Append line
    line := "export PATH=/test/bin:$PATH"
    if err := AppendToRC(rc, line); err != nil {
        t.Fatalf("AppendToRC failed: %v", err)
    }

    // Check line
    ok, err := RCContains(rc, line)
    if err != nil {
        t.Fatalf("RCContains failed: %v", err)
    }
    if !ok {
        t.Fatalf("RCContains returned false for appended line")
    }
}
