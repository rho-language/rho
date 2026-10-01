//
// @file preset_test.go
// @brief Tests for reading a preset, and for the shape of one.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  A preset named by a file                                                   */
/* -------------------------------------------------------------------------- */

// A preset is written in its own file and named by the manifest, because it is
// long: three repositories, their revisions, a dozen directories and the hardware
// they are for. Written inline it makes the manifest unreadable.
func TestAPresetIsReadFromTheFileTheManifestNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "rho.mod", `
module:
  name: hello-world
  type: application
  version: 0.1.0

preset: presets/nucleo-f446re.yaml
`)
	writeFile(t, root, "presets/nucleo-f446re.yaml", `
name: nucleo-f446re
description: A Nucleo-F446RE, which is an STM32F446 on a Nucleo-144 board.
family: STM32F4
mcu: STM32F446RE
core: cortex-m4
device_define: STM32F446xx
board: NUCLEO-F446RE
external_clock: "8000000"

dependencies:
  - name: cmsis-core
    git: https://github.com/STMicroelectronics/STM32CubeF4.git
    branch: master
    include:
      - Drivers/CMSIS/Core/Include
  - name: cmsis-device
    git: https://github.com/STMicroelectronics/cmsis_device_f4.git
    branch: master
    include:
      - Include
    src:
      - Source/Templates/gcc
`)

	preset, err := ReadPreset(root)
	if err != nil {
		t.Fatalf("ReadPreset failed: %v", err)
	}

	// The descriptive half is read as well as the paths, because a build report
	// has to be able to say what it was for and a link failure has to be
	// diagnosed against the device define.
	if preset.MCU != "STM32F446RE" || preset.Core != "cortex-m4" {
		t.Errorf("hardware is %q/%q, want STM32F446RE/cortex-m4", preset.MCU, preset.Core)
	}
	if preset.DeviceDefine != "STM32F446xx" || preset.Board != "NUCLEO-F446RE" {
		t.Errorf("define/board are %q/%q, want STM32F446xx/NUCLEO-F446RE", preset.DeviceDefine, preset.Board)
	}
	if len(preset.Dependencies) != 2 {
		t.Fatalf("read %d dependencies, want 2", len(preset.Dependencies))
	}

	// The startup assembly is the whole point of the device repository: a
	// bare-metal program has no entry point without it, so a preset that dropped
	// it would produce a program that cannot be started.
	device := preset.Dependencies[1]
	found := false
	for _, src := range device.Src {
		if src == "Source/Templates/gcc" {
			found = true
		}
	}
	if !found {
		t.Errorf("the device dependency lost the startup assembly: %v", device.Src)
	}
}

// A preset written inline is read the same way. Both spellings exist because
// the data is long and where it lives is a choice about how the manifest reads,
// not about what the preset means.
func TestAPresetMayBeWrittenInline(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "rho.mod", `
module:
  name: hello-world
  version: 0.1.0

preset:
  name: inline
  mcu: STM32F446RE
  dependencies:
    - name: hal
      git: https://example.com/hal.git
      branch: main
      include: Inc
`)

	preset, err := ReadPreset(root)
	if err != nil {
		t.Fatalf("ReadPreset failed: %v", err)
	}
	if preset.Name != "inline" || preset.MCU != "STM32F446RE" {
		t.Errorf("read %+v, want the inline preset", preset)
	}
	if len(preset.Dependencies) != 1 || preset.Dependencies[0].Name != "hal" {
		t.Fatalf("read %v, want one dependency named hal", preset.Dependencies)
	}
	// One path written as a scalar reads the same as one written in a list. A
	// preset that needs one directory should not have to learn a list for it.
	if got := preset.Dependencies[0].Include; len(got) != 1 || got[0] != "Inc" {
		t.Errorf("include is %v, want [Inc]", got)
	}
}

// Most programs are not for anything in particular, and a reader that refused to
// answer without a preset would be a reader every build had to guard.
func TestAProjectWithNoPresetIsNotAProblem(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "rho.mod", `
module:
  name: plain
  version: 0.1.0
`)
	preset, err := ReadPreset(root)
	if err != nil {
		t.Fatalf("a project with no preset was refused: %v", err)
	}
	if preset.Name != "" {
		t.Errorf("read a preset named %q from a project that declares none", preset.Name)
	}
}

// A preset that names a file which is not there is a mistake in the manifest,
// and the error says which file — a build that cannot be arranged should say
// what it was looking for rather than that something was missing.
func TestAPresetNamingAMissingFileSaysWhich(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "rho.mod", "module:\n  name: x\n  version: 0.1.0\n\npreset: presets/absent.yaml\n")
	_, err := ReadPreset(root)
	if err == nil {
		t.Fatalf("a preset naming a file that is not there was accepted")
	}
	if got := err.Error(); !contains(got, "absent.yaml") {
		t.Errorf("the error is %q, and does not name the file it could not read", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  One preset out of a library                                               */
/* -------------------------------------------------------------------------- */

// A platform holds a preset per board, and a build needs one of them. Reading
// the one file that could hold it is the difference between a build that parses
// one preset and one that parses forty.
func TestOnePresetIsReadOutOfALibraryByName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pico.yaml", "name: pico\ndependencies:\n  - name: sdk\n    git: https://example.com/pico-sdk.git\n")
	writeFile(t, dir, "esp32.yaml", "name: esp32\ndependencies:\n  - name: sdk\n    git: https://example.com/esp32-sdk.git\n")
	writeFile(t, dir, "nucleo-f446re.json", `{"name":"nucleo-f446re","dependencies":[{"name":"hal","git":"https://example.com/hal.git"}]}`)

	// Both formats, because a platform may ship either and a build should not
	// care which.
	preset, err := LoadPresetByName(dir, "esp32")
	if err != nil {
		t.Fatalf("LoadPresetByName failed: %v", err)
	}
	if preset.Name != "esp32" {
		t.Errorf("read the preset named %q, want esp32", preset.Name)
	}

	preset, err = LoadPresetByName(dir, "nucleo-f446re")
	if err != nil {
		t.Fatalf("LoadPresetByName failed for the JSON preset: %v", err)
	}
	if preset.Dependencies[0].Git != "https://example.com/hal.git" {
		t.Errorf("read %+v from the JSON preset", preset.Dependencies[0])
	}
}

// A preset that is not there says what was looked for. The three names are the
// whole convention, and a caller who spelled one differently needs to see that
// rather than an error that only says no.
func TestAMissingPresetSaysWhatWasLookedFor(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadPresetByName(dir, "absent")
	if err == nil {
		t.Fatalf("a preset that is not there was returned")
	}
	for _, want := range []string{"absent", "absent.yaml", "absent.json"} {
		if !contains(err.Error(), want) {
			t.Errorf("the error is %q, and does not mention %q", err, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  A preset that says too little                                              */
/* -------------------------------------------------------------------------- */

// A preset with no dependencies is a build with nothing to build against, and
// saying so is better than fetching nothing and reporting a missing header.
func TestAPresetWithNoDependenciesIsRefused(t *testing.T) {
	if err := (Preset{Name: "empty"}).Validate(); err == nil {
		t.Errorf("a preset with no dependencies was accepted")
	}
}

// A dependency that says where it comes from nowhere cannot be fetched, and the
// error names it.
// A dependency need not name a git URL or a path of its own: the board says
// where each of its vendor repositories lives. What must still be refused is a
// dependency that names nothing at all, because then there is nowhere for a build
// to look — which is decided where it is used, since only there is the board that
// might have said.
func TestADependencyWithNoSourceIsResolvedOrRefused(t *testing.T) {
	dependency := CDep{Name: "sdk"}

	// No source and nowhere to look: refused.
	resolver := &Resolver{ProjectRoot: t.TempDir(), Cache: t.TempDir()}
	if _, err := resolver.Resolve(Preset{Name: "p", Dependencies: []CDep{dependency}}); err == nil {
		t.Error("expected a report: nothing says where this dependency is")
	}

	// No source of its own, but the board says where it is: resolved, not refused.
	//
	// The board is reached the way it really is: through the platform the project
	// depends on. A board is not a dependency of the program — the platform
	// implements every board of the family at once.
	root := t.TempDir()
	writeFile(t, root, "module.yml", "module:\n  name: platform\n  type: platform\n  version: 0.1.0\n")
	project := t.TempDir()
	writeFile(t, project, "module.yml",
		"module:\n  name: app\n  type: module\n  version: 0.1.0\ndepends:\n  - name: platform\n    type: c\n    path: "+root+"\n")
	vendor := filepath.Join(root, "vendor", "sdk")
	if err := os.MkdirAll(filepath.Join(vendor, "Inc"), 0o755); err != nil {
		t.Fatal(err)
	}
	board := filepath.Join(root, "boards", "b")
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, board, "module.yml",
		"module:\n  name: b\n  type: platform-board\n  version: 0.1.0\nvendor:\n  dir: ../../vendor\n  repos:\n    - name: sdk\n      path: sdk\n")

	found := &Resolver{ProjectRoot: project, Cache: t.TempDir(), BoardName: "b"}
	if _, err := found.Resolve(Preset{
		Name:         "p",
		Dependencies: []CDep{dependency},
	}); err != nil {
		t.Errorf("a dependency the board places should resolve, got: %v", err)
	}
}

// A dependency that names both a repository and a path is one or the other, and
// which was meant is a question only its author can answer.
func TestADependencyNamingBothSourcesIsRefused(t *testing.T) {
	dep := CDep{Name: "hal", Git: "https://example.com/hal.git", Path: "../hal"}
	if err := dep.Validate(); err == nil {
		t.Errorf("a dependency naming both a repository and a path was accepted")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// writeFile writes a file under a directory, creating what it needs.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// contains reports whether a value is among a string's substrings.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

// indexOf is strings.Index, written here so the test file does not import
// strings for one call.
func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
