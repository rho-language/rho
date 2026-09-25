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
	"testing"
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
	if err := UnpublishModule("math", "1.0.0"); err != nil {
		t.Fatalf("failed to unpublish non-active version: %v", err)
	}

	// Now math@1.2.0 is the only version and it's active
	// Should succeed and clear from active list
	err := UnpublishModule("math", "1.2.0")
	if err != nil {
		t.Fatalf("UnpublishModule should succeed for only active version: %v", err)
	}

	// Verify math is completely removed
	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	if len(results) != 0 {
		t.Fatalf("expected no results for math after unpublish")
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