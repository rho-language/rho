//
// @file plugins_test.go
// @brief Tests for the plugin mechanism the toolchain owns.
//
// Every component that takes plugins uses this, so what is tested here is what
// they all rely on: a plugin is found because it names a component, a run asks
// only for the hooks it runs, an answer is folded in, and a plugin that fails is
// reported rather than allowed to stop the work.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package plugins

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// writePlugin writes a plugin attached to a component, carrying the helper that
// answers for it.
func writePlugin(t *testing.T, root, name, component, hooks, commands string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n" +
		"  name: " + name + "\n" +
		"  type: plugin\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: " + name + "\n" +
		"\n" +
		"extends: " + component + "\n" +
		"\n" +
		"hooks:\n" + hooks +
		commands
	if err := os.WriteFile(filepath.Join(path, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// carryHelper puts this test binary inside a plugin as the program the plugin
// runs, which is the shape a real plugin has: it carries its own command.
func carryHelper(t *testing.T, pluginDir string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(pluginDir, helperName)); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

const helperName = "plugin-helper"

/* -------------------------------------------------------------------------- */
/*  Discovery                                                                 */
/* -------------------------------------------------------------------------- */

func TestAPluginIsFoundBecauseItNamesTheComponent(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "toc", "rho-doc", "  - index\n", "")
	writePlugin(t, root, "other", "rho-test", "  - run\n", "")

	found := Source{Tree: root, TrustTree: true}
	if plugins := found.Discover("rho-doc"); len(plugins) != 1 || plugins[0].Name != "toc" {
		t.Fatalf("expected only the plugin that names this component, got %+v", plugins)
	}
	if plugins := found.Discover("rho-test"); len(plugins) != 1 || plugins[0].Name != "other" {
		t.Fatalf("expected the other component's own plugin, got %+v", plugins)
	}
}

func TestTheTreesOwnPluginsAreLeftAloneUnlessTheyAreTrusted(t *testing.T) {
	// A plugin is a program, and the tree a component works in may have been
	// written by someone the user does not know.
	root := t.TempDir()
	writePlugin(t, root, "toc", "rho-test", "  - run\n", "")

	if found := (Source{Tree: root}).Discover("rho-test"); len(found) != 0 {
		t.Fatalf("expected a tree's plugins to be left alone by default, got %+v", found)
	}
	if found := (Source{Tree: root, TrustTree: true}).Discover("rho-test"); len(found) != 1 {
		t.Fatalf("expected the tree's plugins to be read when trusted, got %+v", found)
	}
}

func TestAModuleThatIsNotAPluginIsNotAnExtension(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"
	if err := os.WriteFile(filepath.Join(root, "app", "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if found := (Source{Tree: root, TrustTree: true}).All(); len(found) != 0 {
		t.Fatalf("expected no plugins, got %+v", found)
	}
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

func TestAnAnswerIsFoldedIntoTheRun(t *testing.T) {
	root := t.TempDir()
	path := writePlugin(t, root, "reporter", "rho-test", "  - run\n", "")
	carryHelper(t, path)
	writeCommand(t, path, "report", "run", "report")

	found := (Source{Tree: root, TrustTree: true}).Discover("rho-test")
	result := RunHooks(found, "rho-test", "run", map[string]any{"results": 3})
	if len(result.Payload) != 1 {
		t.Fatalf("expected the plugin's answer, got %+v", result.Payload)
	}
	if strings.Contains(string(result.Payload[0]), "could not decode") {
		t.Fatalf("expected the payload to be what the plugin was given, got %s", result.Payload[0])
	}
}

func TestAHookTheComponentDoesNotRunIsNeverAsked(t *testing.T) {
	root := t.TempDir()
	path := writePlugin(t, root, "eager", "rho-test", "  - shutdown\n", "")
	carryHelper(t, path)
	writeCommand(t, path, "shutdown", "shutdown", "report")

	found := (Source{Tree: root, TrustTree: true}).Discover("rho-test")
	result := RunHooks(found, "rho-test", "run", map[string]any{})
	if len(result.Payload) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("expected a plugin for another hook to be left alone, got %+v", result)
	}
}

func TestAPluginThatFailsIsReportedAndTheRunContinues(t *testing.T) {
	// One broken extension must not cost the work of everything else.
	root := t.TempDir()
	path := writePlugin(t, root, "broken", "rho-test", "  - run\n", "")
	carryHelper(t, path)
	writeCommand(t, path, "report", "run", "fail")

	found := (Source{Tree: root, TrustTree: true}).Discover("rho-test")
	result := RunHooks(found, "rho-test", "run", map[string]any{})
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Message, "failed") {
		t.Fatalf("expected the failure to be reported, got %+v", result.Diagnostics)
	}
}

func TestAPluginAnsweringNonsenseIsReported(t *testing.T) {
	root := t.TempDir()
	path := writePlugin(t, root, "chatty", "rho-test", "  - run\n", "")
	carryHelper(t, path)
	writeCommand(t, path, "report", "run", "garbage")

	found := (Source{Tree: root, TrustTree: true}).Discover("rho-test")
	result := RunHooks(found, "rho-test", "run", map[string]any{})
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Message, "cannot read the response") {
		t.Fatalf("expected a response that is not the protocol to be reported, got %+v", result.Diagnostics)
	}
}

func TestACommandCannotLeaveThePluginDirectory(t *testing.T) {
	// A plugin runs a program it carries, or one on PATH. A path that climbs out
	// is neither.
	root := t.TempDir()
	path := writePlugin(t, root, "climber", "rho-test", "  - run\n",
		"commands:\n  - name: report\n    run: ../../../bin/sh\n    hook: run\n")

	found := (Source{Tree: root, TrustTree: true}).Discover("rho-test")
	if len(found) != 1 {
		t.Fatalf("expected the plugin to be discovered, got %+v", found)
	}
	_ = path
	result := RunHooks(found, "rho-test", "run", map[string]any{})
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Message, "leaves the plugin directory") {
		t.Fatalf("expected the escape to be refused, got %+v", result.Diagnostics)
	}
}

func TestACommandCarriesItsArguments(t *testing.T) {
	// A manifest has no way to say "this program, these arguments" otherwise,
	// so a plugin that needs a path with a space in it has to be able to say it.
	argv := splitCommand(`tool --flag "two words" value`)
	want := []string{"tool", "--flag", "two words", "value"}
	if len(argv) != len(want) {
		t.Fatalf("expected %v, got %v", want, argv)
	}
	for index := range want {
		if argv[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, argv)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The helper process                                                        */
/* -------------------------------------------------------------------------- */

// writeCommand adds a command to a plugin's manifest, naming the helper.
func writeCommand(t *testing.T, pluginDir, name, hook, behaviour string) {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(pluginDir, "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	command := "\ncommands:\n  - name: " + name + "\n    run: " + helperName +
		" -test.run TestHelperProcess -- " + behaviour + "\n    hook: " + hook + "\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "module.yml"), append(manifest, []byte(command)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestHelperProcess is not a test: it is the program a test plugin runs. It
// answers the protocol in the way it was asked to, and does nothing otherwise.
func TestHelperProcess(t *testing.T) {
	mode := helperMode()
	if mode == "" {
		t.Skip("not running as a plugin helper")
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		os.Exit(4)
	}
	if request.Version != ProtocolVersion {
		os.Exit(5)
	}
	switch mode {
	case "fail":
		os.Exit(1)
	case "garbage":
		os.Stdout.WriteString("this is not json\n")
		os.Exit(0)
	case "report":
		// The answer repeats what it was given, so a test can see that the
		// payload travelled intact.
		response := Response{Version: ProtocolVersion, Payload: request.Payload}
		encoded, _ := json.Marshal(response)
		os.Stdout.Write(encoded)
	}
	os.Exit(0)
}

func helperMode() string {
	for index, argument := range os.Args {
		if argument == "--" && index+1 < len(os.Args) {
			return os.Args[index+1]
		}
	}
	return ""
}
