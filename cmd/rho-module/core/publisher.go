//
// @file publisher.go
// @brief Rho Module Publisher - Enterprise v3.
//
// Handles publishing modules and plugins to a remote registry.
// Steps:
//   1. Validate module
//   2. Read module metadata
//   3. Pack module into `.rho-module` archive
//   4. Upload archive to remote registry
//
// Archive format: tar.gz
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
	"rho/src/schema"

	"rho/cmd/rho-module/remote"

	pkgcore "rho/cmd/rho-pkg/core"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func PublishModule(dir string) error {
	// Step 1: Validate module
	if err := ValidateModule(dir); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Read metadata
	meta, err := ReadModuleInfo(dir)
	if err != nil {
		return fmt.Errorf("cannot read module info: %w", err)
	}

	// Convert to pkg ModuleYAML
	pkgManifest := pkgcore.ModuleYAML{
		Module: pkgcore.ModuleSection{
			Name:    meta.Module.Name,
			Type:    meta.Module.Type,
			Version: meta.Module.Version,
			Command: meta.Module.Command,
		},
		Main:     meta.Main,
		Deps:     meta.Deps,
		Features: meta.Features,
	}

	var binaryPath string
	var pkgPath string
	var sourceArchive string

	// Cleanup function
	cleanup := func() {
		if binaryPath != "" {
			os.Remove(binaryPath)
		}
		if pkgPath != "" {
			os.Remove(pkgPath)
		}
		if sourceArchive != "" {
			os.Remove(sourceArchive)
		}
	}
	defer cleanup()

	// Step 3: Build binary (for component/plugin)
	if pkgManifest.Module.Type == "component" || pkgManifest.Module.Type == "plugin" {
		binaryPath, err = pkgcore.BuildBinary(dir, pkgManifest)
		if err != nil {
			return fmt.Errorf("build binary: %w", err)
		}
	}

	// Step 4: Pack module with pre-built binary
	pkgPath, err = pkgcore.PackProjectWithBinary(dir, binaryPath)
	if err != nil {
		return fmt.Errorf("cannot pack module: %w", err)
	}
	// Don't cleanup binary yet - it's in the package

	// Step 5: Create .rho-module source archive for remote registry
	sourceArchive, err = packModuleSources(dir, meta)
	if err != nil {
		return fmt.Errorf("cannot pack sources: %w", err)
	}

	// Step 6: Upload to remote registry (both binary pkg and source archive)
	if err := uploadModule(meta, pkgPath, sourceArchive); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	// Step 7: Install locally
	if err := installLocally(pkgManifest, pkgPath); err != nil {
		return fmt.Errorf("local install failed: %w", err)
	}

	// Step 8: Clean up generated files (pkg, binary, source archive)
	cleanup()

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Pack Module Sources (for remote registry)                                  */
/* -------------------------------------------------------------------------- */

func packModuleSources(dir string, meta ModuleInfo) (string, error) {
	archiveName := fmt.Sprintf("%s-%s.rho-module",
		meta.Module.Name,
		meta.Module.Version,
	)
	archivePath := filepath.Join(dir, archiveName)

	out, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("cannot create archive: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Collect source files (module.yml, src/, plugin/, README.md, config/)
	files, err := collectModuleFiles(dir)
	if err != nil {
		return "", err
	}

	// Write files
	for _, file := range files {
		rel, _ := filepath.Rel(dir, file)
		info, _ := os.Stat(file)

		hdr, _ := tar.FileInfoHeader(info, "")
		hdr.Name = rel

		if err := tw.WriteHeader(hdr); err != nil {
			return "", err
		}

		f, _ := os.Open(file)
		io.Copy(tw, f)
		f.Close()
	}

	return archivePath, nil
}

/* -------------------------------------------------------------------------- */
/*  Collect Files                                                              */
/* -------------------------------------------------------------------------- */

func collectModuleFiles(dir string) ([]string, error) {
	var out []string

	required := []string{
		"module.yml",
		"README.md",
	}

	for _, f := range required {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("missing required file: %s", f)
		}
		out = append(out, path)
	}

	// src/
	srcDir := filepath.Join(dir, "src")
	if _, err := os.Stat(srcDir); err != nil {
		return nil, fmt.Errorf("missing src/ directory")
	}

	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, path)
		}
		return nil
	})

	// config/ (optional)
	configDir := filepath.Join(dir, "config")
	if _, err := os.Stat(configDir); err == nil {
		filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				out = append(out, path)
			}
			return nil
		})
	}

	// plugin/ (optional)
	pluginDir := filepath.Join(dir, "plugin")
	if _, err := os.Stat(pluginDir); err == nil {
		filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				out = append(out, path)
			}
			return nil
		})
	}

	return out, nil
}

/* -------------------------------------------------------------------------- */
/*  Upload Module                                                              */
/* -------------------------------------------------------------------------- */

func uploadModule(meta ModuleInfo, pkgPath, sourceArchive string) error {
	// Load remote config
	cfg, err := remote.LoadConfig()
	if err != nil {
		return fmt.Errorf("cannot load registry config: %w", err)
	}

	// If remote registry is configured, upload there
	if cfg.Remote.URL != "" && cfg.Remote.Token != "" {
		client := remote.NewClient(cfg)

		pkgData, err := os.ReadFile(pkgPath)
		if err != nil {
			return fmt.Errorf("cannot read package: %w", err)
		}

		sourceData, err := os.ReadFile(sourceArchive)
		if err != nil {
			return fmt.Errorf("cannot read source archive: %w", err)
		}

		// The manifest travels with the upload so a registry can record what
		// was published rather than storing blanks.
		manifest := map[string]string{
			"type":        meta.Module.Type,
			"description": meta.Module.Description,
			"main":        meta.Module.Main,
			"command":     meta.Module.Command,
		}
		if err := remote.UploadModule(client, meta.Module.Name, meta.Module.Version, pkgData, sourceData, manifest); err != nil {
			return fmt.Errorf("remote upload failed: %w", err)
		}
	}

	// Local mode: publish to local registry only
	fmt.Println("Publishing", pkgPath, "and", sourceArchive, "to local registry...")
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Local Install (same as build)                                              */
/* -------------------------------------------------------------------------- */

func installLocally(meta pkgcore.ModuleYAML, pkgPath string) error {
	// Install to ~/.rho/modules/installed/<type>/
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return err
	}
	moduleDir := filepath.Join(modulesDir, meta.Module.Type, fmt.Sprintf("%s@%s", meta.Module.Name, meta.Module.Version))

	if err := pkgcore.UnpackArchive(pkgPath, moduleDir); err != nil {
		return fmt.Errorf("failed to install module: %w", err)
	}

	// Update registry.yaml
	if err := updateRegistryForPublish(meta); err != nil {
		return fmt.Errorf("failed to update registry: %w", err)
	}

	fmt.Printf("Published and installed %s@%s to %s\n", meta.Module.Name, meta.Module.Version, moduleDir)
	return nil
}

func updateRegistryForPublish(manifest pkgcore.ModuleYAML) error {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return err
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	// Read existing registry
	registry, err := readRegistry(registryFile)
	if err != nil {
		return err
	}

	entry := struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}{
		Name:    manifest.Module.Name,
		Version: manifest.Module.Version,
	}

	// Add to version list (avoid duplicates)
	addVersionEntry(registry, manifest.Module.Type, entry)

	// If no active version for this module, make this one active
	if !hasActiveVersion(registry, manifest.Module.Type, manifest.Module.Name) {
		setActiveVersion(registry, manifest.Module.Type, manifest.Module.Name, manifest.Module.Version)
	}

	// Write registry
	return writeRegistry(registryFile, registry)
}

func readRegistry(registryPath string) (*Registry, error) {
	registry := &Registry{
		Components:       []VersionEntry{},
		Plugins:          []VersionEntry{},
		ActiveComponents: []ActiveEntry{},
		ActivePlugins:    []ActiveEntry{},
	}

	if _, err := os.Stat(registryPath); err == nil {
		// Schema-validate before parsing
		result, err := schema.ValidateRegistryFile(registryPath)
		if err != nil {
			return registry, fmt.Errorf("cannot read registry: %w", err)
		}
		for _, e := range result {
			if e.IsError() {
				return registry, fmt.Errorf("registry schema error: %s at %s", e.Message, e.Path)
			}
		}

		data, err := os.ReadFile(registryPath)
		if err == nil {
			// Simple YAML parsing for our structure
			parseRegistryYAML(string(data), registry)
		}
	}

	return registry, nil
}

func writeRegistry(registryPath string, registry *Registry) error {
	var out string
	out += "components:\n"
	for _, c := range registry.Components {
		out += fmt.Sprintf("  - name: %s\n    version: %s\n", c.Name, c.Version)
	}
	out += "plugins:\n"
	for _, p := range registry.Plugins {
		out += fmt.Sprintf("  - name: %s\n    version: %s\n", p.Name, p.Version)
	}
	out += "active_components:\n"
	for _, c := range registry.ActiveComponents {
		out += fmt.Sprintf("  - name: %s\n    version: %s\n", c.Name, c.Version)
	}
	out += "active_plugins:\n"
	for _, p := range registry.ActivePlugins {
		out += fmt.Sprintf("  - name: %s\n    version: %s\n", p.Name, p.Version)
	}

	registryDir := filepath.Dir(registryPath)
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(registryPath, []byte(out), 0644)
}

func parseRegistryYAML(data string, registry *Registry) {
	lines := strings.Split(data, "\n")
	var currentSection string
	var currentName string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "components:") {
			currentSection = "components"
			continue
		}
		if strings.HasPrefix(trimmed, "plugins:") {
			currentSection = "plugins"
			continue
		}
		if strings.HasPrefix(trimmed, "active_components:") {
			currentSection = "active_components"
			continue
		}
		if strings.HasPrefix(trimmed, "active_plugins:") {
			currentSection = "active_plugins"
			continue
		}

		if strings.HasPrefix(trimmed, "- name:") {
			currentName = strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
			continue
		}
		if strings.HasPrefix(trimmed, "version:") {
			version := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
			entry := VersionEntry{Name: currentName, Version: version}
			switch currentSection {
			case "components":
				registry.Components = append(registry.Components, entry)
			case "plugins":
				registry.Plugins = append(registry.Plugins, entry)
			case "active_components":
				registry.ActiveComponents = append(registry.ActiveComponents, ActiveEntry{Name: currentName, Version: version})
			case "active_plugins":
				registry.ActivePlugins = append(registry.ActivePlugins, ActiveEntry{Name: currentName, Version: version})
			}
			currentName = ""
		}
	}
}

func addVersionEntry(registry *Registry, moduleType string, entry VersionEntry) {
	versions := versionBucket(registry, moduleType)
	if versions == nil {
		// The local registry only tracks components and plugins, so a module
		// of any other type has no bucket to record.
		return
	}

	// Check for duplicate
	for _, v := range *versions {
		if v.Name == entry.Name && v.Version == entry.Version {
			return
		}
	}
	*versions = append(*versions, entry)
}

func hasActiveVersion(registry *Registry, moduleType, name string) bool {
	active := activeBucket(registry, moduleType)
	if active == nil {
		return false
	}
	for _, a := range *active {
		if a.Name == name {
			return true
		}
	}
	return false
}

// versionBucket returns the registry list a module type is recorded in, or nil
// when the registry has no bucket for it.
func versionBucket(registry *Registry, moduleType string) *[]VersionEntry {
	switch moduleType {
	case "component":
		return &registry.Components
	case "plugin":
		return &registry.Plugins
	default:
		return nil
	}
}

// activeBucket returns the active list a module type is recorded in, or nil
// when the registry has no bucket for it.
func activeBucket(registry *Registry, moduleType string) *[]ActiveEntry {
	switch moduleType {
	case "component":
		return &registry.ActiveComponents
	case "plugin":
		return &registry.ActivePlugins
	default:
		return nil
	}
}

func setActiveVersion(registry *Registry, moduleType, name, version string) {
	active := activeBucket(registry, moduleType)
	if active == nil {
		return
	}

	// Remove existing active version for this module
	newActive := []ActiveEntry{}
	for _, a := range *active {
		if a.Name != name {
			newActive = append(newActive, a)
		}
	}
	// Add new active version
	newActive = append(newActive, ActiveEntry{Name: name, Version: version})
	*active = newActive
}

type Registry struct {
	Components       []VersionEntry
	Plugins          []VersionEntry
	ActiveComponents []ActiveEntry
	ActivePlugins    []ActiveEntry
}

type VersionEntry struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type ActiveEntry struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}
