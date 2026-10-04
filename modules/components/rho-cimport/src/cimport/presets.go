//
// @file presets.go
// @brief A preset: everything a build needs to target one piece of hardware.
//
// A preset is a self-contained description of a target's C side: which
// repositories, at which revisions, which directories of them go on the include
// path, which files are compiled, and what the hardware is. An application says
// which one it builds against, and the build is arranged from it.
//
// The shape is the same as a dependency's, with the descriptive facts alongside.
// That is deliberate: a preset is what somebody reads to find out how a build
// was produced, and a record of four repositories with four revisions and
// twelve directories says nothing about what it was for. So the MCU, its core,
// the device define the HAL selects on and the board are in the same place as
// the paths, and a report can print one and be understood.
//
// There is exactly one preset per application. Not a choice among several: the
// application knows what it is for, and a build that let a program pick its own
// hardware would be a build nobody could reproduce from reading the program. An
// application that supports more than one board ships more than one
// application, or names a different preset in its manifest, and both are one
// decision in one file.
//
// This component has no opinion about which presets exist. A preset is a fact
// about a piece of hardware, and the component that carries the hardware writes
// it: the presets for an STM32 are written by the STM32 platform module, and the
// presets for anything else by whatever platform that is. What lives here is the
// shape and the reader, so this component is the same for every part.
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
	"strings"

	"rho/src/yaml"
)

// Preset is everything a build needs to target one piece of hardware, and what
// it is.
//
// The descriptive half — MCU, core, device define, board — is not decoration.
// A link failure is diagnosed against the device define, a memory map is
// interpreted against the part, and a person reading a build report needs to know
// which part it was for. All of that belongs beside the paths rather than in
// another file that can disagree with them.
type Preset struct {
	// Name is what the application calls this preset.
	Name string
	// Description is what this preset is for, in a sentence.
	Description string

	// The hardware. Descriptive: it changes what is compiled and how, and it is
	// what a report prints to say what was built for.
	Family        string
	MCU           string
	Core          string
	DeviceDefine  string
	Board         string
	FlashOffset   string
	ExternalClock string

	// Target is the rho-target spec that builds this preset. A preset says what
	// to fetch and a target says how, and the two belong together: a program
	// built for a board with another board's compiler and memory map is a program
	// that does not run.
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// Inherits is the preset whose C sources this one builds, when this one is
	// for a board rather than for a family.
	Inherits string `json:"inherits,omitempty" yaml:"inherits,omitempty"`

	// File is where this preset is written at length, relative to the manifest
	// that declares it. A platform's manifest indexes its presets and this is
	// the text they point at, so a list of presets stays a list.
	File string `json:"file,omitempty" yaml:"file,omitempty"`

	// Root is the directory the preset was declared in. Not written by hand: it
	// is filled in by whoever read the declaration, so a preset knows where it
	// came from without a reader having to pass it down.
	Root string `json:"-" yaml:"-"`

	// The C side: what is fetched, and what of it is used.
	// Override narrows a repository this preset inherits from a family, by
	// repository name.
	//
	// A family gives the repositories; a board narrows them to its own part. The
	// CMSIS device repository carries a startup assembly for every part of the
	// family and they all define the same symbols, so a board that inherited the
	// family as written would assemble twenty-two vector tables and link none of
	// them. The board knows which part it carries, so the board says which startup.
	Override map[string]Override `json:"override,omitempty" yaml:"override,omitempty"`

	Dependencies []CDep
}

// Override is what a board narrows an inherited repository to.
type Override struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Src     []string `json:"src,omitempty" yaml:"src,omitempty"`
	Why     string   `json:"why,omitempty" yaml:"why,omitempty"`
}

// Validate reports whether a preset says enough to build against.
func (p Preset) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("a preset names nothing")
	}
	if len(p.Dependencies) == 0 {
		return fmt.Errorf("preset %q lists no dependencies, so there is nothing to build against", p.Name)
	}
	for _, dep := range p.Dependencies {
		if err := dep.Validate(); err != nil {
			return fmt.Errorf("preset %q: %w", p.Name, err)
		}
	}
	return nil
}

// Describe is the descriptive half, for a report or an error.
func (p Preset) Describe() string {
	parts := []string{}
	if p.MCU != "" {
		parts = append(parts, p.MCU)
	}
	if p.Core != "" {
		parts = append(parts, p.Core)
	}
	if p.DeviceDefine != "" {
		parts = append(parts, p.DeviceDefine)
	}
	if p.Board != "" {
		parts = append(parts, p.Board)
	}
	if len(parts) == 0 {
		return p.Name
	}
	return p.Name + " (" + strings.Join(parts, ", ") + ")"
}

/* -------------------------------------------------------------------------- */
/*  Reading a preset                                                           */
/* -------------------------------------------------------------------------- */

// presetField is the manifest field a preset is declared under.
const presetField = "preset"

// ReadPreset reads the preset an application builds against.
//
// The preset is either written in the manifest or named by a file the manifest
// points at. Both exist because a preset is long — three repositories, three
// revisions, a dozen directories and the hardware they are for — and a long
// thing written inline makes the manifest unreadable, while a preset split
// between two files has no single place a reader looks to find out what a
// program is built against.
//
// A project that declares no preset is not an error. Most programs are not for
// anything in particular, and a reader that refused to answer without one would
// be a reader every build had to guard.
func ReadPreset(projectRoot string) (Preset, error) {
	for _, name := range []string{"rho.mod", "module.yml"} {
		path := filepath.Join(projectRoot, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		document, err := yaml.Load(string(content))
		if err != nil {
			return Preset{}, fmt.Errorf("%s: %w", name, err)
		}

		field := document.Map[presetField]
		switch field.Kind {
		case yaml.KindMap:
			// Written inline.
			return presetFromMap(field, name), nil
		case yaml.KindString:
			// Named by a file, relative to the manifest that names it.
			target := field.Str
			if target == "" {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(projectRoot, target)
			}
			return LoadPreset(target)
		default:
			return Preset{}, nil
		}
	}
	return Preset{}, nil
}

// LoadPreset reads a preset from a file, in either format.
func LoadPreset(path string) (Preset, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Preset{}, fmt.Errorf("cannot read the preset: %w", err)
	}
	return ParsePreset(path, content)
}

// ParsePreset reads a preset out of a document.
//
// The same field names are read whichever format wrote them, so a preset means
// the same thing whether it was written as JSON or as YAML. Two readers would
// make it mean two things, and the difference would be found by a build that
// failed rather than by a reader that disagreed.
func ParsePreset(source string, content []byte) (Preset, error) {
	if strings.EqualFold(filepath.Ext(source), ".json") {
		var preset Preset
		if err := json.Unmarshal(content, &preset); err != nil {
			return Preset{}, fmt.Errorf("%s: cannot read the preset: %w", source, err)
		}
		return preset, nil
	}
	document, err := yaml.Load(string(content))
	if err != nil {
		return Preset{}, fmt.Errorf("%s: %w", source, err)
	}
	if document.Kind != yaml.KindMap {
		return Preset{}, fmt.Errorf("%s: a preset is a mapping", source)
	}
	return presetFromMap(document, source), nil
}

// presetFromMap reads one preset out of a mapping.
func presetFromMap(item yaml.Value, source string) Preset {
	preset := Preset{
		Name:          readString(item.Map["name"]),
		Description:   readString(item.Map["description"]),
		Family:        readString(item.Map["family"]),
		MCU:           readString(item.Map["mcu"]),
		Core:          readString(item.Map["core"]),
		DeviceDefine:  readString(item.Map["device_define"]),
		Board:         readString(item.Map["board"]),
		FlashOffset:   readString(item.Map["flash_offset"]),
		ExternalClock: readString(item.Map["external_clock"]),
	}

	// The dependencies may be under `dependencies`, or written directly in the
	// preset as a list of repositories, which is the shape a preset read by a
	// person tends to take. Both are read, because a preset that is a list of
	// repositories plus some descriptions about them is the same statement.
	entries := item.Map["dependencies"].List
	if len(entries) == 0 {
		entries = item.Map["repositories"].List
	}
	for _, entry := range entries {
		if entry.Kind != yaml.KindMap {
			continue
		}
		preset.Dependencies = append(preset.Dependencies, CDep{
			Name:    readString(entry.Map["name"]),
			Git:     readString(entry.Map["git"]),
			Branch:  readString(entry.Map["branch"]),
			Tag:     readString(entry.Map["tag"]),
			Path:    readString(entry.Map["path"]),
			Include: readStrings(entry.Map["include"]),
			Src:     readStrings(entry.Map["src"]),
			Source:  source,
		})
	}
	return preset
}

/* -------------------------------------------------------------------------- */
/*  One preset out of a library                                               */
/* -------------------------------------------------------------------------- */

// LoadPresetByName reads one preset out of a directory of them, by name.
//
// It reads the one file that could hold it rather than reading the directory
// and choosing. A platform holds a preset per board it supports, and a build
// needs one: reading forty of them to use one is forty parses of data the build
// will not look at, and it is paid on every build of every application that
// platform supports.
//
// The cost of that is a convention — a preset named `nucleo-f446re` lives in
// `nucleo-f446re.yaml` or `nucleo-f446re.json` — and the convention is stated
// rather than discovered, so a preset in a file named something else is one a
// caller names by path instead.
func LoadPresetByName(dir, name string) (Preset, error) {
	if name == "" {
		return Preset{}, fmt.Errorf("no preset was named")
	}
	tried := []string{}
	for _, extension := range []string{".yaml", ".yml", ".json"} {
		candidate := filepath.Join(dir, name+extension)
		tried = append(tried, filepath.Base(candidate))
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		preset, err := LoadPreset(candidate)
		if err != nil {
			return Preset{}, err
		}
		// A preset whose file is named for it is read as named, so that a report
		// can name it without reading the file. A preset that names something
		// else is left alone: the file name is what the caller asked for, and
		// overriding it with the file's contents would make the name in a
		// manifest and the name in a report two different names.
		if preset.Name == "" {
			preset.Name = name
		}
		return preset, nil
	}
	return Preset{}, fmt.Errorf("no preset named %q in %s (looked for %s)", name, dir, strings.Join(tried, ", "))
}

/* -------------------------------------------------------------------------- */
/*  Reading values                                                             */
/* -------------------------------------------------------------------------- */

// readString reads a scalar, and nothing else.
//
// A value that is a list where one thing was wanted is an empty string rather
// than the first entry, so that a preset naming no repository is reported as
// that — a clear message — rather than silently taking the first of several.
func readString(value yaml.Value) string {
	if value.Kind != yaml.KindString {
		return ""
	}
	return value.Str
}

// readStrings reads a list that may be written as one thing or as several.
//
// The two spellings mean the same thing. A preset that needs one directory
// should not have to learn a list to say so, and one that needs four should not
// have to write a list of one.
func readStrings(value yaml.Value) []string {
	if value.Kind == yaml.KindString {
		if value.Str == "" {
			return nil
		}
		return []string{value.Str}
	}
	values := []string{}
	for _, item := range value.List {
		if item.Kind == yaml.KindString && item.Str != "" {
			values = append(values, item.Str)
		}
	}
	return values
}
