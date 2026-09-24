//
// @file manifest_test.go
// @brief Tests for Rho Package Manifest - Enterprise v3.
//
// Validates:
//   - loading module.yml
//   - saving module.yml
//   - validation rules
//   - features list
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
/*  Test: Load Valid Manifest                                                  */
/* -------------------------------------------------------------------------- */

func TestLoadValidManifest(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "module.yml")

    content := `
module:
  name: hello-rho
  type: rho
  version: 0.1.0

main: src/main.rho

features:
  - math
  - io
`
    os.WriteFile(path, []byte(content), 0644)

    m, err := LoadModuleYAML(path)
    if err != nil {
        t.Fatalf("LoadModuleYAML failed: %v", err)
    }

    if m.Module.Name != "hello-rho" {
        t.Fatalf("wrong module name")
    }
    if m.Main != "src/main.rho" {
        t.Fatalf("wrong main")
    }
    if len(m.Features) != 2 {
        t.Fatalf("expected 2 features")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Save Manifest                                                        */
/* -------------------------------------------------------------------------- */

func TestSaveManifest(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "module.yml")

    m := ModuleYAML{
        Module: ModuleSection{
            Name:    "hello-rho",
            Type:    "rho",
            Version: "0.1.0",
        },
        Main:     "src/main.rho",
        Features: []string{"math"},
    }

    if err := SaveModuleYAML(path, m); err != nil {
        t.Fatalf("SaveModuleYAML failed: %v", err)
    }

    if _, err := os.Stat(path); err != nil {
        t.Fatalf("module.yml not written")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Validation Errors                                                    */
/* -------------------------------------------------------------------------- */

func TestValidateManifestErrors(t *testing.T) {
    m := ModuleYAML{
        Module: ModuleSection{
            Name:    "",
            Type:    "rho",
            Version: "0.1.0",
        },
        Main: "src/main.rho",
    }

    if err := ValidateModuleYAML(m); err == nil {
        t.Fatalf("expected validation error for missing name")
    }
}
