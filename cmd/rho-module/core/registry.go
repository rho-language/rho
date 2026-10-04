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
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"fmt"
)

/* -------------------------------------------------------------------------- */
/*  Public API - Search                                                        */
/* -------------------------------------------------------------------------- */

// SearchRegistry lists what is installed in the local registry.
//
// It reads the installed tree rather than the index, because the tree is what
// the toolchain dispatches to and a Rho import resolves against: a module that
// is not on disk is not installed however well the index records it. The index
// is reconciled first, because it is the only place that says which of the
// installed versions is selected and which registry each one came from.
func SearchRegistry(query string) ([]RegistryEntry, error) {
	if _, err := Reconcile(); err != nil {
		return nil, err
	}
	installed, err := ReadInstalled()
	if err != nil {
		return nil, err
	}

	results := []RegistryEntry{}
	for _, entry := range installed {
		if query != "" && !containsIgnoreCase(entry.Name, query) {
			continue
		}
		results = append(results, RegistryEntry{
			Name:    entry.Name,
			Version: entry.Version,
			Type:    entry.Kind,
			Active:  entry.IsSelected(),
			Origin:  entry.Origin,
		})
	}
	return results, nil
}

// RegistryEntry represents a module entry in search results
type RegistryEntry struct {
	Name    string
	Version string
	Type    string
	// Active is true for the version the toolchain will resolve to.
	Active bool
	// Origin is "local" for a module published from a directory and "remote"
	// for one fetched from a registry.
	Origin string
}

// IsRemote reports whether a module came from a configured remote registry.
func (e RegistryEntry) IsRemote() bool { return e.Origin == originRemote }

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

// UnpublishModule removes a module version from the local registry.
//
// It is the name this operation had before the local and remote registries were
// separated, and it is kept because it is what the tests and any caller from
// before that separation call. The rule is UninstallLocal's: the selected
// version is refused until it is deactivated, because it is the version a
// project is currently resolved against.
func UnpublishModule(name, version string) error {
	return UninstallLocal(name, version)
}

// findModuleType reports which kind a name@version is installed under, over
// every kind the local registry tracks rather than the two that predate the
// rest. A kind left out here is a module that is installed and that no command
// can activate or unpublish, because nothing would recognise what it is.
func findModuleType(registry *Registry, name, version string) (string, bool) {
	for _, kind := range IndexKinds {
		for _, entry := range *versionBucketOf(registry, kind) {
			if entry.Name == name && entry.Version == version {
				return kind, true
			}
		}
	}
	return "", false
}

// The five helpers below all ask the same question of one kind's list, so they
// all go through the kind's bucket rather than repeating the two-kind switch
// they all used to carry. A switch that lists two kinds out of four is a bucket
// lookup that someone has to remember to extend.

func isActiveVersion(registry *Registry, moduleType, name, version string) bool {
	active := activeBucketOf(registry, moduleType)
	if active == nil {
		return false
	}
	for _, a := range *active {
		if a.Name == name && a.Version == version {
			return true
		}
	}
	return false
}

func countOtherVersions(registry *Registry, moduleType, name, version string) int {
	versions := versionBucketOf(registry, moduleType)
	if versions == nil {
		return 0
	}
	count := 0
	for _, v := range *versions {
		if v.Name == name && v.Version != version {
			count++
		}
	}
	return count
}

func removeVersionEntry(registry *Registry, moduleType, name, version string) {
	versions := versionBucketOf(registry, moduleType)
	if versions == nil {
		return
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
	active := activeBucketOf(registry, moduleType)
	if active == nil {
		return
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
	versions := versionBucketOf(registry, moduleType)
	if versions == nil {
		return ""
	}
	var latest string
	for _, v := range *versions {
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
	registry, err := Reconcile()
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
	registryFile, err := registryFilePath()
	if err != nil {
		return err
	}
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
