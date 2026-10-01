//
// @file resolve.go
// @brief Finding the preset a build is for, and the target that goes with it.
//
// A build that names a preset has to find it, and the finding is a contract
// between three files that are allowed to disagree:
//
//   - the application's manifest says which presets the program is written for;
//   - each dependency's manifest says which presets it provides;
//   - the preset's own descriptor says what to fetch, what hardware it is for,
//     and which target builds it.
//
// They are three files because a program is written for a board and not for a
// platform, a platform supports many boards, and neither should be edited when
// the other grows. What they must not do is disagree quietly, so every step
// that can fail says which step refused and what there was instead.
//
// The resolution produces both halves of what a build needs: the C sources to
// fetch, and the target to build them for. They are one answer because a preset
// that named one without the other would be a program built for a board it does
// not belong to — which is the one mistake a target list is there to prevent.
//
// This is the embedded side's resolution, and it shares nothing with the Rho
// module resolver. One walks a registry for the newest matching version of a
// component; this walks a handful of manifests for the one preset a program
// named, and fetches repositories rather than installing anything.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/yaml"
)

// PresetResolution is everything a build needs to know about the preset it was named,
// and nothing else. It is what a build reads, so it is the shape of the answer
// rather than the internals that produced it.
type PresetResolution struct {
	// Preset is the one that was asked for.
	Preset string `json:"preset"`
	// Description is what it is for.
	Description string `json:"description,omitempty"`

	// The hardware, described.
	Family       string `json:"family,omitempty"`
	MCU          string `json:"mcu,omitempty"`
	Core         string `json:"core,omitempty"`
	DeviceDefine string `json:"device_define,omitempty"`
	Board        string `json:"board,omitempty"`

	// Target is the rho-target spec that builds this preset, and Specs is where
	// those specs live — a platform's own targets directory, handed over so the
	// build does not have to know which platform it came from.
	Target string `json:"target"`
	Specs  string `json:"specs,omitempty"`

	// Inherits is the preset whose C sources this one builds, when this one is
	// for a board rather than for a family. A board and its part share their
	// repositories; what differs is the memory map and the flash command, and
	// those are the target's. So a board preset says which family it belongs to
	// and the two halves stay where they belong.
	Inherits string `json:"inherits,omitempty"`

	// Repositories are what the C side fetches.
	Repositories []CDep `json:"repositories"`

	// Override narrows what this preset inherited, by repository name. It is the
	// board's half: the family says which repositories, and a board says which
	// of each repository's files belong to it.
	Override map[string]Override `json:"override,omitempty"`

	// ProjectRoot is the application this was resolved for.
	ProjectRoot string `json:"project_root"`
	// ProvidedBy is the manifest that declared the preset.
	ProvidedBy string `json:"provided_by,omitempty"`
}

// ResolveError is a preset that could not be resolved.
type ResolveError struct {
	Preset     string
	Reason     string
	Candidates []string
	// Declared is what the program said it is written for, when the refusal was
	// that it did not say this one.
	Declared []string
}

func (e *ResolveError) Error() string {
	message := fmt.Sprintf("preset %q: %s", e.Preset, e.Reason)
	switch {
	case len(e.Declared) > 0:
		message += "\n  this program is written for: " + strings.Join(e.Declared, ", ")
	case len(e.Candidates) > 0:
		message += "\n  the presets a dependency provides are: " +
			strings.Join(e.Candidates, ", ")
	}
	return message
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

// PresetResolver finds a preset for a project, among the projects that project
// depends on.
type PresetResolver struct {
	// ProjectRoot is the application the build is for.
	ProjectRoot string
}

// Resolve finds a named preset and reports what building it needs.
//
// The order is the contract. A preset the program did not declare is refused
// first, because a program is written against a board and building it for
// another compiles something its author never checked — and that is refused
// before any dependency is read, so a typo is reported as a typo rather than as
// something every dependency disagreed about. Only then are the dependencies
// searched, in the order the program listed them, and the first that provides
// the preset is the one that provides it.
func (r *PresetResolver) Resolve(name string) (PresetResolution, error) {
	if name == "" {
		return PresetResolution{}, &ResolveError{Reason: "no preset was named"}
	}

	declared, err := r.declaredPresets()
	if err != nil {
		return PresetResolution{}, err
	}
	if !listed(declared, name) {
		return PresetResolution{}, &ResolveError{
			Preset:   name,
			Reason:   "this program does not declare that it is written for it",
			Declared: declared,
		}
	}

	available := []string{}
	for _, root := range r.dependencyRoots() {
		presets, err := presetsDeclaredBy(root)
		if err != nil {
			return PresetResolution{}, err
		}
		for _, preset := range presets {
			if preset.Name == name {
				return r.complete(preset)
			}
			available = append(available, preset.Name)
		}
	}

	return PresetResolution{}, &ResolveError{
		Preset:     name,
		Reason:     "no dependency of this program provides it",
		Candidates: available,
	}
}

// complete fills in the parts of a preset that its declaration points at rather
// than repeats.
//
// A platform's manifest says which presets it has and where each one is written
// at length. Repeating the hardware in both places would be a second place for
// it to be wrong, so the declaration is the index and the file is the text.
func (r *PresetResolver) complete(preset Preset) (PresetResolution, error) {
	resolution := PresetResolution{
		Preset:      preset.Name,
		Description: preset.Description,
		Family:      preset.Family,
		MCU:         preset.MCU,
		Core:        preset.Core,
		Target:      preset.Target,
		Board:       preset.Board,
		ProjectRoot: r.ProjectRoot,
	}

	// The long form, when the declaration names a file for it.
	if file := preset.File; file != "" && (preset.MCU == "" || len(preset.Dependencies) == 0) {
		full := file
		if !filepath.IsAbs(full) {
			full = filepath.Join(preset.Root, full)
		}
		loaded, err := LoadPreset(full)
		if err != nil {
			return PresetResolution{}, fmt.Errorf("preset %q: %w", preset.Name, err)
		}
		if resolution.Description == "" {
			resolution.Description = loaded.Description
		}
		if resolution.MCU == "" {
			resolution.MCU = loaded.MCU
			resolution.Family = loaded.Family
			resolution.Core = loaded.Core
		}
		if resolution.DeviceDefine == "" {
			resolution.DeviceDefine = loaded.DeviceDefine
		}
		if resolution.Inherits == "" {
			resolution.Inherits = loaded.Inherits
		}
		if len(loaded.Override) > 0 {
			resolution.Override = loaded.Override
		}
		resolution.Repositories = loaded.Dependencies
		if len(loaded.Override) > 0 {
			resolution.Override = loaded.Override
		}
	}
	if len(resolution.Repositories) == 0 {
		resolution.Repositories = preset.Dependencies
	}
	if resolution.Target == "" {
		resolution.Target = preset.Name
	}

	// A board preset says which family's sources it builds. Following it here
	// rather than copying them into every board is what keeps one repository list
	// per family: a revision that moves is changed in one file, and every board on
	// that part picks it up.
	if resolution.Inherits != "" && len(resolution.Repositories) == 0 {
		inherited, err := presetsDeclaredBy(preset.Root)
		if err == nil {
			for _, candidate := range inherited {
				if candidate.Name != resolution.Inherits {
					continue
				}
				loaded, loadErr := loadInherited(preset.Root, candidate)
				if loadErr == nil {
					resolution.Repositories = loaded
				}
			}
		}
	}
	if len(resolution.Repositories) == 0 && resolution.Inherits != "" {
		return PresetResolution{}, &ResolveError{
			Preset: preset.Name,
			Reason: "it builds from the " + resolution.Inherits + " sources, and no dependency declares them",
		}
	}

	// The specs directory is the one the preset's own platform declared, so that
	// the build hands over the hardware's target specs and not a guess at where
	// they are.
	if dir := specDirOf(preset.Root); dir != "" {
		resolution.Specs = dir
	}
	if resolution.Target == "" {
		return PresetResolution{}, &ResolveError{
			Preset: preset.Name,
			Reason: "it names no target to build for, and a preset without one is a description of hardware nothing builds",
		}
	}
	return resolution, nil
}

// specDirOf is the directory a platform's manifest declares its target specs in.
//
// It is read from the manifest rather than assumed, because "targets" is a
// convention this would otherwise be inventing: a platform that keeps them
// somewhere else has said where, and a build that looked in the wrong place
// would report a target that does not exist rather than one that does.
func specDirOf(root string) string {
	for _, name := range []string{"rho.mod", "module.yml"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		document, err := yaml.Load(string(content))
		if err != nil {
			return ""
		}
		// `targets` may be a directory name, or a mapping with a `dir`.
		build := document.Map["build"]
		value := readString(build.Map["targets"])
		if value == "" {
			value = readString(document.Map["targets"])
		}
		if targets := document.Map["targets"]; value == "" && targets.Kind == yaml.KindMap {
			value = readString(targets.Map["dir"])
		}
		if value == "" {
			return ""
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(root, value)
		}
		if info, err := os.Stat(value); err == nil && info.IsDir() {
			return value
		}
	}
	return ""
}

// loadInherited reads the preset a board preset inherits its sources from.
func loadInherited(root string, preset Preset) ([]CDep, error) {
	file := preset.File
	if file == "" {
		file = filepath.Join("presets", preset.Name+".json")
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	loaded, err := LoadPreset(file)
	if err != nil {
		return nil, err
	}
	return loaded.Dependencies, nil
}

/* -------------------------------------------------------------------------- */
/*  The program's declarations                                                 */
/* -------------------------------------------------------------------------- */

// declaredPresets is the set of presets the program says it is written for.
func (r *PresetResolver) declaredPresets() ([]string, error) {
	document, err := loadDocument(r.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if document.Kind != yaml.KindMap {
		return nil, nil
	}
	build := document.Map["build"]
	if build.Kind != yaml.KindMap {
		return nil, nil
	}
	return readStrings(build.Map["presets"]), nil
}

// dependencyRoots is where this program's dependencies were resolved to, in the
// order it named them.
//
// The paths come from the manifest rather than from a search: a build resolves
// the graph first and then asks what those resolved directories declare, so that
// the answer is about the dependencies the build actually has rather than about
// whatever happens to be lying beside the project.
func (r *PresetResolver) dependencyRoots() []string {
	document, err := loadDocument(r.ProjectRoot)
	if err != nil {
		return nil
	}
	roots := []string{}
	for _, item := range document.Map["depends"].List {
		if item.Kind != yaml.KindMap {
			continue
		}
		path := readString(item.Map["path"])
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.ProjectRoot, path)
		}
		roots = append(roots, path)
	}
	return roots
}

// loadDocument reads a project's manifest, by either name.
func loadDocument(root string) (yaml.Value, error) {
	for _, name := range []string{"rho.mod", "module.yml"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		return yaml.Load(string(content))
	}
	return yaml.Value{}, fmt.Errorf("no manifest in %s", root)
}

/* -------------------------------------------------------------------------- */
/*  A platform's declarations                                                  */
/* -------------------------------------------------------------------------- */

// presetsDeclaredBy reads the presets a manifest declares.
//
// Either a list of them, or one written inline — the right shape for a platform
// that supports one thing, and a list of one is a list a reader looks past.
func presetsDeclaredBy(root string) ([]Preset, error) {
	document, err := loadDocument(root)
	if err != nil {
		return nil, err
	}
	if document.Kind != yaml.KindMap {
		return nil, nil
	}
	section := document.Map["presets"]
	if section.Kind != yaml.KindMap && section.Kind != yaml.KindList {
		return nil, nil
	}
	items := section.List
	if section.Kind == yaml.KindMap {
		items = []yaml.Value{section}
	}

	presets := []Preset{}
	for _, item := range items {
		if item.Kind != yaml.KindMap {
			continue
		}
		presets = append(presets, Preset{
			Name:        readString(item.Map["name"]),
			Description: readString(item.Map["description"]),
			Family:      readString(item.Map["family"]),
			MCU:         readString(item.Map["mcu"]),
			Core:        readString(item.Map["core"]),
			Board:       readString(item.Map["board"]),
			Target:      readString(item.Map["target"]),
			Inherits:    readString(item.Map["inherits"]),
			File:        readString(item.Map["file"]),
			Root:        root,
		})
	}
	return presets, nil
}

/* -------------------------------------------------------------------------- */
/*  Reporting                                                                  */
/* -------------------------------------------------------------------------- */

// WriteResolution prints a resolution as JSON, which is what a build reads.
//
// JSON rather than a format of this component's own, because what is read here
// is read by something else — the toolchain, handing the target and the
// repositories to the components that act on them — and a format one side
// invented is a format the other has to be taught.
func WriteResolution(w *os.File, resolution PresetResolution) error {
	encoded, err := json.MarshalIndent(resolution, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(encoded, '\n'))
	return err
}

// ReadResolution reads a resolution written by WriteResolution.
func ReadResolution(data []byte) (PresetResolution, error) {
	resolution := PresetResolution{}
	if err := json.Unmarshal(data, &resolution); err != nil {
		return PresetResolution{}, err
	}
	return resolution, nil
}

/* -------------------------------------------------------------------------- */
/*  Small helpers                                                              */
/* -------------------------------------------------------------------------- */

// listed reports whether a value is among a list. It is not called `contains`
// because that name is a substring test in the tests beside it, and a reader
// reaching for one and getting the other is a test that passes and a bug that
// does not.
func listed(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// sortedUnique is a list with its duplicates gone, in order. A report of what was
// considered reads the same twice.
func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
