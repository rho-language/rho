//
// @file module_manifest.go
// @brief Schema definitions for Rho module.yml manifests.
//
// Provides type-aware schema validation for module.yml files.
// Different module types have different valid fields:
//
//   - rho/app:     main is required (source file path)
//   - module:      main is required (source file path), command not allowed
//   - component:   main is required (binary name), command optional
//   - plugin:      main is required (source file), hooks.yml validated separately
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
	"strings"

	"rho/src/yaml"
)

// ValidModuleTypes lists all allowed module.type values.
var ValidModuleTypes = []string{"rho", "module", "component", "plugin"}

// ModuleSchema returns a schema for validating module.yml content.
// It returns a base schema that requires module.name, module.type,
// module.version, and main at minimum.
func ModuleSchema() Schema {
	return Schema{
		Name: "module.yml",
		Fields: []FieldSchema{
			{
				Name:     "module",
				Required: true,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "name", Required: true, RejectNull: true, Kind: KindString},
					{Name: "type", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidModuleTypes},
					{Name: "version", Required: true, RejectNull: true, Kind: KindString},
					{Name: "command", Required: false, Kind: KindString},
				},
			},
			{
				Name:      "main",
				Required:  true,
				RejectNull: true,
				Kind:      KindString,
			},
			{
				Name:     "deps",
				Required: false,
				Kind:     KindMap,
			},
			{
				Name:     "features",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
			{
				Name:     "depends",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindMap,
					Fields: []FieldSchema{
						{Name: "name", Required: true, RejectNull: true, Kind: KindString},
						// type is optional: the resolver infers "c" or "rho"
						// from the dependency's own module.yml when absent.
						{Name: "type", Required: false, Kind: KindString, AllowedValues: []string{"c", "rho"}},
						{Name: "git", Required: false, Kind: KindString},
						{Name: "tag", Required: false, Kind: KindString},
						{Name: "branch", Required: false, Kind: KindString},
						{Name: "version", Required: false, Kind: KindString},
						{Name: "path", Required: false, Kind: KindString},
						{Name: "env", Required: false, Kind: KindString},
						{Name: "include", Required: false, Kind: KindString},
						{Name: "src", Required: false, Kind: KindString},
					},
				},
			},
			{
				Name:     "build",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "language", Required: true, RejectNull: true, Kind: KindString, AllowedValues: []string{"c", "go", "cgo"}},
					{Name: "include_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "source_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "headers", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
		},
	}
}

// ValidateModuleManifest performs type-aware validation of a module.yml.
// It validates the base schema and adds type-specific rules:
//   - component: 'command' is recommended for dynamic dispatch
//   - plugin: 'hooks.yml' should exist with at least one hook
//   - rho/module: 'main' should point to a .rho source file
//   - depends: each entry must specify 'git' or 'path'
func ValidateModuleManifest(data yaml.Value, moduleType string) ValidationErrors {
	result := Validate(data, ModuleSchema())
	errs := ValidationErrors(result.Errors)

	// Type-specific validations
	switch moduleType {
	case "component":
		modMap, ok := data.Map["module"]
		if ok && modMap.Kind == yaml.KindMap {
			if _, hasCmd := modMap.Map["command"]; !hasCmd {
				errs = append(errs, ValidationError{
					Path:      "module.command",
					Message:   "component modules should specify a 'command' for dynamic dispatch",
					Severity:  SeverityWarning,
				})
			}
		}

	case "plugin":
		modMap, ok := data.Map["module"]
		if ok && modMap.Kind == yaml.KindMap {
			if cmd, exists := modMap.Map["command"]; exists && cmd.Str != "" {
				errs = append(errs, ValidationError{
					Path:      "module.command",
					Message:   "plugin modules should not specify 'command' (use hooks.yml instead)",
					Severity:  SeverityError,
				})
			}
		}

	case "rho", "module":
		mainVal, hasMain := data.Map["main"]
		if hasMain && mainVal.Kind == yaml.KindString {
			if !strings.HasSuffix(mainVal.Str, ".rho") {
				errs = append(errs, ValidationError{
					Path:     "main",
					Message:  fmt.Sprintf("main for type '%s' should reference a .rho source file, got '%s'", moduleType, mainVal.Str),
					Severity: SeverityError,
				})
			}
		}
	}

	// Validate depends entries: each must have at least 'git' or 'path'
	if dependsNode, ok := data.Map["depends"]; ok && dependsNode.Kind == yaml.KindList {
		for i, item := range dependsNode.List {
			if item.Kind != yaml.KindMap {
				continue
			}
			hasGit, _ := item.Map["git"]
			hasPath, _ := item.Map["path"]
			if hasGit.Str == "" && hasPath.Str == "" {
				errs = append(errs, ValidationError{
					Path:      fmt.Sprintf("depends[%d]", i),
					Message:   "dependency must specify either 'git' or 'path'",
					Severity:  SeverityError,
				})
			}
			// tag and branch are mutually exclusive
			if item.Map["tag"].Str != "" && item.Map["branch"].Str != "" {
				errs = append(errs, ValidationError{
					Path:      fmt.Sprintf("depends[%d]", i),
					Message:   "dependency cannot specify both 'tag' and 'branch'",
					Severity:  SeverityError,
				})
			}
		}
	}

	return errs
}

// ValidateModuleYAMLFile reads and validates a module.yml file.
func ValidateModuleYAMLFile(path string) (ValidationErrors, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	moduleType := ""
	if modNode, ok := root.Map["module"]; ok {
		moduleType = modNode.Map["type"].Str
	}

	return ValidateModuleManifest(root, moduleType), nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
