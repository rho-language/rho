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
var ValidModuleTypes = []string{"rho", "module", "component", "plugin", "interfaces"}

// RhoSourceModuleTypes are the module types written in Rho. They declare a
// main entry point, unlike `interfaces` modules which are native libraries
// consumed through the build section.
var RhoSourceModuleTypes = []string{"rho", "module", "component", "plugin"}

// IsRhoSourceType reports whether a module type is written in Rho.
func IsRhoSourceType(moduleType string) bool {
	for _, candidate := range RhoSourceModuleTypes {
		if candidate == moduleType {
			return true
		}
	}
	return false
}

// ValidBuildLanguages lists the source languages a build section may declare.
var ValidBuildLanguages = []string{"c", "go", "cgo"}

// ValidBuildTargets lists the generated-project backends an application may
// request. The set is intentionally open-ended: a descriptor may define new
// targets, and validation of unknown ids happens against that descriptor.
var ValidBuildTargets = []string{"c", "go"}

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
					// import_path is the dotted name Rho code uses to reach this
					// component's Rho sources, which cannot be guessed from the
					// module name: a component published as ext-probe may expose
					// its sources as ext.probe.
					{Name: "import_path", Required: false, Kind: KindString},
					// Descriptive metadata carried by published modules.
					{Name: "description", Required: false, Kind: KindString},
					{Name: "maintainer", Required: false, Kind: KindString},
					{Name: "license", Required: false, Kind: KindString},
					{Name: "created", Required: false, Kind: KindString},
				},
			},
			{
				Name:     "main",
				Required: false,
				Kind:     KindString,
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
					{Name: "language", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidBuildLanguages},
					// targets lists the generated-project backends to emit for
					// this application (for example c and go).
					{Name: "targets", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "include_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "source_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "headers", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				// The package section describes what goes into a published
				// archive. Defaults come from the module type, so a component,
				// a plugin and an application can differ without the packager
				// carrying a hardcoded list per kind.
				Name:     "package",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "kind", Required: false, Kind: KindString, AllowedValues: []string{"component", "plugin", "application", "library"}},
					{Name: "binary", Required: false, Kind: KindString},
					{Name: "artifact_dir", Required: false, Kind: KindString},
					{Name: "include", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// interface lists the Rho declarations a caller compiles
					// against: the header, in C terms. The implementation is
					// supplied by a port, so a library package carries no host
					// source and nothing a user could edit.
					{Name: "interface", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "require", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "exports",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "headers", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "dependencies",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "required", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "optional", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "config",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "kconfig_fragments", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "tests",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "enabled", Required: false, Kind: KindString},
					{Name: "test_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "test_files", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "metadata",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "tags", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "stability", Required: false, Kind: KindString},
					{Name: "visibility", Required: false, Kind: KindString},
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
					Path:     "module.command",
					Message:  "component modules should specify a 'command' for dynamic dispatch",
					Severity: SeverityWarning,
				})
			}
		}

	case "plugin":
		modMap, ok := data.Map["module"]
		if ok && modMap.Kind == yaml.KindMap {
			if cmd, exists := modMap.Map["command"]; exists && cmd.Str != "" {
				errs = append(errs, ValidationError{
					Path:     "module.command",
					Message:  "plugin modules should not specify 'command' (use hooks.yml instead)",
					Severity: SeverityError,
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

	// A Rho-source module must declare an entry point. Native library
	// modules (for example type `interfaces`) legitimately have none.
	if IsRhoSourceType(moduleType) {
		mainVal, hasMain := data.Map["main"]
		if !hasMain {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  fmt.Sprintf("module type '%s' must declare a main entry point", moduleType),
				Severity: SeverityError,
			})
		} else if mainVal.Kind == yaml.KindNull || mainVal.Str == "" {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  "main is declared but has no value",
				Severity: SeverityError,
			})
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
					Path:     fmt.Sprintf("depends[%d]", i),
					Message:  "dependency must specify either 'git' or 'path'",
					Severity: SeverityError,
				})
			}
			// tag and branch are mutually exclusive
			if item.Map["tag"].Str != "" && item.Map["branch"].Str != "" {
				errs = append(errs, ValidationError{
					Path:     fmt.Sprintf("depends[%d]", i),
					Message:  "dependency cannot specify both 'tag' and 'branch'",
					Severity: SeverityError,
				})
			}
		}
	}

	// When targets are declared, the declared language should be covered by
	// one of them. A project generating only a Go server while declaring
	// language: c is misleading, but a mixed C and Go build is legitimate, so
	// this is a warning rather than a structural error.
	if build, ok := data.Map["build"]; ok && build.Kind == yaml.KindMap {
		language := build.Map["language"].Str
		targetsNode, hasTargets := build.Map["targets"]
		if language != "" && hasTargets && targetsNode.Kind == yaml.KindList {
			if len(targetsNode.List) > 0 && !targetCoversLanguage(targetsNode, language) {
				errs = append(errs, ValidationError{
					Path:     "build.language",
					Message:  fmt.Sprintf("language %q is not covered by build.targets", language),
					Severity: SeverityWarning,
				})
			}
		}
	}

	return errs
}

// targetCoversLanguage reports whether any declared target builds the given
// language. A target id either names the language outright or is derived from
// it, as "go-server" is derived from "go".
func targetCoversLanguage(targets yaml.Value, language string) bool {
	for _, target := range targets.List {
		id := target.Str
		if id == language ||
			strings.HasPrefix(id, language+"-") ||
			strings.HasPrefix(id, language+"_") {
			return true
		}
	}
	return false
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
