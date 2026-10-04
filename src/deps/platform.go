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
	seen := map[string]bool{}
	var walk func(dir string, depth int) string
	walk = func(dir string, depth int) string {
		if dir == "" || depth > 8 {
			return ""
		}
		if absolute, err := filepath.Abs(dir); err == nil {
			if seen[absolute] {
				return ""
			}
			seen[absolute] = true
		}
		manifest, err := LoadManifest(ManifestPath(dir))
		if err != nil || manifest == nil {
			return ""
		}
		if identity := strings.TrimSpace(manifest.Platform); identity != "" {
			return identity
		}
		for _, dependency := range manifest.Deps {
			if dependency.Path == "" {
				continue
			}
			if identity := walk(resolveFrom(dir, dependency.Path), depth+1); identity != "" {
				return identity
			}
		}
		// An interfaces module enumerates the platforms that implement it, so
		// a project that named only the interfaces still finds the platform.
		for _, platform := range manifest.Platforms {
			if platform.Path == "" {
				continue
			}
			if identity := walk(resolveFrom(dir, platform.Path), depth+1); identity != "" {
				return identity
			}
		}
		return ""
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
