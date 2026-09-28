//
// @file manifest.go
// @brief The manifest facts a service's own metadata needs.
//
// A document about a service says what the service calls itself. That is the
// manifest's own words — its name, its description, its version — read with the
// toolchain's own YAML loader rather than with a parser of this component's
// own, so a manifest this understands is a manifest the build understood.
//
// Only the fields a document needs are read. A manifest is a build
// instruction, and reading it as one would couple this to every key it may
// carry; reading the four that describe the thing is enough, and leaves the
// rest alone.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/yaml"
)

// manifestInfo is what a manifest says about the thing it describes.
type manifestInfo struct {
	// Module is the module's name, which is also its import path's first part.
	Module string
	// Title is what the service calls itself, where the manifest says.
	Title string
	// Description is the manifest's description.
	Description string
	// ModuleVersion is the manifest's version.
	ModuleVersion string
	// Main is the entry point, relative to the project.
	Main string
}

// readManifest reads a project's manifest.
func readManifest(root string) (manifestInfo, error) {
	info := manifestInfo{}
	path := filepath.Join(root, "module.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		return info, fmt.Errorf("read %s: %w", path, err)
	}
	document, err := yaml.Load(string(content))
	if err != nil {
		return info, fmt.Errorf("read %s: %w", path, err)
	}
	section, ok := document.Map["module"]
	if !ok || section.Kind != yaml.KindMap {
		return info, fmt.Errorf("%s has no module: section", path)
	}
	info.Module = section.Map["name"].Str
	info.Description = section.Map["description"].Str
	info.ModuleVersion = section.Map["version"].Str
	info.Main = document.Map["main"].Str
	if docs, present := document.Map["docs"]; present && docs.Kind == yaml.KindMap {
		info.Title = docs.Map["title"].Str
	}
	return info, nil
}

// moduleOf returns the group a source belongs to, which is the directory it sits
// in below the sources root. A route in src/registry/web.rho is a registry
// operation, and a document groups it under registry.
func moduleOf(source string) string {
	directory := filepath.ToSlash(filepath.Dir(source))
	switch directory {
	case ".", "src", "/":
		return ""
	}
	segments := strings.Split(strings.Trim(directory, "/"), "/")
	return segments[len(segments)-1]
}
