//
// @file main.go
// @brief The rho-doc-toc command.
//
// The plugin is dispatched to as a command, like any other module, and reads
// its request from standard input. It is the same program the documentation
// generator runs.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os"

	"github.com/rho/rho-doc-toc/src/toc"
)

func main() {
	os.Exit(toc.Main())
}
