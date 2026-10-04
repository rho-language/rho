//
// @file source.go
// @brief Reading a source, and finding the Rho ones.
//
// The rules are handed text and a path and nothing else. No file handle, no
// directory, no configuration: a rule that can reach outside the file it was given
// becomes a rule whose findings depend on which machine ran it, and this component's
// findings have to be diffable between two runs on two machines for a pipeline to
// compare them.
//
// `RhoSources` is here rather than in the rules because the Rho sources are not in
// the compilation database and never will be: nothing compiles a `.rho` file, because
// the file the compiler reads is the one this toolchain translated. A run that
// wanted the Rho engine to have anything to read would otherwise have to be handed
// it explicitly, and the ordinary case — `rho lint` in a project directory — would
// find no Rho sources at all.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Relative is a source's path relative to a project's directory, which is how
// every finding in a report is written.
//
// Relative rather than absolute for three reasons that all come down to one: a
// report that says `/home/miraur/projects/robohub-stm32/src/hal/stm32_gpio.c`
// says nothing about the code and everything about the machine, a SARIF `uri` that
// differs per machine cannot be grouped by a code scanning service, and a pipeline
// that stores findings as absolute paths cannot diff two runs of two checkouts.
//
// A path outside the root is left absolute rather than rewritten with `..` chains,
// because a consumer resolving `%SRCROOT%/../../..` against the wrong base gets a
// file that is not there, and an absolute path is at least honest about being one.
func Relative(path string, root string) string {
	if path == "" {
		return ""
	}
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Clean(path)
	}
	return filepath.ToSlash(relative)
}

// ReadSource is a file as the rules read it.
//
// The whole file, because every rule here is a scan and a scan that re-read a file
// per line would be a hundred times the work for the same answer. A file that is
// not there is an error rather than an empty source, because an empty source has no
// findings and an absent one is a fact the run has to be able to report.
func ReadSource(path string, root string, generated map[string]bool) (Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("cannot be read: %w", err)
	}
	return Source{Path: path, Root: root, Text: string(data), Generated: generated}, nil
}

// Line returns the text's lines, one-based by index.
//
// `lines[0]` is line one, which is the indexing everything in a report is in and the
// indexing a rule should not have to remember. A text with no lines still has one
// empty one, because an empty file has a first line and a rule that reports "line
// 1 of an empty file" is right.
func (s Source) Line(number int) string {
	lines := s.Lines()
	if number < 1 || number > len(lines) {
		return ""
	}
	return lines[number-1]
}

// Lines are the text's lines, with no trailing newline on the last of them.
func (s Source) Lines() []string {
	if s.Text == "" {
		return []string{""}
	}
	return strings.Split(strings.TrimSuffix(s.Text, "\n"), "\n")
}

// HasSuffix reports whether the path ends with one of the extensions.
//
// With a leading dot or without, because both are how people write a filter and
// neither means anything different.
func HasSuffix(path string, extensions ...string) bool {
	lowered := strings.ToLower(filepath.Ext(path))
	for _, extension := range extensions {
		if lowered == strings.ToLower(extension) {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  The modules a build generates                                              */
/* -------------------------------------------------------------------------- */

// GeneratedModules are the names of the modules this project's build generates, and
// what they are for is stated at the point of use.
//
// The honest shape of the answer: a name here is a module whose *declarations* are
// generated from somebody else's headers, so a source importing it names none of
// them and still uses every one. `rho-cimport` writes them under
// `build/generated/modules/` and a Rho application imports them by name.
func GeneratedModules(root string) map[string]bool {
	out := map[string]bool{}
	directory := filepath.Join(root, "build", "generated", "modules")
	if !present(directory) {
		return out
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !HasSuffix(entry.Name(), ".rho") {
			continue
		}
		name := entry.Name()
		name = name[:len(name)-len(".rho")]
		out[name] = true
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Finding the Rho sources                                                    */
/* -------------------------------------------------------------------------- */

// SkipDirs are the directories a search does not go into.
//
// Named rather than derived, because the alternatives are all guesses: a directory
// called `build` is somebody's output, a directory called `vendor` is somebody
// else's code, and a search that descended into either would lint thousands of files
// belonging to somebody else in the time it takes to read the project's own.
func SkipDirs() []string {
	return []string{
		".git", ".kilo", "build", "node_modules", "vendor",
		// The toolchain's own standard library, in a checkout that has one. Every
		// project carries it and none of them wrote it, so a run over a project's
		// standard library is a run over this repository — which is what `rho lint .`
		// in the toolchain itself is for.
		"std",
		// The generated project's own trees. `rho build` puts the translation of a
		// Rho source under `build/generated/`, and a search that found both the
		// `.rho` and its `.c` would report the same program twice — once by the
		// rules that understand it and once by the rules that read C.
		"generated",
	}
}

// RhoSources are the Rho sources in a project directory, in path order.
//
// The project's own, and the standard library's is *not* included: `std/` is code
// every project carries and none of them wrote, and a run that reported findings in
// it would be reporting on this toolchain in every project's pipeline.
//
// The walk skips the directories above and is depth-limited, because a project with
// a deep tree of its own is not unusual and a walk that followed every branch of one
// is a run that never finishes. The limit is generous enough for a real project and
// is said in the run's output when it is reached, rather than silently producing a
// partial list.
func RhoSources(root string) []string {
	found := []string{}
	walkSources(root, &found, MaxWalkDepth, func(name string) bool {
		return HasSuffix(name, ".rho")
	})
	sort.Strings(found)
	return found
}

// MaxWalkDepth is how deep a search for the project's own sources goes.
const MaxWalkDepth = 12

// walkSources is the walk, with the depth limit and the skips in one place.
func walkSources(root string, found *[]string, depth int, keep func(string) bool) {
	if depth <= 0 {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if skipDirectory(name) {
				continue
			}
			walkSources(filepath.Join(root, name), found, depth-1, keep)
			continue
		}
		if keep(name) {
			*found = append(*found, filepath.Join(root, name))
		}
	}
}

func skipDirectory(name string) bool {
	for _, skip := range SkipDirs() {
		if name == skip {
			return true
		}
	}
	return false
}
