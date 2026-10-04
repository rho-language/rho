//
// @file schema.go
// @brief Rho YAML Schema Validation Framework.
//
// Provides a lightweight, deterministic schema validator for Rho YAML files.
// Works with the minimal yaml.Value model from `rho/src/yaml`.
//
// Supports:
//   - required/optional fields
//   - type checking (string, map, list)
//   - allowed values (enumerations)
//   - nested field validation
//   - list item validation
//   - error/warning severity
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package schema

import (
	"fmt"
	"strings"

	"rho/src/yaml"
)

// Severity classifies a validation issue.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// ValidationError represents a single schema violation.
type ValidationError struct {
	Path     string
	Message  string
	Severity Severity
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Severity, e.Path, e.Message)
}

// ErrorOrWarning reports whether the issue is an error.
func (e ValidationError) IsError() bool {
	return e.Severity == SeverityError
}

// ValidationErrors is a collection of validation issues.
type ValidationErrors []ValidationError

// HasErrors returns true if any issue in the collection is an error.
func (ve ValidationErrors) HasErrors() bool {
	for _, e := range ve {
		if e.IsError() {
			return true
		}
	}
	return false
}

// Errors returns only the error-level issues.
func (ve ValidationErrors) Errors() []ValidationError {
	out := []ValidationError{}
	for _, e := range ve {
		if e.IsError() {
			out = append(out, e)
		}
	}
	return out
}

// FilterByPath returns issues whose Path starts with the given prefix.
func (ve ValidationErrors) FilterByPath(prefix string) ValidationErrors {
	out := ValidationErrors{}
	for _, e := range ve {
		if strings.HasPrefix(e.Path, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// String formats all validation issues.
func (ve ValidationErrors) String() string {
	if len(ve) == 0 {
		return "ok"
	}
	var b strings.Builder
	for _, e := range ve {
		b.WriteString(e.Error())
		b.WriteString("\n")
	}
	return b.String()
}

// ErrorString formats only error-level issues.
func (ve ValidationErrors) ErrorString() string {
	errs := ve.Errors()
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(fmt.Sprintf("  %s: %s\n", e.Path, e.Message))
	}
	return b.String()
}

// Kind constants for value type matching
const (
	KindString = "string"
	KindMap    = "map"
	KindList   = "list"
	// KindBool is a flag rather than a value. It exists so that a field which is
	// a decision can say so, and a manifest that writes a word where a decision
	// belongs is told rather than read as one.
	KindBool = "bool"
)

// FieldSchema describes a single field in a YAML document.
type FieldSchema struct {
	// Name is the YAML key.
	Name string

	// Required when true, the field must be present.
	Required bool

	// RejectNull when true, the field must carry a value. A key declared
	// with no content (for example `hooks:`) parses as null and fails.
	RejectNull bool

	// Kind specifies the expected YAML value kind: "string", "map", or "list".
	// Empty string means any kind is allowed.
	Kind string

	// AllowedValues restricts a string field to a set of values.
	AllowedValues []string

	// Fields for nested maps: the sub-field schemas.
	Fields []FieldSchema

	// ItemSchema for lists: validates each item. Only applicable when Kind is "list".
	ItemSchema *ListItemSchema
}

// ListItemSchema describes the expected structure of list items.
type ListItemSchema struct {
	// Kind of each item: "string" or "map".
	Kind string

	// Fields for map items.
	Fields []FieldSchema
}

// Schema is the top-level schema definition.
type Schema struct {
	// Name is a human-readable identifier for the schema.
	Name string

	// Fields describes the expected top-level fields.
	Fields []FieldSchema
}

// ValidateResult holds the outcome of validating a YAML value against a schema.
type ValidateResult struct {
	Errors []ValidationError
}

// HasErrors returns true if any validation error was found.
func (r ValidateResult) HasErrors() bool {
	for _, e := range r.Errors {
		if e.IsError() {
			return true
		}
	}
	return false
}

// ErrorString formats only error-level issues.
func (r ValidateResult) ErrorString() string {
	return ValidationErrors(r.Errors).ErrorString()
}

// String formats all validation issues.
func (r ValidateResult) String() string {
	return ValidationErrors(r.Errors).String()
}

// Validate validates a yaml.Value against the given schema.
func Validate(data yaml.Value, schema Schema) ValidateResult {
	result := ValidateResult{}
	validateNode(data, schema, "", &result)
	return result
}

func validateNode(data yaml.Value, schema Schema, path string, result *ValidateResult) {
	for _, field := range schema.Fields {
		fieldPath := field.Name
		if path != "" {
			fieldPath = path + "." + field.Name
		}

		value, exists := data.Map[field.Name]

		if !exists {
			if field.Required {
				result.Errors = append(result.Errors, ValidationError{
					Path:     fieldPath,
					Message:  fmt.Sprintf("required field '%s' is missing", field.Name),
					Severity: SeverityError,
				})
			}
			continue
		}

		validateValue(value, field, fieldPath, result)
	}
}

func validateValue(val yaml.Value, field FieldSchema, path string, result *ValidateResult) {
	// A key with no content parses as null. It satisfies an optional field
	// (an omitted list is an empty list) but fails when the field requires
	// an actual value.
	if val.Kind == yaml.KindNull {
		if field.RejectNull {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path,
				Message:  fmt.Sprintf("field '%s' is declared but has no value", field.Name),
				Severity: SeverityError,
			})
		}
		return
	}

	// Check kind
	if field.Kind != "" && field.Kind != KindBool {
		// A boolean is checked separately, below: the loader keeps `true` and
		// `false` as text, so a strict kind comparison would reject every
		// well-formed flag in a manifest.
		actualKind := valueKind(val)
		if actualKind != field.Kind {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path,
				Message:  fmt.Sprintf("expected %s, got %s", field.Kind, actualKind),
				Severity: SeverityError,
			})
			return
		}
	}

	if field.Kind == KindBool && val.Kind == yaml.KindString {
		if val.Str != "true" && val.Str != "false" {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path,
				Message:  fmt.Sprintf("expected true or false, got '%s'", val.Str),
				Severity: SeverityError,
			})
		}
	}

	// Check allowed values (for strings)
	if len(field.AllowedValues) > 0 {
		if val.Kind == yaml.KindString {
			if !containsString(field.AllowedValues, val.Str) {
				result.Errors = append(result.Errors, ValidationError{
					Path:     path,
					Message:  fmt.Sprintf("value '%s' is not allowed; expected one of: %s", val.Str, strings.Join(field.AllowedValues, ", ")),
					Severity: SeverityError,
				})
			}
		}
	}

	// Validate nested fields for maps
	if val.Kind == yaml.KindMap && len(field.Fields) > 0 {
		validateNode(val, Schema{Fields: field.Fields}, path, result)
	}

	// Validate list items
	if val.Kind == yaml.KindList && field.ItemSchema != nil {
		for i, item := range val.List {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if field.ItemSchema.Kind == KindMap && len(field.ItemSchema.Fields) > 0 {
				if item.Kind != yaml.KindMap {
					result.Errors = append(result.Errors, ValidationError{
						Path:     itemPath,
						Message:  fmt.Sprintf("expected map, got %s", valueKind(item)),
						Severity: SeverityError,
					})
					continue
				}
				subSchema := Schema{Fields: field.ItemSchema.Fields}
				validateNode(item, subSchema, itemPath, result)
			} else if field.ItemSchema.Kind == KindString {
				if item.Kind != yaml.KindString {
					result.Errors = append(result.Errors, ValidationError{
						Path:     itemPath,
						Message:  fmt.Sprintf("expected string, got %s", valueKind(item)),
						Severity: SeverityError,
					})
				}
			}
		}
	}
}

func valueKind(v yaml.Value) string {
	switch v.Kind {
	case yaml.KindString:
		return KindString
	case yaml.KindMap:
		return KindMap
	case yaml.KindList:
		return KindList
	default:
		return "null"
	}
}

func containsString(list []string, val string) bool {
	for _, s := range list {
		if s == val {
			return true
		}
	}
	return false
}

// ValidateAndReport validates data against schema and returns an error
// if any errors (not warnings) were found.
func ValidateAndReport(data yaml.Value, schema Schema) error {
	result := Validate(data, schema)
	if result.HasErrors() {
		return fmt.Errorf("schema validation failed for '%s':\n%s",
			schema.Name, formatErrors(result.Errors))
	}
	return nil
}

func formatErrors(errs []ValidationError) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(fmt.Sprintf("  %s: %s\n", e.Path, e.Message))
	}
	return b.String()
}
