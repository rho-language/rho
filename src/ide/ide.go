//
// @file ide.go
// @brief What an editor needs from a project, written out for it.
//
// An editor does not know a project. It knows files, a build command somebody
// typed, and a debugger somebody configured by hand — and every one of those is
// a fact the project already has written down. The compiler a target is built
// with, where the binary is written, which probe a board is reached through and
// where its register map lives are all in the project's manifest and the target
// spec it names, so an editor configuration is a rendering of that rather than
// something a person has to type.
//
// So this is a generator and not a template: it reads the facts and writes the
// files an editor reads. A project that has a configuration of its own keeps it,
// because a generator that overwrites is a generator that is run once.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

// Package ide renders a project's own declarations into the files an editor
// reads, so that an editor is configured from the project rather than by hand.
package ide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/targetspec"
)

// Target is a target's facts, as a generated configuration is written from.
//
// An alias rather than a second declaration: the editor and the debug adapter read
// the same spec, and two declarations of what a board says is one of them being
// out of date.
type Target = targetspec.Target

// DebuggerFor is the debugger that goes with a compiler, as a launch configuration
// names it.
func DebuggerFor(compiler string) string { return targetspec.DebuggerFor(compiler) }

// ParseTarget reads a resolved target spec as one of these.
func ParseTarget(data []byte) (*Target, error) { return targetspec.ParseTarget(data) }

// Editor is an editor this generator writes configuration for.
type Editor string

const (
	// EditorVSCode is Visual Studio Code, which reads `.vscode/launch.json` for a
	// debug session and `.vscode/tasks.json` for a task.
	EditorVSCode Editor = "vscode"
	// EditorNeovim is Neovim, which reads `.vscode/launch.json` when its DAP
	// client is pointed at it — the same file, so one generation serves both and
	// an editor that reads nothing but it still finds something right.
	EditorNeovim Editor = "neovim"
)

// Editors are the editors configuration can be written for, in name order, which
// is what a listing shows.
func Editors() []Editor {
	return []Editor{EditorNeovim, EditorVSCode}
}

// ParseEditor answers for an editor by name, naming the ones there are when the
// name is not one of them.
func ParseEditor(name string) (Editor, error) {
	for _, editor := range Editors() {
		if string(editor) == strings.TrimSpace(strings.ToLower(name)) {
			return editor, nil
		}
	}
	names := make([]string, 0, 2)
	for _, editor := range Editors() {
		names = append(names, string(editor))
	}
	return "", fmt.Errorf("no editor named %q; this writes configuration for %s", name, strings.Join(names, ", "))
}

// Request is one generation: which project, for which editor, and what the
// person generating asked to be told about it.
type Request struct {
	// ProjectRoot is the project configuration is written for.
	ProjectRoot string
	// Editor is what the configuration is written for.
	Editor Editor
	// Target is the board the configuration is written for. Empty means whatever
	// the project's own manifest says — one board, or the presets it lists — and a
	// project that says neither is configured for the machine it is built on.
	Target string
	// Resolve answers with the facts a named target carries. It is a field rather
	// than a call because which targets exist belongs to the component carrying
	// the specs, and a caller that has no such component can say so rather than
	// have this reach for one.
	Resolve func(projectRoot, target string) (*Target, error)
}

// Result is what one generation wrote, and what it did about the files that were
// already there.
type Result struct {
	// Written are the files that were created.
	Written []string
	// Merged are the files that were already there and were updated in place,
	// keeping whatever they already held.
	Merged []string
	// Unchanged are the files that already said what this generation would have
	// written, so they were left alone rather than rewritten.
	Unchanged []string
}

// Generate writes the configuration an editor reads, and reports what it did.
//
// A file that is already there is merged rather than replaced: the configurations
// and tasks a person has added are theirs, and a generator that drops them is one
// nobody runs twice. A file that cannot be read as JSON at all is reported rather
// than overwritten, because there is no way to merge into something that cannot
// be parsed and no safe way to discard what is in it.
func Generate(request Request) (*Result, error) {
	if request.ProjectRoot == "" {
		request.ProjectRoot = "."
	}
	root, err := filepath.Abs(request.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is not a project directory", request.ProjectRoot)
	}

	project, err := describeProject(root, request)
	if err != nil {
		return nil, err
	}

	files, err := filesFor(request, project)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	for _, file := range files {
		action, err := write(root, file)
		if err != nil {
			return nil, err
		}
		switch action {
		case actionWritten:
			result.Written = append(result.Written, file.Path)
		case actionMerged:
			result.Merged = append(result.Merged, file.Path)
		case actionUnchanged:
			result.Unchanged = append(result.Unchanged, file.Path)
		}
	}
	return result, nil
}

// Project is what the configuration is rendered from: the facts in the project's
// own manifest, and the facts in the target it names.
type Project struct {
	// Root is the project directory.
	Root string
	// Name is the name CMake builds the binary under, which is the module's own
	// name with whatever a target name cannot hold.
	Name string
	// ModuleName is the name the project calls itself. Kept beside Name because the
	// two differ — `hello-world` builds as `hello_world` — and a file a person reads
	// has to be named after the project rather than after its build output.
	ModuleName string
	// Binary is the built program, relative to the root. It is what a debugger
	// loads, so it is resolved here rather than left to a person to spell.
	Binary string
	// BuildDir is the directory a build keeps its own output in, which is where a
	// register map is not looked for: everything in there is generated, and a
	// search that walked it would find a project's own files again.
	BuildDir string
	// AbsoluteBinary is the built program as an absolute path, for the same reason.
	AbsoluteBinary string
	// VendorRoots are the vendor trees of the platforms this project depends on,
	// where a file one of them carries lives.
	VendorRoots []string
	// Declared is the register map the project names for each of its boards, by the
	// name the board's spec answers to. A session reads the entry for the board it
	// is about in preference to anything else. A project that names none has none,
	// and a lookup in a nil map is the same as a lookup that finds nothing.
	Declared map[string]string

	// Target is the board this project is built for, by name.
	Target string
	// Board is the board a configuration is written for, which is the key the
	// project's own register map is under.
	Board string
	// Boards are the boards this project is built for, by name.
	//
	// More than one when the project is written for several, which is what a
	// `build.presets` list means. Each gets its own sessions and its own build
	// task, because a session that builds for one board and debugs another is a
	// session that stops on a line in a program that is not there.
	Boards []string
	// Selected is the board the configuration was written for, when one was named.
	Selected string
	// Facts are the resolved target specs, by board name. A board with no facts is
	// one this could not read, which is reported rather than guessed at.
	Facts map[string]*Target
	// FactsUnavailable says why a target spec could not be read, when it could
	// not. A generation without a target spec is still a generation; this is what
	// makes it say so rather than quietly writing a configuration for a board it
	// knows nothing about.
	FactsUnavailable string
}

// describeProject reads what the project's own manifest says, and what the target
// it names says about itself.
func describeProject(root string, request Request) (*Project, error) {
	project := &Project{Root: root, Name: "rhoapp"}

	manifest, err := deps.LoadManifest(deps.ManifestPath(root))
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("%s declares no module.yml, so there is nothing to write configuration for", root)
	}
	project.ModuleName = manifest.ModuleName
	if manifest.ModuleName != "" {
		project.Name = cmakeProjectName(manifest.ModuleName)
	}
	// The boards, in the order the project declares them: one named by
	// `build.toolchain`, else every preset, else the machine this is built on.
	project.Selected = request.Target
	switch {
	case request.Target != "":
		project.Boards = []string{request.Target}
	case manifest.BuildToolchain != "":
		project.Boards = []string{manifest.BuildToolchain}
	case len(manifest.BuildPresets) > 0:
		project.Boards = append([]string{}, manifest.BuildPresets...)
	default:
		project.Boards = []string{"native"}
	}

	// The binary is written where the project's manifest says, under the name
	// CMake builds it under, because that is where a build actually puts it: a
	// generator that guessed a second location would send a debugger after a file
	// no build produces.
	project.Binary = filepath.Join(
		slash(relative(root, manifest.OutputDirectory())), project.Name)
	project.BuildDir = manifest.BuildDirectory()
	project.AbsoluteBinary = manifest.OutputDirectory() + string(filepath.Separator) + project.Name
	project.VendorRoots = manifest.VendorRoots(root)
	// What the project says about its boards is the answer, in a generated
	// configuration as in a session: somebody wrote it down for this project, per
	// board, because a project built for several boards has several chips.
	if manifest.Debug != nil && len(manifest.Debug.SVD) > 0 {
		project.Declared = map[string]string{}
		for board, path := range manifest.Debug.SVD {
			project.Declared[board] = strings.TrimSpace(path)
		}
	}

	// Every board's own facts. A project whose boards cannot be resolved is still
	// configured — for the machine, and with a debugger that needs no probe — so
	// this is recorded rather than returned as a failure.
	project.Facts = map[string]*Target{}
	if request.Resolve == nil {
		project.FactsUnavailable = "no target spec was read, so these are configurations for this machine"
		return project, nil
	}
	for _, board := range project.Boards {
		facts, resolveErr := request.Resolve(root, board)
		if resolveErr != nil {
			project.FactsUnavailable = resolveErr.Error()
			continue
		}
		project.Facts[board] = facts
	}
	return project, nil
}

// factsFor is what a board's spec said, and whether there was one.
func (p *Project) factsFor(board string) (*Target, bool) {
	facts, found := p.Facts[board]
	return facts, found
}

// targetFor is the board whose third-party session is written: the one that was
// named, or the only one there is. A project written for six boards gets a launch
// and an attach for each; a project written for one also gets the session an editor
// extension provides, because a single board's list has room for it and a person on
// that board wants it.
func (p *Project) targetFor() string {
	if p.Selected != "" {
		return p.Selected
	}
	if len(p.Boards) == 1 {
		return p.Boards[0]
	}
	return ""
}

// defaultBoard is the board the build task with no name of its own builds for.
func (p *Project) defaultBoard() string {
	if len(p.Boards) == 0 {
		return "this machine"
	}
	return p.Boards[0]
}

// relative is a path relative to the root, and slash-separated, because a path
// written into a JSON file is read by an editor on whatever machine it opens the
// project on.
func relative(root, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func slash(path string) string { return filepath.ToSlash(path) }

// cmakeProjectName keeps a module name usable as a CMake target, which is the
// name the binary is written under. It is the same rule the generator applies,
// so the path written here is the path the build produces.
func cmakeProjectName(name string) string {
	if name == "" {
		return "rhoapp"
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "rhoapp"
	}
	return b.String()
}

// file is one file a generation produces.
type file struct {
	// Path is relative to the project root.
	Path string
	// Content is what the file holds when it is written from nothing.
	Content string
	// Order is the key order the file is written in, so that regenerating it does
	// not reorder the file.
	Order []string
	// Merge folds a generated object into what a file already holds, and reports
	// whether anything changed.
	Merge func(existing map[string]any) (map[string]any, bool)
}

// action is what happened to one file.
type action int

const (
	actionWritten action = iota
	actionMerged
	actionUnchanged
)

// write puts one generated file in place, merging into one that is already there.
func write(root string, generated file) (action, error) {
	path := filepath.Join(root, generated.Path)
	if generated.Merge != nil {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			existing := map[string]any{}
			if err := json.Unmarshal(data, &existing); err != nil {
				return actionUnchanged, fmt.Errorf("%s cannot be read as JSON, so nothing was written to it: %w",
					generated.Path, err)
			}
			merged, changed := generated.Merge(existing)
			if !changed {
				return actionUnchanged, nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return actionUnchanged, err
			}
			if err := writeJSON(path, merged, generated.Order); err != nil {
				return actionUnchanged, err
			}
			return actionMerged, nil
		}
		if !os.IsNotExist(readErr) {
			return actionUnchanged, readErr
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return actionUnchanged, err
	}
	if err := os.WriteFile(path, []byte(generated.Content), 0o644); err != nil {
		return actionUnchanged, err
	}
	return actionWritten, nil
}

// writeJSON writes an object the way an editor reads it: two-space indentation,
// the keys in a fixed order, and a trailing newline.
func writeJSON(path string, value map[string]any, order []string) error {
	return os.WriteFile(path, []byte(render(value, order)), 0o644)
}

// marshalCompact renders a value as JSON, indented, and without escaping the
// characters a path is mostly made of.
//
// Indented because this is what a person reads: a launch file whose three
// configurations are one line each is a file nobody edits, and the fields that go
// wrong in a hand-written one are exactly the ones that are hard to see in a wall
// of text.
func marshalCompact(value any) ([]byte, error) {
	buffer := &strings.Builder{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(buffer.String(), "\n")), nil
}

/* -------------------------------------------------------------------------- */
/*  Writing                                                                    */
/* -------------------------------------------------------------------------- */

// render writes an object with its keys in a declared order, and any key the
// order does not name after it in name order.
//
// Written rather than left to the encoder because an encoder emits a Go map in
// whatever order it iterates, and that order changes between runs. A generated
// file that came back with its keys in a different order than it went in is a
// file that shows up as changed in every diff, which trains people to ignore the
// diffs of this file — including the one where something actually changed.
func render(document map[string]any, order []string) string {
	keys := []string{}
	for _, key := range order {
		if _, found := document[key]; found {
			keys = append(keys, key)
		}
	}
	named := map[string]bool{}
	for _, key := range keys {
		named[key] = true
	}
	rest := make([]string, 0, len(document))
	for key := range document {
		if !named[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	body := &strings.Builder{}
	body.WriteString("{\n")
	for index, key := range keys {
		if index > 0 {
			body.WriteString(",\n")
		}
		body.WriteString("  " + quote(key) + ": ")
		encoded, err := marshalCompact(document[key])
		if err != nil {
			// A value that cannot be rendered is left out rather than written as
			// something a reader would take for configuration.
			continue
		}
		body.WriteString(indentLines(string(encoded), "  ", index))
	}
	body.WriteString("\n}\n")
	return body.String()
}

// indentLines puts a rendered value under its key. The rendered value is
// indented relative to nothing, so every line but the first is pushed in by the
// key's own indent — which is what puts an array's elements two spaces under its
// bracket rather than flush with it.
func indentLines(value string, indent string, index int) string {
	if index < 0 || !strings.Contains(value, "\n") {
		return value
	}
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	for position := 1; position < len(lines); position++ {
		lines[position] = indent + lines[position]
	}
	return strings.Join(lines, "\n")
}

func quote(key string) string {
	encoded, err := marshalCompact(key)
	if err != nil {
		return `""`
	}
	return strings.TrimSpace(string(encoded))
}

// mergeNamed folds generated entries into a list of entries that are keyed by
// name, which is what both of the files an editor reads are: a list of debug
// configurations, and a list of tasks.
//
// Keyed by name rather than appended, because a name is the identity a person
// recognises, and because a generated file that grows a new entry on every
// generation would need editing by hand to stay readable. A generated entry
// replaces an entry of the same name: these are this toolchain's own names, so
// the generated one is the current answer, and a person who wants theirs kept
// writes it under a name of their own.
func mergeNamed(existing, generated []any, identity string) ([]any, bool) {
	byName := map[string]int{}
	kept := make([]any, 0, len(existing)+len(generated))
	for _, entry := range existing {
		object, isObject := entry.(map[string]any)
		if isObject {
			if name, _ := object[identity].(string); name != "" {
				if _, replaced := byName[name]; replaced {
					// A file that lists the same name twice has a mistake in it.
					// The first is kept and the second dropped rather than the
					// whole file refused: the mistake is the duplicate, not the
					// configurations around it.
					continue
				}
				byName[name] = len(kept)
			}
		}
		kept = append(kept, entry)
	}

	changed := false
	for _, entry := range generated {
		object, isObject := canonical(entry).(map[string]any)
		if !isObject {
			continue
		}
		name, _ := object[identity].(string)
		if name == "" {
			kept = append(kept, entry)
			changed = true
			continue
		}
		if index, replaced := byName[name]; replaced {
			// The generated entry takes the place of the one that was there even
			// when the two say the same thing: it is the same fields in the order
			// this writes them, so a generated file keeps the order it was
			// generated with rather than drifting into whatever order a decoded
			// map had.
			if !sameEntry(kept[index], object) {
				changed = true
			}
			kept[index] = entry
			continue
		}
		byName[name] = len(kept)
		kept = append(kept, entry)
		changed = true
	}
	return kept, changed
}

// sameEntry reports whether two entries are the same.
//
// Compared in canonical form rather than as Go values, because the two sides
// arrive by different roads: one was generated as an ordered list of fields and
// the other came off disk as a decoded map, whose numbers are `float64` where the
// generated one has `int` and whose keys have no order. Comparing what they mean
// rather than how they are spelled is what makes a file that already says the
// right thing be left alone.
func sameEntry(existing, generated any) bool {
	left, leftErr := marshalCompact(canonical(existing))
	right, rightErr := marshalCompact(canonical(generated))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(left) == string(right)
}

// canonical is a value in the form a comparison is made in: decoded, so that a
// generated fragment and a decoded file entry mean the same thing.
func canonical(value any) any {
	fragment, isFragment := value.(json.RawMessage)
	if !isFragment {
		return value
	}
	decoded := any(nil)
	if err := json.Unmarshal(fragment, &decoded); err != nil {
		return value
	}
	return decoded
}

// object reads a key out of a decoded JSON file as a list, which is what the
// configurations and tasks of both files are.
func object(existing map[string]any, key string) []any {
	list, _ := existing[key].([]any)
	return list
}

// svdFile is where this project's register map is, as an editor would name it,
// and whether there is one to name.
//
// A target spec names its register map by file name — `rp2040.svd` — because the
// spec describes the chip rather than any project built for it. Where that file
// lives is a fact about the machine, so it is looked for here rather than
// assumed: a project that vendors it has it somewhere in the tree, and a project
// that does not has no register map to point an editor at.
//
// It is reported absent rather than guessed at. A `svdFile` that names a file
// which is not there is a peripheral tree that comes up empty, and an empty
// peripheral tree reads as "this chip has no peripherals" rather than as "this
// project does not ship the file".
// svdFile is this project's register map, as an editor names it.
//
// The project's own entry for this board first, then the board's spec — because what
// somebody committed to this repository is a fact about the project, and the
// vendor's naming is what happens when nobody has said.
//
// A file in the project's own tree is named relative to the workspace; one in a
// platform's vendor tree is named absolutely, because it is outside the workspace
// and a relative path to it would be a path to nothing in the editor that opens the
// project.
//
// Reported absent rather than guessed at: a `svdFile` naming a file which is not there
// is a peripheral tree that comes up empty, and an empty one reads as "this chip has
// no peripherals" rather than as "this project does not ship the file".
func (p *Project) svdFile(facts *Target) (string, bool) {
	if declared := strings.TrimSpace(p.Declared[p.Board]); declared != "" {
		if path, found := targetspec.FindFile(declared, p.Root, p.AbsoluteBinary, p.VendorRoots); found {
			return p.asEditorPath(path), true
		}
	}
	if facts == nil {
		return "", false
	}
	path, found := targetspec.FindFile(facts.Debug.SVD, p.Root, p.AbsoluteBinary, p.VendorRoots)
	if !found {
		return "", false
	}
	return p.asEditorPath(path), true
}

// asEditorPath is a file as an editor names it: relative to the workspace when it is
// inside it, and absolute when it is not.
func (p *Project) asEditorPath(path string) string {
	if under, err := filepath.Rel(p.Root, path); err == nil && !strings.HasPrefix(under, "..") {
		return slash(under)
	}
	return filepath.ToSlash(path)
}

// mergeList is a file's merge: the named entries are folded in and everything else
// the file holds is left exactly as it was.
//
// The version is written in only when the file has none. A file that declares its
// own schema version is a file a person has looked at, and overwriting that
// because a generator disagreed with them is not a merge.
func mergeList(key, identity, version string, generated []any) func(map[string]any) (map[string]any, bool) {
	return func(existing map[string]any) (map[string]any, bool) {
		merged := map[string]any{}
		for field, value := range existing {
			merged[field] = value
		}
		if merged["version"] == nil {
			merged["version"] = version
		}
		entries, changed := mergeNamed(object(existing, key), generated, identity)
		if !changed && merged["version"] == existing["version"] {
			return existing, false
		}
		merged[key] = entries
		return merged, changed
	}
}
