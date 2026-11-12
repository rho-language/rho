// Package integration is how the toolchain reaches an ecosystem: through an
// integration plugin, and through nothing else.
//
// Rho is a general-purpose language and toolchain. It names no ecosystem. An
// ecosystem (RoboHub is one) meets it through one plugin of kind integration,
// which declares in its manifest's `integration:` section what it bridges: the
// ecosystem it serves, the variable its manifests name the workspace by, and the
// files and ports it provides by the kinds the toolchain knows how to read. Every
// part of the toolchain that needs something an ecosystem supplies asks this
// package for it, by kind, for the platform a project is built for; no part of
// the toolchain looks inside a plugin's folder for a file by convention.
//
// The other half of an integration is the components' own plugin contracts
// (rho-target's `platform`, rho-module's `module-manifest`, ...), which an
// integration plugin fills through `extends:` like any plugin. This package is
// the declarative half: what an integration provides as data.
package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

// The binding kinds the toolchain reads. A binding is a file an ecosystem
// supplies for one purpose; its kind says which purpose and how it is read.
const (
	// BindingProtocolBusC is the event bus generated C reaches (a YAML file:
	// headers, error type and results, event id and size types, publish,
	// subscribe and unsubscribe), read by `rho protocol generate --target c`.
	BindingProtocolBusC = "protocol-bus-c"
	// BindingProtocolTransportRho is the wire framing generated Rho carries
	// (Rho source with @P@ standing for the schema's prefix), read by
	// `rho protocol generate --target rho`.
	BindingProtocolTransportRho = "protocol-transport-rho"
)

// KnownBindings are every binding kind, so a manifest naming another is refused
// rather than silently ignored.
var KnownBindings = []string{BindingProtocolBusC, BindingProtocolTransportRho}

// PortSuffix names a Go port file in an integration's ports directory.
const PortSuffix = ".go"

// Integration is one integration plugin, found and resolved.
type Integration struct {
	Plugin       plugins.Plugin
	Ecosystem    string
	WorkspaceEnv string
	// Bindings are absolute paths by binding kind.
	Bindings map[string]string
	// PortsDir is the absolute ports directory, or "".
	PortsDir string
}

// ForPlatform is the integrations offered for a platform: installed (and, when
// the source trusts it, in the tree), serving that platform. A project with no
// platform has none, which is a standard build.
func ForPlatform(source plugins.Source) []Integration {
	if source.Platform == "" {
		return nil
	}
	var found []Integration
	for _, plugin := range source.All() {
		if len(plugin.Platforms) == 0 || !source.Offered(plugin) {
			continue
		}
		manifest, err := deps.LoadManifest(filepath.Join(plugin.Root, "module.yml"))
		if err != nil || manifest == nil || manifest.Integration == nil {
			continue
		}
		found = append(found, resolve(plugin, manifest.Integration))
	}
	return found
}

func resolve(plugin plugins.Plugin, declared *deps.IntegrationManifest) Integration {
	integration := Integration{
		Plugin: plugin, Ecosystem: declared.Ecosystem, WorkspaceEnv: declared.WorkspaceEnv,
		Bindings: map[string]string{},
	}
	for kind, path := range declared.Bindings {
		integration.Bindings[kind] = filepath.Join(plugin.Root, filepath.FromSlash(path))
	}
	if declared.Ports != "" {
		integration.PortsDir = filepath.Join(plugin.Root, filepath.FromSlash(declared.Ports))
	}
	return integration
}

// Binding is the file of one binding kind the platform's integration provides.
func Binding(source plugins.Source, kind string) (string, error) {
	for _, integration := range ForPlatform(source) {
		if path, ok := integration.Bindings[kind]; ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("no integration for platform %q provides the %s binding", source.Platform, kind)
}

// Ports are the Go ports the platform's integrations provide, by port name, each
// a complete Go file in package main. Test files are the port's own and are not
// ports.
func Ports(source plugins.Source) map[string]string {
	found := map[string]string{}
	for _, integration := range ForPlatform(source) {
		if integration.PortsDir == "" {
			continue
		}
		entries, err := os.ReadDir(integration.PortsDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name, isPort := strings.CutSuffix(entry.Name(), PortSuffix)
			if !isPort || entry.IsDir() || strings.HasSuffix(name, "_test") {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(integration.PortsDir, entry.Name())); err == nil {
				found[name] = string(data)
			}
		}
	}
	return found
}

// Validate checks an integration plugin against the contract and returns every
// way it falls short, so a plugin is refused at validation and not at a build
// that needed what it promised.
func Validate(root string, manifest *deps.Manifest) []string {
	declared := manifest.Integration
	if declared == nil {
		return nil
	}
	var problems []string
	if manifest.ModuleType != "plugin" {
		problems = append(problems, "an `integration:` section belongs to a plugin")
	}
	if declared.Ecosystem == "" {
		problems = append(problems, "integration.ecosystem names the ecosystem the plugin bridges")
	}
	serves := []string{}
	if manifest.Extends != nil {
		serves = manifest.Extends.Platforms
	}
	if declared.Ecosystem != "" && !contains(serves, declared.Ecosystem) {
		problems = append(problems, fmt.Sprintf("an integration for %q must list it under `serves:`, so it runs only for that ecosystem's projects", declared.Ecosystem))
	}
	if declared.WorkspaceEnv != "" && !strings.HasSuffix(declared.WorkspaceEnv, "_WORKSPACE") {
		problems = append(problems, fmt.Sprintf("integration.workspace_env %q must end in _WORKSPACE, the suffix the toolchain resolves to the workspace when it is unset", declared.WorkspaceEnv))
	}
	kinds := make([]string, 0, len(declared.Bindings))
	for kind := range declared.Bindings {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if !contains(KnownBindings, kind) {
			problems = append(problems, fmt.Sprintf("binding kind %q is not one the toolchain reads (%s)", kind, strings.Join(KnownBindings, ", ")))
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(declared.Bindings[kind]))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			problems = append(problems, fmt.Sprintf("binding %s names %s, which is not a file in the plugin", kind, declared.Bindings[kind]))
		}
		if !packaged(manifest, declared.Bindings[kind]) {
			problems = append(problems, fmt.Sprintf("binding %s is not in the package (`package.include`)", kind))
		}
	}
	if declared.Ports != "" {
		dir := filepath.Join(root, filepath.FromSlash(declared.Ports))
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			problems = append(problems, "integration.provides.ports must be a Go module of its own (a go.mod), so the ports are built and tested where they live")
		}
		entries, _ := os.ReadDir(dir)
		ports := 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, PortSuffix) || strings.HasSuffix(name, "_test.go") {
				continue
			}
			ports++
			data, _ := os.ReadFile(filepath.Join(dir, name))
			if !strings.Contains(string(data), "\npackage main\n") && !strings.HasPrefix(string(data), "package main\n") {
				problems = append(problems, fmt.Sprintf("port %s must be in package main", name))
			}
		}
		if ports == 0 {
			problems = append(problems, "integration.provides.ports holds no ports")
		}
		if !packaged(manifest, declared.Ports) {
			problems = append(problems, "the ports are not in the package (`package.include`)")
		}
	}
	return problems
}

// packaged reports whether a path the integration provides is in its package:
// named by `package.include` itself or by a directory above it.
func packaged(manifest *deps.Manifest, path string) bool {
	if manifest.PackageInclude == nil {
		return true // no explicit list: the default layout decides
	}
	clean := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(path)), "/")
	for _, entry := range manifest.PackageInclude {
		entry = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(entry)), "/")
		if clean == entry || strings.HasPrefix(clean, entry+"/") {
			return true
		}
	}
	return false
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
