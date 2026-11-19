//
// @file main.go
// @brief `rho req` — requirements, decisions, and the trace from them to code and tests.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"os"

	"github.com/rho/rho-req/src/rhoreq"
)

func main() {
	os.Exit(rhoreq.Main(os.Args[1:], os.Stdout, os.Stderr))
}
