//
// @file report_test.go
// @brief Tests for what a target says about itself.
//
// A spec is meant to be read, and a resolved one is meant to be read as one
// thing: a person checking which pin the LED is on, or why a build links a
// different library than they expected, should not have to open JSON that refers
// to other JSON. A plugin is the same idea from the other side, so both are here.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhtarget

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/plugins"
)

func reportedSpec() Spec {
	spec := testSpec()
	spec.MCU = "chip"
	spec.Arch = "arm-cortex-m"
	spec.VendorLibrary = "device_board"
	spec.Comment = "A board for the test."
	spec.Pins = map[string]Pin{
		"gp16": {Name: "GP16", Role: "spi0_rx"},
		"gp25": {Name: "GP25", Role: "led", Comment: "the on-board LED"},
	}
	spec.Peripherals = map[string]Peripheral{
		"i2c0": {Instance: "i2c0", Pins: map[string]string{"sda": "GP4", "scl": "GP5"}},
	}
	return spec
}

func TestTheReportNamesTheChainTheFactsCameFrom(t *testing.T) {
	// A board's compiler comes from its chip's architecture, and a person reading
	// the report should be able to see that rather than guess it.
	report := Describe(reportedSpec())
	if !strings.Contains(report, "Resolved from: chip -> arm-cortex-m") {
		t.Fatalf("expected the chain, got:\n%s", report)
	}
	for _, want := range []string{
		"A board for the test.",
		"core:      cortex-m4",
		"compiler:  arm-none-eabi-gcc",
		"links:     device_board",
		"-mcpu=cortex-m4",
		"CHIP=1",
		"FLASH",
		"0x08000000 + 0x100000",
		"openocd",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("expected %q in the report, got:\n%s", want, report)
		}
	}
}

func TestThePinMapListsPinsAndInterfaces(t *testing.T) {
	report := Pins(reportedSpec())
	if !strings.Contains(report, "GP25") || !strings.Contains(report, "led") {
		t.Fatalf("expected the pin and its role, got:\n%s", report)
	}
	if !strings.Contains(report, "the on-board LED") {
		t.Fatalf("expected the pin's comment, got:\n%s", report)
	}
	// An interface names the pins it is on, by role, so a caller asks for sda
	// rather than for a number the board chose.
	if !strings.Contains(report, "scl=GP5 sda=GP4") {
		t.Fatalf("expected the interface's pins, got:\n%s", report)
	}
}

// pluginAnswering writes a plugin that answers the spec hook with a response, and
// returns it as a plugin the component would load.
func pluginAnswering(t *testing.T, response string) []plugins.Plugin {
	t.Helper()
	// The response travels in the environment, because that is what a helper
	// process inherits: the plugin is a program, and a test that faked one would
	// test the fake.
	t.Setenv("RHO_SPEC_TEST_RESPONSE", response)

	root := t.TempDir()
	pluginRoot := filepath.Join(root, "board-spec")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(pluginRoot, "helper")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	manifest := "module:\n" +
		"  name: board-spec\n" +
		"  type: plugin\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: board-spec\n" +
		"\n" +
		"extends: " + ComponentName + "\n" +
		"\n" +
		"hooks:\n  - " + HookSpec + "\n" +
		"\n" +
		"commands:\n  - name: spec\n" +
		"    run: helper -test.run TestSpecHelperProcess\n" +
		"    hook: " + HookSpec + "\n"
	if err := os.WriteFile(filepath.Join(pluginRoot, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	found := plugins.Source{Tree: root, TrustTree: true}.Discover(ComponentName)
	if len(found) != 1 {
		t.Fatalf("expected the plugin to be discovered, got %+v", found)
	}
	return found
}

func TestAPluginCanDeclareATarget(t *testing.T) {
	// A board this component has never heard of is a fact about that board, and
	// the project that knows it says so rather than waiting for this component to
	// ship it.
	library := libraryOf(t, map[string]string{"arch.json": archSpec})
	found := pluginAnswering(t, `{"version":1,"payload":{"specs":[{"kind":"board","name":"teensy","core":"cortex-m7"}]}}`)

	loaded := LoadWith([]string{library.Root}, found)
	spec, ok := loaded.Get("teensy")
	if !ok {
		t.Fatalf("expected the plugin's target, got %v (%+v)", loaded.Names(), loaded.Diagnostics)
	}
	if spec.Core != "cortex-m7" || spec.Source != "a plugin" {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestAPluginDoesNotReplaceATargetThisComponentCarries(t *testing.T) {
	// The targets this component ships are the ones a project builds with, so a
	// plugin that declares one of those names is reported rather than obeyed.
	library := libraryOf(t, map[string]string{"arch.json": archSpec})
	found := pluginAnswering(t, `{"version":1,"payload":{"specs":[{"kind":"arch","name":"arm-cortex-m","core":"invented"}]}}`)

	loaded := LoadWith([]string{library.Root}, found)
	spec, _ := loaded.Get("arm-cortex-m")
	if spec.Core == "invented" {
		t.Fatal("expected the component's own target to win")
	}
	if len(loaded.Diagnostics) == 0 {
		t.Fatal("expected the duplicate to be reported")
	}
}

func TestAPluginAnsweringNonsenseIsReported(t *testing.T) {
	library := libraryOf(t, map[string]string{"arch.json": archSpec})
	found := pluginAnswering(t, `{"version":1,"payload":{"specs":[{"kind":"board"}]}}`)

	loaded := LoadWith([]string{library.Root}, found)
	if len(loaded.Diagnostics) == 0 {
		t.Fatal("expected a target that names nothing to be reported")
	}
}

// TestSpecHelperProcess is not a test: it is the program a test plugin runs. It
// answers with what the environment tells it to, and does nothing otherwise.
func TestSpecHelperProcess(t *testing.T) {
	response := os.Getenv("RHO_SPEC_TEST_RESPONSE")
	if response == "" {
		t.Skip("not running as a plugin helper")
	}
	if _, err := io.ReadAll(os.Stdin); err != nil {
		os.Exit(3)
	}
	os.Stdout.WriteString(response)
	os.Exit(0)
}
