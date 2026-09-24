//
// @file router.go
// @brief Module CLI router - Enterprise v3.
//
// Routes `rho module <command>` calls from the main rho CLI to individual
// command implementations. Returns int exit codes for integration.
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
	"os"
)

/* -------------------------------------------------------------------------- */
/*  Main Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func Main(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: rho module <command> [args]")
		fmt.Fprintln(os.Stderr, "Commands: init, validate, info, publish, download, install, remove, update, search")
		return 2
	}

	cmd := args[0]

	var err error
	switch cmd {
	case "init":
		err = Init(args[1:])
	case "validate":
		err = Validate(args[1:])
	case "info":
		err = Info(args[1:])
	case "publish":
		err = Publish(args[1:])
	case "download":
		err = Download(args[1:])
	case "install":
		err = Install(args[1:])
	case "remove":
		err = Remove(args[1:])
	case "update":
		err = Update(args[1:])
	case "search":
		err = Search(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown module command:", cmd)
		fmt.Fprintln(os.Stderr, "Commands: init, validate, info, publish, download, install, remove, update, search")
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}