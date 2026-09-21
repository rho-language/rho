//
// @file pkg_test.go
// @brief Rho Package Manager tests.
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
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func cleanup() {
    os.Remove(manifestFile)
}

/* -------------------------------------------------------------------------- */
/*  Tests: init                                                               */
/* -------------------------------------------------------------------------- */

func TestPkgInit(t *testing.T) {
    defer cleanup()

    if err := cmdInit(); err != nil {
        t.Fatalf("cmdInit failed: %v", err)
    }

    if _, err := os.Stat(manifestFile); err != nil {
        t.Fatalf("rho.yaml not created")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: add                                                                */
/* -------------------------------------------------------------------------- */

func TestPkgAdd(t *testing.T) {
    defer cleanup()

    if err := cmdInit(); err != nil {
        t.Fatalf("cmdInit failed: %v", err)
    }

    if err := cmdAdd("core", "https://example.com/core.git"); err != nil {
        t.Fatalf("cmdAdd failed: %v", err)
    }

    m, err := loadManifest()
    if err != nil {
        t.Fatalf("loadManifest failed: %v", err)
    }

    if m.Deps["core"] != "https://example.com/core.git" {
        t.Fatalf("dep not stored correctly")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: list                                                               */
/* -------------------------------------------------------------------------- */

func TestPkgListMissing(t *testing.T) {
    cleanup()
    if err := cmdList(); err == nil {
        t.Fatalf("expected error for missing manifest")
    }
}
