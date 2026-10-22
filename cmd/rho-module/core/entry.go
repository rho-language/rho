//
// @file entry.go
// @brief Rho Module Entrypoint Executor - Enterprise v3.
//
// Executes the main entrypoint of a module as declared in module.yml.
//
// Responsibilities:
//   - resolve main file path
//   - execute the module's binary, when main names one
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
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
		// A module is run from where it was installed, and it is told so. Its
		// own files travel with it in the package — a configuration file, a
		// template, anything it reads at run time — and a program that has to
		// find them by walking up from its own path is a program that breaks the
		// moment it is invoked through a link or from a different directory.
		cmd.Env = append(os.Environ(), "RHO_MODULE_ROOT="+e.Module.RootPath)
		return cmd.Run()
	}

	// Anything else is a source file, and there is nothing to run it with. This
	// used to hand it to a `rho-runner` command that this repository has never
	// built — every call ended in "executable file not found", which read as a
	// problem with the machine rather than a statement that the feature does not
	// exist. A Rho program is compiled (`rho build`, `rho run`), and a module
	// that is run as a command is a component with a binary.
	return fmt.Errorf("%s is a source file, not a program: build it with `rho build`, "+
		"or declare a binary as the module's main", e.Module.Manifest.Main)
}
