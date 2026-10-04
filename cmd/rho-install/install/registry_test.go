//
// @file registry_test.go
// @brief Tests for Rho Install - Local registry initialization subsystem.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  InitLocalRegistry                                                          */
/* -------------------------------------------------------------------------- */

func TestInitLocalRegistry(t *testing.T) {
	// Ensure ~/.rho exists
	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Create registry
	if err := InitLocalRegistry(); err != nil {
		t.Fatalf("InitLocalRegistry failed: %v", err)
	}

	regPath, _ := GetRegistryPath()
	file := filepath.Join(regPath, "registry.yaml")

	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Fatalf("registry.yaml missing: %s", file)
	}
}

/* -------------------------------------------------------------------------- */
/*  RegistryExists                                                             */
/* -------------------------------------------------------------------------- */

func TestRegistryExists(t *testing.T) {
	// Ensure ~/.rho exists
	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Create registry
	InitLocalRegistry()

	if !RegistryExists() {
		t.Fatalf("RegistryExists returned false but registry.yaml exists")
	}
}
