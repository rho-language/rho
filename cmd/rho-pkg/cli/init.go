//
// @file init.go
// @brief `rho pkg init` - Enterprise v3.
//
// Creates a new Rho project with the official directory layout:
//   - module.yml
//   - src/main.rho
//   - README.md
//   - config/app.yml (optional)
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"rho/cmd/rho-pkg/core"
)

/* -------------------------------------------------------------------------- */
/*  Init Command                                                               */
/* -------------------------------------------------------------------------- */

func Init(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	if err := runInit(dir); err != nil {
		return fmt.Errorf("rho pkg init: %w", err)
	}
	return nil
}

func runInit(dir string) error {
	// Create project directory if needed
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create directory: %w", err)
		}
	}

	// Check if module.yml exists
	modPath := filepath.Join(dir, "module.yml")
	if _, err := os.Stat(modPath); err == nil {
		return errors.New("module.yml already exists")
	}

	// Create src/
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		return fmt.Errorf("cannot create src/: %w", err)
	}

	// Write main.rho
	mainSrc := `module main

fn main() void {
    print("Hello from Rho!");
}
`
	if err := os.WriteFile(filepath.Join(dir, "src", "main.rho"), []byte(mainSrc), 0644); err != nil {
		return fmt.Errorf("cannot write main.rho: %w", err)
	}

	// Write README.md
	readme := "# Rho Project\n\nGenerated with `rho pkg init`.\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("cannot write README.md: %w", err)
	}

	// Write module.yml
	manifest := core.ModuleYAML{
		Module: core.ModuleSection{
			Name:    filepath.Base(dir),
			Type:    "rho",
			Version: "0.1.0",
		},
		Main:     "src/main.rho",
		Features: []string{},
	}

	if err := core.SaveModuleYAML(modPath, manifest); err != nil {
		return fmt.Errorf("cannot write module.yml: %w", err)
	}

	return nil
}
