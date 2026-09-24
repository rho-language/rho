//
// @file entry.go
// @brief Rho Module Entrypoint Executor - Enterprise v3.
//
// Executes the main entrypoint of a module as declared in module.yml.
//
// Responsibilities:
//   - resolve main file path
//   - execute module via rho-runner
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type ModuleEntry struct {
    Module *Module
}

/* -------------------------------------------------------------------------- */
/*  Entry                                                                      */
/* -------------------------------------------------------------------------- */

func (m *Module) Entry() *ModuleEntry {
    if m.Manifest.Main == "" {
        return nil
    }
    return &ModuleEntry{Module: m}
}

/* -------------------------------------------------------------------------- */
/*  Run                                                                        */
/* -------------------------------------------------------------------------- */

func (e *ModuleEntry) Run(args []string, stdin *os.File, stdout *os.File) error {
    mainFile := filepath.Join(e.Module.RootPath, e.Module.Manifest.Main)

    if _, err := os.Stat(mainFile); err != nil {
        return fmt.Errorf("entrypoint not found: %s", mainFile)
    }

    // Check if mainFile is an executable binary
    info, err := os.Stat(mainFile)
    if err == nil && info.Mode()&0111 != 0 {
        // It's an executable, run it directly
        cmd := exec.Command(mainFile, args...)
        cmd.Stdin = stdin
        cmd.Stdout = stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }

    // Otherwise, run via rho-runner (for .rho source files)
    cmd := exec.Command("rho-runner", append([]string{mainFile}, args...)...)
    cmd.Stdin = stdin
    cmd.Stdout = stdout
    cmd.Stderr = os.Stderr

    return cmd.Run()
}
