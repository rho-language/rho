//
// @file main.go
// @brief The rho-robohub command.
//
// The plugin is dispatched to as a command and reads its request from standard
// input, which is the program rho-target, rho-kconfig, rho-cimport and
// rho-flash run. One program answers all four contracts; the envelope says which.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os"

	"github.com/rho/rho-robohub/src/rhorobohub"
)

func main() {
	os.Exit(rhorobohub.Main(os.Args[1:]))
}
