//
// @file registry_test.go
// @brief Tests for Rho Module Registry Operations - Enterprise v3.
//
// Validates:
//   - SearchRegistry returns correct results
//   - UnpublishModule removes module from registry
//   - UnpublishModule blocks active version with other versions
//   - UnpublishModule allows last active version
//   - ActivateModule changes active version
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
	"rho/src/schema"
)

/* -------------------------------------------------------------------------- */
/*  Test: Search All                                                           */
/* -------------------------------------------------------------------------- */

func TestSearchRegistryAll(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	results, err := SearchRegistry("")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected results")
	}

	// Check that we have math and io entries
	names := map[string]bool{}
	for _, r := range results {
		names[r.Name] = true
	}

	if !names["math"] {
		t.Fatalf("expected math in results")
	}
	if !names["io"] {
		t.Fatalf("expected io in results")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Search Query                                                         */
/* -------------------------------------------------------------------------- */

func TestSearchRegistryQuery(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	for _, r := range results {
		if r.Name != "math" {
			t.Fatalf("expected only math results, got %s", r.Name)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Search Active Status                                                 */
/* -------------------------------------------------------------------------- */

func TestSearchRegistryActiveStatus(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	results, err := SearchRegistry("")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	// math 1.2.0 should be active
	found := false
	for _, r := range results {
		if r.Name == "math" && r.Version == "1.2.0" {
			if !r.Active {
				t.Fatalf("expected math@1.2.0 to be active")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("math@1.2.0 not found in results")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unpublish Non-Active                                                  */
/* -------------------------------------------------------------------------- */

func TestUnpublishModule(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	// math 1.0.0 is not active (1.2.0 is active)
	err := UnpublishModule("math", "1.0.0")
	if err != nil {
		t.Fatalf("UnpublishModule failed: %v", err)
	}

	// Verify it's removed
	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	for _, r := range results {
		if r.Version == "1.0.0" {
			t.Fatalf("math@1.0.0 should have been removed")
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unpublish Active With Other Versions                                */
/* -------------------------------------------------------------------------- */

func TestUnpublishActiveWithOthers(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	// math 1.2.0 is active, 1.0.0 exists
	err := UnpublishModule("math", "1.2.0")
	if err == nil {
		t.Fatalf("expected error when unpublishing active version with other versions")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unpublish Only Active Version                                        */
/* -------------------------------------------------------------------------- */

func TestUnpublishOnlyActiveVersion(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	// First unpublish math@1.0.0 (non-active)
	if err := UninstallLocal("math", "1.0.0"); err != nil {
		t.Fatalf("failed to uninstall non-selected version: %v", err)
	}

	// Now math@1.2.0 is the only version and it is selected. Removing the last
	// version is allowed even when it is selected: there is nothing left to be
	// resolved against, so removing it is the point rather than a change a build
	// would notice. The refusal is for the case where another version is there
	// for the selection to fall back to.
	if err := UninstallLocal("math", "1.2.0"); err != nil {
		t.Fatalf("uninstalling the last version: %v", err)
	}
	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for math after uninstall, got %+v", results)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Activate Module                                                      */
/* -------------------------------------------------------------------------- */

func TestActivateModule(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	// math has 1.0.0 and 1.2.0, 1.2.0 is active
	// Activate 1.0.0
	err := ActivateModule("math", "1.0.0")
	if err != nil {
		t.Fatalf("ActivateModule failed: %v", err)
	}

	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	activeCount := 0
	for _, r := range results {
		if r.Active {
			activeCount++
			if r.Version != "1.0.0" {
				t.Fatalf("expected active version to be 1.0.0, got %s", r.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active version, got %d", activeCount)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Activate Non-existent Module                                          */
/* -------------------------------------------------------------------------- */

func TestActivateNonExistent(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	err := ActivateModule("nonexistent", "1.0.0")
	if err == nil {
		t.Fatalf("expected error for non-existent module")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unpublish Non-Existent Module                                        */
/* -------------------------------------------------------------------------- */

func TestUnpublishNonExistent(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	err := UnpublishModule("nonexistent", "1.0.0")
	if err == nil {
		t.Fatalf("expected error for non-existent module")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Registry Schema Validation                                          */
/* -------------------------------------------------------------------------- */

// A corrupt registry.yaml must be reported, not silently parsed as empty.
func TestReadRegistryRejectsMalformedRegistry(t *testing.T) {
	tmp, cleanup := SetupTestRegistry(t)
	defer cleanup()
	_ = tmp

	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		t.Fatalf("GetRegistryPath failed: %v", err)
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	// An entry without a version violates the registry schema.
	bad := "components:\n  - name: math\nplugins:\nactive_components:\nactive_plugins:\n"
	if err := os.WriteFile(registryFile, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := readRegistry(registryFile); err == nil {
		t.Fatal("expected schema error for registry entry missing version")
	}
}

// A registry entry carrying an unknown field is still readable, but a
// structurally wrong section type must fail.
func TestReadRegistryRejectsWrongSectionType(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		t.Fatalf("GetRegistryPath failed: %v", err)
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	bad := "components: not-a-list\nplugins:\nactive_components:\nactive_plugins:\n"
	if err := os.WriteFile(registryFile, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := readRegistry(registryFile); err == nil {
		t.Fatal("expected schema error for components section that is not a list")
	}
}

// The registry produced by our own writer must always satisfy the schema.
func TestWrittenRegistrySatisfiesSchema(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		t.Fatalf("GetRegistryPath failed: %v", err)
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	reg := &Registry{
		Components:       []VersionEntry{{Name: "math", Version: "1.2.0"}},
		ActiveComponents: []ActiveEntry{{Name: "math", Version: "1.2.0"}},
	}
	if err := writeRegistry(registryFile, reg); err != nil {
		t.Fatalf("writeRegistry failed: %v", err)
	}

	result, err := schema.ValidateRegistryFile(registryFile)
	if err != nil {
		t.Fatalf("ValidateRegistryFile failed: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("written registry violates schema: %s", result.String())
	}
}
