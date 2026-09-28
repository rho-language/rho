//
// @file bridge_test.go
// @brief Tests for resolving a project's configuration end to end.
//
// The reader and the dialects are tested on their own. What is tested here is
// the whole path a build takes: a project's declaration, its layers, the merge,
// and the files that come out — because that is the path a value takes, and a
// value that is silently dropped anywhere along it is a setting that did
// nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhokconfig

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/plugins"
)

// project is a project that declares a bridge: a fragment, a defconfig, a
// module of constants and a manifest that names them.
type project struct {
	root string
}

// newProject writes a project that declares a configuration bridge, and returns
// the paths a test needs to point at.
func newProject(t *testing.T, dialect string) project {
	t.Helper()
	root := t.TempDir()
	write(t, root, "module.yml", `module:
  name: sensorhub
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c

config:
  dialect: `+dialect+`
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_LOG_LEVEL: "debug"
  module: config.rho
`)
	write(t, root, "kconfig/board.Kconfig", fragment)
	write(t, root, "kconfig/rho-defconfig", "CONFIG_UART_BAUD=9600\nCONFIG_SPI_SLAVE=y\n")
	write(t, root, "config.rho", `module config

fn init() {
    let CONFIG_SPI_MASTER: bool = true
}
`)
	return project{root: root}
}

// options is what the generated build asks for.
func (p project) options() Options {
	return Options{
		Root:      p.root,
		Fragments: []string{"kconfig/board.Kconfig"},
		Defconfig: "kconfig/rho-defconfig",
		Dialect:   "linux",
		Values:    map[string]string{"CONFIG_LOG_LEVEL": "debug"},
		Module:    "config.rho",
		Overwrite: true,
	}
}

/* -------------------------------------------------------------------------- */
/*  The whole path                                                            */
/* -------------------------------------------------------------------------- */

func TestAProjectsLayersAreMergedInOrder(t *testing.T) {
	// Each layer is set by the project and the answer says which one won, so a
	// person asking why a value is what it is gets an answer.
	p := newProject(t, "linux")
	result, err := Merge(p.options())
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(result.Problems()) != 0 {
		t.Fatalf("expected a clean merge, got %+v", result.Problems())
	}

	for name, want := range map[string]struct {
		value string
		from  string
	}{
		"CONFIG_UART_BAUD":  {"9600", "defconfig"},
		"CONFIG_LOG_LEVEL":  {"debug", "module.yml"},
		"CONFIG_SPI_MASTER": {"y", "config module"},
		"CONFIG_SPI_SLAVE":  {"y", "defconfig"},
		// Nothing set the level below, so its default stands.
		"CONFIG_UART_TX_PIN": {"0x25", "default"},
	} {
		value, found := result.Configuration.Get(name)
		if !found {
			t.Errorf("expected %s to be resolved", name)
			continue
		}
		if value.Value != want.value || value.From != want.from {
			t.Errorf("%s: expected %s from %s, got %s from %s", name, want.value, want.from, value.Value, value.From)
		}
	}
}

func TestTheFilesAreWrittenWhereTheDialectExpectsThem(t *testing.T) {
	p := newProject(t, "espidf")
	options := p.options()
	options.Dialect = "espidf"
	result, err := Merge(options)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(result.Written) != 2 {
		t.Fatalf("expected the dialect's two files, got %+v", result.Written)
	}
	header, err := os.ReadFile(filepath.Join(p.root, "build", "config", "sdkconfig.h"))
	if err != nil {
		t.Fatalf("the header was not written where ESP-IDF reads it: %v", err)
	}
	if !strings.Contains(string(header), "#define CONFIG_LOG_LEVEL \"debug\"") {
		t.Fatalf("expected the manifest's value in the header, got:\n%s", header)
	}
}

func TestAConfigurationWithAProblemIsNotWrittenQuietly(t *testing.T) {
	// A value for a symbol nothing declares is a setting that did nothing, and
	// a build has to hear about it.
	p := newProject(t, "linux")
	options := p.options()
	options.Values["CONFIG_NOT_A_SYMBOL"] = "1"
	result, err := Merge(options)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(result.Problems()) != 1 {
		t.Fatalf("expected the unknown symbol to be a problem, got %+v", result.Diagnostics)
	}
}

func TestAMergeWithNoFragmentsIsRefused(t *testing.T) {
	// Nothing declared means there is no vocabulary to resolve a value against,
	// and a build that configures nothing by accident is worse than one that
	// stops.
	p := newProject(t, "linux")
	options := p.options()
	options.Fragments = nil
	if _, err := Merge(options); err == nil {
		t.Fatal("expected a merge with no fragments to be refused")
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading what a project declared                                           */
/* -------------------------------------------------------------------------- */

func TestAProjectsDeclarationIsRead(t *testing.T) {
	p := newProject(t, "espidf")
	config, err := ReadProjectConfig(p.root)
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if !config.Configured {
		t.Fatal("expected the bridge to be read")
	}
	if config.Dialect != "espidf" {
		t.Errorf("expected the dialect, got %q", config.Dialect)
	}
	if len(config.Fragments) != 1 || config.Fragments[0] != "kconfig/board.Kconfig" {
		t.Errorf("expected the fragment, got %+v", config.Fragments)
	}
	if config.Values["CONFIG_LOG_LEVEL"] != "debug" {
		t.Errorf("expected the manifest's value, got %+v", config.Values)
	}
	if config.Module != "config.rho" {
		t.Errorf("expected the module of constants, got %q", config.Module)
	}
	if config.Defconfig == "" {
		t.Error("expected the defconfig beside the fragments to be found")
	}
}

func TestAProjectWithNoBridgeIsSaidSo(t *testing.T) {
	// Most projects declare nothing, and that is a decision rather than a
	// failure: they are built with no configuration.
	root := t.TempDir()
	write(t, root, "module.yml", "module:\n  name: plain\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n")

	config, err := ReadProjectConfig(root)
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if config.Configured {
		t.Fatalf("expected no bridge, got %+v", config)
	}
}

/* -------------------------------------------------------------------------- */
/*  Reporting and setting                                                     */
/* -------------------------------------------------------------------------- */

func TestListingSaysWhereEveryValueCameFrom(t *testing.T) {
	// A person asking why a setting is what it is should not have to read the
	// fragments to find out.
	p := newProject(t, "linux")
	result, err := Merge(p.options())
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	listing := List(result.Configuration)
	for _, want := range []string{"CONFIG_UART_BAUD", "9600", "defconfig", "CONFIG_LOG_LEVEL", "debug", "module.yml"} {
		if !strings.Contains(listing, want) {
			t.Errorf("expected %q in the listing, got:\n%s", want, listing)
		}
	}
}

func TestSettingAValueWritesADefconfig(t *testing.T) {
	p := newProject(t, "linux")
	result, err := Merge(p.options())
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	rendered, diagnostics := Set(p.root, "kconfig/rho-defconfig", "CONFIG_UART_BAUD", "230400", result.Configuration)
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean set, got %+v", diagnostics)
	}
	if !strings.Contains(rendered, "CONFIG_UART_BAUD=230400") {
		t.Fatalf("expected the new value, got:\n%s", rendered)
	}
	// The file keeps the other values, because a defconfig is a baseline rather
	// than a single setting.
	if !strings.Contains(rendered, "CONFIG_SPI_SLAVE=y") {
		t.Fatalf("expected the other values to be kept, got:\n%s", rendered)
	}
}

func TestSettingASymbolNothingDeclaresIsRefused(t *testing.T) {
	// A value nothing reads is a mistake worth refusing rather than writing.
	p := newProject(t, "linux")
	result, err := Merge(p.options())
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	_, diagnostics := Set(p.root, "kconfig/rho-defconfig", "CONFIG_MADE_UP", "1", result.Configuration)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "no Kconfig fragment declares") {
		t.Fatalf("expected the unknown symbol to be refused, got %+v", diagnostics)
	}
}

func TestAWrittenFileIsNotAProblem(t *testing.T) {
	// A note is something worth saying; a problem is one that changed the answer.
	// A build that resolved everything it was asked to is not a build that
	// failed, so keeping a file is a note.
	note := note(".config", "a file is already there")
	if note.Problem() {
		t.Fatal("expected a note not to be a problem")
	}
	if !problem(".config", 0, "a value is wrong").Problem() {
		t.Fatal("expected a problem to be a problem")
	}
}

/* -------------------------------------------------------------------------- */
/*  The hook a plugin renders through                                          */
/* -------------------------------------------------------------------------- */

// answeringPlugin is a plugin that answers the render hook with one file, run as
// a helper process: a plugin is a program, so a test that faked one would test
// the fake.
func answeringPlugin(t *testing.T, root string) plugins.Plugin {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "extra-dialect"
	pluginRoot := filepath.Join(root, name)
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(pluginRoot, "helper")); err != nil {
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
		"hooks:\n  - " + HookRender + "\n" +
		"\n" +
		"commands:\n  - name: render\n" +
		"    run: helper -test.run TestRenderHelperProcess\n" +
		"    hook: " + HookRender + "\n"
	if err := os.WriteFile(filepath.Join(pluginRoot, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	found := plugins.Source{Tree: root, TrustTree: true}.Discover(ComponentName)
	if len(found) != 1 {
		t.Fatalf("expected the plugin to be discovered, got %+v", found)
	}
	return found[0]
}

func TestAPluginCanRenderOneMoreDialect(t *testing.T) {
	// The hook exists so a project can target a tool this component has never
	// heard of: the plugin renders it and the run writes what it answered.
	p := newProject(t, "linux")
	plugin := answeringPlugin(t, t.TempDir())

	options := p.options()
	options.Plugins = []plugins.Plugin{plugin}
	result, err := Merge(options)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(result.Problems()) != 0 {
		t.Fatalf("expected a clean run, got %+v", result.Problems())
	}
	written := ""
	for _, name := range result.Written {
		if name == "plugin-extra.conf" {
			written = name
		}
	}
	if written == "" {
		t.Fatalf("expected the plugin's file to be written, got %+v", result.Written)
	}
	content, err := os.ReadFile(filepath.Join(p.root, "plugin-extra.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "from a plugin") {
		t.Fatalf("expected the plugin's content, got %q", content)
	}
}

func TestARunWithNoPluginsWritesWhatItHas(t *testing.T) {
	// Most runs have no plugins, and a run with none writes the dialect it was
	// asked for and nothing else.
	p := newProject(t, "linux")
	result, err := Merge(p.options())
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	for _, name := range result.Written {
		if strings.Contains(name, "plugin") {
			t.Fatalf("expected no plugin files, got %+v", result.Written)
		}
	}
}

// TestRenderHelperProcess is not a test: it is the program a test plugin runs.
// It answers the render hook with one file, and does nothing otherwise.
func TestRenderHelperProcess(t *testing.T) {
	if os.Getenv("RHO_PLUGIN") == "" {
		t.Skip("not running as a plugin helper")
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	// The protocol is an envelope with the component's own payload inside it, so
	// a plugin reads the envelope first and its payload after: decoding the whole
	// thing as the payload would find every field empty and no error.
	envelope := struct {
		Hook    string          `json:"hook"`
		Payload json.RawMessage `json:"payload"`
	}{}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		os.Exit(4)
	}
	request := RenderRequest{}
	if err := json.Unmarshal(envelope.Payload, &request); err != nil {
		os.Exit(4)
	}
	if envelope.Hook != HookRender {
		os.Exit(7)
	}
	// The request carries the resolved symbols, which is what a plugin renders
	// from: a plugin that saw nothing could not render anything.
	if request.Dialect == "" || len(request.Values) == 0 {
		os.Exit(5)
	}
	body, err := json.Marshal(RenderResponse{Files: []RenderFile{{Path: "plugin-extra.conf", Content: "# from a plugin\n"}}})
	if err != nil {
		os.Exit(6)
	}
	answer := struct {
		Version int             `json:"version"`
		Payload json.RawMessage `json:"payload"`
	}{Version: 1, Payload: body}
	encoded, err := json.Marshal(answer)
	if err != nil {
		os.Exit(6)
	}
	os.Stdout.Write(encoded)
	os.Exit(0)
}
