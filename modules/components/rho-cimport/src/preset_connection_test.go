//
// @file preset_connection_test.go
// @brief rho-cimport - the library and headers of a build for one preset.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writePlatform(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "module:\n  name: " + name + "\n  type: platform\n  version: 0.1.0\n" +
		"build:\n  language: c\n  include_dirs: [include]\n"
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAConnectionForAPresetUsesItsVariantsLibraryAndItsOwnPlatform(t *testing.T) {
	root := t.TempDir()
	writePlatform(t, filepath.Join(root, "pico"), "pico")
	writePlatform(t, filepath.Join(root, "esp"), "esp")
	app := "module:\n  name: app\n  type: module\n  version: 0.1.0\nmain: main.c\n" +
		"build:\n  language: c\n  presets: [pico2w, lolin32-lite]\n" +
		"depends:\n" +
		"  - name: pico\n    type: c\n    path: pico\n    presets: [pico2w]\n" +
		"  - name: esp\n    type: c\n    path: esp\n    presets: [lolin32-lite]\n" +
		"c_connection:\n  library: pico\n  headers: [platform.h]\n" +
		"  variants:\n    lolin32-lite:\n      library: esp\n      include_dirs: [extra]\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		library  string
		includes []string
	}{
		"pico2w":       {"pico", []string{filepath.Join(root, "pico", "include")}},
		"lolin32-lite": {"esp", []string{filepath.Join(root, "esp", "include"), "extra"}},
		"":             {"pico", []string{filepath.Join(root, "pico", "include"), filepath.Join(root, "esp", "include")}},
	}
	for preset, want := range cases {
		connection, err := readConnection(root, "", preset)
		if err != nil {
			t.Fatal(err)
		}
		if connection.Library != want.library {
			t.Errorf("%q: library = %q, want %q", preset, connection.Library, want.library)
		}
		if !reflect.DeepEqual(connection.IncludeDirs, want.includes) {
			t.Errorf("%q: include dirs = %v, want %v", preset, connection.IncludeDirs, want.includes)
		}
	}

	// --library still overrides whatever the manifest says.
	if connection, _ := readConnection(root, "other", "lolin32-lite"); connection.Library != "other" {
		t.Errorf("--library is overridden: %q", connection.Library)
	}
}
