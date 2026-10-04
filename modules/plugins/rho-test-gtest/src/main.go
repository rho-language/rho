//
// @file main.go
// @brief The rho-test-gtest command.
//
// The plugin is dispatched to as a command, like any other module, and reads its
// request from standard input. It is the same program the test runner runs — and
// the same program the report step of a resolved suite calls back into.
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

	"github.com/rho/rho-test-gtest/src/gtest"
)

func main() {
	os.Exit(gtest.Main(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
