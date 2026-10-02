//
// @file plugin_test.go
// @brief Tests for the test-run plugins.
//
// The seam is what a run hands a plugin and what it does with the answer, so
// these tests cover the two questions a plugin is asked and what it may say back:
// a file to add to the run's own tests, a suite it knows how to run, or a report
// to write. A plugin that fails is reported, because one broken extension must
// not cost a project its tests.
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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// writePlugin writes a plugin attached to this runner, carrying the helper that
// answers for it. It is the shape a real plugin has: it carries its own command.
func writePlugin(t *testing.T, root, name, hooks, commands string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(path, helperName)); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	manifest := "module:\n" +
		"  name: " + name + "\n" +
		"  type: plugin\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: " + name + "\n" +
		"\n" +
		"extends: " + ComponentName + "\n" +
		"\n" +
		"hooks:\n" + hooks + commands
	if err := os.WriteFile(filepath.Join(path, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// helperName is the program a test plugin carries.
const helperName = "plugin-helper"

// aCommand is a command declaration, with the helper's behaviour as its
// arguments.
func aCommand(hook, behaviour string) string {
	return "commands:\n  - name: " + behaviour + "\n    run: " + helperName +
		" -test.run TestRunnerHelperProcess -- " + behaviour + "\n    hook: " + hook + "\n"
}

// aFile is a test file a run found.
func aFile(path, name string) DiscoveredFile {
	return DiscoveredFile{Path: path, Tests: []Case{{Name: name, File: path, Line: 1}}}
}

// aDeclaration is a suite a run is considering, as a plugin is asked about it.
func aDeclaration(name, kind string, axes map[string]string) DeclaredSuite {
	return DeclaredSuite{Name: name, Kind: kind, Axis: axes}
}

/* -------------------------------------------------------------------------- */
/*  The discover seam                                                          */
/* -------------------------------------------------------------------------- */

func TestAPluginAddsTestFilesToARun(t *testing.T) {
	// The discover hook is how a plugin that knows a project's own test layout
	// takes part in a run.
	root := t.TempDir()
	writePlugin(t, root, "generated", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "add-tests"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(DiscoverContext{
			Root:   root,
			Files:  []DiscoveredFile{aFile("/project/tests/a.rho", "works")},
			Suites: []DeclaredSuite{aDeclaration("generated", "rho", nil)},
		})

	if len(outcome.Files) != 2 {
		t.Fatalf("expected the plugin's file to be added, got %+v", outcome.Files)
	}
	if outcome.Files[1].Path != "/project/tests/generated.rho" {
		t.Fatalf("expected the file the plugin named, got %q", outcome.Files[1].Path)
	}
	if len(outcome.Diagnostics) != 0 {
		t.Fatalf("expected a clean run, got %+v", outcome.Diagnostics)
	}
}

func TestAPluginCannotAddTheSameFileTwice(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "twice", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "add-same-test"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(DiscoverContext{Root: root, Files: []DiscoveredFile{aFile("/project/tests/a.rho", "works")}})

	if len(outcome.Files) != 1 {
		t.Fatalf("expected the file to be added once, got %+v", outcome.Files)
	}
}

// A plugin may resolve a suite the manifest declared, and that suite's steps are
// what the run takes. This is the thing a plugin could not do before: it used to
// be able to name files, which the runner then read as Rho regardless.
func TestAPluginOffersARunnableSuite(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "gtest", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "resolve-suite"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(DiscoverContext{
			Root:   root,
			Suites: []DeclaredSuite{aDeclaration("error", "gtest", nil)},
		})

	if len(outcome.Resolved) != 1 {
		t.Fatalf("expected the plugin's suite, got %+v", outcome.Resolved)
	}
	suite := outcome.Resolved[0]
	if suite.Declared != "error" {
		t.Errorf("a resolved suite should say which declaration it answers, got %q", suite.Declared)
	}
	if len(suite.Steps) != 2 {
		t.Fatalf("expected the plugin's steps, got %+v", suite.Steps)
	}
	if strings.Join(suite.Steps[1].Command, " ") != "build/error-test --gtest_output=xml:out.xml" {
		t.Errorf("expected the step the plugin declared, got %v", suite.Steps[1].Command)
	}
	if suite.Steps[1].Produces == "" {
		t.Error("a step that leaves a file behind should say what, so the reader of it does not have to guess")
	}
}

// A plugin that adds a suite the manifest does not declare is reporting it, and
// the run does not take it. Which tests a project has is the project's to say.
func TestAPluginCannotAddASuiteNobodyDeclared(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "extra", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "resolve-undeclared"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(DiscoverContext{
			Root:   root,
			Suites: []DeclaredSuite{aDeclaration("error", "gtest", nil)},
		})

	if len(outcome.Resolved) != 0 {
		t.Fatalf("expected the undeclared suite to be left out, got %+v", outcome.Resolved)
	}
	found := false
	for _, diagnostic := range outcome.Diagnostics {
		if strings.Contains(diagnostic.Message, "manifest does not declare") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the attempt to be reported, got %+v", outcome.Diagnostics)
	}
}

/* -------------------------------------------------------------------------- */
/*  The run seam                                                               */
/* -------------------------------------------------------------------------- */

func TestAPluginReportsARun(t *testing.T) {
	// The run hook is how a run is reported in a form a system reads.
	root := t.TempDir()
	writePlugin(t, root, "junit", "  - "+HookRun+"\n", aCommand(HookRun, "report"))

	suites := []Suite{{Name: "a.rho", Kind: KindRho}}
	results := []Result{
		{Suite: "a.rho", Case: Case{Name: "works", File: "/project/tests/a.rho", Line: 1}, Status: StatusPass},
		{Suite: "a.rho", Case: Case{Name: "fails", File: "/project/tests/a.rho", Line: 9}, Status: StatusFail},
	}
	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Run(root, DefaultTarget, suites, results)

	if len(outcome.Reports) != 1 {
		t.Fatalf("expected the plugin's report, got %+v", outcome.Reports)
	}
	if !strings.Contains(outcome.Reports[0], "testsuites") {
		t.Fatalf("expected a report document, got:\n%s", outcome.Reports[0])
	}
	if len(outcome.Notes) != 1 || !strings.Contains(outcome.Notes[0], "2 cases") {
		t.Fatalf("expected the plugin's note, got %+v", outcome.Notes)
	}
	if len(outcome.Diagnostics) != 0 {
		t.Fatalf("expected a clean run, got %+v", outcome.Diagnostics)
	}
}

// A skipped suite is a fact a reporter has to be able to write down, so the run
// hook says how many there were and names them. A reporter that only knows about
// passing and failing has to guess, and every guess puts a skipped test somewhere
// it did not happen.
func TestTheRunHookSaysWhatWasSkipped(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "skips", "  - "+HookRun+"\n", aCommand(HookRun, "echo-run"))

	suites := []Suite{
		{Name: "ran", Kind: KindRho},
		{Name: "missing-toolchain", Kind: KindRho, Skip: "no ARM toolchain"},
	}
	results := []Result{
		{Suite: "ran", Case: Case{Name: "works"}, Status: StatusPass},
		{Suite: "missing-toolchain", Case: Case{Name: "missing-toolchain"}, Status: StatusSkip, Reason: "no ARM toolchain"},
	}
	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Run(root, DefaultTarget, suites, results)

	if len(outcome.Notes) != 1 {
		t.Fatalf("expected the plugin to echo what it was asked about, got %+v", outcome.Notes)
	}
	for _, want := range []string{"skipped=1", "missing-toolchain", "no ARM toolchain", "passed=1", "failed=0"} {
		if !strings.Contains(outcome.Notes[0], want) {
			t.Errorf("expected the run to report %q, got %q", want, outcome.Notes[0])
		}
	}
}

func TestAPluginThatFailsDoesNotStopTheRun(t *testing.T) {
	// One broken extension must not cost a project the tests it has, nor the
	// reports the others wrote.
	root := t.TempDir()
	writePlugin(t, root, "broken", "  - "+HookRun+"\n", aCommand(HookRun, "fail"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Run(root, DefaultTarget, nil, []Result{{Case: Case{Name: "works"}, Status: StatusPass}})

	if len(outcome.Diagnostics) != 1 || !strings.Contains(outcome.Diagnostics[0].Message, "failed") {
		t.Fatalf("expected the failure to be reported, got %+v", outcome.Diagnostics)
	}
	if len(outcome.Reports) != 0 {
		t.Fatalf("expected no report from a plugin that failed, got %+v", outcome.Reports)
	}
}

func TestARunWithoutPluginsIsARun(t *testing.T) {
	// Most projects have no plugins, and a run must be exactly the run it was
	// before this existed.
	files := []DiscoveredFile{aFile("/project/tests/a.rho", "works")}
	outcome := WithPlugins(nil).Discover(DiscoverContext{Root: "/project", Files: files})
	if len(outcome.Files) != 1 || len(outcome.Diagnostics) != 0 {
		t.Fatalf("expected the run's own files, got %+v %+v", outcome.Files, outcome.Diagnostics)
	}
	run := WithPlugins(nil).Run("/project", DefaultTarget, nil, nil)
	if len(run.Reports) != 0 || len(run.Notes) != 0 {
		t.Fatalf("expected nothing from no plugins, got %+v", run)
	}
}

/* -------------------------------------------------------------------------- */
/*  Which plugins are found                                                    */
/* -------------------------------------------------------------------------- */

func TestTheRunnerOffersTheHooksItsManifestDeclares(t *testing.T) {
	// A plugin is measured against the manifest, and the runner runs the hooks
	// it says it runs. The two have to agree, or a plugin validates and then
	// never runs.
	manifest, err := os.ReadFile(filepath.Join("..", "..", "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range OfferedHooks {
		if !strings.Contains(string(manifest), "- "+hook) {
			t.Fatalf("the runner runs %q but the manifest does not offer it", hook)
		}
	}
}

func TestAPluginForAnotherComponentIsNotRun(t *testing.T) {
	// A plugin names the component it extends, and a run is not that component.
	root := t.TempDir()
	path := filepath.Join(root, "toc")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: toc\n  type: plugin\n  version: 0.1.0\n\nmain: toc\n\nextends: rho-doc\n\nhooks:\n  - page\n"
	if err := os.WriteFile(filepath.Join(path, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if found := DiscoverPlugins(Source{Tree: root, TrustTree: true}); len(found) != 0 {
		t.Fatalf("expected another component's plugin to be left alone, got %+v", found)
	}
}

/* -------------------------------------------------------------------------- */
/*  The helper process                                                        */
/* -------------------------------------------------------------------------- */

// TestRunnerHelperProcess is not a test: it is the program a test plugin runs. It
// answers the protocol in the way it was asked to, and does nothing otherwise.
func TestRunnerHelperProcess(t *testing.T) {
	mode := helperMode()
	if mode == "" {
		t.Skip("not running as a plugin helper")
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	var request plugins.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		os.Exit(4)
	}
	if request.Version != plugins.ProtocolVersion {
		os.Exit(5)
	}
	switch mode {
	case "fail":
		os.Exit(1)
	case "add-tests":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, DiscoverResponse{
				Files: []DiscoveredFile{{Path: "/project/tests/generated.rho", Tests: []Case{{Name: "generated", File: "/project/tests/generated.rho", Line: 1}}}},
			}),
		})
	case "add-same-test":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, DiscoverResponse{
				Files: []DiscoveredFile{{Path: "/project/tests/a.rho"}},
			}),
		})
	case "resolve-suite":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, DiscoverResponse{
				Suites: []ResolvedSuite{{
					Declared: "error",
					Steps: []PluginStep{
						{Name: "build", Command: []string{"cmake", "--build", "build"}},
						{
							Name:     "run",
							Command:  []string{"build/error-test", "--gtest_output=xml:out.xml"},
							Produces: "out.xml",
						},
					},
				}},
			}),
		})
	case "resolve-undeclared":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, DiscoverResponse{
				Suites: []ResolvedSuite{{
					Declared: "the-runner-never-asked-for-this",
					Steps:    []PluginStep{{Name: "run", Command: []string{"true"}}},
				}},
			}),
		})
	case "report":
		body := struct {
			Report string   `json:"report"`
			Notes  []string `json:"notes"`
		}{
			Report: "<testsuites><testsuite name=\"a.rho\" tests=\"1\"/></testsuites>",
			Notes:  []string{"junit report: 2 cases, 1 passed"},
		}
		answer(t, request, plugins.Response{Version: plugins.ProtocolVersion, Payload: mustPayload(t, body)})
	case "echo-run":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, RunResponse{Notes: []string{echoedRun(request.Payload)}}),
		})
	}
	os.Exit(0)
}

// echoedRun renders the run a plugin was asked about, so a test can check what
// the request carried without the plugin having to assert anything itself.
func echoedRun(payload json.RawMessage) string {
	var request struct {
		Suites  []Suite  `json:"suites"`
		Passed  int      `json:"passed"`
		Failed  int      `json:"failed"`
		Skipped int      `json:"skipped"`
		Results []Result `json:"results"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return "unreadable: " + err.Error()
	}
	out := "passed=" + itoa(request.Passed) + " failed=" + itoa(request.Failed) + " skipped=" + itoa(request.Skipped)
	for _, suite := range request.Suites {
		if suite.Skip != "" {
			out += " suite=" + suite.Name + " skip=" + suite.Skip
		}
	}
	for _, result := range request.Results {
		if result.Status == StatusSkip {
			out += " result=" + result.Suite + " reason=" + result.Reason
		}
	}
	return out
}

// answer writes a plugin's response to standard output.
func answer(t *testing.T, request plugins.Request, response plugins.Response) {
	t.Helper()
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout.Write(encoded)
}

func mustPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func helperMode() string {
	for index, argument := range os.Args {
		if argument == "--" && index+1 < len(os.Args) {
			return os.Args[index+1]
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Where a report goes                                                        */
/* -------------------------------------------------------------------------- */

func TestAReportsAreWrittenWhereABuildLooksForThem(t *testing.T) {
	// A report is a document a system reads, so it is written to a file rather
	// than printed: printed, it is a wall of text in the middle of a run.
	root := t.TempDir()
	report, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	writeReports(report, Options{ReportDir: root}, root, RunOutcome{
		Reports: []string{"<testsuites/>", "<testsuites>second</testsuites>"},
	})

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("expected the reports to be written, got %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected one file per report, got %d", len(entries))
	}
	// Two plugins writing a report do not write over one another.
	if entries[0].Name() == entries[1].Name() {
		t.Fatalf("expected distinct files, got %s twice", entries[0].Name())
	}
}

func TestAReportGoesBesideTheProjectWhenNoDirectoryIsNamed(t *testing.T) {
	// The default is where a build already keeps what a run produced, so a
	// person finds it where they look.
	root := t.TempDir()
	report, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	writeReports(report, Options{}, root, RunOutcome{Reports: []string{"<testsuites/>"}})

	if _, err := os.Stat(filepath.Join(root, "build", "test-reports")); err != nil {
		t.Fatalf("expected the reports beside the project, got %v", err)
	}
}

func TestAReportIsNotWrittenWhereItCannotBe(t *testing.T) {
	// A report that cannot be written is reported and does not stop the run: the
	// tests themselves have already run by this point.
	// A path that is a file cannot hold a directory.
	root := t.TempDir()
	blocker := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	writeReports(report, Options{ReportDir: blocker}, root, RunOutcome{Reports: []string{"<testsuites/>"}})

	// The run carries on: nothing here panics and the file is untouched.
	written, err := os.ReadFile(blocker)
	if err != nil || string(written) != "not a directory" {
		t.Fatalf("expected the existing file to be left alone, got %q", written)
	}
}

func TestTheProjectIsFoundFromTheTestsThatWereRun(t *testing.T) {
	// A plugin is told which project the run was over, so a report can name it,
	// and the project is found by walking up from a test file the way every
	// other part of the toolchain finds one.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	test := filepath.Join(root, "tests", "a_test.rho")
	if err := os.WriteFile(test, []byte("module tests.a_test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(test); got != root {
		t.Fatalf("expected the project root, got %q", got)
	}
	// A run over nothing is over the directory it was pointed at.
	if got := ProjectRoot(""); got == "" {
		t.Fatal("expected a run with no tests to still have a root")
	}
}
