//
// @file manifest.go
// @brief Rho Module Manifest Loader - Enterprise v3.
//
// Loads and validates the `module.yml` manifest for Rho modules.
// Responsibilities:
//   - parse YAML
//   - validate required fields
//   - expose Enterprise manifest structures
//   - integrate with validator, installer, resolver, info
//
// Manifest format:
//
// module:
//   name: mymodule
//   type: module | plugin | rho
//   version: 1.0.0
//
// main: src/lib.rho
//
// deps:
//   math: ">=1.0.0 <2.0.0"
//   io: "1.2.0"
//
// features:
//   - async
//   - crypto
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"fmt"
	"os"

	"rho/src/schema"
	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type ModuleSection struct {
	Name    string
	Type    string
	Version string
	Command string
	// Description, Main and License are the descriptive fields a registry
	// shows for a published module.
	Description string
	Main        string
	License     string
}

type ModuleYAML struct {
	Module   ModuleSection
	Main     string
	Deps     map[string]string
	Features []string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func LoadModuleYAML(path string) (ModuleYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("cannot read module.yml: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("invalid YAML in module.yml: %w", err)
	}

	mod := ModuleYAML{
		Module: ModuleSection{
			Name:        root.Map["module"].Map["name"].Str,
			Type:        root.Map["module"].Map["type"].Str,
			Version:     root.Map["module"].Map["version"].Str,
			Command:     root.Map["module"].Map["command"].Str,
			Description: root.Map["module"].Map["description"].Str,
			License:     root.Map["module"].Map["license"].Str,
		},
		Main:     root.Map["main"].Str,
		Deps:     map[string]string{},
		Features: []string{},
	}

	// deps
	if depsNode, ok := root.Map["deps"]; ok {
		for k, v := range depsNode.Map {
			mod.Deps[k] = v.Str
		}
	}

	// Load features
	if featNode, ok := root.Map["features"]; ok {
		for _, f := range featNode.List {
			mod.Features = append(mod.Features, f.Str)
		}
	}

	// Schema validation
	schemaResult := schema.ValidateModuleManifest(root, mod.Module.Type)
	if schemaResult.HasErrors() {
		return ModuleYAML{}, fmt.Errorf("schema validation failed:\n%s", schemaResult.ErrorString())
	}

	// Validate required fields
	if mod.Module.Name == "" {
		return ModuleYAML{}, fmt.Errorf("module.name missing")
	}
	if mod.Module.Type == "" {
		return ModuleYAML{}, fmt.Errorf("module.type missing")
	}
	if mod.Module.Version == "" {
		return ModuleYAML{}, fmt.Errorf("module.version missing")
	}
	if mod.Main == "" {
		return ModuleYAML{}, fmt.Errorf("main missing")
	}

	return mod, nil
}

/* -------------------------------------------------------------------------- */
/*  Save Manifest                                                               */
/* -------------------------------------------------------------------------- */

func SaveModuleYAML(path string, m ModuleYAML) error {
	out := "module:\n"
	out += fmt.Sprintf("  name: %s\n", m.Module.Name)
	out += fmt.Sprintf("  type: %s\n", m.Module.Type)
	out += fmt.Sprintf("  version: %s\n", m.Module.Version)
	if m.Module.Command != "" {
		out += fmt.Sprintf("  command: %s\n", m.Module.Command)
	}
	out += fmt.Sprintf("\nmain: %s\n", m.Main)

	if len(m.Deps) > 0 {
		out += "\ndeps:\n"
		for k, v := range m.Deps {
			out += fmt.Sprintf("  %s: \"%s\"\n", k, v)
		}
	}

	if len(m.Features) > 0 {
		out += "\nfeatures:\n"
		for _, f := range m.Features {
			out += fmt.Sprintf("  - %s\n", f)
		}
	}

	return os.WriteFile(path, []byte(out), 0644)
}
