//
// @file preset_marker_test.go
// @brief Rho compiler - a generated project for another preset is not reused.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAGeneratedProjectForAnotherPresetIsEmptiedButItsBuildTreesKept(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}

	// Nothing generated yet: nothing is created.
	missing := filepath.Join(dir, "absent")
	if err := forgetOtherPreset(missing, "pico2w"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("a project that was never generated was created")
	}

	// Generated before a preset was recorded: kept, and the preset recorded.
	write("rho-config.cmake")
	if err := forgetOtherPreset(dir, "pico2w"); err != nil {
		t.Fatal(err)
	}
	if !exists("rho-config.cmake") {
		t.Fatal("a project with no recorded preset lost its files")
	}

	// The same preset again: kept.
	write("build/pico2w/CMakeCache.txt")
	if err := forgetOtherPreset(dir, "pico2w"); err != nil {
		t.Fatal(err)
	}
	if !exists("rho-config.cmake") {
		t.Fatal("a project regenerated for its own preset lost its files")
	}

	// Another preset: emptied, but the build trees stay.
	write("kconfig/robohub-pico-Kconfig")
	if err := forgetOtherPreset(dir, "lolin32-lite"); err != nil {
		t.Fatal(err)
	}
	if exists("rho-config.cmake") || exists("kconfig/robohub-pico-Kconfig") {
		t.Error("a project generated for pico2w kept its files for lolin32-lite")
	}
	if !exists("build/pico2w/CMakeCache.txt") {
		t.Error("the pico2w build tree was removed")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, presetMarker)); string(data) != "lolin32-lite\n" {
		t.Errorf("recorded preset = %q", data)
	}
}
