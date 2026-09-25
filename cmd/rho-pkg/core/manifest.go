//
// @file manifest.go
// @brief Rho Package Manifest - Enterprise v3.
//
// Handles loading and validating `module.yml`, the official manifest
// format for Rho projects. Used by `rho pkg pack`, `rho pkg inspect`,
// and `rho pkg unpack`.
//
// Structure:
//
// module:
//   name: hello-rho
//   type: rho | module | plugin
//   version: 0.1.0
//
// main: src/main.rho
//
// features:
//   - math
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"errors"
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
}

type ModuleYAML struct {
    Module   ModuleSection
    Main     string
    Deps     map[string]string
    Features []string
}

/* -------------------------------------------------------------------------- */
/*  Load Manifest                                                              */
/* -------------------------------------------------------------------------- */

func LoadModuleYAML(path string) (ModuleYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("cannot read module.yml: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("invalid YAML: %w", err)
	}

	modMap := root.Map["module"].Map

	m := ModuleYAML{
		Module: ModuleSection{
			Name:    modMap["name"].Str,
			Type:    modMap["type"].Str,
			Version: modMap["version"].Str,
			Command: modMap["command"].Str,
		},
		Main:     root.Map["main"].Str,
		Features: []string{},
	}

	// Load features
	if f, ok := root.Map["features"]; ok {
		for _, item := range f.List {
			m.Features = append(m.Features, item.Str)
		}
	}

	// Schema validation
	moduleType := modMap["type"].Str
	result := schema.ValidateModuleManifest(root, moduleType)
	if result.HasErrors() {
		return ModuleYAML{}, fmt.Errorf("schema validation failed:\n%s", result.ErrorString())
	}

	// Validate manifest
	if err := ValidateModuleYAML(m); err != nil {
		return ModuleYAML{}, err
	}

	return m, nil
}

/* -------------------------------------------------------------------------- */
/*  Save Manifest                                                              */
/* -------------------------------------------------------------------------- */

func SaveModuleYAML(path string, m ModuleYAML) error {
    out := "module:\n"
    out += "  name: " + m.Module.Name + "\n"
    out += "  type: " + m.Module.Type + "\n"
    out += "  version: " + m.Module.Version + "\n"
    if m.Module.Command != "" {
        out += "  command: " + m.Module.Command + "\n"
    }
    out += "\n"

    out += "main: " + m.Main + "\n\n"

    if len(m.Features) > 0 {
        out += "features:\n"
        for _, f := range m.Features {
            out += "  - " + f + "\n"
        }
    }

    return os.WriteFile(path, []byte(out), 0644)
}

/* -------------------------------------------------------------------------- */
/*  Validation                                                                 */
/* -------------------------------------------------------------------------- */

func ValidateModuleYAML(m ModuleYAML) error {
	if m.Module.Name == "" {
		return errors.New("module.name is required")
	}
	if m.Module.Type == "" {
		return errors.New("module.type is required")
	}
	if m.Module.Version == "" {
		return errors.New("module.version is required")
	}
	if m.Main == "" {
		return errors.New("main is required")
	}
	return nil
}
