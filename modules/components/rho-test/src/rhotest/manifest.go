//
// @file manifest.go
// @brief The manifest that says which tests a project has.
//
// A project has tests of several kinds — its own unit tests, a build of every
// example for every board, and eventually a board flashed and talked to — and a
// run needs to be able to say which of them it is being asked for. Three answers
// to "what should I run" are enough and none of them is the whole list: by name,
// by kind, by tag.
//
// So the project says. This is the manifest that describes it: a list of suites,
// each with a name, a kind, and whatever that kind needs to be run. The runner
// reads it and does not go looking for anything it did not declare, because a
// runner that discovers tests by walking a directory compiles whatever a person
// left in it.
//
// Declared rather than found, which is the whole argument for this file: a
// project's tests are a statement about what it claims, and the claim is
// checkable.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

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

/* -------------------------------------------------------------------------- */
/*  Where the manifest is                                                      */
/* -------------------------------------------------------------------------- */

// DefaultManifestPaths are where a project's suite manifest is looked for, in
// order.
//
// The first is the convention and the second is the older one, so a project that
// already keeps its tests described does not have to move anything. A project
// with both gets an error rather than one of the two silently: two manifests
// saying different things is a project whose tests depend on which file was read,
// and nothing about that is a thing to preserve.
var DefaultManifestPaths = []string{
	filepath.Join("tests", "tests.yml"),
	"tests.yml",
}

// ManifestFileName is what a report and a `--list` call the manifest by.
const ManifestFileName = "tests.yml"

// FindManifest returns the path to a project's suite manifest, and whether there
// was one.
func FindManifest(root string) (string, bool) {
	found := ""
	for _, candidate := range DefaultManifestPaths {
		path := filepath.Join(root, candidate)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if found != "" {
			return "", false
		}
		found = path
	}
	return found, found != ""
}

/* -------------------------------------------------------------------------- */
/*  What the manifest says                                                    */
/* -------------------------------------------------------------------------- */

// Manifest is what a project declares about its tests.
type Manifest struct {
	// Name is the project the manifest belongs to. Reported so a report read away
	// from the directory it was produced in still says what it is about.
	Name string
	// Declared are the suites the project says it has, in declaration order.
	Declared []Declared
	// Source is where it was read from, for a diagnostic that says which file.
	Source string
}

// FindDeclared returns the declared suite with a name.
func (m Manifest) FindDeclared(name string) (Declared, bool) {
	for _, suite := range m.Declared {
		if suite.Name == name {
			return suite, true
		}
	}
	return Declared{}, false
}

// Kinds returns every kind the manifest declares, sorted, for a report that says
// what kinds of test this project has.
func (m Manifest) Kinds() []string {
	seen := map[string]bool{}
	kinds := []string{}
	for _, suite := range m.Declared {
		if suite.Kind == "" || seen[suite.Kind] {
			continue
		}
		seen[suite.Kind] = true
		kinds = append(kinds, suite.Kind)
	}
	sort.Strings(kinds)
	return kinds
}

// Tags returns every tag the manifest uses, sorted.
func (m Manifest) Tags() []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, suite := range m.Declared {
		for _, tag := range suite.Tags {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	return tags
}

/* -------------------------------------------------------------------------- */
/*  Reading it                                                                */
/* -------------------------------------------------------------------------- */

// LoadManifest reads a project's suite manifest.
//
// A manifest that cannot be read, or that says something this runner cannot act
// on, is an error rather than an empty run. A run that found no tests because the
// file was misspelled and a run that found no tests because there are none are
// the same event from the outside, and only one of them is true.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read suite manifest: %w", err)
	}
	document, err := yaml.Load(string(data))
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	manifest, err := parseManifest(document, ProjectRoot(path))
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	manifest.Source = path
	return manifest, nil
}

// LoadProjectManifest finds and reads a project's suite manifest, from its root.
func LoadProjectManifest(root string) (Manifest, error) {
	path, ok := FindManifest(root)
	if !ok {
		return Manifest{}, fmt.Errorf("no suite manifest: looked for %s", strings.Join(DefaultManifestPaths, " and "))
	}
	return LoadManifest(path)
}

func parseManifest(document yaml.Value, root string) (Manifest, error) {
	manifest := Manifest{}
	if document.Kind != yaml.KindMap {
		return manifest, fmt.Errorf("a suite manifest is a mapping")
	}
	manifest.Name = document.Map["name"].Str

	suites, ok := document.Map["suites"]
	if !ok || suites.Kind != yaml.KindList {
		return manifest, fmt.Errorf("a suite manifest declares a `suites` list")
	}
	if len(suites.List) == 0 {
		return manifest, fmt.Errorf("a suite manifest declares no suites")
	}
	seen := map[string]bool{}
	for index, entry := range suites.List {
		suite, err := parseSuite(entry, root)
		if err != nil {
			return manifest, fmt.Errorf("suite %d: %w", index+1, err)
		}
		if seen[suite.Name] {
			// Two suites with one name cannot be told apart in a report, and a
			// filter for one would silently run the other.
			return manifest, fmt.Errorf("two suites are named %q", suite.Name)
		}
		seen[suite.Name] = true
		manifest.Declared = append(manifest.Declared, suite)
	}
	return manifest, nil
}

/* -------------------------------------------------------------------------- */
/*  One suite                                                                 */
/* -------------------------------------------------------------------------- */

// parseSuite reads one declared suite.
//
// The shape is deliberately shallow: a name, a kind, tags, an optional matrix,
// and whatever that kind reads out of the keys it recognises. Everything else on
// the entry is left for whoever runs that kind — the gtest plugin reads the
// `gtest:` mapping, and a suite of a kind nobody has written a plugin for reports
// that it has no runner rather than failing on a key it does not recognise. That
// is what lets a manifest carry a suite for a plugin that is not installed yet.
func parseSuite(entry yaml.Value, root string) (Declared, error) {
	suite := Declared{}
	if entry.Kind != yaml.KindMap {
		return suite, fmt.Errorf("a suite is a mapping")
	}
	suite.Name = entry.Map["name"].Str
	if suite.Name == "" {
		return suite, fmt.Errorf("a suite has no name")
	}
	suite.Kind = entry.Map["kind"].Str
	if suite.Kind == "" {
		return suite, fmt.Errorf("suite %q does not say what kind it is", suite.Name)
	}
	if tags, ok := entry.Map["tags"]; ok && tags.Kind == yaml.KindList {
		for _, tag := range tags.List {
			if tag.Str != "" {
				suite.Tags = append(suite.Tags, tag.Str)
			}
		}
	}
	// A suite declared as not run is a suite somebody wanted to write down
	// before it could be written. Saying so here rather than failing is what
	// lets a project record the hardware test it has not written yet — and what
	// keeps that suite from being reported as a pass.
	if status, ok := entry.Map["status"]; ok {
		if status.Str == string(StatusSkip) {
			suite.Skip = planned(entry.Map["reason"].Str, suite.Name, suite.Kind)
		}
	}
	if command, ok := entry.Map["command"]; ok {
		suite.Command = splitCommand(command)
	}
	if node, ok := entry.Map["matrix"]; ok {
		matrix, err := parseMatrix(node, root)
		if err != nil {
			return suite, fmt.Errorf("suite %q: %w", suite.Name, err)
		}
		suite.Matrix = matrix
	}
	suite.Options = suiteOptions(entry)
	return suite, nil
}

// knownSuiteKeys are the keys the runner reads itself. Everything else on a suite
// belongs to whatever knows how to run that kind of it.
var knownSuiteKeys = map[string]bool{
	"name": true, "kind": true, "tags": true,
	"status": true, "reason": true,
	"command": true, "matrix": true,
}

func suiteOptions(entry yaml.Value) map[string]any {
	options := map[string]any{}
	for key, value := range entry.Map {
		if knownSuiteKeys[key] {
			continue
		}
		options[key] = yaml.ToAny(value)
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

// splitCommand reads a command written as a list of words or as one string.
//
// Both, because a manifest is read by people: `["./smoke.sh", "--fast"]` is
// unambiguous and `"./smoke.sh --fast"` is what most people write first. A command
// that cannot be split at all is the shell's problem, and is reported by the step
// that runs it rather than guessed at here.
func splitCommand(value yaml.Value) []string {
	if value.Kind == yaml.KindList {
		words := make([]string, 0, len(value.List))
		for _, word := range value.List {
			if word.Str != "" {
				words = append(words, word.Str)
			}
		}
		return words
	}
	text := strings.TrimSpace(value.Str)
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "[") {
		var words []string
		if err := json.Unmarshal([]byte(text), &words); err == nil && len(words) > 0 {
			return words
		}
	}
	return strings.Fields(text)
}

// parseMatrix reads the axes a suite expands over.
//
// Each axis is a named list, so a cell can be read back out of a suite's name
// and a report can say which board it was for. An axis with no values is an
// error rather than an empty suite: a smoke test over a list of boards that is
// currently empty is a test that passes without testing anything, which is the
// one outcome a smoke test must never have.
func parseMatrix(node yaml.Value, root string) (*Matrix, error) {
	if node.Kind != yaml.KindMap {
		return nil, fmt.Errorf("a matrix is a mapping of axis name to its values")
	}
	matrix := &Matrix{Values: map[string][]string{}}
	for axis, values := range node.Map {
		held, err := axisValues(axis, values, root)
		if err != nil {
			return nil, err
		}
		matrix.Axes = append(matrix.Axes, axis)
		matrix.Values[axis] = held
	}
	if len(matrix.Axes) == 0 {
		return nil, nil
	}
	// Sorted, because the order axes are expanded in decides the order cells come
	// out in, and a manifest that reads the same twice must report the same order.
	sort.Strings(matrix.Axes)
	return matrix, nil
}

// axisValues is what one axis takes, and it is either written down here or read
// from a contract the project already declares.
func axisValues(axis string, node yaml.Value, root string) ([]string, error) {
	// An axis that reads its values from somewhere else:
	//
	//   board:
	//     from: platform-manifest
	//     key: hardware.supported
	//
	// Which exists because restating a list is how two lists stop agreeing. A
	// platform says which boards it supports, and a test suite that builds a
	// matrix over the boards says it again; the two are equal on the day they are
	// written and after that one of them changes alone. A suite that reads the
	// contract cannot fall behind it, and adding a board to the platform does not
	// also mean remembering to add it to a test.
	if node.Kind == yaml.KindMap {
		return axisValuesFromContract(axis, node, root)
	}

	if node.Kind != yaml.KindList {
		return nil, fmt.Errorf("axis %q is a list of values or a contract to read them from, not %s",
			axis, kindOf(node))
	}
	if len(node.List) == 0 {
		return nil, fmt.Errorf("axis %q has no values, so every suite it would expand into is empty", axis)
	}
	held := make([]string, 0, len(node.List))
	for _, value := range node.List {
		held = append(held, value.Str)
	}
	return held, nil
}

// axisValuesFromContract reads an axis from a manifest the project depends on.
func axisValuesFromContract(axis string, node yaml.Value, root string) ([]string, error) {
	manifest := node.Map["from"].Str
	key := node.Map["key"].Str
	if manifest == "" {
		return nil, fmt.Errorf("axis %q reads its values from a contract, but names no manifest to read them from", axis)
	}
	if key == "" {
		return nil, fmt.Errorf("axis %q reads its values from %s, but names no key in it", axis, manifest)
	}

	path := findDependencyManifest(root, manifest)
	if path == "" {
		// Said rather than left to expand into nothing. A matrix whose axis is
		// silently empty is a suite that passes without testing anything, which is
		// the one outcome a matrix must never have.
		return nil, fmt.Errorf("axis %q reads its values from %s, which this project does not depend on", axis, manifest)
	}
	document, err := loadYAMLFile(path)
	if err != nil {
		return nil, fmt.Errorf("axis %q reads its values from %s: %w", axis, manifest, err)
	}
	value, found := lookupPath(document, strings.Split(key, "."))
	if !found || value.Kind != yaml.KindList || len(value.List) == 0 {
		return nil, fmt.Errorf("axis %q reads its values from %s, but %s is not a list with anything in it",
			axis, manifest, key)
	}
	held := make([]string, 0, len(value.List))
	for _, item := range value.List {
		held = append(held, item.Str)
	}
	return held, nil
}

// findDependencyManifest is the manifest that declares a named contract: the
// project's own when the name is the project, and a dependency's otherwise.
//
// The project's own is included because the commonest contract of all is a
// module declaring something about itself — which boards it supports, which
// presets it offers. Its tests live beside that declaration, so reading it from
// the test would be reading a copy of the thing being tested.
//
// Everything else is a dependency, because a contract another module declares is
// the one thing that cannot be restated here without becoming a second copy of it.
func findDependencyManifest(root, name string) string {
	if root == "" {
		return ""
	}
	manifest := deps.ManifestPath(root)
	document, err := loadYAMLFile(manifest)
	if err != nil {
		return ""
	}
	if document.Map["module"].Map["name"].Str == name {
		return manifest
	}
	for _, item := range document.Map["depends"].List {
		if item.Kind != yaml.KindMap || item.Map["name"].Str != name {
			continue
		}
		path := item.Map["path"].Str
		if path == "" {
			return ""
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return deps.ManifestPath(path)
	}
	return ""
}

// lookupPath walks a dotted key into a document: `hardware.supported`.
func lookupPath(document yaml.Value, path []string) (yaml.Value, bool) {
	current := document
	for _, step := range path {
		if current.Kind != yaml.KindMap {
			return yaml.Value{}, false
		}
		next, found := current.Map[step]
		if !found {
			return yaml.Value{}, false
		}
		current = next
	}
	return current, true
}

func kindOf(value yaml.Value) string {
	switch value.Kind {
	case yaml.KindMap:
		return "a mapping"
	case yaml.KindList:
		return "a list"
	case yaml.KindNull:
		return "empty"
	default:
		return "a value"
	}
}

func planned(reason, name, kind string) string {
	if reason != "" {
		return reason
	}
	return fmt.Sprintf("%s suite %q is declared but not written yet", kind, name)
}

/* -------------------------------------------------------------------------- */
/*  Declaring one kind of test                                                */
/* -------------------------------------------------------------------------- */

// Declared is one suite as the manifest stated it, before anything has run.
//
// It is kept apart from Suite because the two answer different questions. A
// Suite is a runnable unit with steps; a Declared is a claim, and the difference
// is what a dry run reports: what a project says it has, as distinct from what a
// run went and did about it.
type Declared struct {
	// Name is the suite's name, or its base name when it came out of a matrix.
	Name string
	// Kind is what knows how to run it.
	Kind string
	// Tags are what selection matches against.
	Tags []string
	// Skip is why it will not be run, when it will not.
	Skip string
	// Command is the program a command suite runs, as the manifest wrote it. Left
	// for the kind rather than parsed here: a key is read by whoever knows what it
	// means, and only one kind reads this one.
	Command []string
	// Options is everything else the suite said.
	//
	// Passed through rather than named, and that is the point: the runner knows the
	// keys every kind shares and cannot know the ones a kind invented, so a suite
	// whose kind is answered by a plugin carries that plugin's own settings to it
	// untouched. A manifest may therefore describe a test for a plugin that is not
	// installed, and installing it needs no change to this file or to the manifest.
	Options map[string]any
	// Matrix is what it expands into, when it expands.
	Matrix *Matrix
}

// MatrixAxes are the axes this declaration was expanded from. Empty for a suite
// that was not expanded.
func (d Declared) MatrixAxes() []string {
	if d.Matrix == nil {
		return nil
	}
	return d.Matrix.Axes
}

// Matrix is a suite declared over several axes: a list of examples and a list of
// boards, and one suite per cell.
//
// Declared rather than written out, because thirty cells written by hand are
// thirty places to forget one, and a smoke suite that quietly lost a board is the
// kind of gap nobody notices until a board stops building.
type Matrix struct {
	// Axes are the names of the axes, in order.
	Axes []string
	// Values maps an axis name to the values it takes.
	Values map[string][]string
}

// Cells returns the number of suites the matrix expands into.
func (m Matrix) Cells() int {
	total := 1
	for _, axis := range m.Axes {
		total *= len(m.Values[axis])
	}
	return total
}

// Expand turns a matrix into one name per cell, in a stable order.
func (m Matrix) Expand(base string) []string {
	names := []string{base}
	for _, axis := range m.Axes {
		values := append([]string{}, m.Values[axis]...)
		sort.Strings(values)
		grown := make([]string, 0, len(names)*len(values))
		for _, prefix := range names {
			for _, value := range values {
				grown = append(grown, MatrixName(prefix, axis, value))
			}
		}
		names = grown
	}
	return names
}

// AxisValue reads the value a cell was expanded with, so a suite can find the
// board it is for.
//
// A cell's values are paths — `examples/c/hello_world` is a directory a project
// really has — which is why the axes are joined by AxisSeparator rather than by a
// slash: a path holds slashes, and a name that cannot be read back is a name a
// suite cannot act on.
func AxisValue(name, axis string) string {
	for _, part := range strings.Split(name, AxisSeparator) {
		if strings.HasPrefix(part, axis+"=") {
			return strings.TrimPrefix(part, axis+"=")
		}
	}
	return ""
}

// loadYAMLFile reads a document from a file, for the one place that reads a
// manifest other than the project's own.
func loadYAMLFile(path string) (yaml.Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return yaml.Value{}, err
	}
	return yaml.Load(string(data))
}
