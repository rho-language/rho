//
// @file modulecontract.go
// @brief Finding the module contract an ecosystem publishes.
//
// The toolchain does not carry an ecosystem's module types. It asks the
// ecosystem's platform plugin for them: the plugin answers the `rho-module`
// component's `contract` hook with the contract a module in the graph carries.
// The toolchain parses the answer and validates against it, without knowing
// which ecosystem the words belong to.
//
// The plugin is the connection point. A project whose platform is recognized
// runs the plugins that serve it, and a project whose platform is not
// recognized loads no contract and is validated against the toolchain's own
// types alone.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package modulecontract

import (
	"encoding/json"
	"path/filepath"

	"rho/src/deps"
	"rho/src/plugins"
	"rho/src/schema"
)

// Component is the toolchain component a module contract is asked of.
//
// It is a component name rather than a program: the answer is data about how a
// manifest is written, so any plugin that serves the project's platform can
// answer it whether or not it also maps a toolchain component.
const Component = "rho-module"

// Hook is the stage that asks for a module contract.
const Hook = "contract"

// Request is what the toolchain sends the plugin: the roots a contract may be
// found from. The toolchain does not say which file to read, because that is
// the ecosystem's convention; it says which modules the project is made of.
type Request struct {
	ProjectRoot  string   `json:"project_root"`
	PlatformRoot string   `json:"platform_root,omitempty"`
	ModuleRoots  []string `json:"module_roots,omitempty"`
}

// Response is the plugin's answer. A contract is returned as the document the
// ecosystem published, so the plugin does not have to know its shape.
type Response struct {
	Contract json.RawMessage `json:"contract,omitempty"`
}

// Load returns the contract the project's ecosystem exposes, or nil when the
// project has no recognized platform or the platform's plugin publishes none.
func Load(projectRoot string) (*schema.ModuleContract, []plugins.Diagnostic) {
	platform := deps.Platform(projectRoot)
	if platform == "" {
		return nil, nil
	}

	source := plugins.Source{
		RhoHome:  deps.RhoHome(),
		Platform: platform,
	}
	found := source.Discover(Component)
	if len(found) == 0 {
		return nil, nil
	}

	// The modules the project is built from, and those its platform is: a board
	// depends on nothing and lives inside its platform, whose own dependencies
	// are where an ecosystem keeps its contract.
	platformRoot := deps.PlatformRoot(projectRoot)
	roots := moduleRoots(projectRoot)
	if platformRoot != "" {
		for _, root := range moduleRoots(platformRoot) {
			if !containsRoot(roots, root) {
				roots = append(roots, root)
			}
		}
	}
	request := Request{
		ProjectRoot:  absolute(projectRoot),
		PlatformRoot: absolute(platformRoot),
		ModuleRoots:  roots,
	}
	result := plugins.RunHooks(found, Component, Hook, request)

	var contract *schema.ModuleContract
	for _, payload := range result.Payload {
		var response Response
		if err := json.Unmarshal(payload, &response); err != nil {
			result.Diagnostics = append(result.Diagnostics, plugins.Diagnostic{
				Level:   "warning",
				Message: "a module contract could not be read: " + err.Error(),
			})
			continue
		}
		if len(response.Contract) == 0 {
			continue
		}
		parsed, err := schema.ParseModuleContract(response.Contract)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, plugins.Diagnostic{
				Level:   "warning",
				Message: "a module contract was refused: " + err.Error(),
			})
			continue
		}
		if contract == nil {
			contract = parsed
			continue
		}
		contract.Merge(parsed)
	}
	return contract, result.Diagnostics
}

// moduleRoots is the project and the modules it is built from, depth first.
//
// A contract is data an ecosystem can keep in any of them — the project itself,
// the platform it is built for, or the interfaces every module depends on — so
// the plugin is given the whole graph rather than having to guess a directory.
func moduleRoots(projectRoot string) []string {
	seen := map[string]bool{}
	roots := []string{}

	var walk func(root string, depth int)
	walk = func(root string, depth int) {
		if root == "" || depth > 8 {
			return
		}
		root = absolute(root)
		if seen[root] {
			return
		}
		seen[root] = true
		roots = append(roots, root)

		manifest, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || manifest == nil {
			return
		}
		for _, dependency := range manifest.Deps {
			if dependency.Path == "" {
				continue
			}
			walk(resolve(root, dependency.Path), depth+1)
		}
		for _, platform := range manifest.Platforms {
			if platform.Path == "" {
				continue
			}
			walk(resolve(root, platform.Path), depth+1)
		}
	}
	walk(projectRoot, 0)
	return roots
}

// resolve reads a path a manifest declared relative to itself.
func resolve(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// absolute is a path a plugin can read whichever directory it runs from.
func absolute(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func containsRoot(roots []string, root string) bool {
	for _, existing := range roots {
		if existing == root {
			return true
		}
	}
	return false
}
