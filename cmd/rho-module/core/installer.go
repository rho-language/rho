//
// @file installer.go
// @brief Rho Module Installer - Enterprise v3.
//
// Installs all dependencies declared in module.yml by:
//   - Resolving dependency versions
//   - Downloading missing modules
//   - Installing modules into local cache (.rho-deps/)
//   - Generating rho.lock
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

	"rho/internal/paths"

	pkgcore "rho/cmd/rho-pkg/core"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func InstallDependencies(dir string) error {
	// Step 1: Load manifest
	modPath := filepath.Join(dir, "module.yml")
	manifest, err := LoadModuleYAML(modPath)
	if err != nil {
		return fmt.Errorf("cannot load module.yml: %w", err)
	}

	// No deps → nothing to do
	if len(manifest.Deps) == 0 {
		return nil
	}

	// Step 2: Resolve versions
	resolved, err := resolveDependencies(manifest.Deps)
	if err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	// Step 3: Download missing modules, recording what each package hashed to
	// so the lock names the bytes and not only the version.
	hashes := map[string]string{}
	for name, version := range resolved {
		spec := fmt.Sprintf("%s@%s", name, version)
		digest, err := DownloadModuleDigest(spec)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", spec, err)
		}
		hashes[name] = digest
	}

	// Step 4: Install into local cache
	if err := installLocalCache(dir, resolved); err != nil {
		return fmt.Errorf("local cache error: %w", err)
	}

	// Step 5: Generate lockfile
	if err := writeLockfile(dir, resolved, hashes); err != nil {
		return fmt.Errorf("cannot write rho.lock: %w", err)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Build Module (binary only)                                                 */
/* -------------------------------------------------------------------------- */

// BuildModule builds the module binary to the specified output directory.
// If outputDir is empty, builds to the current directory.
func BuildModule(dir, outputDir string) (string, error) {
	// Step 1: Load manifest
	modPath := filepath.Join(dir, "module.yml")
	manifest, err := pkgcore.LoadModuleYAML(modPath)
	if err != nil {
		return "", fmt.Errorf("cannot load module.yml: %w", err)
	}

	// Only component/plugin types are Go projects with binaries
	if manifest.Module.Type != "component" && manifest.Module.Type != "plugin" {
		return "", fmt.Errorf("module type %s does not produce a binary", manifest.Module.Type)
	}

	// Step 2: Build binary using rho-pkg's build logic
	binaryPath, err := pkgcore.BuildBinary(dir, manifest)
	if err != nil {
		return "", fmt.Errorf("failed to build binary: %w", err)
	}

	// Step 3: Copy to output directory if specified
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("cannot create output dir: %w", err)
		}
		destPath := filepath.Join(outputDir, filepath.Base(binaryPath))
		if err := copyFile(binaryPath, destPath); err != nil {
			return "", fmt.Errorf("failed to copy binary: %w", err)
		}
		// Clean up original
		os.Remove(binaryPath)
		return destPath, nil
	}

	return binaryPath, nil
}

/* -------------------------------------------------------------------------- */
/*  Copy File                                                                  */
/* -------------------------------------------------------------------------- */

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

type InstallRegistryEntry struct {
	Name    string
	Version string
}

/* -------------------------------------------------------------------------- */
/*  Dependency Resolver                                                        */
/* -------------------------------------------------------------------------- */

func resolveDependencies(deps map[string]string) (map[string]string, error) {
	out := map[string]string{}

	for name, constraint := range deps {
		version, err := ResolveVersion(name, constraint)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve %s: %w", name, err)
		}
		out[name] = version
	}

	return out, nil
}

/* -------------------------------------------------------------------------- */
/*  Local Cache Installer                                                      */
/* -------------------------------------------------------------------------- */

func installLocalCache(dir string, deps map[string]string) error {
	cacheDir := filepath.Join(dir, ".rho-deps")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	for name, version := range deps {
		cachePath, err := paths.GetCachePath()
		if err != nil {
			return err
		}
		global := filepath.Join(cachePath, fmt.Sprintf("%s@%s", name, version))
		local := filepath.Join(cacheDir, fmt.Sprintf("%s@%s", name, version))

		if err := copyDir(global, local); err != nil {
			return fmt.Errorf("cannot install %s@%s: %w", name, version, err)
		}
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Directory Copy                                                             */
/* -------------------------------------------------------------------------- */

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		out := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(out, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(out, data, 0644)
	})
}

/* -------------------------------------------------------------------------- */
/*  Lockfile Writer                                                            */
/* -------------------------------------------------------------------------- */

func writeLockfile(dir string, deps map[string]string, hashes map[string]string) error {
	return WriteLockfileWithHashes(dir, deps, hashes)
}
