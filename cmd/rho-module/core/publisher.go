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
// SPDX-License-Identifier: Apache-2.0
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

	// The manifest is loaded rather than rebuilt from metadata, so every
	// section the packager reads travels with it. Rebuilding it dropped the
	// `package:` section, which is what decides whether a command is built.
	pkgManifest, err := pkgcore.LoadModuleYAML(filepath.Join(dir, "module.yml"))
	if err != nil {
		return fmt.Errorf("cannot load module.yml: %w", err)
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

	// Step 3: Build the command, when the package layout says this kind of
	// module has one. A component published as a library has no command, the
	// same way a C project with no main is built as a library rather than an
	// executable, and only its interface is published.
	if pkgManifest.Layout().Binary {
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
		return err
	}

	// Step 7: Install locally
	if err := installLocally(pkgManifest, pkgPath, sourceArchive); err != nil {
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
	// The source archive is a build artefact, not a source file.
	archivePath := filepath.Join(pkgcore.ArtifactDirFor(dir, mustManifest(dir)), archiveName)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return "", fmt.Errorf("cannot create the build directory: %w", err)
	}

	out, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("cannot create archive: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	// The layout decides what the source archive holds.
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

// mustManifest loads a manifest for a path that is known to have one.
func mustManifest(dir string) pkgcore.ModuleYAML {
	manifest, err := pkgcore.LoadModuleYAML(filepath.Join(dir, "module.yml"))
	if err != nil {
		return pkgcore.ModuleYAML{}
	}
	return manifest
}

// collectModuleFiles gathers what the source archive holds, which is the same
// layout the binary package uses, so the two cannot disagree.
func collectModuleFiles(dir string) ([]string, error) {
	manifest, err := pkgcore.LoadModuleYAML(filepath.Join(dir, "module.yml"))
	if err != nil {
		return nil, err
	}
	return pkgcore.CollectLayoutFiles(dir, manifest.Layout())
}

// moduleVisibility is whether a published module is anybody's to install or its
// owner's alone.
//
// A module that says nothing is public: publishing to a registry is the act of
// offering a module to whoever wants it, and a module that wants to be offered
// to fewer people has to say so. A value this version does not know is public
// rather than an error, because refusing to publish over a word added to a
// later schema would be a worse outcome than ignoring it.
func moduleVisibility(meta ModuleInfo) string {
	if strings.EqualFold(strings.TrimSpace(meta.Module.Visibility), "private") {
		return "private"
	}
	return "public"
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

	// If a remote registry is configured, upload there.
	//
	// The gate is the client, not the config's token: `rho auth login` stores a
	// credential for a registry and the client presents it in preference to the
	// configured token, so a user who logged in never had to write a token into
	// the config at all. Gating on the config would publish locally for every
	// user who did what the auth component told them to do, and name them as
	// nobody on the registry that keeps their ownership of the module.
	client := remote.NewClient(cfg)
	if cfg.Remote.URL != "" && client.HasCredential() {
		pkgData, err := os.ReadFile(pkgPath)
		if err != nil {
			return fmt.Errorf("cannot read package: %w", err)
		}

		sourceData, err := os.ReadFile(sourceArchive)
		if err != nil {
			return fmt.Errorf("cannot read source archive: %w", err)
		}

		// The manifest travels with the upload so a registry can record what
		// was published rather than storing blanks. Author and maintainer are
		// the module's own claim about who wrote it; the registry keeps its own
		// record of who owns it, taken from the credential that published it.
		manifest := map[string]string{
			"type":        meta.Module.Type,
			"description": meta.Module.Description,
			"main":        meta.Module.Main,
			"command":     meta.Module.Command,
			"author":      meta.Module.Author,
			"maintainer":  meta.Module.Maintainer,
			"visibility":  moduleVisibility(meta),
		}
		if err := remote.UploadModule(client, meta.Module.Name, meta.Module.Version, pkgData, sourceData, manifest); err != nil {
			return err
		}
	}

	// Local mode: publish to local registry only
	fmt.Println("Publishing", pkgPath, "and", sourceArchive, "to local registry...")
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Local Install (same as build)                                              */
/* -------------------------------------------------------------------------- */

func installLocally(meta pkgcore.ModuleYAML, pkgPath, sourceArchive string) error {
	// Install to ~/.rho/modules/installed/<type>/
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return err
	}
	moduleDir := filepath.Join(modulesDir, meta.Module.Type, fmt.Sprintf("%s@%s", meta.Module.Name, meta.Module.Version))

	// The target is replaced rather than merged. Publishing a component in a
	// different form, or one whose file list changed, must not leave the
	// previous form's files behind.
	if err := os.RemoveAll(moduleDir); err != nil {
		return fmt.Errorf("cannot clear the previous installation: %w", err)
	}
	if err := pkgcore.UnpackArchive(pkgPath, moduleDir); err != nil {
		return fmt.Errorf("failed to install module: %w", err)
	}

	// The source archive is unpacked as well, so an installed component is the
	// whole project rather than just its command. That is what lets a project
	// depend on the component and import the Rho library it publishes.
	if sourceArchive != "" {
		if _, statErr := os.Stat(sourceArchive); statErr == nil {
			if err := pkgcore.UnpackArchive(sourceArchive, moduleDir); err != nil {
				return fmt.Errorf("failed to install module sources: %w", err)
			}
		}
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

	// A publish is from this machine, and the index says so: an origin of
	// "local" is what a listing uses to tell a published module from one
	// fetched from a registry.
	entry := VersionEntry{
		Name:    manifest.Module.Name,
		Version: manifest.Module.Version,
		Origin:  originLocal,
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
		Modules:          []VersionEntry{},
		Libraries:        []VersionEntry{},
		ActiveComponents: []ActiveEntry{},
		ActivePlugins:    []ActiveEntry{},
		ActiveModules:    []ActiveEntry{},
		ActiveLibraries:  []ActiveEntry{},
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

// writeRegistry writes the index. The index itself is written and read by
// writeIndex and parseIndex, so that a list which is written and read by two
// sets of code cannot drift.
func writeRegistry(registryPath string, registry *Registry) error {
	return writeIndex(registryPath, registry)
}

// parseRegistryYAML reads the index. See writeRegistry for why the reading and
// the writing share one pair of functions.
func parseRegistryYAML(data string, registry *Registry) {
	parseIndex(data, registry)
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

// versionBucketOf is the list a kind records its versions in.
//
// Every kind the toolchain installs under has one, because a kind without a
// bucket is a module that is installed and invisible: it sits in the tree, the
// resolver finds it, and no command can list, activate or unpublish it.
func versionBucketOf(registry *Registry, kind string) *[]VersionEntry {
	switch kind {
	case KindComponent:
		return &registry.Components
	case KindPlugin:
		return &registry.Plugins
	case KindModule:
		return &registry.Modules
	case KindLibrary:
		return &registry.Libraries
	default:
		return nil
	}
}

// activeBucketOf is the list a kind records its selection in.
func activeBucketOf(registry *Registry, kind string) *[]ActiveEntry {
	switch kind {
	case KindComponent:
		return &registry.ActiveComponents
	case KindPlugin:
		return &registry.ActivePlugins
	case KindModule:
		return &registry.ActiveModules
	case KindLibrary:
		return &registry.ActiveLibraries
	default:
		return nil
	}
}

// versionBucket is the name the publisher used, pointing at the same list.
func versionBucket(registry *Registry, moduleType string) *[]VersionEntry {
	return versionBucketOf(registry, moduleType)
}

// activeBucket is the name the index used, pointing at the same list.
func activeBucket(registry *Registry, moduleType string) *[]ActiveEntry {
	return activeBucketOf(registry, moduleType)
}

// hasActiveName reports whether any version of a module is selected.
func hasActiveName(registry *Registry, kind, name string) bool {
	for _, entry := range *activeBucketOf(registry, kind) {
		if entry.Name == name {
			return true
		}
	}
	return false
}

// setActiveName selects a version of a module.
func setActiveName(registry *Registry, kind, name, version string) {
	active := activeBucketOf(registry, kind)
	kept := []ActiveEntry{}
	for _, entry := range *active {
		if entry.Name != name {
			kept = append(kept, entry)
		}
	}
	*active = append(kept, ActiveEntry{Name: name, Version: version})
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

// Registry is what the local registry knows about the modules installed in it.
//
// It records one list of versions per kind and one selection per module, and
// nothing else. What is installed is read from the tree
// (modules/installed/<kind>/<name>@<version>), and the origin of each entry is
// carried here because a directory cannot say whether a module was published
// from this machine or fetched from a registry.
type Registry struct {
	Components       []VersionEntry
	Plugins          []VersionEntry
	Modules          []VersionEntry
	Libraries        []VersionEntry
	ActiveComponents []ActiveEntry
	ActivePlugins    []ActiveEntry
	ActiveModules    []ActiveEntry
	ActiveLibraries  []ActiveEntry
}

type VersionEntry struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	// Origin is where the module came from: local for one published from a
	// directory, remote for one fetched from a registry. It is what lets a
	// listing say which of the two a name in it is.
	Origin string `yaml:"origin,omitempty"`
}

type ActiveEntry struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}
