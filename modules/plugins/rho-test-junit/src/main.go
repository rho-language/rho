//
// @file main.go
// @brief The rho-test-junit command.
//
// The plugin is dispatched to as a command, like any other module, and reads
// its request from standard input. It is the same program the test runner runs.
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

	"github.com/rho/rho-test-junit/src/junit"
)

func main() {
	os.Exit(junit.Main())
}
