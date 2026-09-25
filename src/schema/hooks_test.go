//
// @file hooks_test.go
// @brief Tests for plugin/hooks.yml schema validation.
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
/*  Tests: Valid Hooks                                                         */
/* -------------------------------------------------------------------------- */

func TestHooksSchemaValid(t *testing.T) {
	src := `hooks:
  - init
  - configure
  - shutdown
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, HooksSchema())
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Missing Hooks Field                                                 */
/* -------------------------------------------------------------------------- */

func TestHooksSchemaMissingHooks(t *testing.T) {
	src := `other: value
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, HooksSchema())
	if !result.HasErrors() {
		t.Fatal("expected error for missing hooks field")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Empty Hook Name                                                     */
/* -------------------------------------------------------------------------- */

func TestValidateHooksFileEmptyHookName(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hooks.yml")
	content := `hooks:
  - init
  -
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateHooksFile(path)
	if err != nil {
		t.Fatalf("ValidateHooksFile error: %v", err)
	}
	if !result.HasErrors() {
		t.Fatal("expected error for empty hook name")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: ValidateHooksFile                                                   */
/* -------------------------------------------------------------------------- */

func TestValidateHooksFileValid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hooks.yml")
	content := `hooks:
  - init
  - configure
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateHooksFile(path)
	if err != nil {
		t.Fatalf("ValidateHooksFile error: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

func TestValidateHooksFileMissing(t *testing.T) {
	_, err := ValidateHooksFile("/nonexistent/hooks.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateHooksFileInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hooks.yml")
	// Duplicate keys are rejected by the loader.
	if err := os.WriteFile(path, []byte("hooks:\n  - init\nhooks:\n  - shutdown\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateHooksFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: No Hooks Defined                                                    */
/* -------------------------------------------------------------------------- */

func TestHooksSchemaNoHooksContent(t *testing.T) {
	src := `hooks:
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, HooksSchema())
	if !result.HasErrors() {
		t.Fatal("expected error for hooks field being nil/empty")
	}
}
