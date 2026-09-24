package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: LoadModuleYAML                                                       */
/* -------------------------------------------------------------------------- */

func TestLoadModuleYAML(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "module.yml")

    content := `
module:
  name: testmod
  type: module
  version: 1.0.0

main: src/lib.rho

deps:
  math: ">=1.0.0 <2.0.0"

features:
  - async
`
    os.WriteFile(path, []byte(content), 0644)

    mod, err := LoadModuleYAML(path)
    if err != nil {
        t.Fatalf("LoadModuleYAML failed: %v", err)
    }

    if mod.Module.Name != "testmod" {
        t.Fatalf("wrong module name")
    }
    if mod.Main != "src/lib.rho" {
        t.Fatalf("wrong main")
    }
    if mod.Deps["math"] == "" {
        t.Fatalf("missing dep math")
    }
    if mod.Features[0] != "async" {
        t.Fatalf("missing feature async")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing Required Fields                                               */
/* -------------------------------------------------------------------------- */

func TestLoadModuleYAMLMissingFields(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "module.yml")

    content := `
module:
  name: testmod
  type: module

main: src/lib.rho
`
    os.WriteFile(path, []byte(content), 0644)

    if _, err := LoadModuleYAML(path); err == nil {
        t.Fatalf("expected error for missing version")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: SaveModuleYAML                                                       */
/* -------------------------------------------------------------------------- */

func TestSaveModuleYAML(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "module.yml")

    mod := ModuleYAML{
        Module: ModuleSection{
            Name:    "hello",
            Type:    "module",
            Version: "1.0.0",
        },
        Main:     "src/lib.rho",
        Deps:     map[string]string{"math": "1.2.0"},
        Features: []string{"async"},
    }

    if err := SaveModuleYAML(path, mod); err != nil {
        t.Fatalf("SaveModuleYAML failed: %v", err)
    }

    data, _ := os.ReadFile(path)
    s := string(data)

    if !strings.Contains(s, "name: hello") {
        t.Fatalf("missing name in saved manifest")
    }
    if !strings.Contains(s, "math: \"1.2.0\"") {
        t.Fatalf("missing dep math")
    }
    if !strings.Contains(s, "- async") {
        t.Fatalf("missing feature async")
    }
}
