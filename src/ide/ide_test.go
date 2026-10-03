//
// @file ide_test.go
// @brief Tests for what `rho ide` writes.
//
// These are about the file a person opens: that the paths in it are the paths a
// build produces, that the two kinds of session agree about the board, and that
// a generation over a file somebody has edited keeps what they wrote. A generator
// that produces valid JSON which names a path no build writes is the failure
// these exist to catch.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectOf writes a project with a manifest and returns its directory.
func projectOf(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const blinkyManifest = `module:
  name: blinky
  type: rho
  version: 0.1.0
  description: A blinking LED

main: src/main.rho

build:
  language: c
  toolchain: pico
`

// picoFacts is what `rho target show -json pico` answers with: the resolved spec,
// with the board's chip and architecture already folded in.
const picoFacts = `{
  "name": "pico", "kind": "board", "core": "cortex-m0plus",
  "toolchain": {"compiler": "arm-none-eabi-gcc", "objcopy": "arm-none-eabi-objcopy"},
  "memory": {"flash": {"origin": "0x10000000", "length": "0x200000"},
             "ram": {"origin": "0x20000000", "length": "0x40000"}},
  "flash": {"tool": "picotool", "command": "picotool load -f -x {binary}"},
  "debug": {
    "tool": "arm-none-eabi-gdb",
    "server": "openocd",
    "adapter": "cortex-debug",
    "server_type": "gdb",
    "interface": "cmsis-dap",
    "config": ["interface/cmsis-dap.cfg", "target/rp2040.cfg"],
    "svd": "rp2040.svd",
    "port": "3333",
    "load": true
  }
}`

// forPico is a resolver that answers the way the component would: with the
// board's resolved spec when the board is what was asked for, and with the
// host's when it is not. A resolver that answered with a board whatever it was
// asked for would make the host cases test nothing.
func forPico(asked *string) func(string, string) (*Target, error) {
	return func(_ string, target string) (*Target, error) {
		*asked = target
		if target == "pico" {
			return ParseTarget([]byte(picoFacts))
		}
		return ParseTarget([]byte(`{
		  "name": "native", "kind": "arch",
		  "toolchain": {"compiler": "gcc", "size": "size"}
		}`))
	}
}

// raw reads a file a generation wrote, as it is on disk.
func raw(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// generated reads one of the files a generation wrote.
func generated(t *testing.T, root, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", name, err, data)
	}
	return document
}

// configurations are the entries of a launch file, by name.
func configurations(document map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	entries, _ := document["configurations"].([]any)
	for _, entry := range entries {
		object, isObject := entry.(map[string]any)
		if !isObject {
			continue
		}
		name, _ := object["name"].(string)
		out[name] = object
	}
	return out
}

// tasks are the entries of a tasks file, by label.
func tasks(document map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	entries, _ := document["tasks"].([]any)
	for _, entry := range entries {
		object, isObject := entry.(map[string]any)
		if !isObject {
			continue
		}
		label, _ := object["label"].(string)
		out[label] = object
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  launch.json                                                                */
/* -------------------------------------------------------------------------- */

// The program a session loads has to be the file a build writes, which is the
// project's own output directory under the name CMake builds it under. A
// configuration that names anything else opens a session against a program that
// is not there.
func TestTheProgramIsWhereTheBuildWritesIt(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	if asked != "pico" {
		t.Fatalf("expected the target the manifest declares to be resolved, got %q", asked)
	}

	entries := configurations(generated(t, root, launchPath))
	session, found := entries["Debug pico"]
	if !found {
		t.Fatalf("expected a session for the board, got %v", names(entries))
	}
	if got := session["program"]; got != "${workspaceFolder}/build/output/blinky" {
		t.Errorf("expected the built binary where the build writes it, got %v", got)
	}
	if got := session["cwd"]; got != "${workspaceFolder}" {
		t.Errorf("expected the session to run in the workspace, got %v", got)
	}
}

// A board's facts reach both sessions that can start on it, and they say the same
// board. Two configurations that disagree about the probe are two answers to the
// question "what board is this", and a person picks between them without noticing.
func TestBothSessionsAgreeAboutTheBoard(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := configurations(generated(t, root, launchPath))

	own, found := entries["Debug pico"]
	if !found {
		t.Fatalf("expected a session for the board, got %v", names(entries))
	}
	third, found := entries["Debug pico (cortex-debug)"]
	if !found {
		t.Fatalf("expected a session for the adapter the board names, got %v", names(entries))
	}

	if own["type"] != "rho" {
		t.Errorf("expected this toolchain's own adapter, got %v", own["type"])
	}
	if third["type"] != "cortex-debug" {
		t.Errorf("expected the adapter the board names, got %v", third["type"])
	}
	if own["program"] != third["executable"] {
		t.Errorf("expected both sessions to load the same program, got %v and %v", own["program"], third["executable"])
	}
	if third["gdbPath"] != "arm-none-eabi-gdb" {
		t.Errorf("expected the board's debugger, got %v", third["gdbPath"])
	}
	if third["servertype"] != "gdb" {
		t.Errorf("expected the probe's protocol, got %v", third["servertype"])
	}
	if third["interface"] != "cmsis-dap" {
		t.Errorf("expected the board's probe, got %v", third["interface"])
	}
	// The probe server is told which config files to read, in the order the
	// server itself wants them: the interface first, because the target config
	// refers to it.
	configFiles, _ := third["configFiles"].([]any)
	if len(configFiles) != 2 || configFiles[0] != "interface/cmsis-dap.cfg" || configFiles[1] != "target/rp2040.cfg" {
		t.Errorf("expected the board's probe configuration, got %v", third["configFiles"])
	}
	if third["serverpath"] != "openocd" {
		t.Errorf("expected the probe server to be named, got %v", third["serverpath"])
	}
	if third["preLaunchTask"] != TaskBuild {
		t.Errorf("expected a session to build first, got %v", third["preLaunchTask"])
	}
}

// Attaching is a different thing to reach for than launching, so it is a separate
// entry: to a program that is already running, with nothing built first.
func TestAttachingIsItsOwnSession(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := configurations(generated(t, root, launchPath))

	attach, found := entries["Debug pico (attach)"]
	if !found {
		t.Fatalf("expected an attach session, got %v", names(entries))
	}
	if attach["request"] != "attach" {
		t.Errorf("expected an attach request, got %v", attach["request"])
	}
	// The symbols are still needed, so the program is named; there is nothing to
	// build first, because nothing is being started.
	if attach["program"] == nil {
		t.Error("expected the program to be named for its symbols")
	}
	if _, present := attach["preLaunchTask"]; present {
		t.Error("expected an attach not to build a program it is not starting")
	}
}

// A register map is named only when this project has the file. A `svdFile` that
// points at a file which is not there gives a person a peripheral tree that is
// empty, and an empty one reads as a chip with no peripherals in it.
func TestTheRegisterMapIsNamedOnlyWhenTheProjectHasIt(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, map[string]string{
		"vendor/rp2040.svd": "<device/>",
	})
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := configurations(generated(t, root, launchPath))
	third := entries["Debug pico (cortex-debug)"]
	if third["svdFile"] != "vendor/rp2040.svd" {
		t.Errorf("expected the register map the project carries, got %v", third["svdFile"])
	}

	// The same board, in a project that does not vendor the file.
	bare := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: bare, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries = configurations(generated(t, bare, launchPath))
	if _, present := entries["Debug pico (cortex-debug)"]["svdFile"]; present {
		t.Error("expected no register map to be named for a project that has none")
	}
}

// A project that builds for the machine it is built on has no board, no probe and
// no register map, and is configured for exactly that: a debugger on a program.
func TestAHostProjectIsConfiguredForThisMachine(t *testing.T) {
	asked := ""
	root := projectOf(t, `module:
  name: tool
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
`, nil)
	_, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "native" {
		t.Fatalf("expected the host to be the target of a project that declares none, got %q", asked)
	}
	entries := configurations(generated(t, root, launchPath))

	session, found := entries["Debug native"]
	if !found {
		t.Fatalf("expected a session on the host, got %v", names(entries))
	}
	if session["type"] != "rho" || session["program"] != "${workspaceFolder}/build/output/tool" {
		t.Errorf("expected a session on the built program, got %v", session)
	}
	// No board means no probe server and no register map, and a session that
	// carries them would ask a machine to start a board it does not have.
	for _, key := range []string{"servertype", "gdbPath", "device", "interface", "svdFile", "serverpath"} {
		if _, present := session[key]; present {
			t.Errorf("expected no %s on a host session, got %v", key, session[key])
		}
	}
	// The host's own debugger is derived from its compiler rather than declared.
	if _, present := entries["Debug native (attach)"]; !present {
		t.Errorf("expected an attach session as well, got %v", names(entries))
	}
	for name := range entries {
		if name != "Debug native" && name != "Debug native (attach)" {
			t.Errorf("expected no third-party session for a host build, got %q", name)
		}
	}
}

// A project whose target cannot be read is still configured, and is configured
// for the host rather than for a board nobody could find out about.
func TestATargetThatCannotBeReadStillGetsAConfiguration(t *testing.T) {
	root := projectOf(t, blinkyManifest, nil)
	result, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode,
		Resolve: func(string, string) (*Target, error) {
			return nil, errNoSuchTarget
		},
	})
	if err != nil {
		t.Fatalf("expected a configuration even with no target spec, got %v", err)
	}
	if len(result.Written) != 3 {
		t.Fatalf("expected every file to be written, got %v", result.Written)
	}
	entries := configurations(generated(t, root, launchPath))
	// A launch and an attach, for the machine: the board could not be read, and a
	// project whose board cannot be read still needs the other thing to work.
	if len(entries) != 2 {
		t.Fatalf("expected a launch and an attach for the host, got %v", names(entries))
	}
	if entries["Debug pico"]["type"] != "rho" || entries["Debug pico (attach)"]["type"] != "rho" {
		t.Errorf("expected sessions this toolchain can run, got %v", entries)
	}
}

// With no resolver at all — no target component on this machine — the same holds:
// the host needs no probe, so the configuration is written and the reason it is
// the simpler one is reported rather than guessed at.
func TestWithoutATargetResolverTheHostIsStillConfigured(t *testing.T) {
	root := projectOf(t, blinkyManifest, nil)
	result, err := Generate(Request{ProjectRoot: root, Editor: EditorVSCode})
	if err != nil {
		t.Fatalf("expected a configuration without a resolver, got %v", err)
	}
	if len(result.Written) != 3 {
		t.Fatalf("expected every file to be written, got %v", result.Written)
	}
}

/* -------------------------------------------------------------------------- */
/*  tasks.json                                                                 */
/* -------------------------------------------------------------------------- */

// A task is the toolchain's own command rather than the build system underneath
// it, because only the toolchain knows which target to build for and where the
// binary lands.
func TestTasksAreThisToolchainsOwnCommands(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := tasks(generated(t, root, tasksPath))

	build, found := entries[TaskBuild]
	if !found {
		t.Fatalf("expected a build task, got %v", names(entries))
	}
	if build["command"] != "rho" {
		t.Errorf("expected the toolchain to be run, got %v", build["command"])
	}
	arguments, _ := build["args"].([]any)
	if len(arguments) != 2 || arguments[0] != "build" || arguments[1] != "." {
		t.Errorf("expected the build command, got %v", build["args"])
	}
	// Without a matcher a failing build shows errors as terminal output and
	// nothing navigable, which is most of what a task list is for.
	if matchers, _ := build["problemMatcher"].([]any); len(matchers) != 1 || matchers[0] != "$gcc" {
		t.Errorf("expected the C toolchain's diagnostics, got %v", build["problemMatcher"])
	}
	// The build task is the default, because it is the one a session runs first
	// and the one a person reaches for.
	group, _ := build["group"].(map[string]any)
	if group["isDefault"] != true {
		t.Errorf("expected the build to be the default task, got %v", build["group"])
	}

	// Flashing is a board's own command, so it is here for a board that has one.
	flash, found := entries[TaskFlash]
	if !found {
		t.Fatalf("expected a flash task for a board that declares one, got %v", names(entries))
	}
	arguments, _ = flash["args"].([]any)
	if len(arguments) != 4 || arguments[0] != "flash" || arguments[1] != "-target" ||
		arguments[2] != "pico" || arguments[3] != "${workspaceFolder}/build/output/blinky" {
		t.Errorf("expected the flash command for the board, got %v", flash["args"])
	}
}

// A target with no way of reaching a board gets no flash task: a task in the list
// that fails when it is chosen is worse than a task that is not there.
func TestABoardWithNoFlashToolHasNoFlashToolTask(t *testing.T) {
	root := projectOf(t, blinkyManifest, nil)
	resolve := func(string, string) (*Target, error) {
		return ParseTarget([]byte(`{"name": "pico", "kind": "board",
		  "debug": {"tool": "arm-none-eabi-gdb"}}`))
	}
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: resolve,
	}); err != nil {
		t.Fatal(err)
	}
	entries := tasks(generated(t, root, tasksPath))
	if _, found := entries[TaskFlash]; found {
		t.Error("expected no flash task for a board that declares no way to reach it")
	}
	if _, found := entries[TaskBuild]; !found {
		t.Error("expected a build task regardless of what the board declares")
	}
}

/* -------------------------------------------------------------------------- */
/*  Merging                                                                    */
/* -------------------------------------------------------------------------- */

// The files are generated, not written over: a project that has configurations or
// tasks of its own keeps them, and the generated ones are refreshed.
func TestAGeneratedFileKeepsWhatWasInIt(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, map[string]string{
		launchPath: `{
		  "version": "0.2.0",
		  "configurations": [
		    {"name": "Mine", "type": "rho", "request": "launch"},
		    {"name": "Debug pico", "type": "mine", "request": "launch"}
		  ]
		}`,
		tasksPath: `{"version": "2.0.0", "tasks": [{"label": "mine", "type": "shell", "command": "make"}]}`,
	})
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}

	entries := configurations(generated(t, root, launchPath))
	if _, found := entries["Mine"]; !found {
		t.Errorf("expected the configuration that was already there to be kept, got %v", names(entries))
	}
	// A generated name is this toolchain's own, so the generated entry is the
	// current answer. Had the file been merged by position, the stale entry would
	// have been kept instead and every session would have launched the wrong
	// thing.
	if entries["Debug pico"]["type"] != "rho" {
		t.Errorf("expected the generated session to replace the stale one of the same name, got %v", entries["Debug pico"])
	}
	if _, found := entries["Debug pico (cortex-debug)"]; !found {
		t.Errorf("expected the generated sessions to be added, got %v", names(entries))
	}

	taskEntries := tasks(generated(t, root, tasksPath))
	if _, found := taskEntries["mine"]; !found {
		t.Errorf("expected the task that was already there to be kept, got %v", names(taskEntries))
	}
	if _, found := taskEntries[TaskBuild]; !found {
		t.Errorf("expected the generated tasks to be added, got %v", names(taskEntries))
	}
}

// A file that is not JSON at all is refused rather than replaced. There is no way
// to merge into something that cannot be parsed, and no safe way to discard what
// is in it — the comments in a launch.json a person wrote are as much of theirs
// as the entries are.
func TestAFileThatIsNotJSONIsRefused(t *testing.T) {
	root := projectOf(t, blinkyManifest, map[string]string{
		launchPath: "// mine\n{\n  \"version\": \"0.2.0\",\n}\n",
	})
	_, err := Generate(Request{ProjectRoot: root, Editor: EditorVSCode})
	if err == nil {
		t.Fatal("expected a launch file with comments in it to be refused")
	}
	if !strings.Contains(err.Error(), launchPath) {
		t.Errorf("expected the refusal to name the file, got %v", err)
	}
	// And the file is still there, untouched.
	kept, readErr := os.ReadFile(filepath.Join(root, launchPath))
	if readErr != nil || !strings.Contains(string(kept), "// mine") {
		t.Errorf("expected the file to be left as it was, got %q (%v)", kept, readErr)
	}
}

// Running a generation twice changes nothing the second time. A generated file
// that comes back different on every run teaches people to ignore its diffs,
// which is how the one that mattered goes unread.
func TestAGenerationIsIdempotent(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	for round := 0; round < 2; round++ {
		result, err := Generate(Request{
			ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
		})
		if err != nil {
			t.Fatal(err)
		}
		if round == 0 {
			if len(result.Written) != 3 {
				t.Fatalf("expected every file to be written, got %v", result.Written)
			}
			continue
		}
		if len(result.Written) != 0 {
			t.Errorf("expected nothing to be written the second time, got %v", result.Written)
		}
		if len(result.Merged) != 0 {
			t.Errorf("expected nothing to be merged the second time, got %v", result.Merged)
		}
		if len(result.Unchanged) != 3 {
			t.Errorf("expected every file to be reported unchanged, got %v", result.Unchanged)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Editors                                                                    */
/* -------------------------------------------------------------------------- */

func TestAnEditorIsNamedByWhatItIs(t *testing.T) {
	for _, name := range []string{"vscode", "VSCode", " vscode "} {
		if editor, err := ParseEditor(name); err != nil || editor != EditorVSCode {
			t.Errorf("%q: expected vscode, got %q (%v)", name, editor, err)
		}
	}
	if _, err := ParseEditor("emacs"); err == nil {
		t.Error("expected an editor this does not write for to be refused by name")
	}
}

// Neovim has no task system of its own, so a launch configuration is the whole of
// what can be written for it — and it is the same file, which is what stops the
// two editors' configurations from drifting apart.
func TestNeovimGetsALaunchConfigurationAndNoTasks(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	result, err := Generate(Request{
		ProjectRoot: root, Editor: EditorNeovim, Resolve: forPico(&asked),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 1 || result.Written[0] != launchPath {
		t.Fatalf("expected just a launch configuration, got %v", result.Written)
	}
	if _, err := os.Stat(filepath.Join(root, tasksPath)); err == nil {
		t.Error("expected no tasks file for an editor that has no tasks")
	}
}

func TestAProjectWithoutAManifestIsRefused(t *testing.T) {
	root := projectOf(t, "", nil)
	if _, err := Generate(Request{ProjectRoot: root, Editor: EditorVSCode}); err == nil {
		t.Error("expected a directory that is not a project to be refused")
	}
	if _, err := Generate(Request{
		ProjectRoot: filepath.Join(root, "nowhere"), Editor: EditorVSCode,
	}); err == nil {
		t.Error("expected a directory that is not there to be refused")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// errNoSuchTarget is what a resolver says when the component cannot find the
// target. A local error rather than a formatted one, so the test does not depend
// on the wording.
var errNoSuchTarget = errorString("no target named \"pico\"")

type errorString string

func (e errorString) Error() string { return string(e) }

func names(entries map[string]map[string]any) []string {
	out := make([]string, 0, len(entries))
	for name := range entries {
		out = append(out, name)
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  The workspace file                                                         */
/* -------------------------------------------------------------------------- */

// One file that opens a project ready to build and debug: the sessions and the
// tasks inline, the compiler settings the target declares, and the extensions
// they need. Opening a workspace file is then the whole of setting an editor up,
// which is what a person means by "configure and run the debugger" without
// having to.
func TestAWorkspaceCarriesTheSessionsTheTasksAndTheTarget(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	workspace := generated(t, root, "blinky.code-workspace")

	// The same sessions and the same tasks as the two folder files, so a person
	// who opens either gets the same thing.
	inlined := configurations(workspace["launch"].(map[string]any))
	if len(inlined) != 3 {
		t.Errorf("expected the sessions inline in the workspace, got %v", names(inlined))
	}
	folder := configurations(generated(t, root, launchPath))
	for name, entry := range folder {
		if inlined[name]["program"] != entry["program"] && inlined[name]["executable"] != entry["executable"] {
			t.Errorf("expected %q to load the same program in both places", name)
		}
	}
	inlinedTasks := tasks(workspace["tasks"].(map[string]any))
	if _, found := inlinedTasks[TaskBuild]; !found {
		t.Errorf("expected the tasks inline in the workspace, got %v", names(inlinedTasks))
	}

	// The compiler the target is built with, and the language the generated
	// project is built as: an editor parsing a bare-metal header with the host's
	// settings reports errors in code that compiles.
	settings := workspace["settings"].(map[string]any)
	cpp, isObject := settings["C_Cpp.default"].(map[string]any)
	if !isObject {
		t.Fatalf("expected the target's own compiler settings, got %v", settings)
	}
	if cpp["compilerPath"] != "arm-none-eabi-gcc" {
		t.Errorf("expected the target's compiler, got %v", cpp["compilerPath"])
	}
	if cpp["cStandard"] != "c11" {
		t.Errorf("expected the standard the generated project is built as, got %v", cpp["cStandard"])
	}
	if cpp["intelliSenseMode"] != "gcc-arm" {
		t.Errorf("expected the parsing mode for a cross toolchain, got %v", cpp["intelliSenseMode"])
	}
	if associations, _ := settings["files.associations"].(map[string]any); associations["*.rho"] != "rho" {
		t.Errorf("expected a Rho file to be associated with the Rho language, got %v", settings["files.associations"])
	}

	// The adapter the board names, and this toolchain's own language support.
	recommendations := recommendationsOf(workspace)
	for _, wanted := range []string{ExtensionLanguage, "marus25.cortex-debug"} {
		found := false
		for _, got := range recommendations {
			if got == wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q to be recommended, got %v", wanted, recommendations)
		}
	}
}

// A host build needs no probe adapter, so none is recommended: recommending one
// would install an extension nothing in the project uses.
func TestAHostWorkspaceRecommendsNoProbeAdapter(t *testing.T) {
	asked := ""
	root := projectOf(t, `module:
  name: tool
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
`, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	workspace := generated(t, root, "tool.code-workspace")
	settings := workspace["settings"].(map[string]any)
	if _, present := settings["C_Cpp.default"]; present {
		// The host's own compiler is the one the editor is already using, so
		// naming it here would only pin it to whatever this machine had.
		t.Errorf("expected no compiler settings for a host build, got %v", settings["C_Cpp.default"])
	}
	recommendations := recommendationsOf(workspace)
	for _, got := range recommendations {
		if got == "marus25.cortex-debug" {
			t.Errorf("expected no probe adapter for a host build, got %v", recommendations)
		}
	}
}

// A setting a person has changed is one they changed for a reason, so a generation
// fills in only what is missing. The sessions still refresh — those are this
// toolchain's own names, and a stale one launches the wrong thing.
func TestAWorkspaceKeepsTheSettingsSomebodyChanged(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, map[string]string{
		"blinky.code-workspace": `{
		  "folders": [{"path": "."}],
		  "settings": {"C_Cpp.default": {"compilerPath": "/opt/my/gcc"}, "editor.tabSize": 2},
		  "extensions": {"recommendations": ["someone.else"]}
		}`,
	})
	result, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Merged) != 1 || result.Merged[0] != "blinky.code-workspace" {
		t.Fatalf("expected the workspace to be updated, got %v %v", result.Merged, result.Unchanged)
	}
	workspace := generated(t, root, "blinky.code-workspace")

	settings := workspace["settings"].(map[string]any)
	cpp := settings["C_Cpp.default"].(map[string]any)
	if cpp["compilerPath"] != "/opt/my/gcc" {
		t.Errorf("expected the compiler that was already set to be kept, got %v", cpp["compilerPath"])
	}
	if tabSize := settings["editor.tabSize"]; tabSize != float64(2) {
		t.Errorf("expected a setting of their own to be kept, got %v", tabSize)
	}
	// What was missing is filled in, which is what makes the file worth writing.
	if _, present := settings["files.associations"]; !present {
		t.Errorf("expected the missing settings to be filled in, got %v", settings)
	}
	recommendations := recommendationsOf(workspace)
	for _, wanted := range []string{"someone.else", ExtensionLanguage} {
		found := false
		for _, got := range recommendations {
			if got == wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q to be recommended, got %v", wanted, recommendations)
		}
	}
	// And the sessions are still there, refreshed from the board.
	if len(configurations(workspace["launch"].(map[string]any))) != 3 {
		t.Errorf("expected the sessions inline in the updated workspace")
	}
}

// The workspace is a file like the other two: running a generation over it again
// changes nothing.
func TestAWorkspaceGenerationIsIdempotent(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, map[string]string{
		"blinky.code-workspace": `{"folders": [{"path": "."}], "settings": {"editor.tabSize": 2}}`,
	})
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	before := raw(t, root, "blinky.code-workspace")

	result, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Merged) != 0 || len(result.Written) != 0 {
		t.Errorf("expected nothing to change the second time, got %v %v", result.Written, result.Merged)
	}
	if after := raw(t, root, "blinky.code-workspace"); after != before {
		t.Errorf("expected the same bytes the second time:\n%s\n%s", before, after)
	}
}

// recommendationsOf are the extensions a workspace asks for.
func recommendationsOf(workspace map[string]any) []string {
	extensions, isObject := workspace["extensions"].(map[string]any)
	if !isObject {
		return nil
	}
	out := []string{}
	for _, entry := range object(extensions, "recommendations") {
		if identifier, isString := entry.(string); isString {
			out = append(out, identifier)
		}
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  A project written for several boards                                       */
/* -------------------------------------------------------------------------- */

// The board this project is written for comes from its presets when it says no
// single one, and each of them gets a session — because a person building this
// project chose a board when they built it, and the dropdown is where they say so.
func TestAPresetListIsOneSessionPerBoard(t *testing.T) {
	root := projectOf(t, `module:
  name: hello-world
  type: module
  version: 0.1.0
  description: The same program on several boards

main: src/main.c

build:
  language: c
  presets:
    - pico
    - esp32c3
`, nil)

	resolved := []string{}
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode,
		Resolve: func(_ string, board string) (*Target, error) {
			resolved = append(resolved, board)
			if board == "esp32c3" {
				return nil, errorString("no such target")
			}
			return ParseTarget([]byte(picoFacts))
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Every board is resolved, because every entry in the dropdown has to say which
	// debugger and which probe its board uses.
	if len(resolved) != 2 {
		t.Errorf("expected both boards to be resolved, got %v", resolved)
	}

	entries := configurations(generated(t, root, launchPath))
	for _, wanted := range []string{"Debug pico", "Debug pico (attach)", "Debug esp32c3", "Debug esp32c3 (attach)"} {
		if _, found := entries[wanted]; !found {
			t.Errorf("expected a session for %q, got %v", wanted, names(entries))
		}
	}
	// And no third-party session for either: six boards times three sessions is a
	// list nobody reads, and the one is offered to whoever names a board.
	for name := range entries {
		if name != "Debug pico" && name != "Debug pico (attach)" &&
			name != "Debug esp32c3" && name != "Debug esp32c3 (attach)" {
			t.Errorf("expected no third-party session for a board nobody named, got %q", name)
		}
	}

	// A board nobody named is configured anyway, and the failure to read it is what
	// the session says it has instead.
	if entries["Debug esp32c3"]["type"] != "rho" {
		t.Errorf("expected a session for a board that could not be read, got %+v", entries["Debug esp32c3"])
	}
}

// A build and a session that are about different boards stop on a line in a program
// that is not the one on the chip. So each session names the task that builds for
// its own board.
func TestEachBoardIsBuiltForAndDebuggedOnTheSameOne(t *testing.T) {
	root := projectOf(t, `module:
  name: hello-world
  type: module
  version: 0.1.0

main: src/main.c

build:
  language: c
  presets:
    - pico
    - esp32c3
`, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&unusedTarget),
	}); err != nil {
		t.Fatal(err)
	}

	entries := configurations(generated(t, root, launchPath))
	if got := entries["Debug pico"]["preLaunchTask"]; got != "rho: build" {
		t.Errorf("expected the session to build for its own board, got %v", got)
	}
	if got := entries["Debug esp32c3"]["preLaunchTask"]; got != "rho: build (esp32c3)" {
		t.Errorf("expected the session to build for its own board, got %v", got)
	}
	// An attach builds nothing: it is to a program that is already running.
	if _, present := entries["Debug pico (attach)"]["preLaunchTask"]; present {
		t.Error("expected an attach not to build a program it is not starting")
	}

	// The first declared board is the project's default, so its task keeps the plain
	// name — the one the editor's build button finds — and carries the board in its
	// arguments. The rest are named for theirs.
	taskEntries := tasks(generated(t, root, tasksPath))
	for label, arguments := range map[string]string{
		"rho: build":           "build . -target pico",
		"rho: build (esp32c3)": "build . -target esp32c3",
	} {
		entry, found := taskEntries[label]
		if !found {
			t.Errorf("expected a build task for the board, got %v", names(taskEntries))
			continue
		}
		if got := strings.Join(anyStrings(entry["args"]), " "); got != arguments {
			t.Errorf("%s: expected %q, got %q", label, arguments, got)
		}
	}
}

// A project written for one board keeps the plain names: a list with one build in
// it does not need to say which build it is.
func TestOneBoardKeepsThePlainTaskNames(t *testing.T) {
	asked := ""
	root := projectOf(t, blinkyManifest, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := tasks(generated(t, root, tasksPath))
	if _, found := entries[TaskBuild]; !found {
		t.Errorf("expected a plainly named build task, got %v", names(entries))
	}
	if _, found := entries[TaskFlash]; !found {
		t.Errorf("expected a plainly named flash task, got %v", names(entries))
	}
	// And the third-party session is offered, because there is room for it.
	if _, found := configurations(generated(t, root, launchPath))["Debug pico (cortex-debug)"]; !found {
		t.Error("expected the adapter session for a project written for one board")
	}
}

// Naming one board gives that board the third-party session and only that board's —
// which is what a person asking for a specific board is asking for.
func TestNamingOneBoardChoosesItAndOnlyIt(t *testing.T) {
	asked := ""
	root := projectOf(t, `module:
  name: hello-world
  type: module
  version: 0.1.0

main: src/main.c

build:
  language: c
  presets:
    - pico
    - esp32c3
`, nil)
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Target: "pico", Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := configurations(generated(t, root, launchPath))
	if _, found := entries["Debug pico (cortex-debug)"]; !found {
		t.Errorf("expected the adapter session for the board that was named, got %v", names(entries))
	}
	if _, found := entries["Debug esp32c3 (cortex-debug)"]; found {
		t.Error("expected no adapter session for a board nobody named")
	}
	// And the workspace's recommendations are that board's too.
	recommendations := recommendationsOf(generated(t, root, "hello-world.code-workspace"))
	for _, got := range recommendations {
		if got == "marus25.cortex-debug" {
			return
		}
	}
	t.Errorf("expected the adapter to be recommended for the named board, got %v", recommendations)
}

// A platform that names a board but gives no spec for it is reported, not guessed
// at: a session written from another board's spec is one that connects to the wrong
// chip.
func TestABoardWithNoSpecIsNamedInTheNote(t *testing.T) {
	root := projectOf(t, blinkyManifest, nil)
	_, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode,
		Resolve: func(string, string) (*Target, error) {
			return nil, errorString(`no spec named "pico"`)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, launchPath)); err != nil {
		t.Error("expected a configuration to be written even with no spec")
	}
}

// anyStrings is a JSON array read back as the strings it holds.
func anyStrings(value any) []string {
	entries, isList := value.([]any)
	if !isList {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text, isText := entry.(string); isText {
			out = append(out, text)
		}
	}
	return out
}

// unusedTarget is a resolver record for tests that do not read what was asked for.
var unusedTarget string

// What the project says about a board is the answer, in a generated configuration
// as in a session: a file somebody committed to a repository is a fact about the
// project, and the vendor's naming is what happens when nobody has said.
//
// Per board, because a project built for several boards has several chips: one
// board's register map named for all of them would put the wrong chip's registers
// in a configuration for the others, and nothing in the file would say so.
func TestAProjectsOwnRegisterMapIsTheOneAConfigurationNames(t *testing.T) {
	root := projectOf(t, `module:
  name: blinky
  type: module
  version: 0.1.0
  description: A project that says which chip it is

main: src/main.c

build:
  language: c
  toolchain: pico

debug:
  svd:
    pico: vendor/chip.svd
`, map[string]string{"vendor/chip.svd": "<device/>"})

	asked := ""
	if _, err := Generate(Request{
		ProjectRoot: root, Editor: EditorVSCode, Resolve: forPico(&asked),
	}); err != nil {
		t.Fatal(err)
	}
	entries := configurations(generated(t, root, launchPath))
	third := entries["Debug pico (cortex-debug)"]
	if third["svdFile"] != "vendor/chip.svd" {
		t.Errorf("expected the project's own register map, got %v", third["svdFile"])
	}
}
