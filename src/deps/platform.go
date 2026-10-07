//
// @file platform.go
// @brief Recognizing the platform a project is built for.
//
// A platform is a module that says so, with `platform: zenith` in its own
// manifest. It is recognized from the module graph rather than from a name, a
// repository or a directory: a module named something else that declares the
// identity is that platform, and a module whose name suggests one that declares
// nothing is not.
//
// The project's own manifest is read first, then the modules it depends on,
// depth first, through both `deps` and the `platforms` an interfaces module
// enumerates as its implementations. The first identity found wins, because a
// project is built for one platform; a project with no such module in its graph
// has no platform, and is built the standard way.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package deps

import (
	"path/filepath"
	"strings"
)

// Platform returns the platform identity a project is built for, or "" when its
// module graph names none.
func Platform(projectRoot string) string {
	root, manifest := platformModule(projectRoot)
	if root == "" {
		return ""
	}
	return strings.TrimSpace(manifest.Platform)
}

// PlatformRoot returns the directory of the module that declares the project's
// platform, or "" when its graph names none.
//
// The directory is what a platform's own tools are found through: a board's
// flash block can name one relative to the platform rather than to the project,
// because the tool travels with the platform and not with whatever is built.
func PlatformRoot(projectRoot string) string {
	root, _ := platformModule(projectRoot)
	return root
}

// platformModule walks the module graph for the module that declares the
// platform, returning its directory and its manifest.
func platformModule(projectRoot string) (string, *Manifest) {
	seen := map[string]bool{}
	var walk func(dir string, depth int) (string, *Manifest)
	walk = func(dir string, depth int) (string, *Manifest) {
		if dir == "" || depth > 8 {
			return "", nil
		}
		if absolute, err := filepath.Abs(dir); err == nil {
			if seen[absolute] {
				return "", nil
			}
			seen[absolute] = true
		}
		manifest, err := LoadManifest(ManifestPath(dir))
		if err != nil || manifest == nil {
			return "", nil
		}
		if strings.TrimSpace(manifest.Platform) != "" {
			return dir, manifest
		}
		for _, dependency := range manifest.Deps {
			if dependency.Path == "" {
				continue
			}
			if root, declared := walk(resolveFrom(dir, dependency.Path), depth+1); root != "" {
				return root, declared
			}
		}
		// An interfaces module enumerates the platforms that implement it, so
		// a project that named only the interfaces still finds the platform.
		for _, platform := range manifest.Platforms {
			if platform.Path == "" {
				continue
			}
			if root, declared := walk(resolveFrom(dir, platform.Path), depth+1); root != "" {
				return root, declared
			}
		}
		return "", nil
	}
	return walk(projectRoot, 0)
}

// resolveFrom resolves a path a manifest declared relative to itself.
func resolveFrom(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
