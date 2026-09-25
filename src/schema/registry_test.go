//
// @file registry_test.go
// @brief Tests for registry.yaml schema validation.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Valid Registry                                                      */
/* -------------------------------------------------------------------------- */

func TestRegistrySchemaValid(t *testing.T) {
	src := `components:
  - name: math
    version: 1.0.0
  - name: math
    version: 1.2.0
plugins:
  - name: http
    version: 0.9.0
active_components:
  - name: math
    version: 1.2.0
active_plugins:
  - name: http
    version: 0.9.0
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, RegistrySchema())
	if result.HasErrors() {
		t.Fatalf("expected no errors for valid registry, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Missing Entry Fields                                                 */
/* -------------------------------------------------------------------------- */

func TestRegistrySchemaMissingVersion(t *testing.T) {
	src := `components:
  - name: math
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, RegistrySchema())
	if !result.HasErrors() {
		t.Fatal("expected error for missing version in registry entry")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Empty Registry                                                      */
/* -------------------------------------------------------------------------- */

func TestRegistrySchemaEmptyValid(t *testing.T) {
	src := `components:
plugins:
active_components:
active_plugins:
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, RegistrySchema())
	if result.HasErrors() {
		t.Fatalf("expected no errors for empty registry, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: ValidateRegistryFile                                                */
/* -------------------------------------------------------------------------- */

func TestValidateRegistryFileValid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "registry.yaml")
	content := `components:
  - name: math
    version: 1.2.0
active_components:
  - name: math
    version: 1.2.0
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateRegistryFile(path)
	if err != nil {
		t.Fatalf("ValidateRegistryFile error: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

func TestValidateRegistryFileMissing(t *testing.T) {
	_, err := ValidateRegistryFile("/nonexistent/registry.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateRegistryFileInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "registry.yaml")
	// Duplicate keys are rejected by the loader.
	if err := os.WriteFile(path, []byte("components:\nplugins:\ncomponents:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateRegistryFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
