//
// @file config.go
// @brief What a project declared, and the budgets in it.
//
// The `mem:` block is a project's own declaration, read through the manifest
// model every component shares — so what `rho mem` checks is the same block a
// person can read in `module.yml`, parsed by the same parser that validates it,
// rather than a second reading written next to it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"fmt"
	"path/filepath"
	"strings"

	"rho/src/deps"
)

// Config is a project's memory declaration, as this component runs on it.
type Config struct {
	// MapFile and ELFFile are the artifacts to read, relative to the project.
	// Empty means this component finds them.
	MapFile string
	ELFFile string
	// Baseline is where the figures are recorded, relative to the project.
	Baseline string
	// Budgets are the per-region budgets, keyed by region name.
	Budgets map[string]Budget
	// Enforce is what a check is allowed to fail on.
	Enforce Enforcement
	// Declared says whether the project wrote a `mem:` block at all, which is
	// different from one that wrote an empty one: a project with no budgets is a
	// project that has not decided yet.
	Declared bool
	// Warnings is what could not be read, reported rather than dropped.
	Warnings []string
	// Toolchain is the board or chip the project declared it builds for, from
	// `build.toolchain`.
	Toolchain string
	// Preset is the board the project said it is written for, from
	// `build.presets`, when it named one rather than a toolchain.
	Preset string
}

// DefaultBaselineFile is where a project's figures are recorded when it did not
// say.
//
// Beside the manifest rather than inside `build/`, because it is a thing to be
// committed and reviewed: a number that only exists in a build directory cannot be
// diffed on a pull request, which is the only moment anybody is in a position to
// object to a firmware growing.
const DefaultBaselineFile = "mem.baseline.json"

// ReadConfig is a project's manifest, read for its memory declaration.
func ReadConfig(projectRoot string) Config {
	out := Config{Budgets: map[string]Budget{}}
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return out
	}
	if manifest.Mem == nil {
		return out
	}

	out.Declared = true
	declared := manifest.Mem
	out.Toolchain = manifest.BuildToolchain
	if len(manifest.BuildPresets) > 0 {
		out.Preset = manifest.BuildPresets[0]
	}
	out.MapFile = declared.MapFile
	out.ELFFile = declared.ELFFile
	out.Baseline = declared.Baseline
	for name, region := range declared.Regions {
		out.Budgets[name] = Budget{
			Name:           name,
			MaxBytes:       region.MaxBytes,
			WarnAt:         region.WarnAt,
			WarnAtDeclared: region.WarnAtDeclared,
		}
	}
	out.Enforce = Enforcement{
		FailOnOverflow:       declared.Enforce.FailOnOverflow,
		FailOnGrowthBytes:    declared.Enforce.FailOnGrowthBytes,
		FailOnGrowthDeclared: declared.Enforce.FailOnGrowthDeclared,
	}
	return out
}

// Resolve is where the artifacts to read actually are.
//
// The project says what it wants read; this says what was found, and which of the
// two was used. A project that named a path which is not there is told so rather
// than quietly analysed against whatever was found instead — a budget checked
// against the wrong build is worse than no check, because it passes.
func (c Config) Resolve(projectRoot string) (Artifacts, []string) {
	notes := []string{}
	out := Artifacts{}

	mapPath := c.MapFile
	if mapPath != "" {
		mapPath = absolute(projectRoot, mapPath)
		if !isFile(mapPath) {
			notes = append(notes, fmt.Sprintf(
				"`mem.map_file` names %s, which is not there; the program was read on its own", c.MapFile))
			mapPath = ""
		}
	}
	elfPath := c.ELFFile
	if elfPath != "" {
		elfPath = absolute(projectRoot, elfPath)
		if !isFile(elfPath) {
			notes = append(notes, fmt.Sprintf(
				"`mem.elf_file` names %s, which is not there; the linker map was read on its own", c.ELFFile))
			elfPath = ""
		}
	}

	// Both named: the project said exactly what to read, and what was found is
	// irrelevant.
	if mapPath != "" || elfPath != "" {
		out.MapFile, out.ELFFile = mapPath, elfPath
		return out, notes
	}

	// Neither named: this is where a build puts them, so that is where they are
	// read from. A project that names its output directory has named where its
	// program is, and the map is beside it.
	found := Find(projectRoot, "")
	out.MapFile, out.ELFFile = found.MapFile, found.ELFFile
	notes = append(notes, found.Notes...)
	if out.ELFFile == "" && out.MapFile == "" {
		notes = append(notes, "no built program and no linker map were found under `build/`; "+
			"build the project first, or name the artifacts under `mem:`")
	}
	return out, notes
}

// BaselinePath is where a project's figures are recorded.
func (c Config) BaselinePath(projectRoot string) string {
	name := c.Baseline
	if strings.TrimSpace(name) == "" {
		name = DefaultBaselineFile
	}
	return absolute(projectRoot, name)
}

// TargetOf is the board or chip a build was for, which is what a report is titled
// with and what a baseline is checked against.
//
// Read from the project's own `build.toolchain` where it declared one, because
// that is the declaration and it is the answer for a project that names its board.
// Failing that, the directory a CMake build ran in, which is where the generated
// tree writes it down for a project that builds for several boards and was asked
// about one of them.
//
// Empty is a real answer and means "not known": a baseline for one board compared
// against a build for another is a meaningless number, so a run that does not know
// says so rather than guessing from the file it happens to be reading.
func TargetOf(artifacts Artifacts, config Config, projectRoot string) string {
	if config.Toolchain != "" {
		return config.Toolchain
	}
	if artifacts.MapFile != "" {
		if target := targetFromPath(artifacts.MapFile); target != "" {
			return target
		}
	}
	if artifacts.ELFFile != "" {
		if target := targetFromPath(artifacts.ELFFile); target != "" {
			return target
		}
	}
	if config.Preset != "" {
		return config.Preset
	}
	return ""
}

func absolute(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, filepath.FromSlash(path))
}
