//
// @file preset_deps_test.go
// @brief rho-target - a dependency declared for other presets is not published.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExportingModule writes a C module that exports a CMake target.
func writeExportingModule(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "include"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "module:\n  name: " + name + "\n  type: platform\n  version: 0.1.0\n" +
		"exports:\n  cmake:\n    target: rho::" + name + "\n    include_dirs: [include]\n"
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsFilesLeaveOutAnotherPresetsDependency(t *testing.T) {
	root := t.TempDir()
	writeExportingModule(t, filepath.Join(root, "pico"), "pico")
	writeExportingModule(t, filepath.Join(root, "esp"), "esp")
	writeExportingModule(t, filepath.Join(root, "common"), "common")
	app := "module:\n  name: app\n  type: module\n  version: 0.1.0\nmain: main.c\n" +
		"build:\n  language: c\n  presets: [pico2w, lolin32-lite]\n" +
		"depends:\n" +
		"  - name: pico\n    type: c\n    path: pico\n    presets: [pico2w]\n" +
		"  - name: esp\n    type: c\n    path: esp\n    presets: [lolin32-lite]\n" +
		"  - name: common\n    type: c\n    path: common\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}

	for preset, names := range map[string][2]string{
		"pico2w":       {"pico", "esp"},
		"lolin32-lite": {"esp", "pico"},
	} {
		out := filepath.Join(t.TempDir(), "deps.cmake")
		if err := writeDepsFiles(root, out, preset); err != nil {
			t.Fatal(err)
		}
		// deps.cmake names each dependency it publishes in a heading; the alias
		// file bridges each one's exported target.
		files := map[string]func(string) string{
			out: func(name string) string { return "# " + name + " 0.1.0\n" },
			strings.TrimSuffix(out, ".cmake") + "-aliases.cmake": func(name string) string { return "rho::" + name + " " },
		}
		for file, mark := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if !strings.Contains(text, mark(names[0])) || !strings.Contains(text, mark("common")) {
				t.Errorf("%s: %s lacks %s or common:\n%s", preset, filepath.Base(file), names[0], text)
			}
			if strings.Contains(text, mark(names[1])) {
				t.Errorf("%s: %s has %s:\n%s", preset, filepath.Base(file), names[1], text)
			}
		}
	}

	// No preset publishes every dependency, as a manifest without the keys does.
	if got := len(resolvedExports(root, "")); got != 3 {
		t.Errorf("with no preset, %d dependencies are published, want 3", got)
	}
}
