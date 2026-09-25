//
// @file schema_test.go
// @brief Tests for the Rho YAML Schema Validation Framework.
//
// Validates:
//   - basic field validation (required, kind, allowed values)
//   - nested map validation
//   - list item validation
//   - error/warning severity
//   - HasErrors / ErrorString behavior
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"strings"
	"testing"

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Required Fields                                                     */
/* -------------------------------------------------------------------------- */

func TestValidateRequiredMissing(t *testing.T) {
	data, err := yaml.Load("name: test\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{Name: "name", Required: true, Kind: KindString},
			{Name: "missing", Required: true, Kind: KindString},
		},
	}

	result := Validate(data, sc)
	if !result.HasErrors() {
		t.Fatal("expected errors for missing required field")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Valid Value                                                         */
/* -------------------------------------------------------------------------- */

func TestValidateValidData(t *testing.T) {
	data, err := yaml.Load("name: test\nversion: 1.0.0\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{Name: "name", Required: true, Kind: KindString},
			{Name: "version", Required: true, Kind: KindString},
		},
	}

	result := Validate(data, sc)
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Kind Mismatch                                                      */
/* -------------------------------------------------------------------------- */

func TestValidateKindMismatch(t *testing.T) {
	data, err := yaml.Load("name:\n  nested: value\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{Name: "name", Required: true, Kind: KindString},
		},
	}

	result := Validate(data, sc)
	if !result.HasErrors() {
		t.Fatal("expected error for kind mismatch")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Allowed Values                                                      */
/* -------------------------------------------------------------------------- */

func TestValidateAllowedValues(t *testing.T) {
	data, err := yaml.Load("type: invalid\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{Name: "type", Required: true, Kind: KindString, AllowedValues: []string{"rho", "plugin"}},
		},
	}

	result := Validate(data, sc)
	if !result.HasErrors() {
		t.Fatal("expected error for invalid allowed value")
	}
}

func TestValidateAllowedValuesValid(t *testing.T) {
	data, err := yaml.Load("type: rho\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{Name: "type", Required: true, Kind: KindString, AllowedValues: []string{"rho", "plugin"}},
		},
	}

	result := Validate(data, sc)
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: List Validation                                                     */
/* -------------------------------------------------------------------------- */

func TestValidateListItems(t *testing.T) {
	data, err := yaml.Load("items:\n  - one\n  - two\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{
				Name: "items",
				Required: true,
				Kind: KindList,
				ItemSchema: &ListItemSchema{Kind: KindString},
			},
		},
	}

	result := Validate(data, sc)
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

func TestValidateListItemsWrongKind(t *testing.T) {
	data, err := yaml.Load("items:\n  - name: test\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{
				Name: "items",
				Required: true,
				Kind: KindList,
				ItemSchema: &ListItemSchema{Kind: KindString},
			},
		},
	}

	result := Validate(data, sc)
	if !result.HasErrors() {
		t.Fatal("expected error for list item kind mismatch")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Nested Map Validation                                               */
/* -------------------------------------------------------------------------- */

func TestValidateNestedMap(t *testing.T) {
	data, err := yaml.Load("module:\n  name: test\n  type: rho\n")
	if err != nil {
		t.Fatal(err)
	}

	sc := Schema{
		Name: "test",
		Fields: []FieldSchema{
			{
				Name: "module",
				Required: true,
				Kind: KindMap,
				Fields: []FieldSchema{
					{Name: "name", Required: true, Kind: KindString},
					{Name: "type", Required: true, Kind: KindString, AllowedValues: []string{"rho", "plugin"}},
				},
			},
		},
	}

	result := Validate(data, sc)
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: ValidationErrors Methods                                           */
/* -------------------------------------------------------------------------- */

func TestValidationErrorsFilterByPath(t *testing.T) {
	errs := ValidationErrors{
		{Path: "module.name", Message: "missing", Severity: SeverityError},
		{Path: "module.type", Message: "missing", Severity: SeverityError},
		{Path: "main", Message: "missing", Severity: SeverityError},
	}

	filtered := errs.FilterByPath("module")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered results, got %d", len(filtered))
	}
}

func TestValidationErrorsErrorString(t *testing.T) {
	errs := ValidationErrors{
		{Path: "module.name", Message: "missing", Severity: SeverityError},
		{Path: "module.command", Message: "warning", Severity: SeverityWarning},
	}

	s := errs.ErrorString()
	if s == "" {
		t.Fatal("expected error string to be non-empty")
	}
	// Should only contain errors, not warnings
	if !strings.Contains(s, "module.name") {
		t.Fatalf("expected 'module.name' in output: %s", s)
	}
}

