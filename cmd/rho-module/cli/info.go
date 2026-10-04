//
// @file info.go
// @brief `rho module info` - Enterprise v3.
//
// Displays metadata about a Rho module or plugin by reading module.yml.
// For plugins, also displays available hooks.
//
// Usage:
//   rho module info
//   rho module info <dir>
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Info Command                                                               */
/* -------------------------------------------------------------------------- */

func Info(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	meta, err := core.ReadModuleInfo(dir)
	if err != nil {
		return fmt.Errorf("rho module info: %w", err)
	}

	fmt.Println("Module:", meta.Module.Name)
	fmt.Println("Type:   ", meta.Module.Type)
	fmt.Println("Version:", meta.Module.Version)
	fmt.Println("Main:   ", meta.Main)

	if len(meta.Features) > 0 {
		fmt.Println("Features:")
		for _, f := range meta.Features {
			fmt.Println("  -", f)
		}
	}

	// A plugin is attached to a component, so the report leads with what it
	// plugs into: the hooks alone say nothing about whether it will ever run.
	if meta.Module.Type == "plugin" && meta.Extends != "" {
		fmt.Println("Extends:", meta.Extends)
		fmt.Println("Hooks:")
		for _, h := range meta.Hooks {
			fmt.Println("  -", h)
		}
		fmt.Println("Commands:")
		for _, command := range meta.Commands {
			fmt.Println("  -", command)
		}
	}

	return nil
}
