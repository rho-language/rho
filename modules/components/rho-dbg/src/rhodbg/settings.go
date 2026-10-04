//
// @file settings.go
// @brief What this component is set to, read from its own manifest and its own file.
//
// The contract is declared in the component's manifest and the values are in the
// file that manifest names, so what a person reads to find out what can be
// configured is the same thing the component runs on. Reading them is the same
// reading rho-keeper does, because two components reading their configuration two
// ways is one of them reading it wrong.
//
// The values are read when a command runs, not once when this is compiled, because
// a person changes the file and then runs again — and a component that had already
// read it would be fetching from wherever it was told to fetch from before the file
// said so.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// Settings is what a session may be configured with.
//
// Everything here is about one thing: where a chip's register map comes from. The
// rest of a session is decided by the board's target spec, because the rest of it is
// a fact about the board rather than a choice anybody makes about a tool.
type Settings struct {
	// Repository is where a register map is fetched from. Empty means a session
	// reads only what a project carries and never reaches for the network.
	Repository string
	// Ref is the branch or tag of that repository a file is taken from.
	Ref string
	// Cache is where a fetched file is kept.
	Cache string
	// Fetch is whether to fetch one when nothing has it.
	Fetch bool
	// CacheHome is the Rho home the cache path is relative to, kept because the
	// cache is shared by every project on the machine and a person looking at the
	// files needs to be told where they are.
	CacheHome string
}

// DefaultRepository is the CMSIS device data repository, which is where every
// vendor's register map is collected.
const DefaultRepository = "https://github.com/cmsis-svd/cmsis-svd-data"

// DefaultRef is the branch of it a machine takes files from by default: the
// current one, because a machine that has never fetched one has no reason to
// prefer a version of the vendor's data over a newer one.
const DefaultRef = "main"

// DefaultCache is where fetched register maps are kept, relative to the Rho home.
const DefaultCache = "cache/svd"

// ReadSettings is this component's manifest and the file it names.
//
// The manifest is parsed with the loader every manifest is parsed with, so the
// contract is the toolchain's reading of it rather than a second reading written
// next to it. A manifest that cannot be read has declared no contract, and the
// built-in defaults are better than refusing a session over a file that was never
// going to be read.
func ReadSettings() Settings {
	out := Settings{
		Repository: DefaultRepository,
		Ref:        DefaultRef,
		Cache:      DefaultCache,
		Fetch:      true,
		CacheHome:  deps.RhoHome(),
	}

	root := ModuleRoot()
	if root == "" {
		return out
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return out
	}

	if manifest.Settings != nil {
		for _, setting := range manifest.Settings.Settings {
			if setting.Default == "" {
				continue
			}
			switch setting.Name {
			case "svd_repository":
				out.Repository = setting.Default
			case "svd_branch":
				out.Ref = setting.Default
			case "svd_cache":
				out.Cache = setting.Default
			case "svd_fetch":
				out.Fetch = !strings.EqualFold(setting.Default, "false")
			}
		}
		// The file is where the values are. It travels with the module, so a
		// rho-dbg installed from a registry reads the same file wherever it lands.
		if manifest.Settings.File == "" {
			return out
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifest.Settings.File)))
		if readErr != nil {
			// A file that is not there is a component nobody has changed, which is
			// not a failure. A file that is there and unreadable is reported by the
			// caller that cares, rather than here: a session can debug a board with
			// no register map, and one that cannot be told where it was going to
			// look has lost nothing it could have shown.
			return out
		}
		document, parseErr := yaml.Load(string(data))
		if parseErr != nil {
			return out
		}
		for key, value := range document.Map {
			if value.Kind == yaml.KindMap || value.Kind == yaml.KindList {
				continue
			}
			switch key {
			case "svd_repository":
				out.Repository = strings.TrimSpace(value.Str)
			case "svd_branch":
				out.Ref = strings.TrimSpace(value.Str)
			case "svd_cache":
				out.Cache = strings.TrimSpace(value.Str)
			case "svd_fetch":
				out.Fetch = !strings.EqualFold(strings.TrimSpace(value.Str), "false")
			}
		}
	}
	return out
}

// CacheDirectory is where fetched register maps are kept.
//
// A path is read as it is given when it is absolute and as relative to the Rho home
// when it is not — which is what a path written in a config file means, and what
// makes the same file work on a machine whose Rho home is somewhere else.
func (s Settings) CacheDirectory() string {
	if strings.TrimSpace(s.Cache) == "" {
		return ""
	}
	if filepath.IsAbs(s.Cache) {
		return filepath.Clean(s.Cache)
	}
	if strings.TrimSpace(s.CacheHome) == "" {
		// No Rho home to be relative to. Said rather than guessed: a cache written
		// beside the working directory is one that a project would find, and a
		// project's files are somewhere people commit from.
		return ""
	}
	return filepath.Join(s.CacheHome, filepath.FromSlash(s.Cache))
}

// Fetchable is whether a session may reach for the network for a register map.
func (s Settings) Fetchable() bool {
	return s.Fetch && strings.TrimSpace(s.Repository) != "" && s.CacheDirectory() != ""
}

// ModuleRoot is where this component's own files are.
//
// The environment first, because a component run from a checkout is told where the
// checkout is by whoever runs it; then the directory the binary is in, because a
// component installed from a registry has its manifest and its settings file beside
// the binary — which is what makes a rho-dbg configured by its manifest rather than
// by wherever the installation happened to put things.
func ModuleRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_MODULE_ROOT")); root != "" {
		return root
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	return ""
}
