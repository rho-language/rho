//
// @file preset_deps_test.go
// @brief Rho dependencies - a dependency, and the platform library, for named presets.
//
// One manifest written for two boards names a platform for each: the dependency
// says which presets it belongs to, and the connection's variant for a preset
// says which library that build links.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package deps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const presetManifest = `module:
  name: app
  version: 0.1.0
  type: module
  description: x

main: src/main.c

build:
  language: c
  presets: [pico2w, lolin32-lite]

depends:
  - name: robohub-pico
    type: c
    path: ${RHO_WORKSPACE}/robohub-pico
    presets: [pico2w]
  - name: robohub-esp32
    type: c
    path: ${RHO_WORKSPACE}/robohub-esp32
    presets:
      - lolin32-lite
  - name: robohub-common
    type: c
    path: ${RHO_WORKSPACE}/robohub-common

c_connection:
  library: robohub-pico
  headers: [platform.h]
  variants:
    lolin32-lite:
      library: robohub-esp32
      include_dirs:
        - ${RHO_WORKSPACE}/robohub-esp32/include
    pico2w:
      include_dirs:
        - ${RHO_WORKSPACE}/robohub-pico/include
`

func loadPresetManifest(t *testing.T, body string) (*Manifest, string) {
	t.Helper()
	ws := t.TempDir()
	t.Setenv("RHO_WORKSPACE", ws)
	file := filepath.Join(t.TempDir(), "module.yml")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(file)
	if err != nil {
		t.Fatal(err)
	}
	return m, ws
}

func depNamed(t *testing.T, m *Manifest, name string) DepSpec {
	t.Helper()
	for _, dep := range m.Deps {
		if dep.Name == name {
			return dep
		}
	}
	t.Fatalf("no dependency %s in %+v", name, m.Deps)
	return DepSpec{}
}

func TestDependencyPresetsAreRead(t *testing.T) {
	m, _ := loadPresetManifest(t, presetManifest)
	if got := depNamed(t, m, "robohub-pico").Presets; !reflect.DeepEqual(got, []string{"pico2w"}) {
		t.Errorf("robohub-pico presets = %v", got)
	}
	if got := depNamed(t, m, "robohub-esp32").Presets; !reflect.DeepEqual(got, []string{"lolin32-lite"}) {
		t.Errorf("robohub-esp32 presets = %v", got)
	}
	if got := depNamed(t, m, "robohub-common").Presets; got != nil {
		t.Errorf("a dependency without presets has %v, want nil", got)
	}
}

func TestDependencyAppliesToItsOwnPresetsOnly(t *testing.T) {
	m, _ := loadPresetManifest(t, presetManifest)
	pico := depNamed(t, m, "robohub-pico")
	common := depNamed(t, m, "robohub-common")
	cases := []struct {
		dep    DepSpec
		preset string
		want   bool
	}{
		{pico, "pico2w", true},
		{pico, "lolin32-lite", false},
		{pico, "", true},
		{common, "pico2w", true},
		{common, "lolin32-lite", true},
	}
	for _, c := range cases {
		if got := c.dep.AppliesTo(c.preset); got != c.want {
			t.Errorf("%s.AppliesTo(%q) = %v, want %v", c.dep.Name, c.preset, got, c.want)
		}
	}
}

func TestVariantLibraryReplacesTheConnectionsForItsPreset(t *testing.T) {
	m, ws := loadPresetManifest(t, presetManifest)
	connection := m.Connection
	if got := connection.Variants["lolin32-lite"].Library; got != "robohub-esp32" {
		t.Errorf("variant library = %q", got)
	}
	for preset, want := range map[string]string{
		"lolin32-lite": "robohub-esp32",
		"pico2w":       "robohub-pico", // a variant that names no library keeps the connection's
		"native":       "robohub-pico", // no variant at all
		"":             "robohub-pico",
	} {
		if got := connection.LibraryFor(preset); got != want {
			t.Errorf("LibraryFor(%q) = %q, want %q", preset, got, want)
		}
	}
	// A variant's include directories are still paths the workspace expands.
	if got := connection.Variants["lolin32-lite"].IncludeDirs; !reflect.DeepEqual(got, []string{ws + "/robohub-esp32/include"}) {
		t.Errorf("variant include_dirs = %v", got)
	}
	var none *CConnection
	if got := none.LibraryFor("pico2w"); got != "" {
		t.Errorf("a nil connection links %q", got)
	}
}

// A graph keeps every package; a build for one preset uses the ones reached
// through dependencies that belong to it, and what only another preset's
// dependency reaches is left out.
func TestGraphForPresetLeavesOutAnotherPresetsDependencies(t *testing.T) {
	manifest := &Manifest{Deps: []DepSpec{
		{Name: "pico", Presets: []string{"pico2w"}},
		{Name: "esp", Presets: []string{"lolin32-lite"}},
		{Name: "app-lib"},
	}}
	graph := &DependencyGraph{Nodes: map[string]*PackageNode{
		"pico":     {Name: "pico", Deps: []DepSpec{{Name: "common"}, {Name: "pico-sdk"}}},
		"esp":      {Name: "esp", Deps: []DepSpec{{Name: "common"}, {Name: "esp-idf"}}},
		"app-lib":  {Name: "app-lib"},
		"common":   {Name: "common"},
		"pico-sdk": {Name: "pico-sdk"},
		"esp-idf":  {Name: "esp-idf"},
	}}

	pico := graph.ForPreset(manifest, "pico2w")
	want := map[string]bool{"pico": true, "common": true, "pico-sdk": true, "app-lib": true}
	if !reflect.DeepEqual(pico, want) {
		t.Errorf("pico2w uses %v, want %v", pico, want)
	}
	esp := graph.ForPreset(manifest, "lolin32-lite")
	want = map[string]bool{"esp": true, "common": true, "esp-idf": true, "app-lib": true}
	if !reflect.DeepEqual(esp, want) {
		t.Errorf("lolin32-lite uses %v, want %v", esp, want)
	}
	if all := graph.ForPreset(manifest, ""); len(all) != len(graph.Nodes) {
		t.Errorf("no preset uses %d packages, want all %d", len(all), len(graph.Nodes))
	}
}
