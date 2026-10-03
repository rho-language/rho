//
// @file workspace.go
// @brief The workspace file: one file that opens a project ready to build and
// debug.
//
// A `.vscode/launch.json` is read only when a folder is open on its own, and a
// project that has more than one folder — the generated tree beside the sources,
// a vendored library beside the project — cannot have both of its folders open at
// once. A workspace file is the answer to that, and it is also where the editor's
// settings and its recommended extensions belong: a project that names them in one
// file gets them the same way on every machine, rather than each person having
// set them once by hand.
//
// So it is generated from the same facts as the other two files, and holds the
// same sessions and tasks inline. Opening the workspace is then the whole of
// setting an editor up: the configurations, the tasks, the compiler paths and the
// extensions are all already in it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package ide

import (
	"sort"
	"strings"
)

// The extension that provides this toolchain's language support, and the ones a
// target's own adapter needs. Named here rather than left to a person to know,
// because the cost of it being missing is a session that fails to start with a
// message about a missing debug type.
// ExtensionLanguage is the extension that provides this toolchain's language
// support, named in a workspace so a machine that does not have it is offered it
// rather than failing to parse every file in the project.
const ExtensionLanguage = "robohubplatform.rho"

// adapterExtensions maps the adapter a target spec names to the extension that
// provides it. Only the adapters this tree knows are here; one that is not is
// simply not recommended, which is better than recommending an identifier that
// resolves to somebody else's unrelated extension.
var adapterExtensions = map[string]string{
	"cortex-debug":   "marus25.cortex-debug",
	"cppdbg":         "ms-vscode.cpptools",
	"probe-rs-debug": "probe-rs.probe-rs-debugger",
	"gdb":            "ms-vscode.cpptools",
	"lldb":           "vadimcn.vscode-lldb",
	"probe-rs":       "probe-rs.probe-rs-debugger",
}

// vscodeWorkspace is the file a person opens to work on this project.
func vscodeWorkspace(project *Project) file {
	launch := []any{}
	for _, entry := range project.debugConfigurations() {
		launch = append(launch, entry.fragment())
	}
	tasks := []any{}
	for _, entry := range project.tasks() {
		tasks = append(tasks, entry.fragment())
	}

	document := map[string]any{
		"folders": []any{
			map[string]any{"name": project.Name, "path": "."},
		},
		"extensions": map[string]any{
			"recommendations": recommended(project),
		},
		"settings": project.settings(),
		"launch": map[string]any{
			"version":        launchSchemaVersion,
			"configurations": launch,
		},
		"tasks": map[string]any{
			"version": tasksSchemaVersion,
			"tasks":   tasks,
		},
	}
	order := []string{"folders", "settings", "extensions", "launch", "tasks"}
	return file{
		Path:    workspaceName(project),
		Order:   order,
		Content: render(document, order),
		Merge:   mergeWorkspace(project, launch, tasks),
	}
}

// workspaceSuffix is what makes a file a workspace. It is part of the name rather
// than of a directory, so it sits beside the project it opens.
const workspaceSuffix = ".code-workspace"

// workspaceName is the file's name: the name the project gives itself.
//
// Not the directory's, because a project's directory is often something else — an
// example lives in `examples/c/hello_world` — and not the CMake target's, because
// `hello_world.code-workspace` is a file named after a build output. The project's
// own name is the one it answers to.
func workspaceName(project *Project) string {
	if name := strings.TrimSpace(project.ModuleName); name != "" {
		return name + workspaceSuffix
	}
	return project.Name + workspaceSuffix
}

// recommended are the extensions this project is debugged and edited with.
//
// Named from what the target spec says rather than from a list of editors: a
// project built for a Cortex-M board needs the adapter that drives one, and a
// project built for the host does not.
func recommended(project *Project) []any {
	out := []any{ExtensionLanguage}
	// The adapter is the one the board this workspace is opened for names. A project
	// written for several boards gets one recommendation — the board it was opened
	// for — and each session in the list carries its own.
	facts := project.defaultFacts()
	if facts == nil {
		return out
	}
	if extension, found := adapterExtensions[facts.Debug.Adapter]; found {
		out = append(out, extension)
	}
	return out
}

// settings are the editor settings this project is built under.
//
// The C ones are the target's own: which compiler, which language, and which
// macros a build of this target always has. A project that leaves them to the
// editor gets an IntelliSense that parses a cross-compiled bare-metal header with
// the host's own compiler settings, and reports errors in code that compiles.
func (p *Project) settings() map[string]any {
	out := map[string]any{}
	// A generated file is not a Rho file, so it is not a language the editor has
	// heard of until the extension is installed — and when it is not, it reads as
	// plain text, which is what the association gives it either way.
	out["files.associations"] = map[string]any{"*.rho": "rho"}
	// Everything a build generates is not source, and an editor that offers to
	// navigate into a generated project's CMake cache is offering to navigate into
	// a file whose paths were true on somebody else's machine.
	out["files.exclude"] = map[string]any{"**/build/generated": true}

	if !p.bareMetal() {
		// A target with no memory map of its own is the host, whose address space
		// is whatever started the process. Naming the compiler for it would pin the
		// editor to the compiler this machine happens to have — which is the one
		// thing its own defaults already get right, and get right for the machine
		// the file is opened on rather than this one.
		return out
	}
	facts := p.defaultFacts()
	compiler := strings.TrimSpace(facts.Toolchain.Compiler)
	if compiler == "" {
		return out
	}
	cpp := map[string]any{
		// C11 is what the generated project is built as, and an editor that
		// assumes another standard reports errors in code that compiles.
		"cStandard":   "c11",
		"cppStandard": "c++17",
	}
	if intelliSense := intelliSenseMode(compiler); intelliSense != "" {
		cpp["intelliSenseMode"] = intelliSense
	}
	if len(facts.Defines) > 0 {
		// The macros a build of this target always has. Without them the editor
		// parses out the branches the build excludes, which are the ones a project
		// uses to pick its board.
		defines := make([]any, 0, len(facts.Defines))
		for _, define := range facts.Defines {
			defines = append(defines, define)
		}
		cpp["defines"] = defines
	}
	// The compiler is named by prefix rather than by the target, because the
	// editor runs it as a normal host compiler with different arguments, and the
	// name is how it finds the right include paths.
	cpp["compilerPath"] = compiler
	out["C_Cpp.default"] = cpp
	return out
}

// bareMetal is whether this target has an address space of its own.
//
// A target that declares where its flash is is a chip: the editor needs to be
// told how to parse for it. One that does not is a host, whose memory is the
// kernel's business and whose compiler the machine already knows.
func (p *Project) bareMetal() bool {
	facts := p.defaultFacts()
	return facts != nil && strings.TrimSpace(facts.Memory.Flash.Origin) != ""
}

// defaultFacts is the spec of the board this workspace is opened for.
func (p *Project) defaultFacts() *Target {
	board := p.targetFor()
	if board == "" {
		board = p.defaultBoard()
	}
	facts, _ := p.factsFor(board)
	return facts
}

// intelliSenseMode is the parsing mode the editor should use for a cross toolchain
// it recognises, and nothing at all for one it does not.
//
// A mode named for a toolchain that is not installed produces an editor that
// silently fails to index, so the ones that are not recognised are left to the
// editor's own default rather than guessed at.
func intelliSenseMode(compiler string) string {
	switch {
	case strings.HasPrefix(compiler, "arm-none-eabi-"):
		return "gcc-arm"
	case strings.HasPrefix(compiler, "arm-none-eabi"):
		return "gcc-arm"
	}
	return ""
}

// mergeWorkspace folds a generated workspace into one that is already there.
//
// Two rules, and they differ because the two things are not the same kind of
// thing.
//
// A generated entry — a configuration, a task — replaces an entry of the same
// name, as in the other two files: these names are this toolchain's own, so the
// generated one is the current answer.
//
// A generated *setting* is only filled in where there is none. A setting a person
// has changed is one they changed for a reason, and a generator that overwrites it
// on every run produces output nobody trusts — which is worse than a stale one,
// because it is the same kind of failure with a longer fuse.
func mergeWorkspace(project *Project, launch, tasks []any) func(map[string]any) (map[string]any, bool) {
	return func(existing map[string]any) (map[string]any, bool) {
		merged := copyOf(existing)
		changed := false

		if settingsChanged(merged, project.settings()) {
			changed = true
		}
		if extensionsChanged(merged, recommended(project)) {
			changed = true
		}

		launchConfigs := []any{}
		previousLaunch, hasLaunch := existing["launch"].(map[string]any)
		if hasLaunch {
			launchConfigs = object(previousLaunch, "configurations")
		}
		mergedLaunch, launchChanged := mergeNamed(launchConfigs, launch, "name")
		changed = changed || launchChanged
		launchDocument := map[string]any{"version": launchSchemaVersion, "configurations": mergedLaunch}
		if hasLaunch {
			// Whatever else the file's launch section holds — a `compounds` list, a
			// preLaunchTask of its own — is the person's.
			for field, value := range previousLaunch {
				if field != "configurations" && field != "version" {
					launchDocument[field] = value
				}
			}
		}
		merged["launch"] = launchDocument

		existingTasks := []any{}
		previousTasks, hasTasks := existing["tasks"].(map[string]any)
		if hasTasks {
			existingTasks = object(previousTasks, "tasks")
		}
		mergedTasks, tasksChanged := mergeNamed(existingTasks, tasks, "label")
		changed = changed || tasksChanged
		tasksDocument := map[string]any{"version": tasksSchemaVersion, "tasks": mergedTasks}
		if hasTasks {
			for field, value := range previousTasks {
				if field != "tasks" && field != "version" {
					tasksDocument[field] = value
				}
			}
		}
		merged["tasks"] = tasksDocument

		return merged, changed
	}
}

// settingsChanged fills in the generated settings a workspace does not have, and
// reports whether there were any.
func settingsChanged(merged map[string]any, generated map[string]any) bool {
	current, isObject := merged["settings"].(map[string]any)
	if !isObject {
		current = map[string]any{}
	}
	filled := copyOf(current)
	changed := false
	for _, key := range sortedKeysOf(generated) {
		if _, present := filled[key]; present {
			continue
		}
		filled[key] = generated[key]
		changed = true
	}
	merged["settings"] = filled
	return changed
}

// extensionsChanged adds the recommended extensions a workspace does not already
// ask for, and reports whether there were any.
//
// Additive rather than replacing for the same reason settings are: an extension a
// person already recommends in their workspace is one they chose.
func extensionsChanged(merged map[string]any, generated []any) bool {
	current, isObject := merged["extensions"].(map[string]any)
	if !isObject {
		current = map[string]any{}
	}
	present := map[string]bool{}
	for _, entry := range object(current, "recommendations") {
		if identifier, isString := entry.(string); isString {
			present[identifier] = true
		}
	}
	filled := copyOf(current)
	recommendations := append([]any{}, object(current, "recommendations")...)
	changed := false
	for _, entry := range generated {
		identifier, isString := entry.(string)
		if !isString || present[identifier] {
			continue
		}
		recommendations = append(recommendations, identifier)
		present[identifier] = true
		changed = true
	}
	filled["recommendations"] = recommendations
	merged["extensions"] = filled
	return changed
}

// copyOf is a shallow copy, which is what a merge needs: the values are read and
// replaced, never edited in place, so a value shared with the file that was read
// is not modified through the copy.
func copyOf(source map[string]any) map[string]any {
	out := make(map[string]any, len(source)+2)
	for key, value := range source {
		out[key] = value
	}
	return out
}

func sortedKeysOf(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
