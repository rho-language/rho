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
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/src/modulecontract"
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
	// Author is who wrote the module, and Maintainer who looks after it. Both
	// are the module's own claim about itself: a remote registry records the
	// owner of a name from the credential that published it first, which is the
	// one identity it can actually verify, and keeps this for a reader.
	Author     string
	Maintainer string
	// Visibility is whether the module is published for anybody or for its
	// owner alone. An empty value means public, which is what a module that says
	// nothing is taken to be.
	Visibility string
}

type ModuleYAML struct {
	Module   ModuleSection
	Main     string
	Deps     map[string]string
	Features []string
	// SQL is what the project declared about its database, and what the
	// generator is driven by. Declared is not the same as enabled: a project
	// without one has no SQL, and a project with one has generated Rho it did
	// not write.
	SQL SQLConfig
	// Requires are the packages a built project links against, which is a
	// different thing from Deps: a dependency is resolved into the project and a
	// requirement is a package the generated code calls into.
	Requires []Requirement
}

// SQLConfig is a project's declaration that it has a database.
//
// Everything here is a location or a name, and none of it is a setting: the
// driver decides what its own settings are, so a project does not have to know
// that a database is a file on one engine and a connection string on another.
type SQLConfig struct {
	// Driver is the engine plugin the generated code runs against.
	Driver string
	// Schema and Queries are the files the SQL is written in, relative to the
	// project.
	Schema  string
	Queries string
	// Out is where the generated Rho is written, relative to the project. It
	// defaults to the queries file's directory, so a project that keeps its SQL
	// in one place gets its generated code beside it.
	Out string
	// Package is the Rho module the generated file belongs to, defaulting to
	// the project's own name.
	Package string
}

// Declared reports whether a project has said it has a database.
func (c SQLConfig) Declared() bool { return c.Driver != "" || c.Queries != "" }

// Requirement is a package a built project needs at run time.
type Requirement struct {
	Name    string
	Version string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// LoadModuleYAML reads a manifest for a caller that only needs to know what it
// says. A type the toolchain does not define is accepted when its ecosystem's
// contract is unavailable, because an installed module has to remain readable
// on a machine that has not installed the plugin it was built with.
func LoadModuleYAML(path string) (ModuleYAML, error) {
	return loadModuleYAML(path, false)
}

// LoadModuleYAMLStrict reads a manifest and refuses a type whose ecosystem
// contract cannot be loaded. It is what validation and publishing use: the
// contract is the authority on an ecosystem's types, and a machine that cannot
// find it cannot say the module is valid.
func LoadModuleYAMLStrict(path string) (ModuleYAML, error) {
	return loadModuleYAML(path, true)
}

func loadModuleYAML(path string, strict bool) (ModuleYAML, error) {
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
			Author:      root.Map["module"].Map["author"].Str,
			Maintainer:  root.Map["module"].Map["maintainer"].Str,
			Visibility:  root.Map["module"].Map["visibility"].Str,
			// main sits at the top level of the manifest, beside the module
			// section, so it is read from there.
			Main:    root.Map["main"].Str,
			License: root.Map["module"].Map["license"].Str,
		},
		Main:     root.Map["main"].Str,
		Deps:     map[string]string{},
		Features: []string{},
		SQL:      SQLConfig{},
		Requires: []Requirement{},
	}

	// The sql section, which is what a project writes to have its SQL turned
	// into Rho code. A project without one declares no SQL, which is a fact
	// rather than a failure: most modules are libraries.
	if sqlNode, ok := root.Map["sql"]; ok && sqlNode.Kind == yaml.KindMap {
		mod.SQL = SQLConfig{
			Driver:  sqlNode.Map["driver"].Str,
			Schema:  sqlNode.Map["schema"].Str,
			Queries: sqlNode.Map["queries"].Str,
			Out:     sqlNode.Map["out"].Str,
			Package: sqlNode.Map["package"].Str,
		}
	}

	// requires, the packages a built project links against.
	for _, entry := range root.Map["requires"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		mod.Requires = append(mod.Requires, Requirement{
			Name:    entry.Map["name"].Str,
			Version: entry.Map["version"].Str,
		})
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

	// Schema validation. The toolchain's own types are always known; an
	// ecosystem's type is checked against the contract its platform plugin
	// publishes.
	contract, diagnostics := modulecontract.Load(filepath.Dir(path))
	if !schema.IsCoreModuleType(mod.Module.Type) && !contract.Allows(mod.Module.Type) {
		if strict {
			message := fmt.Sprintf(
				"module.type %q is not one this toolchain defines and no module contract for it was found; install the plugin that serves the module's platform",
				mod.Module.Type)
			if len(diagnostics) > 0 {
				message += " (" + diagnostics[0].Message + ")"
			}
			return ModuleYAML{}, fmt.Errorf("%s", message)
		}
		schemaResult := schema.ValidateModuleManifestPackage(root)
		if schemaResult.HasErrors() {
			return ModuleYAML{}, fmt.Errorf("schema validation failed:\n%s", schemaResult.ErrorString())
		}
	} else {
		schemaResult := schema.ValidateModuleManifestWithContract(root, mod.Module.Type, contract)
		if schemaResult.HasErrors() {
			return ModuleYAML{}, fmt.Errorf("schema validation failed:\n%s", schemaResult.ErrorString())
		}
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
	// A module written in Rho is entered through a main, so one is required.
	// A native module is not: it is compiled and linked into a project rather
	// than run, and requiring an entry point of it would mean naming a file
	// that is not one. The schema has said so since `interfaces` was added, and
	// the requirement here said otherwise — so a perfectly good native library
	// was refused with "main missing", and the only way to publish one was to
	// name a build file as its entry point.
	if mod.Main == "" && schema.IsRhoSourceType(mod.Module.Type) {
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
