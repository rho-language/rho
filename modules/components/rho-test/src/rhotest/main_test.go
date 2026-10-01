//
// @file main_test.go
// @brief Test setup for the test runner's package.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"os"
	"testing"
)

// TestMain points the whole test binary at an empty Rho home.
//
// Plugin discovery reads the Rho home as well as the tree it is given, so a test
// that builds a fixture plugin in a temporary directory and asserts on what was
// discovered also discovers whatever the machine running it has installed. The
// assertions then depend on the developer: they pass on a machine with nothing
// published, and fail on one with a plugin installed — the same code, two
// answers, neither of which is about the code.
//
// An empty home says what the tests mean: the tree is the whole world. A test
// that wants a plugin in the Rho home asks for one by writing one there, which
// is what a fixture is for.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "rho-test-home-")
	if err != nil {
		// A home that cannot be created is a home that cannot be isolated, and
		// continuing would mean the tests silently read the developer's. Failing
		// here says so instead.
		panic("rho-test: cannot create an isolated Rho home: " + err.Error())
	}
	// Set rather than used through a setter: the tests in this package reach for
	// the same resolution the toolchain does, and the environment is what that
	// resolution reads.
	if err := os.Setenv("RHO_HOME", home); err != nil {
		panic("rho-test: cannot point RHO_HOME at the isolated home: " + err.Error())
	}

	code := m.Run()

	os.RemoveAll(home)
	os.Exit(code)
}
