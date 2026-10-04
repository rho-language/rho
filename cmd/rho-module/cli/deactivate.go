//
// @file deactivate.go
// @brief `rho module deactivate` - stepping back from the selected version.
//
// Selection is what a project resolves against: the index names one installed
// version of a module, and the resolver uses it. Deactivating clears that
// choice, and is what `uninstall` asks for before it will remove a version — a
// selected version is the one a build is currently linked against, and a build
// that suddenly cannot resolve a component is a worse outcome than a version
// still sitting on disk.
//
// With nothing selected the resolver picks the newest installed version, so
// deactivating one of several moves the selection rather than leaving the module
// resolving to nothing. A user who wants a version gone for good deactivates it
// and then uninstalls it.
//
// Usage:
//   rho module deactivate <name>@<version>
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
/*  Deactivate Command                                                         */
/* -------------------------------------------------------------------------- */

func Deactivate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module deactivate <name>@<version>")
	}
	name, version := parseSpec(args[0])
	if name == "" || version == "" {
		return fmt.Errorf("usage: rho module deactivate <name>@<version>")
	}
	if err := core.Deactivate(name, version); err != nil {
		return fmt.Errorf("rho module deactivate: %w", err)
	}
	fmt.Printf("Deactivated %s@%s\n", name, version)
	return nil
}
