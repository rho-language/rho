//
// @file resolve_test.go
// @brief Finding the preset a program is built for.
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
	"strings"
	"testing"
)

// writePlatform lays down a platform whose manifest declares two presets, with
// the family saying where its sources are and the board narrowing one of them.
// writeProject lays down a project that depends on a platform, and that
// platform declaring a family preset and a board preset that inherits it.
//
// Two roots, because that is the shape a real build has: a project says which
// presets it is written for and depends on the module that provides them, and
// the presets themselves live in that module. A fixture that put both in one
// directory would not exercise the part that decides where a preset is looked
// for, which is where this went wrong.
func writeProject(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	platform := filepath.Join(base, "platform")
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(platform, "presets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	platformManifest := `
module:
  name: platform
  type: platform
  version: 0.1.0
hardware:
  presets: presets
  targets: targets
`
	if err := os.WriteFile(filepath.Join(platform, "module.yml"), []byte(platformManifest), 0o644); err != nil {
		t.Fatalf("write platform manifest: %v", err)
	}

	projectManifest := `
module:
  name: app
  type: module
  version: 0.1.0
main: src/main.rho
depends:
  - name: platform
    type: c
    path: ` + platform + `
build:
  language: c
  presets:
    - board
    - family
`
	if err := os.WriteFile(filepath.Join(project, "module.yml"), []byte(projectManifest), 0o644); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}

	family := `{
  "name": "family",
  "description": "the family",
  "dependencies": [
    {"name": "core", "include": ["Include"], "src": ["Src"]},
    {"name": "device", "include": ["Include"], "src": ["Source"]}
  ]
}`
	if err := os.WriteFile(filepath.Join(platform, "presets", "family.json"), []byte(family), 0o644); err != nil {
		t.Fatal(err)
	}

	// A board says what makes it that board and leaves the rest to its family: no
	// dependencies of its own, and an override narrowing one of them to the
	// startup file for its part.
	board := `{
  "name": "board",
  "family": "TheFamily",
  "inherits": "family",
  "override": {
    "device": {"src": ["Source/startup_for_this_part.s"]}
  }
}`
	if err := os.WriteFile(filepath.Join(platform, "presets", "board.json"), []byte(board), 0o644); err != nil {
		t.Fatal(err)
	}
	return project, platform
}

// A board preset inherits its dependencies from its family.
//
// This is the one that matters and the one that was wrong: the platform's
// manifest is an index that says which presets exist, and the dependencies live
// in the file. Reading the index reported a board with nothing to build against,
// and the family it inherits from reported itself as empty — so a board declared
// correctly was described as unbuildable.
func TestABoardPresetInheritsItsFamilyDependencies(t *testing.T) {
	root, _ := writeProject(t)

	preset, err := (&PresetResolver{ProjectRoot: root}).LookupPreset("board")
	if err != nil {
		t.Fatalf("a board preset should be found: %v", err)
	}
	if len(preset.Dependencies) != 2 {
		t.Fatalf("expected the family's two dependencies, got %d: %+v", len(preset.Dependencies), preset.Dependencies)
	}
	if preset.Name != "board" {
		t.Errorf("expected the preset to be called board, got %q", preset.Name)
	}
}

// The lookup returns the preset as its file writes it, because that is where the
// dependencies and the overrides are. The index carries only names.
func TestLookupPresetReadsTheFileNotTheIndex(t *testing.T) {
	root, _ := writeProject(t)

	preset, err := (&PresetResolver{ProjectRoot: root}).LookupPreset("board")
	if err != nil {
		t.Fatal(err)
	}
	if len(preset.Override) == 0 {
		t.Error("expected the board's override, which only its file declares")
	}
	if preset.Family != "TheFamily" {
		t.Errorf("expected the family the board names, got %q", preset.Family)
	}
}

// A chain that names a preset the platform does not have is a broken
// declaration, and saying which preset broke it is the whole of the report.
func TestAnInheritanceChainThatNamesNothingIsReported(t *testing.T) {
	root, platform := writeProject(t)
	if err := os.WriteFile(filepath.Join(platform, "presets", "board.json"),
		[]byte(`{"name": "board", "inherits": "absent"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&PresetResolver{ProjectRoot: root}).LookupPreset("board")
	if err == nil {
		t.Fatal("expected a report: the chain names a preset that does not exist")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Errorf("the report should name what could not be found, got: %v", err)
	}
}

// An override narrows an inherited dependency rather than replacing the set. The
// board picked one startup file out of a directory of them, and the family's other
// dependency is still part of the build.
func TestAnOverrideNarrowsAnInheritedDependency(t *testing.T) {
	root, _ := writeProject(t)

	preset, err := (&PresetResolver{ProjectRoot: root}).LookupPreset("board")
	if err != nil {
		t.Fatal(err)
	}
	device := dependencyNamed(preset.Dependencies, "device")
	if device == nil {
		t.Fatalf("expected the inherited device dependency, got %+v", preset.Dependencies)
	}
	if len(device.Src) != 1 || device.Src[0] != "Source/startup_for_this_part.s" {
		t.Errorf("expected the override to narrow the device sources, got %v", device.Src)
	}
	core := dependencyNamed(preset.Dependencies, "core")
	if core == nil || len(core.Src) == 0 {
		t.Errorf("the family's other dependency should still be built, got %+v", preset.Dependencies)
	}
}

// dependencyNamed finds a dependency by name, or nil.
func dependencyNamed(dependencies []CDep, name string) *CDep {
	for i := range dependencies {
		if dependencies[i].Name == name {
			return &dependencies[i]
		}
	}
	return nil
}
