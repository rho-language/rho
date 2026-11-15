//
// @file applications_test.go
// @brief The applications this repository carries generate for their targets.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"path/filepath"
	"testing"
)

// The registry and the documentation site are Rho programs this repository
// ships; a change to the checker that refuses one of them is a regression, as
// clock_year being tied to one store's port was.
func TestShippedApplicationsGenerate(t *testing.T) {
	for _, app := range []string{"registry", "docsite"} {
		t.Run(app, func(t *testing.T) {
			entry, err := filepath.Abs(filepath.Join("..", "applications", app, "src", "main.rho"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := GenerateAllTargets(entry, t.TempDir(), nil, nil, nil); err != nil {
				t.Fatalf("%s does not generate: %v", app, err)
			}
		})
	}
}
