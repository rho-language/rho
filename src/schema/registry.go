//
// @file registry.go
// @brief Schema definition for registry.yaml.
//
// Validates the structure of the local module registry file.
//
// Structure:
//
//   components:
//     - name: math
//       version: 1.2.0
//   plugins:
//     - name: http
//       version: 0.9.0
//   active_components:
//     - name: math
//       version: 1.2.0
//   active_plugins:
//     - name: http
//       version: 0.9.0
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
	"os"

	"rho/src/yaml"
)

// entryFields are the shared fields for registry list items.
var entryFields = []FieldSchema{
	{Name: "name", Required: true, Kind: KindString},
	{Name: "version", Required: true, Kind: KindString},
}

// RegistrySchema returns the schema for registry.yaml.
func RegistrySchema() Schema {
	return Schema{
		Name: "registry.yaml",
		Fields: []FieldSchema{
			{
				Name:     "components",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind:   KindMap,
					Fields: entryFields,
				},
			},
			{
				Name:     "plugins",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind:   KindMap,
					Fields: entryFields,
				},
			},
			{
				Name:     "active_components",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind:   KindMap,
					Fields: entryFields,
				},
			},
			{
				Name:     "active_plugins",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind:   KindMap,
					Fields: entryFields,
				},
			},
		},
	}
}

// ValidateRegistryFile reads and validates a registry.yaml file.
func ValidateRegistryFile(path string) (ValidationErrors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	result := Validate(root, RegistrySchema())
	return ValidationErrors(result.Errors), nil
}
