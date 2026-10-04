//
// @file toolchains.go
// @brief Where a fetched program is kept.
//
// A prebuilt archive, put in a directory of the user's own and run from there,
// beside the toolchain's own components. That is the preferred way to get a
// toolchain, and not only because it avoids a password: a program fetched into the
// user's own directory belongs to that user, does not change when the machine's
// package manager does — so a build cannot change out from under a project between
// two runs — and is removed by removing the directory it was put in.
//
// The layout is here, in the package both the doctor and the keeper read, because it
// is a contract between them: a check that could not see a fetched program would
// report one the machine has as missing, and offer to fetch a second copy of it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package sysdep

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ToolchainsDir is where fetched programs are kept, under the Rho home.
const ToolchainsDir = "toolchains"

// FetchedDir is where one version of one program is kept.
//
// Versioned in the path, so a second version lands beside the first rather than on
// top of it: a project that needs 12.1 keeps working while a newer one is fetched,
// and a person who wants the old one back still has it.
func FetchedDir(cacheRoot string, name string, version string) string {
	if cacheRoot == "" || name == "" {
		return ""
	}
	if version == "" {
		version = "current"
	}
	return filepath.Join(cacheRoot, name+"-"+sanitise(version))
}

// FetchedBinary is where a fetched program of this dependency is, and whether it is
// there.
func FetchedBinary(cacheRoot string, dep ExecutableDep, version string) (string, bool) {
	directory := FetchedDir(cacheRoot, dep.Name, version)
	if directory == "" {
		return "", false
	}
	candidate := filepath.Join(directory, filepath.FromSlash(FetchedRelative(dep)))
	if info, err := os.Stat(candidate); err != nil || info.IsDir() {
		return "", false
	}
	return candidate, true
}

// FetchedVersion is the version a fetched program is kept under: what the module
// asked for, which is not the same string as a distribution's package number.
func FetchedVersion(dep ExecutableDep) string {
	if dep.Fetch != nil && strings.TrimSpace(dep.Fetch.Version) != "" {
		return dep.Fetch.Version
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(dep.Version), ">="))
}

// FetchBinDirs is every directory a fetched program's `bin` may be in.
//
// Both shapes, because archives differ: most put `bin` at the root of what was
// kept, and some keep the whole tree and bury it.
func FetchBinDirs(cacheRoot string) []string {
	if cacheRoot == "" {
		return nil
	}
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// The bin of a fetched toolchain is under the directory its archive unpacked
		// into, and a recipe that names no directory has the binary at the root — so
		// both are looked in, and both are the layout `FetchedRelative` writes down.
		root := filepath.Join(cacheRoot, entry.Name())
		candidates := []string{filepath.Join(root, "bin"), root}
		candidates = append(candidates, subdirectoriesOf(root)...)
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				out = append(out, candidate)
			}
		}
	}
	return out
}

// subdirectoriesOf is every directory inside a path, one level deep, which is where
// an archive's top-level directory is.
func subdirectoriesOf(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, filepath.Join(root, entry.Name(), "bin"))
	}
	return out
}

// ToolchainRoot is where fetched programs are kept on this machine.
func ToolchainRoot(rhoHome string) string {
	if rhoHome == "" {
		return ""
	}
	return filepath.Join(rhoHome, ToolchainsDir)
}

// sanitise is a version turned into something a directory can be called.
//
// Not only for legality: `12.1-3ubuntu1` and `12.1/3` are both real answers from
// two different tools, and a path cannot hold the second.
func sanitise(value string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(value))
	return strings.Trim(out, "-")
}

// FetchedRelative is the program inside the cache directory, as a path.
//
// Both halves of the recipe, in the order the archive puts them: the directory the
// archive unpacks into, and then the binary within it. A vendor's archive almost
// always has a top-level directory — `arm-gnu-toolchain-14.2.rel1-x86_64-arm-none-eabi/`
// — so a recipe that names only the binary inside it is naming a file at the root
// that is not there, and the toolchain that was fetched reads as missing.
//
// The default is `bin/<name>`, which is what an archive with no top-level
// directory holds and what every well-behaved one does.
func FetchedRelative(dep ExecutableDep) string {
	relative := "bin/" + dep.Name
	if dep.Fetch != nil {
		if binary := strings.TrimSpace(dep.Fetch.Binary); binary != "" {
			relative = binary
		}
	}
	if dep.Fetch == nil {
		return relative
	}
	if directory := strings.Trim(strings.TrimSpace(dep.Fetch.Dir), "/"); directory != "" {
		return path.Join(directory, relative)
	}
	return relative
}
