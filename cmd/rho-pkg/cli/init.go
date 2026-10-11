//
// @file init.go
// @brief `rho pkg init` — start a new Rho project.
//
// The generator is `src/scaffold`, the same one `rho init` and the keeper use.
// A package command that wrote its own files would be a second description of a
// project, and the two would disagree the first time either changed.
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

	"rho/src/scaffold"
)

// Init writes a new project into the directory named, or the working directory.
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

// runInit is the work, separated so a test can call it without the arguments.
func runInit(dir string) error {
	_, err := scaffold.Application(scaffold.Options{Dir: dir})
	return err
}
