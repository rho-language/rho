//
// @file hooks.go
// @brief Schema definition for plugin/hooks.yml.
//
// Validates the structure of hooks.yml files used by plugin-type modules.
//
// Structure:
//
//   hooks:
//     - init
//     - configure
//     - shutdown
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"fmt"
	"os"

	"rho/src/yaml"
)

// HooksSchema returns the schema for plugin/hooks.yml.
func HooksSchema() Schema {
	return Schema{
		Name: "hooks.yml",
		Fields: []FieldSchema{
			{
				Name:      "hooks",
				Required:  true,
				RejectNull: true,
				Kind:      KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
		},
	}
}

// ValidateHooksFile reads and validates a hooks.yml file.
func ValidateHooksFile(path string) (ValidationErrors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	result := Validate(root, HooksSchema())
	errs := ValidationErrors(result.Errors)

	// Additional checks: hook names must be non-empty
	if hooksNode, ok := root.Map["hooks"]; ok && hooksNode.Kind == yaml.KindList {
		for i, h := range hooksNode.List {
			if h.Kind == yaml.KindString && h.Str == "" {
				errs = append(errs, ValidationError{
					Path:     fmt.Sprintf("hooks[%d]", i),
					Message:  "hook name is empty",
					Severity: SeverityError,
				})
			}
		}
	}

	return errs, nil
}
