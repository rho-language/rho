//
// @file vscode.go
// @brief The two files Visual Studio Code reads, rendered from a project.
//
// VS Code keeps a project's debug configurations and its tasks in JSON files
// under `.vscode/`, and both of them are the same shape: a list of entries, each
// named, each a command to run or a program to debug. So one generator writes
// both, from the same facts, and the entries it writes are the commands this
// toolchain actually has — `rho build`, `rho flash` — rather than a build
// invocation spelled out with absolute paths.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package ide

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"rho/src/targetspec"
)

// editorSchemaVersion is what VS Code's own files declare. It is the launch
// schema's version, which is separate from the tasks schema's below and is not
// the same number.
const (
	launchSchemaVersion = "0.2.0"
	tasksSchemaVersion  = "2.0.0"
)

// The key order each file is written in, so that regenerating it does not reorder
// it: a file that comes back different on every run is a file whose diffs stop
// being read.
var (
	launchOrder = []string{"version", "configurations", "compounds"}
	tasksOrder  = []string{"version", "tasks"}
)

// The tasks this toolchain contributes. A person sees these in the task list
// under their labels, so they are named the way they would be said.
const (
	TaskBuild = "rho: build"
	TaskClean = "rho: clean"
	TaskFlash = "rho: flash"
	TaskDocs  = "rho: docs"
	// taskType is how VS Code is told to run a task. A shell task because what is
	// run is a command a person could type, which is also how it is debugged by
	// hand when the task list is not enough.
	taskType = "shell"
	// problemsGC is the matcher for a C toolchain's diagnostics. Without one, a
	// build that fails shows its errors as terminal output and nothing navigable.
	problemsGC = "$gcc"
)

// The files an editor reads, relative to the project root.
const (
	launchPath = ".vscode/launch.json"
	tasksPath  = ".vscode/tasks.json"
)

// filesFor is what one generation writes, for the editor it is written for.
func filesFor(request Request, project *Project) ([]file, error) {
	switch request.Editor {
	case EditorVSCode:
		// The workspace file last: it is the one that refers to the other two, and
		// writing it after them means a generation that stops halfway leaves the
		// files an editor reads on their own still correct.
		return []file{vscodeLaunch(project), vscodeTasks(project), vscodeWorkspace(project)}, nil
	case EditorNeovim:
		// Neovim has no task system of its own, so a launch configuration is the
		// whole of what can be written for it. Its DAP client reads the same file
		// VS Code does when it is asked to, which is what makes one generation
		// serve both editors rather than two that drift apart.
		return []file{vscodeLaunch(project)}, nil
	}
	return nil, fmt.Errorf("no editor named %q", request.Editor)
}

/* -------------------------------------------------------------------------- */
/*  launch.json                                                                */
/* -------------------------------------------------------------------------- */

// vscodeLaunch is the file a debug session is configured in.
func vscodeLaunch(project *Project) file {
	generated := []any{}
	for _, configuration := range project.debugConfigurations() {
		generated = append(generated, configuration.fragment())
	}
	return file{
		Path:    launchPath,
		Order:   launchOrder,
		Content: render(launchDocument(generated), launchOrder),
		Merge:   mergeList("configurations", "name", launchSchemaVersion, generated),
	}
}

// launchDocument is a whole launch file: what an editor requires it to say about
// itself, and the configurations in it.
func launchDocument(configurations []any) map[string]any {
	return map[string]any{
		"version":        launchSchemaVersion,
		"configurations": configurations,
	}
}

// configuration is one entry in launch.json, as an ordered set of fields.
//
// Written as a list of pairs rather than a map so that the file reads in the
// order a person would write it in — the name, the adapter, the program — rather
// than in whatever order Go happens to iterate a map. Everything an editor reads
// from this file is a key it looks up, so the order here is for the reader.
type configuration []field

// field is one line of a configuration.
type field struct {
	key   string
	value any
}

func (c configuration) add(key string, value any) configuration {
	return append(c, field{key: key, value: value})
}

// addIf adds a field only when there is something to add, which is what keeps a
// generated entry from saying `"device": ""` to an adapter that would then be
// asked to use an empty board.
func (c configuration) addIf(key string, value string) configuration {
	if strings.TrimSpace(value) == "" {
		return c
	}
	return c.add(key, value)
}

// addListIf is addIf for a list, which an editor reads as absent when it is
// missing and as "an adapter with nothing configured" when it is empty.
func (c configuration) addListIf(key string, value []any) configuration {
	if len(value) == 0 {
		return c
	}
	return c.add(key, value)
}

// fragment renders the configuration in the order its fields were written in.
//
// Held as a rendered fragment rather than handed back as a map because a map has
// no order: this is what keeps a generated configuration reading as the person
// would have written it, and what makes regenerating the file produce the same
// bytes it produced last time.
func (c configuration) fragment() json.RawMessage {
	body := &strings.Builder{}
	body.WriteString("{")
	for index, one := range c {
		if index > 0 {
			body.WriteString(",")
		}
		body.WriteString("\n")
		body.WriteString(quote(one.key))
		body.WriteString(": ")
		encoded, err := marshalCompact(one.value)
		if err != nil {
			// A field whose value cannot be rendered is dropped rather than
			// written as something a reader would take for configuration.
			continue
		}
		body.WriteString(string(encoded))
	}
	body.WriteString("\n}")
	return json.RawMessage(body.String())
}

// debugConfigurations are the sessions a person can start on this project.
//
// Two kinds per board, and the difference is not a preference: a session driven by
// this toolchain's own adapter understands a target spec, starts the probe server
// if the board needs one, and exposes the peripheral register map; a session driven
// by a third-party adapter is what a project gets when that adapter is installed,
// and it is written from the same spec so the two agree about the board rather
// than disagreeing about it.
//
// A project written for several boards gets a launch and an attach for each of
// them, and the third-party session only for the board that was named — or for
// the only board, where the list has room for it. Six boards times three sessions
// is eighteen entries in a dropdown, which is a list nobody reads.
func (p *Project) debugConfigurations() []configuration {
	out := []configuration{}
	selected := p.targetFor()
	for _, board := range p.Boards {
		facts, _ := p.factsFor(board)
		if facts == nil {
			// No spec for this board, so there is nothing to say about its debugger
			// or its probe. A session is written anyway — for the machine rather than
			// for the board, which is what is true when the board could not be read —
			// because a project whose board cannot be read is a project whose person
			// needs the other thing on the board to work.
			p.Board = board
			out = append(out, p.hostConfiguration(board, "launch"))
			out = append(out, p.hostConfiguration(board, "attach"))
			continue
		}
		p.Board = board
		out = append(out, p.ownConfiguration(board, facts, "launch"))
		if board == selected {
			if adapter := facts.Debug.Adapter; adapter != "" && adapter != ownAdapter {
				out = append(out, p.adapterConfiguration(board, facts, adapter))
			}
		}
		out = append(out, p.ownConfiguration(board, facts, "attach"))
	}
	return out
}

// ownAdapter is the debug type this toolchain contributes. A launch configuration
// that names it is one this toolchain drives, through `rho dbg --dap`, which
// speaks the Debug Adapter Protocol to the editor and the Machine Interface to
// the debugger.
const ownAdapter = "rho"

// ownConfiguration is a session driven by this toolchain's own adapter.
//
// Every field it carries is a fact: the program is where the build writes it, the
// target is the board the spec names, and stopping at the entry point is what
// makes a session that flashes a board useful — otherwise the program is already
// past its own startup before anybody sees it.
func (p *Project) ownConfiguration(board string, facts *Target, request string) configuration {
	entry := configuration{}.add("name", sessionName(board, p.Name, request, "")).
		add("type", ownAdapter).
		add("request", request).
		add("cwd", workspaceFolder).
		add("program", workspacePath(p.Binary)).
		add("target", board)
	if request == "launch" {
		// The build names the same board this session does, keyed on the board that
		// was asked for rather than on whatever the spec calls itself: a session that
		// built for one board and debugged another stops on a line in a program that
		// is not the one on the chip, which reads as a debugger that is wrong.
		entry = entry.add("stopAtEntry", true).add("preLaunchTask", p.buildTask(board))
	}
	// An attach is to something already running: the program is loaded for its
	// symbols, not run, so there is nothing to build first and no entry point to
	// stop before.
	return entry
}

// adapterConfiguration is a session driven by the adapter a target spec names.
//
// Only the fields that adapter is known to read are written. An adapter that was
// handed a field it does not know says so in its own log, and a configuration
// that produces a warning on every launch is worse than one that says less.
func (p *Project) adapterConfiguration(board string, facts *Target, adapter string) configuration {
	debug := facts.Debug
	entry := configuration{}.add("name", sessionName(board, p.Name, "adapter", adapter)).
		add("type", adapter).
		add("request", "launch").
		add("cwd", workspaceFolder).
		add("executable", workspacePath(p.Binary)).
		add("preLaunchTask", p.buildTask(board)).
		add("stopAtEntry", true).
		add("runToEntryPoint", "main")
	entry = entry.addIf("servertype", targetspec.FirstNotEmpty(debug.ServerType, "gdb"))
	entry = entry.addIf("gdbPath", p.debuggerFor(facts))
	entry = entry.addIf("device", debug.Device)
	entry = entry.addIf("interface", debug.Interface)
	entry = entry.addListIf("configFiles", targetspec.StringsOf(debug.Config))
	entry = entry.addIf("serverpath", p.serverPath(facts))
	if svd, found := p.svdFile(facts); found {
		entry = entry.addIf("svdFile", svd)
	}
	return entry
}

// hostConfiguration is a session for a program running on this machine.
//
// No board, so no probe and no register map: what is left is a debugger started
// on a program, which is the host case and the reason this works at all.
func (p *Project) hostConfiguration(board string, request string) configuration {
	entry := configuration{}.add("name", sessionName(board, p.Name, request, "")).
		add("type", ownAdapter).
		add("request", request).
		add("cwd", workspaceFolder).
		add("program", workspacePath(p.Binary))
	if facts, found := p.factsFor(board); found {
		entry = entry.addIf("gdbPath", p.debuggerFor(facts))
	} else {
		entry = entry.addIf("gdbPath", DebuggerFor(""))
	}
	if request == "launch" {
		entry = entry.add("stopAtEntry", true).add("preLaunchTask", p.buildTask(board))
	}
	return entry
}

// sessionName is what a person reads in the dropdown, so it says the board and
// what the session is doing. `Debug pico (attach)` is a different thing to reach
// for than `Debug pico`.
// sessionName is what a person reads in the dropdown, so it says the board and
// what the session is doing. `Debug pico (attach)` is a different thing to reach
// for than `Debug pico`.
func sessionName(board string, project string, request string, adapter string) string {
	switch request {
	case "attach":
		return fmt.Sprintf("Debug %s (attach)", board)
	case "adapter":
		return fmt.Sprintf("Debug %s (%s)", board, adapter)
	default:
		return fmt.Sprintf("Debug %s", board)
	}
}

// debugger is the debugger program a session runs, which is either the one the
// target named or the one that goes with the compiler this target is built with.
func (p *Project) debuggerFor(facts *Target) string {
	if facts == nil {
		return ""
	}
	if tool := strings.TrimSpace(facts.Debug.Tool); tool != "" {
		return tool
	}
	return DebuggerFor(facts.Toolchain.Compiler)
}

// serverPath is where the probe server is, when the board needs one running and
// this machine says where it is.
func (p *Project) serverPath(facts *Target) string {
	if facts == nil {
		return ""
	}
	return facts.Debug.Server
}

// workspaceFolder is the token VS Code substitutes with the folder the project is
// open at. It is used rather than a path written here, because the configuration
// is committed and the folder it opens at is not the same on the machine that
// wrote it.
const workspaceFolder = "${workspaceFolder}"

// TaskBuildFor is the build task for one board, named for it.
//
// Named rather than numbered, because a person choosing between two build tasks is
// choosing a board, and `rho: build 2` is not something anybody can choose.
func TaskBuildFor(board string) string { return fmt.Sprintf("%s (%s)", TaskBuild, board) }

// taskLabel is a task's name for one board: the bare name when the project is
// written for one board, and the board's when it is written for several.
//
// A single board's list has one build in it, and `rho: build (pico)` in it says
// something — that a choice was made — that nobody needs told about.
func taskLabel(label string, board string, boards []string) string {
	if len(boards) <= 1 {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, board)
}

// TaskFlashFor is the flash task for one board.
func TaskFlashFor(board string) string { return fmt.Sprintf("%s (%s)", TaskFlash, board) }

// workspacePath is a project path as an editor reads it: rooted at the workspace
// rather than absolute, for the same reason.
func workspacePath(relative string) string {
	if relative == "" {
		return workspaceFolder
	}
	return workspaceFolder + "/" + path.Clean(relative)
}

/* -------------------------------------------------------------------------- */
/*  tasks.json                                                                  */
/* -------------------------------------------------------------------------- */

// vscodeTasks is the file a project's tasks are declared in.
func vscodeTasks(project *Project) file {
	generated := []any{}
	for _, one := range project.tasks() {
		generated = append(generated, one.fragment())
	}
	return file{
		Path:    tasksPath,
		Order:   tasksOrder,
		Content: render(tasksDocument(generated), tasksOrder),
		Merge:   mergeList("tasks", "label", tasksSchemaVersion, generated),
	}
}

func tasksDocument(tasks []any) map[string]any {
	return map[string]any{
		"version": tasksSchemaVersion,
		"tasks":   tasks,
	}
}

// task is one entry in tasks.json.
type task []field

func (t task) add(key string, value any) task { return append(t, field{key: key, value: value}) }

func (t task) addIf(key string, value string) task {
	if strings.TrimSpace(value) == "" {
		return t
	}
	return t.add(key, value)
}

// fragment renders the task in the order its fields were written in, which is why
// it is held as fields rather than as a map: see configuration.fragment.
func (t task) fragment() json.RawMessage {
	fields := configuration(t)
	return fields.fragment()
}

// tasks are the commands a person can run from this project, each of them one
// this toolchain has.
//
// They are the toolchain's own commands rather than the build systems underneath
// it: `rho build` knows which board to build for and where the binary lands, and a
// task that spelled out a cmake invocation would have to be rewritten by hand
// every time the board changed.
//
// A project written for several boards has a build for each, named for the board,
// so that a session can build for the board it is about to debug. The one with no
// board in its name is the project's default and is the one the editor's build
// button finds.
func (p *Project) tasks() []task {
	out := []task{
		buildTaskFor(TaskBuild, p.defaultBoard(), p.buildArguments(p.defaultBoard()), true),
		cleanTask(),
		docsTask(),
	}

	for _, board := range p.Boards {
		if board == p.defaultBoard() {
			continue
		}
		out = append(out, buildTaskFor(
			p.buildTask(board), board, p.buildArguments(board), false))
	}

	// Flashing is a board's own command, so it is written only for the boards that
	// declare one — a task that flashes nothing is a task in the list that fails
	// when it is chosen. One per board, because writing to the wrong board is not a
	// mistake worth a button with another board's name on it.
	for _, board := range p.Boards {
		facts, found := p.factsFor(board)
		if !found || facts.Flash.Tool == "" {
			continue
		}
		out = append(out, task{}.add("label", taskLabel(TaskFlash, board, p.Boards)).
			add("type", taskType).
			add("command", "rho").
			add("args", []any{"flash", "-target", board, workspacePath(p.Binary)}).
			add("options", map[string]any{"cwd": workspaceFolder}).
			add("problemMatcher", []any{problemsGC}).
			addIf("detail", fmt.Sprintf("Write the built binary to %s with %s", board, facts.Flash.Tool)))
	}
	return out
}

// buildTaskFor is the task that builds for one board.
func buildTaskFor(label string, board string, arguments []any, isDefault bool) task {
	built := task{}.add("label", label).
		add("type", taskType).
		add("command", "rho").
		add("args", arguments).
		add("options", map[string]any{"cwd": workspaceFolder}).
		add("problemMatcher", []any{problemsGC}).
		addIf("detail", fmt.Sprintf("Build this project for %s", board))
	if isDefault {
		// The default is what the editor's build button runs, and the Problems panel
		// needs a matcher to have anything to show at all.
		built = built.add("group", map[string]any{"kind": "build", "isDefault": true})
	}
	return built
}

// cleanTask removes everything a build generated.
//
// Through the toolchain's own command rather than a deletion: a generated tree holds
// CMake caches with absolute paths in them, and a build directory half-removed by
// hand fails in ways that read like the toolchain is at fault.
func cleanTask() task {
	return task{}.add("label", TaskClean).
		add("type", taskType).
		add("command", "rho").
		add("args", []any{"build", ".", "-clean"}).
		add("options", map[string]any{"cwd": workspaceFolder}).
		add("problemMatcher", []any{problemsGC}).
		add("detail", "Remove everything a build generated")
}

// docsTask generates this project's documentation, which is the other thing a build
// produces and the other thing a project wants in a task list.
func docsTask() task {
	return task{}.add("label", TaskDocs).
		add("type", taskType).
		add("command", "rho").
		add("args", []any{"doc", ".", "-out", "build/docs"}).
		add("options", map[string]any{"cwd": workspaceFolder}).
		add("detail", "Generate this project's documentation")
}

// buildArguments are the build command's arguments for one board.
//
// The board is named with `-target` when the project has more than one, which is
// also how the build itself is told: a build with no board named defaults to the
// host, and a project written for boards is not written for the host.
func (p *Project) buildArguments(board string) []any {
	arguments := []any{"build", "."}
	if len(p.Boards) > 1 || (p.Selected != "" && p.Selected != "native") {
		arguments = append(arguments, "-target", board)
	}
	return arguments
}

// buildTask is the label of the task that builds for a board.
//
// The same label the task is written under: the project's default build for the
// first board it declares, because that is the one the editor's build button finds,
// and the board's own name for the rest.
func (p *Project) buildTask(board string) string {
	if board == p.defaultBoard() {
		return TaskBuild
	}
	return taskLabel(TaskBuild, board, p.Boards)
}
