//
// @file component_test.go
// @brief The component's own declaration, checked against its code.
//
// A plugin is measured against what the component's manifest says it offers, so
// the manifest and the code have to say the same thing. They are written in
// two places on purpose: the manifest is what a plugin is validated against, and
// the code is what a build asks for. This test is the check that keeps them from
// drifting, which is why the list of hooks is not read from a file at run time.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/deps"
)

// manifest is the component's own manifest, two directories up from the package.
func manifest(t *testing.T) *deps.Manifest {
	t.Helper()
	path := filepath.Join("..", "..", "module.yml")
	loaded, err := deps.LoadManifest(path)
	if err != nil {
		t.Fatalf("cannot read the component manifest %s: %v", path, err)
	}
	return loaded
}

func TestTheManifestNamesThisComponent(t *testing.T) {
	// A plugin says which component it plugs into, and the only component that
	// loads it is the one whose name it says. If the two disagree, no plugin
	// ever runs and nothing reports it.
	if got := manifest(t).ModuleName; got != ComponentName {
		t.Fatalf("the manifest names %q but the code answers to %q", got, ComponentName)
	}
}

func TestTheManifestDeclaresTheHooksThisGeneratorRuns(t *testing.T) {
	// The manifest is what a plugin is validated against, so a hook the code
	// runs but the manifest does not offer would be one no plugin could answer.
	provides := manifest(t).Provides
	if provides == nil || len(provides.Hooks) == 0 {
		t.Fatal("the component manifest offers no hooks, so no plugin can attach to it")
	}
	if len(provides.Hooks) != len(OfferedHooks) {
		t.Fatalf("the manifest offers %v but the code runs %v", provides.Hooks, OfferedHooks)
	}
	for index, hook := range provides.Hooks {
		if hook != OfferedHooks[index] {
			t.Fatalf("the manifest offers %v but the code runs %v", provides.Hooks, OfferedHooks)
		}
	}
}

func TestTheComponentIsPublishable(t *testing.T) {
	// The generator is reached through the command the manifest publishes, and
	// through the interface a dependency imports. Both are declared here, and
	// both are what makes the component a component rather than a folder.
	loaded := manifest(t)
	if loaded.ModuleType != "component" {
		t.Fatalf("expected a component, got %q", loaded.ModuleType)
	}
	if loaded.Command == "" {
		t.Fatal("expected the component to publish a command")
	}
	if loaded.ImportPath != "rho.doc" {
		t.Fatalf("expected the library to be rho.doc, got %q", loaded.ImportPath)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "src", "doc.rho")); err != nil {
		t.Fatalf("the published interface is missing: %v", err)
	}
}
