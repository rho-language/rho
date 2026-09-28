//
// @file main.go
// @brief The rho-doc-api command.
//
// The plugin is dispatched to as a command, like any other module, and reads
// its request from standard input. It is the same program the documentation
// generator runs, so a page a build writes and a page a service serves are one
// page.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"

	"github.com/rho/rho-doc-api/src/apidoc"
)

func main() {
	os.Exit(apidoc.Main())
}
