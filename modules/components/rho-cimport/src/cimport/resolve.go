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
// SPDX-License-Identifier: Apache-2.0
//

package cimport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
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
	for _, root := range r.presetRoots() {
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
// loadInherited follows a preset's `inherits` chain and returns the
// dependencies it ends at.
//
// A board preset says what makes it that board — the startup file for its part —
// and leaves the rest to its family. Reading the board's own dependency list and
// finding it empty says nothing about what the board builds against: it says the
// board did not repeat its family, which is the whole point of having one.
//
// The chain is followed rather than read once because a family may itself inherit,
// and a preset that names a preset which in turn names one is still a statement
// about what the board builds.
func loadInherited(root string, preset Preset) ([]CDep, error) {
	index, err := presetsDeclaredBy(root)
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	for _, one := range index {
		declared[one.Name] = true
	}

	current := preset
	for depth := 0; depth < 16; depth++ {
		if current.Inherits == "" {
			break
		}
		if !declared[current.Inherits] {
			return nil, fmt.Errorf("preset %q inherits %q, which is not one of this platform's presets",
				preset.Name, current.Inherits)
		}
		// The index says which presets exist; the file says what one of them is.
		// Following the chain through the index would arrive at a name with no
		// dependencies, which is what a board preset inherits its dependencies
		// from — so the family would report itself as empty and a board would
		// report that it had nothing to build against.
		parent, loadErr := loadPresetIn(root, current.Inherits, current.File)
		if loadErr != nil {
			return nil, loadErr
		}
		current = parent
	}
	return current.Dependencies, nil
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
// dependencyRoots are the modules a build may find a preset in.
//
// The project's own dependencies come first, because that is what the project
// asked for and nothing else should take precedence over it. After them come the
// platforms the interfaces themselves enumerate, which is the case of a program
// written against the interfaces and not against a platform: it said what it
// calls and did not say what implements it, and somewhere has to say which
// implementations exist.
//
// Paths come from the manifests. A root that is not there is not a dependency
// that failed to resolve — it is a dependency reached by git that this build has
// not fetched, and what it would provide is read by whoever fetches it.
func (r *PresetResolver) dependencyRoots() []string {
	document, err := loadDocument(r.ProjectRoot)
	if err != nil {
		return nil
	}
	roots := []string{}
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.ProjectRoot, path)
		}
		if seen[path] {
			return
		}
		seen[path] = true
		roots = append(roots, path)
	}

	for _, item := range document.Map["depends"].List {
		if item.Kind != yaml.KindMap {
			continue
		}
		add(readString(item.Map["path"]))
	}

	// The platforms the interfaces enumerate, which is where a build looks when
	// the project named the interfaces but not a platform. Resolved from the
	// interface module's own root, because that is the module that lists them.
	for _, root := range append([]string{}, roots...) {
		manifest, loadErr := deps.LoadManifest(deps.ManifestPath(root))
		if loadErr != nil || manifest == nil || len(manifest.Platforms) == 0 {
			continue
		}
		for _, platform := range manifest.Platforms {
			if platform.Path == "" {
				continue
			}
			path := platform.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			add(path)
		}
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
	// A platform declares where its presets are, under `hardware`, and that is
	// read first because it is the contract.
	//
	// It is read first because a manifest also has a top-level `presets`, and
	// reading that one instead finds nothing useful: a platform writes it as a
	// nested block, which parses as a map whose only key is the directory name,
	// and a preset described by a key is not a preset. Reading the declared
	// directory avoids depending on the shape of the older key at all.
	if hardware, ok := document.Map["hardware"]; ok && hardware.Kind == yaml.KindMap {
		own := []Preset{}
		if directory := readString(hardware.Map["presets"]); directory != "" {
			declared, err := presetsInDirectory(filepath.Join(root, directory))
			if err != nil {
				return nil, err
			}
			own = declared
		}
		// Then every board it declares, read through that board's own manifest.
		//
		// A board may have several presets, and the platform does not list them:
		// a list there would be a second thing to keep in step with the board, and
		// it would cap a board at one. So the boards are declared as a folder and
		// each one answers for itself.
		if boards := readString(hardware.Map["boards"]); boards != "" {
			for _, boardRoot := range boardModulesIn(filepath.Join(root, boards)) {
				declared, err := presetsDeclaredBy(boardRoot)
				if err != nil {
					return nil, err
				}
				own = append(own, declared...)
			}
		}
		return own, nil
	}

	// A manifest that inlines its presets, or lists them, is still read: both say
	// the same thing, and refusing one would make a platform change its layout and
	// its consumers at once.
	section := document.Map["presets"]
	if section.Kind == yaml.KindString && section.Str != "" {
		return presetsInDirectory(filepath.Join(root, section.Str))
	}
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

// LookupPreset returns the preset a program is being built for, by name.
//
// This is the same search `Resolve` performs, without the description it
// produces for a report. A caller that wants to *act* on a preset — to resolve
// the sources it declares, say — needs the preset itself, and reading the
// manifest a second time to get it would be a second reading of the same
// decision.
func (r *PresetResolver) LookupPreset(name string) (Preset, error) {
	if name == "" {
		return Preset{}, &ResolveError{Reason: "no preset was named"}
	}
	declared, err := r.declaredPresets()
	if err != nil {
		return Preset{}, err
	}
	if !listed(declared, name) {
		return Preset{}, &ResolveError{
			Preset:   name,
			Reason:   "this program does not declare that it is written for it",
			Declared: declared,
		}
	}
	for _, root := range r.dependencyRoots() {
		presets, err := presetsDeclaredBy(root)
		if err != nil {
			return Preset{}, err
		}
		for _, entry := range presets {
			if entry.Name != name {
				continue
			}
			// The index says which presets exist; the file says what one of them
			// is. Returning the index entry would return a preset with a name and
			// nothing else — and the dependencies, the sources and the overrides
			// are all in the file.
			preset, loadErr := loadPresetIn(root, entry.Name, entry.File)
			if loadErr != nil {
				return Preset{}, loadErr
			}
			// A board preset inherits its family's dependencies, so one that lists
			// none is not one that has none: it is one that says what makes it that
			// board and leaves the rest to its family.
			if len(preset.Dependencies) == 0 {
				inherited, inheritErr := loadInherited(root, preset)
				if inheritErr != nil {
					return Preset{}, inheritErr
				}
				preset.Dependencies = inherited
			}
			// The override is applied here rather than at each use, so that
			// everything downstream reads one dependency list: the sources that get
			// compiled, the include paths that get added, and the report that says
			// what the build is made of. A board narrows a family to the startup
			// file for its part; a build that compiled the family's whole
			// directory of them would compile a startup for every part in the
			// family, and they all define the same symbols.
			for i := range preset.Dependencies {
				if override, ok := preset.Override[preset.Dependencies[i].Name]; ok && len(override.Src) > 0 {
					preset.Dependencies[i].Src = override.Src
				}
			}
			return preset, nil
		}
	}
	return Preset{}, &ResolveError{
		Preset:   name,
		Reason:   "no dependency of this program provides it",
		Declared: declared,
	}
}

// loadPresetIn reads the preset a platform's index names.
//
// The index is a table of contents and the file is the text, and a caller that
// wants to know what a preset *is* has to read the file. The file is taken from
// the index where it says one, and by name otherwise, because a platform may
// name its presets for what they are — `board_nucleo-h753zi.json` — while the
// preset inside is called `nucleo-h753zi`.
func loadPresetIn(root, name, file string) (Preset, error) {
	if file == "" {
		return LoadPresetByName(filepath.Join(root, "presets"), name)
	}
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	preset, err := LoadPreset(path)
	if err != nil {
		return Preset{}, err
	}
	if preset.Name == "" {
		preset.Name = name
	}
	preset.Root = root
	return preset, nil
}

// presetsInDirectory reads every preset a declared directory holds.
//
// The file name is not the preset's name: files are named for what they are
// (`board_nucleo-h753zi.json`) and the name is inside, because a preset inherits
// and refers to its family by name and the two have to be the same string.
func presetsInDirectory(dir string) ([]Preset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A directory that is not there is not a platform with no presets; it is a
		// platform that has not been laid out yet, and the caller reports that
		// when it looks for something by name.
		return nil, nil
	}
	presets := []Preset{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".json", ".yaml", ".yml":
		default:
			continue
		}
		preset, loadErr := LoadPreset(filepath.Join(dir, entry.Name()))
		if loadErr != nil || preset.Name == "" {
			continue
		}
		if preset.File == "" {
			preset.File = filepath.Join(dir, entry.Name())
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

// RootOf returns the dependency root that provides a named preset.
//
// The root matters as much as the preset: a preset says what to build against,
// and the module that provides it is what that applies to. A board's support
// package belongs to the platform, so looking for it beside the *project* would
// look in a tree that has no boards in it, and looking for it by walking
// upwards would be guessing where the platform was checked out.
func (r *PresetResolver) RootOf(name string) (string, error) {
	index, err := r.declaredPresets()
	if err != nil {
		return "", err
	}
	if !listed(index, name) {
		return "", &ResolveError{
			Preset:   name,
			Reason:   "this program does not declare that it is written for it",
			Declared: index,
		}
	}
	for _, root := range r.presetRoots() {
		presets, err := presetsDeclaredBy(root)
		if err != nil {
			return "", err
		}
		for _, preset := range presets {
			if preset.Name == name {
				return root, nil
			}
		}
	}
	return "", &ResolveError{
		Preset:   name,
		Reason:   "no dependency of this program provides it",
		Declared: index,
	}
}

// presetRoots returns every place a preset may be declared: the platform's own,
// and the `presets` each of its boards carries.
//
// A preset that says which board it is for belongs to that board, and the board
// is not a dependency of the program: a program depends on the platform, and the
// platform implements every board of the family at once. So a board's presets are
// found through the platform that owns the boards, not through a dependency graph
// that does not contain them.
func (r *PresetResolver) presetRoots() []string {
	roots := []string{}
	seen := map[string]bool{}
	for _, root := range r.dependencyRoots() {
		if seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

// boardModulesIn returns the module root of every board in a declared directory.
//
// A directory entry counts as a board when it carries a manifest, which is what
// separates a board from anything else that happens to be a folder beside it.
func boardModulesIn(boardsDir string) []string {
	entries, err := os.ReadDir(boardsDir)
	if err != nil {
		return nil
	}
	roots := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		boardRoot := filepath.Join(boardsDir, entry.Name())
		if _, err := deps.LoadManifest(deps.ManifestPath(boardRoot)); err != nil {
			continue
		}
		roots = append(roots, boardRoot)
	}
	return roots
}
