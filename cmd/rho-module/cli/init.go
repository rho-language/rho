//
// @file init.go
// @brief `rho module init` - Enterprise v3.
//
// Creates a new Rho module or plugin with the official directory layout.
// Supported types:
//   - rho      (application module)
//   - module   (library)
//   - plugin   (toolchain extension)
//
// Usage:
//   rho module init <name> <type>
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Init Command                                                               */
/* -------------------------------------------------------------------------- */

func Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: rho module init <name> <type>")
	}

	name := args[0]
	mtype := args[1]

	if err := runInit(name, mtype); err != nil {
		return fmt.Errorf("rho module init: %w", err)
	}

	fmt.Println("Module created:", name)
	return nil
}

func runInit(name, mtype string) error {
	if mtype != "rho" && mtype != "module" && mtype != "plugin" {
		return errors.New("type must be one of: rho, module, plugin")
	}

	// Create directory
	if err := os.MkdirAll(name, 0755); err != nil {
		return fmt.Errorf("cannot create module directory: %w", err)
	}

	// module.yml path
	modPath := filepath.Join(name, "module.yml")

	if _, err := os.Stat(modPath); err == nil {
		return errors.New("module.yml already exists")
	}

	// Create src/
	srcDir := filepath.Join(name, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("cannot create src/: %w", err)
	}

	// Write default source file
	var srcFile string
	var srcContent string

	switch mtype {
	case "rho":
		srcFile = "main.rho"
		srcContent = `module main

fn main() void {
    print("Hello from Rho module!");
}
`
	case "module":
		srcFile = "lib.rho"
		srcContent = `module ` + name + `

pub fn hello() void {
    print("Hello from Rho library!");
}
`
	case "plugin":
		srcFile = "plugin.rho"
		srcContent = `module ` + name + `

pub fn init() void {
    print("Plugin initialized");
}
`
		// Create plugin/ directory
		if err := os.MkdirAll(filepath.Join(name, "plugin"), 0755); err != nil {
			return fmt.Errorf("cannot create plugin/: %w", err)
		}

		// Write hooks.yml
		hooks := "hooks:\n  - init\n"
		os.WriteFile(filepath.Join(name, "plugin", "hooks.yml"), []byte(hooks), 0644)
	}

	if err := os.WriteFile(filepath.Join(srcDir, srcFile), []byte(srcContent), 0644); err != nil {
		return fmt.Errorf("cannot write source file: %w", err)
	}

	// Write README.md
	readme := "# " + name + "\n\nGenerated with `rho module init`.\n"
	if err := os.WriteFile(filepath.Join(name, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("cannot write README.md: %w", err)
	}

	// Write module.yml
	manifest := core.ModuleYAML{
		Module: core.ModuleSection{
			Name:    name,
			Type:    mtype,
			Version: "0.1.0",
		},
		Main:     "src/" + srcFile,
		Features: []string{},
	}

	if err := core.SaveModuleYAML(modPath, manifest); err != nil {
		return fmt.Errorf("cannot write module.yml: %w", err)
	}

	return nil
}
