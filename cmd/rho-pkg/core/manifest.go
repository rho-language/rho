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
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/modulecontract"
	"rho/src/plugins"
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

// PackageSpec is the manifest's `package:` section: what the archive holds.
type PackageSpec struct {
	// Kind overrides which default layout applies.
	Kind string
	// Binary says whether to compile a command into the archive.
	Binary bool
	// HasBinary distinguishes an explicit false from an absent value.
	HasBinary bool
	// ArtifactDir is where intermediate build files are written.
	ArtifactDir string
	// Include lists the paths that go into the archive.
	Include []string
	// Interface lists the Rho declarations a caller compiles against, the way
	// a C header declares what a library offers. A library package is the
	// manifest, the documentation and these files, and nothing else.
	Interface []string
	// Require lists the paths that must exist.
	Require []string
	// KeepSources lists the included directories whose Go files travel in the
	// package. Go sources are left out by default, because a command is shipped
	// prebuilt; a directory that is Go *as data* — the ports a platform plugin
	// hands to the projects it serves — is the exception, and says so here.
	KeepSources []string
}

type ModuleYAML struct {
	Module ModuleSection
	Main   string
	// Package describes what a published archive holds.
	Package  PackageSpec
	Deps     map[string]string
	Features []string
}

/* -------------------------------------------------------------------------- */
/*  Load Manifest                                                              */
/* -------------------------------------------------------------------------- */

// readPackageSpec reads the `package:` section, if the manifest has one.
func readPackageSpec(root yaml.Value) PackageSpec {
	var spec PackageSpec
	section, ok := root.Map["package"]
	if !ok || section.Kind != yaml.KindMap {
		return spec
	}
	spec.Kind = section.Map["kind"].Str
	if value, present := section.Map["binary"]; present {
		spec.Binary = isTrueWord(value.Str)
		spec.HasBinary = true
	}
	spec.ArtifactDir = section.Map["artifact_dir"].Str
	for _, item := range section.Map["include"].List {
		if item.Str != "" {
			spec.Include = append(spec.Include, item.Str)
		}
	}
	for _, item := range section.Map["interface"].List {
		if item.Str != "" {
			spec.Interface = append(spec.Interface, item.Str)
		}
	}
	for _, item := range section.Map["require"].List {
		if item.Str != "" {
			spec.Require = append(spec.Require, item.Str)
		}
	}
	for _, item := range section.Map["keep_sources"].List {
		if item.Str != "" {
			spec.KeepSources = append(spec.KeepSources, item.Str)
		}
	}
	return spec
}

// isTrueWord reads a boolean written as a word, since the manifest loader
// keeps map values as strings.
func isTrueWord(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1", "on":
		return true
	default:
		return false
	}
}

// Layout returns the package layout for this manifest, with the defaults for
// the module type and any `package:` overrides applied.
func (m ModuleYAML) Layout() PackageLayout {
	moduleType := m.Module.Type
	// A Rho module whose entry is Rho source is an application: what it ships is
	// a program, not a command written in Go.
	if moduleType == "rho" && strings.HasSuffix(m.Main, ".rho") {
		moduleType = "application"
	}
	layout := LayoutFor(moduleType).WithOverrides(
		m.Package.Kind, m.Package.Binary, m.Package.HasBinary,
		m.Package.ArtifactDir, m.Package.Include, m.Package.Require)
	layout.KeepSources = m.Package.KeepSources

	// A package with a declared interface is a library: it carries the header
	// and nothing else. The implementation is supplied by a port, so shipping
	// host sources would only invite someone to edit them.
	if len(m.Package.Interface) > 0 && !layout.Binary {
		layout.Interface = m.Package.Interface
		layout.Include = append([]string{"module.yml", "README.md"}, m.Package.Interface...)
		layout.Require = append([]string{"module.yml"}, m.Package.Interface...)
	}
	return layout
}

// LoadModuleYAML reads a manifest for a caller that only needs to know what a
// package says. A type the toolchain does not define is accepted when its
// ecosystem contract is unavailable: the package was validated when it was
// published, and installing it is not the authority on another ecosystem's
// words.
func LoadModuleYAML(path string) (ModuleYAML, error) {
	return loadModuleYAML(path, false)
}

// LoadModuleYAMLStrict reads a manifest and refuses a type whose ecosystem
// contract cannot be loaded. Publishing uses it, because the publisher is the
// gate that decides a module is valid.
func LoadModuleYAMLStrict(path string) (ModuleYAML, error) {
	return loadModuleYAML(path, true)
}

func loadModuleYAML(path string, strict bool) (ModuleYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("cannot read module.yml: %w", err)
	}
	return parseModuleYAML(string(data), filepath.Dir(path), strict)
}

// ParseModuleYAML reads a manifest that is already in memory, which is what a
// package carries and what a caller reading one has rather than a file.
func ParseModuleYAML(text string) (ModuleYAML, error) {
	return parseModuleYAML(text, "", false)
}

func parseModuleYAML(text, dir string, strict bool) (ModuleYAML, error) {
	root, err := yaml.Load(text)
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
		Package:  readPackageSpec(root),
		Features: []string{},
	}

	// Load features
	if f, ok := root.Map["features"]; ok {
		for _, item := range f.List {
			m.Features = append(m.Features, item.Str)
		}
	}

	// Schema validation. The toolchain's own types are always known; an
	// ecosystem's type is checked against the contract its platform plugin
	// publishes.
	moduleType := modMap["type"].Str
	var contract *schema.ModuleContract
	diagnostics := []plugins.Diagnostic(nil)
	if dir != "" {
		contract, diagnostics = modulecontract.Load(dir)
	}
	var result schema.ValidationErrors
	if !schema.IsCoreModuleType(moduleType) && !contract.Allows(moduleType) {
		if strict {
			message := fmt.Sprintf(
				"module.type %q is not one this toolchain defines and no module contract for it was found; install the plugin that serves the module's platform",
				moduleType)
			if len(diagnostics) > 0 {
				message += " (" + diagnostics[0].Message + ")"
			}
			return ModuleYAML{}, fmt.Errorf("%s", message)
		}
		result = schema.ValidateModuleManifestPackage(root)
	} else {
		result = schema.ValidateModuleManifestWithContract(root, moduleType, contract)
	}
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
	// A native module declares no main: it is linked into a project rather than
	// run, so there is no entry point to require.
	if m.Main == "" && schema.IsRhoSourceType(m.Module.Type) {
		return errors.New("main is required")
	}
	return nil
}
