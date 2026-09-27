//
// @file plugin_test.go
// @brief Tests for the plugin protocol.
//
// A plugin is another process, so the protocol is a contract rather than a
// call. These tests cover what a plugin is given, what it may answer with, and
// what happens when it fails: a build that stops because one extension broke
// has cost the reader every other page.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

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

// writePlugin writes a plugin whose command runs a helper program.
//
// The helper is a Go test binary of this package run as a subprocess, which is
// the only way to exercise a real process without shipping a shell script: the
// behaviour under test is the pipe, not the helper.
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

// aCommand is one command declaration, written the way a manifest spells it:
// the hook it answers, and the program that answers it. The program is the test
// binary asked to behave in the named way, which is how a real process is
// exercised without shipping a shell script.
func aCommand(t *testing.T, name, hook, behaviour string) string {
	return "  - name: " + name + "\n    run: " + helperCommand(t, behaviour) + "\n    hook: " + hook + "\n"
}

// helperCommand is the command a test plugin runs: this test binary, in the
// helper mode the test below defines.
func helperCommand(t *testing.T, name string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return quoteArg(executable) + " -test.run TestPluginHelperProcess -- " + quoteArg(name)
}

func quoteArg(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}

/* -------------------------------------------------------------------------- */
/*  The protocol                                                              */
/* -------------------------------------------------------------------------- */

func TestPluginRequestCarriesTheWholeSite(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "toc", ComponentName, "  - "+HookIndex+"\n",
		"\ncommands:\n"+aCommand(t, "contents", HookIndex, "contents"))
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  section: guides\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	plugins := DiscoverPlugins(root, "", ComponentName)
	if len(plugins) != 1 || plugins[0].Name != "toc" {
		t.Fatalf("expected the plugin to be discovered, got %+v", plugins)
	}
	if len(plugins[0].Commands) != 1 || plugins[0].Commands[0].Hook != "index" {
		t.Fatalf("unexpected commands: %+v", plugins[0].Commands)
	}

	site := BuildSite(modules, ApiSet{}, SiteOptions{Title: "Rho"})
	// The response is written by the helper, which is where the request is
	// checked; a failure here means the request was not what the protocol says.
	site.RunPlugins(plugins, filepath.Join(root, "out"))

	if len(site.Pages) != 2 {
		t.Fatalf("expected the plugin's page to be added, got %d pages", len(site.Pages))
	}
	added := findPage(site, "contents")
	if added == nil {
		t.Fatalf("expected the contents page, got %+v", site.Pages)
	}
	if added.Module != "toc" || added.Section != "extensions" {
		t.Fatalf("expected the page attributed to the plugin, got %+v", added)
	}
	if !strings.Contains(added.Body, "Index") {
		t.Fatalf("expected the page the helper built from the request, got:\n%s", added.Body)
	}
}

func TestPluginCanRewriteAPage(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "banner", ComponentName, "  - "+HookPage+"\n",
		"\ncommands:\n"+aCommand(t, "page", HookPage, "banner"))
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	plugins := DiscoverPlugins(root, "", ComponentName)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	site.RunPlugins(plugins, filepath.Join(root, "out"))

	page := findPage(site, "index")
	if page == nil {
		t.Fatal("expected the page")
	}
	if !strings.Contains(page.Body, "banner-added") {
		t.Fatalf("expected the plugin's body, got:\n%s", page.Body)
	}
}

func TestAFailingPluginIsReportedAndSkipped(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "broken", ComponentName, "  - "+HookPage+"\n",
		"\ncommands:\n"+aCommand(t, "page", HookPage, "fail"))
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	plugins := DiscoverPlugins(root, "", ComponentName)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	site.RunPlugins(plugins, filepath.Join(root, "out"))

	// The documentation still exists: one broken extension does not cost the
	// reader the pages of everything else.
	if len(site.Pages) != 1 {
		t.Fatalf("expected the page to survive, got %d", len(site.Pages))
	}
	if len(site.Diagnostics) == 0 {
		t.Fatal("expected the failure to be reported")
	}
	if !strings.Contains(site.Diagnostics[0].Message, "failed") {
		t.Fatalf("expected the failure to be named, got %q", site.Diagnostics[0].Message)
	}
}

func TestAPluginAnsweringNonsenseIsReported(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "chatty", ComponentName, "  - "+HookPage+"\n",
		"\ncommands:\n"+aCommand(t, "page", HookPage, "garbage"))
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	plugins := DiscoverPlugins(root, "", ComponentName)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	site.RunPlugins(plugins, filepath.Join(root, "out"))

	if len(site.Diagnostics) == 0 {
		t.Fatal("expected a response that is not JSON to be reported")
	}
}

func TestAPluginThatIsNotThereIsReported(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "absent", ComponentName, "  - "+HookPage+"\n",
		"\ncommands:\n  - name: page\n    run: rho-doc-plugin-that-does-not-exist\n    hook: page\n")
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	plugins := DiscoverPlugins(root, "", ComponentName)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	site.RunPlugins(plugins, filepath.Join(root, "out"))

	if len(site.Diagnostics) == 0 || !strings.Contains(site.Diagnostics[0].Message, "cannot find") {
		t.Fatalf("expected a missing executable to be reported, got %+v", site.Diagnostics)
	}
}

func TestADiscoveredPluginCarriesItsRelation(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "guide", ComponentName, "  - "+HookPage+"\n",
		"\ncommands:\n"+aCommand(t, "page", HookPage, "banner"))

	plugins := DiscoverPlugins(root, "", ComponentName)
	if len(plugins) != 1 {
		t.Fatalf("expected one plugin, got %+v", plugins)
	}
	plugin := plugins[0]
	if plugin.Extends != ComponentName {
		t.Fatalf("expected the component it extends, got %q", plugin.Extends)
	}
	if !plugin.Answers(HookPage) {
		t.Fatalf("expected the hook to be recorded, got %+v", plugin.Hooks)
	}
	if plugin.Answers(HookIndex) {
		t.Fatal("expected an undeclared hook to stay unanswered")
	}
}

func TestAPluginForAnotherComponentIsNotLoaded(t *testing.T) {
	// The relation is the filter: a plugin written for a different tool is
	// inert here rather than being called with a request it cannot answer.
	root := t.TempDir()
	writePlugin(t, root, "other", "rho-fmt", "  - "+HookPage+"\n",
		"\ncommands:\n"+aCommand(t, "page", HookPage, "banner"))

	if plugins := DiscoverPlugins(root, "", ComponentName); len(plugins) != 0 {
		t.Fatalf("expected a plugin for another component to be left out, got %+v", plugins)
	}
	all := DiscoverAllPlugins(root, "")
	if len(all) != 1 || all[0].Extends != "rho-fmt" {
		t.Fatalf("expected it to be readable as somebody else's plugin, got %+v", all)
	}
}

func TestAHookThisGeneratorDoesNotRunIsNeverCalled(t *testing.T) {
	// A plugin may declare a hook the component does not offer, if it was
	// written against another version of the manifest. The build asks for
	// nothing it does not run, whatever a plugin claims.
	root := t.TempDir()
	writePlugin(t, root, "eager", ComponentName, "  - shutdown\n",
		"\ncommands:\n"+aCommand(t, "shutdown", "shutdown", "contents"))

	plugins := DiscoverPlugins(root, "", ComponentName)
	result := RunHooks(plugins, "shutdown", PluginRequest{Version: ProtocolVersion})
	if len(result.Pages) != 0 {
		t.Fatalf("expected no answer from a hook the generator does not run, got %+v", result.Pages)
	}
}

func TestAModuleThatIsNotAPluginIsNotAnExtension(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")

	if plugins := DiscoverPlugins(root, "", ComponentName); len(plugins) != 0 {
		t.Fatalf("expected no plugins, got %+v", plugins)
	}
}

/* -------------------------------------------------------------------------- */
/*  The helper process                                                        */
/* -------------------------------------------------------------------------- */

// TestPluginHelperProcess is not a test: it is the program a test plugin runs.
// It answers the protocol according to the mode named after "--", and a test
// binary that is not running as a helper does nothing.
func TestPluginHelperProcess(t *testing.T) {
	mode := helperMode()
	if mode == "" {
		t.Skip("not running as a plugin helper")
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	var request PluginRequest
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
	case "contents":
		// A table of contents over the pages the request carries, which is
		// the whole point of a whole-site hook.
		var items []string
		for _, page := range request.Pages {
			items = append(items, "<li>"+page.Title+"</li>")
		}
		response := PluginResponse{
			Version: ProtocolVersion,
			Pages: []PluginPage{{
				Target: "contents",
				Title:  "Contents",
				HTML:   "<nav><ul>" + strings.Join(items, "") + "</ul></nav>",
				Plain:  "contents",
			}},
			Formats: []string{"latex"},
		}
		encoded, _ := json.Marshal(response)
		os.Stdout.Write(encoded)
	case "banner":
		response := PluginResponse{
			Version:      ProtocolVersion,
			Replacements: map[string]string{request.Page.Target: "<p>banner-added</p>"},
		}
		encoded, _ := json.Marshal(response)
		os.Stdout.Write(encoded)
	}
	os.Exit(0)
}

// helperMode is the mode a helper process was asked for, which is the argument
// after "--".
func helperMode() string {
	arguments := os.Args
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}
