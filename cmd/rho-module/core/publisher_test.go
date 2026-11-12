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
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgcore "rho/cmd/rho-pkg/core"
	"rho/src/exportsig"
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

// A published version's exports are recorded beside it, and a later version
// that drops one of them without the version that says so is refused.
func TestPublishRecordsExportsAndRefusesABreakingPatch(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module shape\n\ngo 1.22\n"), 0644)
	os.Mkdir(filepath.Join(tmp, "src"), 0755)
	os.WriteFile(filepath.Join(tmp, "src", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	manifest := func(version string) []byte {
		return []byte("module:\n  name: shape-tool\n  type: component\n  version: " + version + "\n\nmain: src/main.go\n")
	}
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Shapes"), 0644)
	os.WriteFile(filepath.Join(tmp, "module.yml"), manifest("0.1.0"), 0644)
	if err := PublishModule(tmp); err != nil {
		t.Fatalf("publish 0.1.0: %v", err)
	}
	loaded, err := pkgcore.LoadModuleYAMLStrict(filepath.Join(tmp, "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := installedModuleDir(loaded)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := exportsig.Load(dir)
	if err != nil || recorded.Version != "0.1.0" {
		t.Fatalf("the exports of 0.1.0 were not recorded: %+v, %v", recorded, err)
	}

	// Say 0.1.0 had exported a function that 0.1.1 no longer has.
	recorded.Entries["area"] = exportsig.Entry{Name: "area", Kind: "fn", Signature: "fn(u32, u32) u32", Display: "fn area(w: u32, h: u32) u32"}
	exportsig.Write(dir, recorded)
	os.WriteFile(filepath.Join(tmp, "module.yml"), manifest("0.1.1"), 0644)
	err = PublishModule(tmp)
	if err == nil || !strings.Contains(err.Error(), "needs at least 0.2.0") || !strings.Contains(err.Error(), "rho module check") {
		t.Fatalf("a patch release that drops an export: %v", err)
	}
	os.WriteFile(filepath.Join(tmp, "module.yml"), manifest("0.2.0"), 0644)
	if err := PublishModule(tmp); err != nil {
		t.Fatalf("publish 0.2.0: %v", err)
	}
}
