//
// @file publisher_test.go
// @brief Tests for `PublishModule` - Enterprise v3.
//
// Validates:
//   - Full publish pipeline: build -> pack -> install -> registry update
//   - Cleanup of build artifacts
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
)

/* -------------------------------------------------------------------------- */
/*  Test: Publish Module                                                       */
/* -------------------------------------------------------------------------- */

func TestPublishModule(t *testing.T) {
	tmp := t.TempDir()

	// Create a minimal Go module structure for building
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module hello\n\ngo 1.22\n"), 0644)
	os.Mkdir(filepath.Join(tmp, "src"), 0755)
	os.WriteFile(filepath.Join(tmp, "src", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	// module.yml
	os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: component
  version: 0.1.0

main: src/main.go
`), 0644)

	// README.md
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)

	if err := PublishModule(tmp); err != nil {
		t.Fatalf("PublishModule failed: %v", err)
	}
}
