//
// @file main_test.go
// @brief Tests for Rho Install - CLI orchestrator.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
    "os"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Basic Smoke Test                                                           */
/* -------------------------------------------------------------------------- */

func TestMainSmoke(t *testing.T) {
    // Just ensure main() runs without panic when no args are provided
    os.Args = []string{"rho-install"}
    main()
}
