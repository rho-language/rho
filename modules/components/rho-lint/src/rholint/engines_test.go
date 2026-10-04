//
// @file engines_test.go
// @brief Tests for the engine contract and for what a run decides.
//
// The three decisions a run makes are tested here: which engines it uses, what it does
// with one that is not there, and what its exit code means. The third is the one with
// the most room to be quietly wrong — "nothing was installed" and "the code is clean"
// both come out of a run as silence — so each of those three answers has a test that
// says which one it is.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  The contract                                                               */
/* -------------------------------------------------------------------------- */

func TestSpecValidateSaysWhatIsWrongWithIt(t *testing.T) {
	good := Spec{Name: "clang-tidy", Program: "clang-tidy", Format: FormatGCC}
	if err := good.Validate(); err != nil {
		t.Fatalf("a usable spec was refused: %v", err)
	}

	cases := map[string]Spec{
		"no name":    {Program: "clang-tidy"},
		"no program": {Name: "clang-tidy"},
		// A plugin may only provide a program. An in-process engine would have to be
		// compiled into this component, which is the thing being avoided.
		"a driver": {Name: "x", Kind: KindDriver, Program: "x"},
		// The failure that matters most: a format this component cannot read would
		// produce an engine whose findings silently vanish, and a finding that is not
		// reported is indistinguishable from a defect that is not there.
		"an unreadable format": {Name: "x", Program: "x", Format: "vscode-diagnostics"},
	}
	for name, spec := range cases {
		if err := spec.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSpecDefaultsAreTheOnesAMachineHas(t *testing.T) {
	// A plugin that names no format has said nothing, and `gcc` is what clang,
	// clang-tidy and most C tooling print by default. Defaulting to anything else
	// would lose every finding from the analyzer it was written for.
	spec := Spec{Name: "x", Program: "x"}
	if got := spec.ReadFormat(); got != FormatGCC {
		t.Errorf("format %q, want %q", got, FormatGCC)
	}
	// And one that names no language is an analyzer of generated C, which is what a
	// static analyzer almost always is.
	if got := spec.ReadLanguages(); len(got) != 1 || got[0] != LanguageC {
		t.Errorf("languages %v, want [c]", got)
	}
}

func TestSpecQualifiesByDefault(t *testing.T) {
	// `unusedVariable` and `unusedFunction` are different checks in different
	// analyzers, and a project that suppresses one has said nothing about the other.
	// A plugin can turn this off deliberately; it is not on by accident.
	if !(Spec{}).Qualifies() {
		t.Error("a spec that says nothing does not qualify its rules")
	}
	off := false
	if (Spec{Qualified: &off}).Qualifies() {
		t.Error("a spec that says not to, qualifies anyway")
	}
	// A rule that already carries the engine's name must not get it twice.
	if got := qualify(Spec{Name: "cppcheck"}, "cppcheck-unusedVariable"); got != "cppcheck-unusedVariable" {
		t.Errorf("rule %q, want it left alone", got)
	}
}

func TestExpansionSplicesTheFileList(t *testing.T) {
	// The reason this is not a string replacement: a linter takes its sources as
	// separate arguments and there is no spelling of one argument that means several
	// paths.
	spec := Spec{
		Name:    "clang-tidy",
		Program: "clang-tidy",
		Args: []string{
			"-p", "${database}",
			"--export-fixes=${root}/fixes",
			"${files}",
		},
	}
	program, arguments := spec.Expansion(Request{
		Root:     "/project",
		Database: "/project/build/compile_commands.json",
		Files:    []string{"/project/src/main.c", "/project/src/other.c"},
	})
	if program != "clang-tidy" {
		t.Errorf("program %q", program)
	}
	want := []string{
		"-p", "/project/build/compile_commands.json",
		"--export-fixes=/project/fixes",
		"/project/src/main.c", "/project/src/other.c",
	}
	if len(arguments) != len(want) {
		t.Fatalf("arguments %v, want %v", arguments, want)
	}
	for index := range want {
		if arguments[index] != want[index] {
			t.Fatalf("arguments %v, want %v", arguments, want)
		}
	}
}

func TestSpecRefusesAnArgumentBuiltOutOfTheFileList(t *testing.T) {
	// Building one argument out of the list would produce a single argument holding
	// several paths, and the analyzer would look for a file with that name.
	spec := Spec{Name: "x", Program: "x", Args: []string{"--sources=${files}"}}
	if err := spec.Validate(); err == nil {
		t.Error("an argument built out of ${files} was accepted")
	}
}

func TestSpecServes(t *testing.T) {
	// An engine that names no board is a general one, which is what an analyzer of C
	// is: the flags come from the database rather than from the engine.
	general := Spec{Name: "cppcheck", Program: "cppcheck"}
	if !general.Serves("nucleo-h753zi") {
		t.Error("an engine with no board does not serve one")
	}
	boardSpecific := Spec{Name: "x", Program: "x", Targets: []string{"nucleo-h753zi"}}
	if !boardSpecific.Serves("nucleo-h753zi") || boardSpecific.Serves("pico") {
		t.Error("a board-specific engine served the wrong board")
	}
}

/* -------------------------------------------------------------------------- */
/*  Which engines a run uses                                                    */
/* -------------------------------------------------------------------------- */

// noPlugins is a machine with no plugin offering anything.
func noPlugins() Engines { return Engines{Builtin: Builtin()} }

func TestChooseUsesEveryEngineWhenNothingIsNamed(t *testing.T) {
	specs, missing, err := noPlugins().Choose(nil, nil, "nucleo-h753zi")
	if err != nil {
		t.Fatalf("choose: %v", err)
	}
	if len(specs) != 2 || len(missing) != 0 {
		t.Errorf("chose %d engines and reported %d missing, want 2 and 0", len(specs), len(missing))
	}
	// `rho` before `c`, because a finding in the source a person wrote should come
	// before the same finding in the translation of it.
	if specs[0].Name != EngineRho || specs[1].Name != EngineC {
		t.Errorf("chose %s and %s", specs[0].Name, specs[1].Name)
	}
}

func TestChooseNamesWhatIsMissingAndRunsTheRest(t *testing.T) {
	// The decision that decides whether a machine part-way through being set up gets
	// any analysis at all. The shipped default names `clang-tidy` and `cppcheck`,
	// neither of which is an engine until a plugin provides one, and a run that
	// refused on the missing one would find nothing on every machine that has not
	// installed them.
	specs, missing, err := noPlugins().Choose([]string{"rho", "clang-tidy", "cppcheck"}, nil, "")
	if err != nil {
		t.Fatalf("choose: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != EngineRho {
		t.Errorf("chose %d engines, want the one that is there", len(specs))
	}
	if len(missing) != 2 || missing[0] != "clang-tidy" {
		t.Errorf("reported %v missing, want both named", missing)
	}
}

func TestChooseRefusesARequestThatResolvedToNothing(t *testing.T) {
	// The other half of the same decision: a person who asked for an analyzer and
	// there is none is a different situation from a default naming one that is not
	// installed yet, and the second is the only one that is not worth stopping for.
	_, _, err := noPlugins().Choose([]string{"clang-tidy"}, nil, "")
	if err == nil {
		t.Fatal("a request for an analyzer this machine does not have was accepted")
	}
	if !strings.Contains(err.Error(), "rho") {
		t.Errorf("the message %q does not say what the machine does have", err)
	}
}

func TestTrimEngineNameAcceptsEitherSpelling(t *testing.T) {
	// A settings file and a command line are written by hand, and a rule about the
	// spelling is a rule that will be broken once and reported as a missing engine.
	for _, written := range []string{"clang-tidy", "clang_tidy", " clang-tidy ",
		"clang-tidy@17", "clang_tidy@17"} {
		if got := trimEngineName(written); got != "clang-tidy" {
			t.Errorf("%q became %q", written, got)
		}
	}
}

func TestDiscoverReadsWhatAPluginOffers(t *testing.T) {
	// A plugin is loaded from a tree of modules, and its answer is a JSON object on
	// its standard output. The spec it offers is a description of a program, and a
	// program that is not there is reported rather than dropped.
	plugin := aPlugin(t, "rho-lint-cppcheck", map[string]any{
		"engines": []map[string]any{{
			"name":    "cppcheck",
			"kind":    KindProgram,
			"program": "/usr/bin/cppcheck",
			"args":    []string{"--enable=all", "--xml", "--xml-version=2", "-p", "${database}", "${files}"},
			"format":  FormatCPPCheck,
		}},
	})

	available := Discover(aPluginSource(filepath.Dir(plugin)))
	if len(available.FromPlugins) != 1 {
		t.Fatalf("discovered %d, want 1", len(available.FromPlugins))
	}
	spec := available.FromPlugins[0]
	if spec.Name != "cppcheck" || spec.ReadFormat() != FormatCPPCheck {
		t.Errorf("spec %+v", spec)
	}
	if spec.Root != plugin {
		t.Errorf("root %q, want the plugin's own directory %q", spec.Root, plugin)
	}

	// The same plugin offering something unusable is reported rather than silently
	// left out, so a typo in a manifest is a diagnostic and not a missing engine.
	broken := aPlugin(t, "rho-lint-broken", map[string]any{
		"engines": []map[string]any{{"name": "x", "program": "x", "format": "nonsense"}},
	})
	available = Discover(aPluginSource(filepath.Dir(broken)))
	if len(available.FromPlugins) != 0 {
		t.Errorf("kept %d, want none", len(available.FromPlugins))
	}
	if len(available.Problems) == 0 {
		t.Error("a spec that cannot be read produced no diagnostic")
	}
}

// aPlugin is a plugin module in a tree, with the answer its hook prints.
//
// The manifest is written as YAML because a manifest is YAML and the loader's schema
// checks one — writing it as JSON and calling it a manifest would test a shape no
// real plugin has.
func aPlugin(t *testing.T, name string, answer map[string]any) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, name)
	writeFile(t, filepath.Join(directory, "module.yml"), `module:
  name: `+name+`
  type: plugin
  version: 0.1.0

main: engine

extends: `+ComponentName+`

implements:
  - `+HookEngine+`

hooks:
  - `+HookEngine+`

commands:
  - name: engine
    run: engine
    hook: `+HookEngine+`
`)

	// The binary is named in the plugin's manifest and lives in its root. It is handed
	// the request as JSON on standard input and answers with a `Response` on standard
	// output — the shared plugin protocol, not a bare object, so the fixture answers
	// the way a real plugin does rather than the way this component happens to parse.
	script := "#!/bin/sh\ncat > /dev/null\ncat <<'ANSWER'\n" +
		`{"version":` + mustJSON(t, plugins.ProtocolVersion) +
		`,"payload":` + mustJSON(t, answer) + "}\nANSWER\n"
	binary := filepath.Join(directory, "engine")
	writeFile(t, binary, script)
	if err := os.Chmod(binary, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return directory
}

// aPluginSource is where to look for plugins a test has written, and it trusts the
// tree.
//
// The installed registry is not touched: a test that wrote into somebody's Rho home
// would be a test that depends on the machine it ran on. Trusting the tree is the
// point — a project's own `modules/plugins` is where a project adds an analyzer, and
// a run that ignored it would be a run that only worked for an installed plugin.
func aPluginSource(root string) plugins.Source {
	source := PluginSource(root)
	source.TrustTree = true
	return source
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(encoded)
}

/* -------------------------------------------------------------------------- */
/*  What a run decides                                                          */
/* -------------------------------------------------------------------------- */

// aRhoProject is a project with one Rho source that parses and is clean.
func aRhoProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "module.yml"), `module:
  name: demo
  type: module
  version: 0.1.0
main: src/main.rho
`)
	writeFile(t, filepath.Join(root, "src", "main.rho"), "module main\n\nfn main() void {\n}\n")
	return root
}

func TestAnalyseOverACleanProject(t *testing.T) {
	result, err := Analyse(Options{Root: aRhoProject(t), FailOn: SeverityError})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if result.Counts.Total() != 0 {
		t.Errorf("found %v", idsOf(result.Diagnostics))
	}
	if result.Failed {
		t.Errorf("failed on a clean project: %s", result.Reason)
	}
	// The `c` engine has no C here, and a report that could not say so would be
	// showing a run with no findings from it and leaving the reader to guess.
	for _, outcome := range result.Engines {
		if outcome.Description.Name == EngineC && outcome.Ran {
			t.Error("the `c` engine ran on a project with no C")
		}
	}
}

func TestAnalyseCarriesTheDerivedDatabaseFact(t *testing.T) {
	// A finding from an analyzer given a guessed command line is a weaker claim than
	// one given the real one, and a report that flattens the two will be believed once
	// too often.
	root := aRhoProject(t)
	result, err := Analyse(Options{Root: root, FailOn: SeverityError})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if !result.Database.Derived {
		t.Error("a derived database is not labelled as derived")
	}
	if result.Database.From == "" {
		t.Error("a derived database does not say what it was derived from")
	}

	// With a build's own database, it is read and not derived.
	cmake := writeFile(t, filepath.Join(root, "build", "compile_commands.json"),
		databaseFor("/project/src/app.c"))
	result, err = Analyse(Options{Root: root, FailOn: SeverityError})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if result.Database.Derived {
		t.Error("a database a build wrote was reported as derived")
	}
	if result.Database.Path != cmake {
		t.Errorf("read %s, want %s", result.Database.Path, cmake)
	}
}

func TestAnalyseExcludesAnotherCheckoutsSources(t *testing.T) {
	// An application's generated project compiles its platform's sources beside its
	// own, and none of them are the application's to fix. A run that reported findings
	// in them would be reporting on somebody else's checkout, which is what running a
	// lint over *that* checkout is for.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "module.yml"), "module:\n  name: demo\n  type: module\n  version: 0.1.0\n")
	writeFile(t, filepath.Join(root, "src", "main.c"), "int main(void){}\n")
	entries := []map[string]any{
		{"directory": "/", "file": filepath.Join(root, "src", "main.c"),
			"arguments": []string{"cc", "-c", filepath.Join(root, "src", "main.c")}},
		{"directory": "/", "file": "/elsewhere/platform/src/hal.c",
			"arguments": []string{"cc", "-c", "/elsewhere/platform/src/hal.c"}},
	}
	writeFile(t, filepath.Join(root, "build", "compile_commands.json"), mustJSON(t, entries))

	excluding, err := Analyse(Options{Root: root, FailOn: SeverityError, Outside: OutsideExclude})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if excluding.LeftOut != 1 {
		t.Errorf("left out %d, want the dependency's one source", excluding.LeftOut)
	}
	including, err := Analyse(Options{Root: root, FailOn: SeverityError, Outside: OutsideInclude})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if including.LeftOut != 0 {
		t.Errorf("left out %d, want nothing when the project is the platform", including.LeftOut)
	}
}

func TestAnalyseWritesADatabaseOnlyWhenAsked(t *testing.T) {
	// A `rho lint` in somebody's working tree should not create files. A run still
	// derives one in memory, because an analyzer given no database finds nothing.
	root := aRhoProject(t)
	if _, err := Analyse(Options{Root: root, FailOn: SeverityError}); err != nil {
		t.Fatalf("analyse: %v", err)
	}
	written := filepath.Join(root, "build", "compile_commands.json")
	if _, err := os.Stat(written); err == nil {
		t.Error("a run created a compilation database without being asked to")
	}

	if _, err := Analyse(Options{
		Root: root, FailOn: SeverityError, Write: "build/compile_commands.json",
	}); err != nil {
		t.Fatalf("analyse with --write-db: %v", err)
	}
	if _, err := os.Stat(written); err != nil {
		t.Errorf("the database was not written: %v", err)
	}
}

func TestAnalyseFailsWhenNothingRan(t *testing.T) {
	// The one wrong answer this component exists to prevent: a run where every engine
	// did not work has found nothing, and reporting that as a clean tree is a
	// pipeline that is green because it has been linting nothing.
	result := &Result{
		Engines: []Outcome{
			{Description: Description{Name: "clang-tidy"}, Failed: true, Message: "not installed"},
			{Description: Description{Name: "cppcheck"}, Failed: true, Message: "not installed"},
		},
	}
	failed, reason := decide(SeverityError, result)
	if !failed {
		t.Errorf("a run where nothing ran did not fail: %s", reason)
	}

	// One engine failing out of two is not a failure: the other ran, and its findings
	// are real.
	result = &Result{
		Engines: []Outcome{
			{Description: Description{Name: "clang-tidy"}, Failed: true, Message: "not installed"},
			{Description: Description{Name: "rho"}, Ran: true},
		},
		Diagnostics: []Diagnostic{{Severity: SeverityNote, Rule: "rho-todo-comment"}},
	}
	failed, reason = decide(SeverityError, result)
	if failed {
		t.Errorf("a run with one working engine failed: %s", reason)
	}
}

func TestDecideMovesTheExitCodeAndNothingElse(t *testing.T) {
	// Every finding is printed whatever the threshold says; the threshold moves the
	// exit code. A threshold that also hid findings would hide the ones nobody had
	// decided about yet.
	notes := &Result{
		Engines:     []Outcome{{Description: Description{Name: "rho"}, Ran: true}},
		Diagnostics: []Diagnostic{{Severity: SeverityNote, Rule: "rho-todo-comment"}},
	}
	if failed, _ := decide(SeverityError, notes); failed {
		t.Error("a note fails a run set to fail on an error")
	}
	if failed, _ := decide(SeverityNote, notes); !failed {
		t.Error("a note did not fail a run set to fail on a note")
	}
	if failed, _ := decide(ThresholdNever, notes); failed {
		t.Error("`never` failed a run")
	}

	errors := &Result{
		Engines:     []Outcome{{Description: Description{Name: "cppcheck"}, Ran: true}},
		Diagnostics: []Diagnostic{{Severity: SeverityError, Rule: "x"}},
	}
	if failed, _ := decide(ThresholdNever, errors); failed {
		t.Error("`never` failed a run with an error in it")
	}
}

func TestAnalyseProducesOneSarifRunPerEngineThatRan(t *testing.T) {
	// The format says a run is one tool's results, and the attribution is what makes
	// "these two disagree about this line" answerable at all.
	root := aRhoProject(t)
	writeFile(t, filepath.Join(root, "src", "other.rho"), "module wrong_name\n")
	result, err := Analyse(Options{Root: root, FailOn: SeverityError})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if len(result.Runs) != 2 {
		t.Fatalf("%d runs, want one per chosen engine", len(result.Runs))
	}
	names := map[string]bool{}
	for _, run := range result.Runs {
		names[run.Tool.Name] = true
	}
	if !names[EngineRho] || !names[EngineC] {
		t.Errorf("runs are %v, want one for each engine", names)
	}
	if result.Counts.Warnings == 0 {
		t.Errorf("found %v, want the mismatched module name", idsOf(result.Diagnostics))
	}
}

/* -------------------------------------------------------------------------- */
/*  Settings                                                                    */
/* -------------------------------------------------------------------------- */

func TestSettingsFallBackToTheManifestsOwnDefaults(t *testing.T) {
	// A component installed without its configuration directory must still run, and
	// it must run with the values the manifest declares rather than with an empty
	// list — which would mean "everything" or "nothing" depending on the field.
	settings := Settings{
		Engines: append([]string{}, DefaultEngines...),
		FailOn:  DefaultFailOn,
		Exclude: append([]string{}, DefaultExclude...),
		Outside: DefaultOutside,
	}
	if len(settings.Engines) != 4 {
		t.Errorf("default engines %v, want the two carried and the two plugin ones",
			settings.Engines)
	}
	if settings.FailOn != SeverityError {
		t.Errorf("default threshold %q, want an error", settings.FailOn)
	}
	if len(settings.Exclude) != 1 || settings.Exclude[0] != "vendor/" {
		t.Errorf("default exclusions %v", settings.Exclude)
	}
}

func TestFailOnRefusesWhatItCannotHonour(t *testing.T) {
	// `fail_on: eror` quietly failing on errors would be a silent wrong answer, and
	// failing on nothing would be a worse one.
	if KnownFailOn("eror") {
		t.Error("a misspelling was accepted as a severity")
	}
	for _, good := range []string{"error", "warning", "note", "never", "ERROR"} {
		if !KnownFailOn(good) {
			t.Errorf("%q was refused", good)
		}
	}
	if got := failOn("eror", SeverityError); got != SeverityError {
		t.Errorf("fell back to %q, want the caller's own value", got)
	}
}

func TestSplitListAnEmptyValueMeansTheDefault(t *testing.T) {
	// Clearing a setting should silence nothing. Treating an empty value as "none"
	// would make clearing `default_engines` turn the component off, which is not a
	// thing anybody means by clearing a setting.
	fallback := []string{"rho", "c"}
	if got := splitList("", fallback); len(got) != 2 {
		t.Errorf("an empty value gave %v, want the fallback", got)
	}
	if got := splitList(" rho , c ", fallback); len(got) != 2 || got[0] != "rho" || got[1] != "c" {
		t.Errorf("split gave %v", got)
	}
}

func TestOutsideIsTheTwoItCanBe(t *testing.T) {
	if got := outside("include", OutsideExclude); got != OutsideInclude {
		t.Errorf("got %q", got)
	}
	if got := outside("sometimes", OutsideExclude); got != OutsideExclude {
		t.Errorf("got %q, want the fallback", got)
	}
}
