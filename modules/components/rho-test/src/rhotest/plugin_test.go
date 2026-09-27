//
// @file plugin_test.go
// @brief Tests for the test-run plugins.
//
// The seam is what a run hands a plugin and what it does with the answer, so
// these tests cover the two questions a plugin is asked and the two things it
// may say back: a file to add, and a report to write. A plugin that fails is
// reported, because one broken extension must not cost a project its tests.
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
	return DiscoveredFile{Path: path, Tests: []TestCase{{Name: name, File: path, Line: 1}}}
}

/* -------------------------------------------------------------------------- */
/*  The seam                                                                  */
/* -------------------------------------------------------------------------- */

func TestAPluginAddsTestFilesToARun(t *testing.T) {
	// The discover hook is how a plugin that knows a project's own test layout
	// takes part in a run.
	root := t.TempDir()
	writePlugin(t, root, "generated", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "add-tests"))

	files, diagnostics := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(root, DefaultTarget, []DiscoveredFile{aFile("/project/tests/a.rho", "works")})

	if len(files) != 2 {
		t.Fatalf("expected the plugin's file to be added, got %+v", files)
	}
	if files[1].Path != "/project/tests/generated.rho" {
		t.Fatalf("expected the file the plugin named, got %q", files[1].Path)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean run, got %+v", diagnostics)
	}
}

func TestAPluginCannotAddTheSameFileTwice(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "twice", "  - "+HookDiscover+"\n", aCommand(HookDiscover, "add-same-test"))

	existing := []DiscoveredFile{aFile("/project/tests/a.rho", "works")}
	files, _ := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Discover(root, DefaultTarget, existing)

	if len(files) != 1 {
		t.Fatalf("expected the file to be added once, got %+v", files)
	}
}

func TestAPluginReportsARun(t *testing.T) {
	// The run hook is how a run is reported in a form a system reads.
	root := t.TempDir()
	writePlugin(t, root, "junit", "  - "+HookRun+"\n", aCommand(HookRun, "report"))

	results := []TestResult{
		{Case: TestCase{Name: "works", File: "/project/tests/a.rho", Line: 1}, Passed: true},
		{Case: TestCase{Name: "fails", File: "/project/tests/a.rho", Line: 9}},
	}
	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Run(root, DefaultTarget, results)

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

func TestAPluginThatFailsDoesNotStopTheRun(t *testing.T) {
	// One broken extension must not cost a project the tests it has, nor the
	// reports the others wrote.
	root := t.TempDir()
	writePlugin(t, root, "broken", "  - "+HookRun+"\n", aCommand(HookRun, "fail"))

	outcome := WithPlugins(DiscoverPlugins(Source{Tree: root, TrustTree: true})).
		Run(root, DefaultTarget, []TestResult{{Case: TestCase{Name: "works"}}})

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
	kept, diagnostics := WithPlugins(nil).Discover("/project", DefaultTarget, files)
	if len(kept) != 1 || len(diagnostics) != 0 {
		t.Fatalf("expected the run's own files, got %+v %+v", kept, diagnostics)
	}
	outcome := WithPlugins(nil).Run("/project", DefaultTarget, nil)
	if len(outcome.Reports) != 0 || len(outcome.Notes) != 0 {
		t.Fatalf("expected nothing from no plugins, got %+v", outcome)
	}
}

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
				Files: []DiscoveredFile{{Path: "/project/tests/generated.rho", Tests: []TestCase{{Name: "generated", File: "/project/tests/generated.rho", Line: 1}}}},
			}),
		})
	case "add-same-test":
		answer(t, request, plugins.Response{
			Version: plugins.ProtocolVersion,
			Payload: mustPayload(t, DiscoverResponse{
				Files: []DiscoveredFile{{Path: "/project/tests/a.rho"}},
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
	}
	os.Exit(0)
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
