//
// @file go.mod
// @brief Requirements, decisions and the trace from them to code and tests.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

module github.com/rho/rho-req

go 1.26.11

toolchain go1.26.11

// The module manifest is read with the toolchain's own YAML loader, and the
// command line with the toolchain's own interspersed-flag parse, so `rho req`
// reads a manifest and a flag the way `rho build` does.
require rho v0.0.0
