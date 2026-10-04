//
// @file platform_type_test.go
// @brief Tests for the native module types and what they require of a manifest.
//
// A `platform` module implements the interfaces an `interfaces` module declares,
// against one piece of hardware. It is native code a project links rather than
// something it runs, so like `interfaces` it declares no Rho entry point — and
// the publisher, the packer and the validator all have to agree on that, or a
// native library is refused for a file it has no reason to have.
//
// @project Rho Programming Language
// @license Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  A native module declares no main                                            */
/* -------------------------------------------------------------------------- */

// writeNativeModule writes a module of the given type with no main, which is
// what a native module looks like.
func writeNativeModule(t *testing.T, moduleType string) string {
	t.Helper()
	dir := t.TempDir()
	content := "module:\n  name: native-mod\n  type: " + moduleType + "\n  version: 1.0.0\n\nbuild:\n  language: c\n"
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Both native types load without a main. This is the case that used to be
// refused with "main missing", which made a perfectly good native library
// unpublishable unless it named a build file as its entry point.
func TestNativeModuleLoadsWithoutMain(t *testing.T) {
	for _, moduleType := range []string{"interfaces", "platform"} {
		dir := writeNativeModule(t, moduleType)
		mod, err := LoadModuleYAML(filepath.Join(dir, "module.yml"))
		if err != nil {
			t.Errorf("a %s module with no main was refused: %v", moduleType, err)
			continue
		}
		if mod.Main != "" {
			t.Errorf("%s module reported a main of %q, want none", moduleType, mod.Main)
		}
	}
}

// A Rho library still needs its entry point, and losing the native exemption
// must not have taken the requirement with it.
func TestRhoLibraryStillRequiresMain(t *testing.T) {
	for _, moduleType := range []string{"rho", "module"} {
		dir := t.TempDir()
		content := "module:\n  name: rho-mod\n  type: " + moduleType + "\n  version: 1.0.0\n"
		path := filepath.Join(dir, "module.yml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadModuleYAML(path)
		if err == nil {
			t.Errorf("a %s module with no main was accepted", moduleType)
			continue
		}
		if !strings.Contains(err.Error(), "main") {
			t.Errorf("a %s module with no main was refused for the wrong reason: %v", moduleType, err)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Validating a native module                                                 */
/* -------------------------------------------------------------------------- */

// Validating a native module does not look for a main file that is not there.
// A component's and a plugin's main is a binary built while packing, so they
// are exempt for a different reason; a native module is exempt because it has no
// main at all, and a check that only remembered the first case refused the
// second.
func TestValidateAcceptsNativeModuleWithoutMainFile(t *testing.T) {
	for _, moduleType := range []string{"interfaces", "platform"} {
		dir := writeNativeModule(t, moduleType)
		if err := ValidateModule(dir); err != nil {
			t.Errorf("a %s module was rejected: %v", moduleType, err)
		}
	}
}

// A Rho library whose main is not on disk is still a broken module.
func TestValidateRejectsRhoLibraryWithMissingMain(t *testing.T) {
	dir := t.TempDir()
	content := "module:\n  name: rho-mod\n  type: module\n  version: 1.0.0\n\nmain: src/lib.rho\n"
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateModule(dir); err == nil {
		t.Fatal("a module whose main is not on disk was accepted")
	}
}
