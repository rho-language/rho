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
// SPDX-License-Identifier: MIT
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

	if meta.Module.Type == "plugin" && len(meta.Hooks) > 0 {
		fmt.Println("Hooks:")
		for _, h := range meta.Hooks {
			fmt.Println("  -", h)
		}
	}

	return nil
}
