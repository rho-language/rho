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
// SPDX-License-Identifier: MIT
//

package core

import (
    "fmt"
    "os"
    "path/filepath"
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

    // Step 3: Download missing modules
    for name, version := range resolved {
        spec := fmt.Sprintf("%s@%s", name, version)
        if err := DownloadModule(spec); err != nil {
            return fmt.Errorf("failed to download %s: %w", spec, err)
        }
    }

    // Step 4: Install into local cache
    if err := installLocalCache(dir, resolved); err != nil {
        return fmt.Errorf("local cache error: %w", err)
    }

    // Step 5: Generate lockfile
    if err := writeLockfile(dir, resolved); err != nil {
        return fmt.Errorf("cannot write rho.lock: %w", err)
    }

    return nil
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
        global := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod", fmt.Sprintf("%s@%s", name, version))
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

func writeLockfile(dir string, deps map[string]string) error {
    out := "lock:\n"
    for name, version := range deps {
        out += fmt.Sprintf("  %s: \"%s\"\n", name, version)
    }

    return os.WriteFile(filepath.Join(dir, "rho.lock"), []byte(out), 0644)
}
