//
// @file c_dependency_roots_test.go
// @brief Rho project generator - C dependency root collection tests.
//
// A dependency reached through two parents is one root to add. The two parents
// are a common shape once a platform and a driver both depend on the same
// module, and adding its directory twice is two CMake targets of one name.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package projectgen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"rho/src/deps"
)

// writeModule writes a manifest with the given dependencies under dir and a
// CMakeLists.txt beside it, which is what marks the directory as a C root.
func writeModule(t *testing.T, dir string, name string, depends []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write CMakeLists.txt: %v", err)
	}

	body := "module:\n  name: " + name + "\n  type: platform\n  version: 0.1.0\n"
	if len(depends) > 0 {
		body += "depends:\n"
		for _, path := range depends {
			body += "  - name: " + filepath.Base(path) + "\n    type: c\n    path: " + path + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// A diamond — the application depends on two modules that both depend on a
// third — must add the shared root once. Two `add_subdirectory` calls for one
// source tree are two targets of one name, and the configure fails.
func TestCDependencyRootsDeduplicatesADiamond(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "app", []string{"a", "b"})
	writeModule(t, filepath.Join(root, "a"), "a", []string{"../common"})
	writeModule(t, filepath.Join(root, "b"), "b", []string{"../common"})
	writeModule(t, filepath.Join(root, "common"), "common", nil)

	roots := cDependencyRoots(root, "")

	common := filepath.Join(root, "common")
	count := 0
	for _, r := range roots {
		if r == common {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the shared dependency appears %d times, want 1; roots=%v", count, roots)
	}
	if len(roots) != 3 {
		t.Fatalf("got %d roots, want 3 (a, common, b); roots=%v", len(roots), roots)
	}
}

// writePresetApp writes an application with one dependency per preset and one
// shared by both, each of which depends on a common module.
func writePresetApp(t *testing.T, root string, presetKeys bool) {
	t.Helper()
	writeModule(t, filepath.Join(root, "pico"), "pico", []string{"../common"})
	writeModule(t, filepath.Join(root, "esp"), "esp", []string{"../esp-only"})
	writeModule(t, filepath.Join(root, "esp-only"), "esp-only", nil)
	writeModule(t, filepath.Join(root, "common"), "common", nil)
	writeModule(t, filepath.Join(root, "shared"), "shared", nil)
	body := "module:\n  name: app\n  type: module\n  version: 0.1.0\n" + "main: main.c\n" +
		"build:\n  language: c\n  presets: [pico2w, lolin32-lite]\n" +
		"depends:\n" +
		"  - name: pico\n    type: c\n    path: pico\n"
	if presetKeys {
		body += "    presets: [pico2w]\n"
	}
	body += "  - name: esp\n    type: c\n    path: esp\n"
	if presetKeys {
		body += "    presets: [lolin32-lite]\n"
	}
	body += "  - name: shared\n    type: c\n    path: shared\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rootNames(roots []string) []string {
	names := []string{}
	for _, root := range roots {
		names = append(names, filepath.Base(root))
	}
	return names
}

// A dependency declared for another preset is not added, and nor is what only
// it reaches; the one declared for this preset, and the ones declared for every
// preset, are.
func TestCDependencyRootsLeaveOutAnotherPresetsDependency(t *testing.T) {
	root := t.TempDir()
	writePresetApp(t, root, true)

	if got, want := rootNames(cDependencyRoots(root, "pico2w")), []string{"pico", "common", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pico2w roots = %v, want %v", got, want)
	}
	if got, want := rootNames(cDependencyRoots(root, "lolin32-lite")), []string{"esp", "esp-only", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lolin32-lite roots = %v, want %v", got, want)
	}
	if got, want := rootNames(cDependencyRoots(root, "")), []string{"pico", "common", "esp", "esp-only", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("roots with no preset = %v, want %v", got, want)
	}
}

// A manifest without the keys is built as it always was, for any preset.
func TestCDependencyRootsWithoutPresetKeysAreUnchanged(t *testing.T) {
	root := t.TempDir()
	writePresetApp(t, root, false)
	all := rootNames(cDependencyRoots(root, ""))
	for _, preset := range []string{"pico2w", "lolin32-lite", "native"} {
		if got := rootNames(cDependencyRoots(root, preset)); !reflect.DeepEqual(got, all) {
			t.Errorf("%s roots = %v, want %v", preset, got, all)
		}
	}
}

// The generated CMakeLists names, adds and links only the preset's own
// platform: no `set(ESP_DIR ...)` and no `add_subdirectory` of it in a pico2w
// project, and the variant's library replaces the connection's.
func TestGeneratedCMakeForAPresetLinksItsOwnPlatform(t *testing.T) {
	root := t.TempDir()
	writePresetApp(t, root, true)
	connection := &deps.CConnection{
		Library: "pico",
		Variants: map[string]deps.CVariant{
			"lolin32-lite": {Library: "esp"},
			"pico2w":       {},
		},
	}
	generate := func(preset string) string {
		ctx := Context{ModuleName: "app", ProjectRoot: root, Connection: connection, Variant: preset, Preset: preset}
		return emitCMakeLists(ctx)
	}

	pico := generate("pico2w")
	for _, want := range []string{"set(PICO_DIR", "set(COMMON_DIR", "set(SHARED_DIR", "    pico\n"} {
		if !strings.Contains(pico, want) {
			t.Errorf("the pico2w project lacks %q:\n%s", want, pico)
		}
	}
	for _, unwanted := range []string{"ESP_DIR", "ESP_ONLY_DIR", "/esp\"", "    esp\n"} {
		if strings.Contains(pico, unwanted) {
			t.Errorf("the pico2w project has %q:\n%s", unwanted, pico)
		}
	}

	esp := generate("lolin32-lite")
	for _, want := range []string{"set(ESP_DIR", "set(ESP_ONLY_DIR", "set(SHARED_DIR", "    esp\n"} {
		if !strings.Contains(esp, want) {
			t.Errorf("the lolin32-lite project lacks %q:\n%s", want, esp)
		}
	}
	for _, unwanted := range []string{"PICO_DIR", "COMMON_DIR", "    pico\n"} {
		if strings.Contains(esp, unwanted) {
			t.Errorf("the lolin32-lite project has %q:\n%s", unwanted, esp)
		}
	}
}

// A program a framework builds as a component hands the platform's file the
// libraries its variant links, without the platform library, in declared order;
// and an empty list when there are none.
func TestProgramComponentListsTheVariantsLibraries(t *testing.T) {
	root := t.TempDir()
	writePresetApp(t, root, true)
	connection := &deps.CConnection{
		Library: "robohub-pico",
		Variants: map[string]deps.CVariant{
			"lolin32-lite": {Library: "robohub-esp32", Libraries: []string{"robohub-core", "robohub-esp32", "robohub-transport"}},
			"pico2w":       {},
		},
	}
	component := func(preset string) string {
		return emitProgramComponent(Context{ModuleName: "app", ProjectRoot: root, Connection: connection, Variant: preset, Preset: preset})
	}
	esp := component("lolin32-lite")
	want := "set(RHO_PROGRAM_LIBRARIES\n    robohub-core\n    robohub-transport\n)\n\ninclude(${CMAKE_CURRENT_LIST_DIR}/component.cmake)\n"
	if !strings.HasSuffix(esp, want) {
		t.Errorf("the lolin32-lite component does not end with\n%s\ngot:\n%s", want, esp)
	}
	if pico := component("pico2w"); !strings.Contains(pico, "set(RHO_PROGRAM_LIBRARIES\n)\n") {
		t.Errorf("a variant with no libraries does not set an empty list:\n%s", pico)
	}
	if none := emitProgramComponent(Context{ModuleName: "app", ProjectRoot: root}); !strings.Contains(none, "set(RHO_PROGRAM_LIBRARIES\n)\n") {
		t.Errorf("a project with no connection does not set an empty list:\n%s", none)
	}
}

// A program built as a framework's component resolves its configuration when
// its file is read, and lists the header's directory after the merge; one with
// no bridge says nothing about configuration.
func TestProgramComponentResolvesItsConfigurationAtConfigure(t *testing.T) {
	root := t.TempDir()
	writePresetApp(t, root, true)
	out := filepath.Join(root, "build", "generated", "c")
	ctx := Context{ModuleName: "app", ProjectRoot: root, OutputDir: out, Config: &Config{
		Dialect:   "linux",
		Fragments: []string{"kconfig/Kconfig"},
		Defconfig: "/project/project.conf",
		OutputDir: out,
	}}
	text := emitProgramComponent(ctx)

	merge := strings.Index(text, "execute_process(")
	includes := strings.Index(text, "    ${RHO_CONFIG_OUT}/include/generated\n")
	component := strings.Index(text, "include(${CMAKE_CURRENT_LIST_DIR}/component.cmake)")
	if merge < 0 || includes < 0 || component < 0 || !(merge < includes && includes < component) {
		t.Fatalf("want the merge, then the header directory, then the platform's file:\n%s", text)
	}
	for _, want := range []string{
		"set(RHO_CONFIG_OUT \"" + filepath.ToSlash(out) + "\")",
		"if(NOT CMAKE_SCRIPT_MODE_FILE AND NOT CMAKE_BUILD_EARLY_EXPANSION)",
		"kconfig merge --quiet --force",
		"--dialect \"linux\"",
		"\"--defconfig=/project/project.conf\"",
		"\"" + filepath.ToSlash(filepath.Join(out, "kconfig", "Kconfig")) + "\"",
		"CMAKE_CONFIGURE_DEPENDS",
		"message(FATAL_ERROR",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the component file lacks %q:\n%s", want, text)
		}
	}

	ctx.Config = nil
	if plain := emitProgramComponent(ctx); strings.Contains(plain, "RHO_CONFIG") {
		t.Errorf("a program with no bridge resolves a configuration:\n%s", plain)
	}
}
