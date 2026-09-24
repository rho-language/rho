//
// @file registry.go
// @brief Rho Module Registry Operations - Enterprise v3.
//
// Handles registry operations: search, unpublish, activate, deactivate
// Supports multiple versions with one active at a time.
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
	"path/filepath"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Public API - Search                                                        */
/* -------------------------------------------------------------------------- */

// SearchRegistry searches for modules in the local registry
func SearchRegistry(query string) ([]RegistryEntry, error) {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return nil, err
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	registry, err := readRegistry(registryFile)
	if err != nil {
		return nil, err
	}

	var results []RegistryEntry

	// Search components
	for _, c := range registry.Components {
		if query == "" || containsIgnoreCase(c.Name, query) {
			active := findActiveVersion(registry.ActiveComponents, c.Name)
			results = append(results, RegistryEntry{
				Name:    c.Name,
				Version: c.Version,
				Type:    "component",
				Active:  active == c.Version,
			})
		}
	}

	// Search plugins
	for _, p := range registry.Plugins {
		if query == "" || containsIgnoreCase(p.Name, query) {
			active := findActiveVersion(registry.ActivePlugins, p.Name)
			results = append(results, RegistryEntry{
				Name:    p.Name,
				Version: p.Version,
				Type:    "plugin",
				Active:  active == p.Version,
			})
		}
	}

	return results, nil
}

// RegistryEntry represents a module entry in search results
type RegistryEntry struct {
	Name    string
	Version string
	Type    string
	Active  bool
}

func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsAtLower(s, substr)))
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func containsAtLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func findActiveVersion(active []ActiveEntry, name string) string {
	for _, a := range active {
		if a.Name == name {
			return a.Version
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Public API - Unpublish                                                     */
/* -------------------------------------------------------------------------- */

// UnpublishModule removes a module version from the registry and installed modules
// Returns error if trying to unpublish the active version without other versions available
func UnpublishModule(name, version string) error {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return err
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	registry, err := readRegistry(registryFile)
	if err != nil {
		return err
	}

	// Find module type and validate
	moduleType, found := findModuleType(registry, name, version)
	if !found {
		return fmt.Errorf("module %s@%s not found in registry", name, version)
	}

	// Check if this version is active
	isActive := isActiveVersion(registry, moduleType, name, version)

	// Count other versions of this module
	otherVersions := countOtherVersions(registry, moduleType, name, version)

	if isActive && otherVersions > 0 {
		return fmt.Errorf("cannot unpublish active version %s@%s (other versions exist). Use 'rho module activate' to switch active version first", name, version)
	}

	// If it's the only version and active, allow unpublish but clear from active list
	if isActive && otherVersions == 0 {
		clearActiveVersion(registry, moduleType, name)
	}

	// Remove from version list
	removeVersionEntry(registry, moduleType, name, version)

	// If was active, clear active (will be set to another version if available)
	if isActive {
		clearActiveVersion(registry, moduleType, name)
		// Auto-activate another version if available
		if otherVersions > 0 {
			newActive := getLatestVersion(registry, moduleType, name)
			if newActive != "" {
				setActiveVersion(registry, moduleType, name, newActive)
			}
		}
	}

	// Write updated registry
	if err := writeRegistry(registryFile, registry); err != nil {
		return err
	}

	// Remove installed module directory
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return err
	}
	moduleDir := filepath.Join(modulesDir, moduleType, fmt.Sprintf("%s@%s", name, version))
	os.RemoveAll(moduleDir)

	fmt.Printf("Unpublished %s@%s\n", name, version)
	return nil
}

func findModuleType(registry *Registry, name, version string) (string, bool) {
	for _, c := range registry.Components {
		if c.Name == name && c.Version == version {
			return "component", true
		}
	}
	for _, p := range registry.Plugins {
		if p.Name == name && p.Version == version {
			return "plugin", true
		}
	}
	return "", false
}

func isActiveVersion(registry *Registry, moduleType, name, version string) bool {
	var active []ActiveEntry
	if moduleType == "component" {
		active = registry.ActiveComponents
	} else if moduleType == "plugin" {
		active = registry.ActivePlugins
	}
	for _, a := range active {
		if a.Name == name && a.Version == version {
			return true
		}
	}
	return false
}

func countOtherVersions(registry *Registry, moduleType, name, version string) int {
	var versions []VersionEntry
	if moduleType == "component" {
		versions = registry.Components
	} else if moduleType == "plugin" {
		versions = registry.Plugins
	}
	count := 0
	for _, v := range versions {
		if v.Name == name && v.Version != version {
			count++
		}
	}
	return count
}

func removeVersionEntry(registry *Registry, moduleType, name, version string) {
	var versions *[]VersionEntry
	if moduleType == "component" {
		versions = &registry.Components
	} else if moduleType == "plugin" {
		versions = &registry.Plugins
	}

	newList := []VersionEntry{}
	for _, v := range *versions {
		if !(v.Name == name && v.Version == version) {
			newList = append(newList, v)
		}
	}
	*versions = newList
}

func clearActiveVersion(registry *Registry, moduleType, name string) {
	var active *[]ActiveEntry
	if moduleType == "component" {
		active = &registry.ActiveComponents
	} else if moduleType == "plugin" {
		active = &registry.ActivePlugins
	}

	newList := []ActiveEntry{}
	for _, a := range *active {
		if a.Name != name {
			newList = append(newList, a)
		}
	}
	*active = newList
}

func getLatestVersion(registry *Registry, moduleType, name string) string {
	var versions []VersionEntry
	if moduleType == "component" {
		versions = registry.Components
	} else if moduleType == "plugin" {
		versions = registry.Plugins
	}

	var latest string
	for _, v := range versions {
		if v.Name == name {
			if latest == "" || compareVersions(v.Version, latest) > 0 {
				latest = v.Version
			}
		}
	}
	return latest
}

func compareVersions(v1, v2 string) int {
	// Simple semantic version comparison
	parts1 := splitVersion(v1)
	parts2 := splitVersion(v2)

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		n1 := parseInt(parts1[i])
		n2 := parseInt(parts2[i])
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []string {
	// Split by . and -
	result := []string{}
	current := ""
	for _, c := range v {
		if c == '.' || c == '-' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func parseInt(s string) int {
	result := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

/* -------------------------------------------------------------------------- */
/*  Public API - Activate                                                      */
/* -------------------------------------------------------------------------- */

// ActivateModule sets a specific version as active
func ActivateModule(name, version string) error {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return err
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	registry, err := readRegistry(registryFile)
	if err != nil {
		return err
	}

	// Find module type
	moduleType, found := findModuleType(registry, name, version)
	if !found {
		return fmt.Errorf("module %s@%s not found in registry", name, version)
	}

	// Set as active
	setActiveVersion(registry, moduleType, name, version)

	// Write registry
	if err := writeRegistry(registryFile, registry); err != nil {
		return err
	}

	fmt.Printf("Activated %s@%s\n", name, version)
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Public API - List Installed                                                */
/* -------------------------------------------------------------------------- */

// ListInstalledModules returns all installed modules with their active status
func ListInstalledModules() ([]RegistryEntry, error) {
	return SearchRegistry("")
}