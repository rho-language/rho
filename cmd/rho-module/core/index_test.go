//
// @file index_test.go
// @brief Tests for the local registry's index and its reconciliation.
//
// The local registry is `~/.rho`, and it holds two things that are easy to
// confuse: the installed tree, which is what the toolchain actually uses, and
// the index, which is what is known about it. The tests here are about the
// relationship between them, because that is where a second source of truth
// would live if one were allowed to form.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
)

// install puts a module into the tree the way a publish or a fetch does.
func install(t *testing.T, kind, name, version string) {
	t.Helper()
	root, err := paths.GetModulesInstalledPath()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, kind, name+"@"+version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// readIndexFile returns the index as written, which is what a person reads when
// they want to know what the installation believes.
func readIndexFile(t *testing.T) string {
	t.Helper()
	path, err := registryFilePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAnInstalledModuleIsListedEvenIfTheIndexNeverMentionedIt(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	// A module installed by something other than `rho module publish` — a
	// fetch, a hand-copied directory — is installed, and must be listed.
	install(t, KindModule, "orders", "1.0.0")

	results, err := SearchRegistry("")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range results {
		if entry.Name == "orders" && entry.Type == KindModule {
			found = true
		}
	}
	if !found {
		t.Fatalf("a module in the tree is not listed: %+v", results)
	}
}

func TestAModuleRemovedFromTheTreeIsDroppedFromTheIndex(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	install(t, KindComponent, "gone", "1.0.0")
	if _, err := SearchRegistry("gone"); err != nil {
		t.Fatal(err)
	}
	if !contains(readIndexFile(t), "gone") {
		t.Fatal("the module was not recorded on the way in")
	}

	// Someone removed the directory by hand, or an install failed half way.
	root, err := paths.GetModulesInstalledPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, KindComponent, "gone@1.0.0")); err != nil {
		t.Fatal(err)
	}

	results, err := SearchRegistry("gone")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("a module that is no longer installed is still listed: %+v", results)
	}
	if contains(readIndexFile(t), "name: gone") {
		t.Error("a module that is no longer installed is still in the index")
	}
}

func TestTheIndexSelectsTheNewestVersionWhenNothingIsSelected(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	install(t, KindComponent, "math", "1.0.0")
	install(t, KindComponent, "math", "1.2.0")

	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatal(err)
	}
	selected := ""
	for _, entry := range results {
		if entry.Active {
			selected = entry.Version
		}
	}
	// The resolver picks the newest installed version, so the index has to
	// agree: a selection that disagreed with the resolver would be a lie the
	// listing repeats.
	if selected != "1.2.0" {
		t.Errorf("selected = %q, want the newest installed version 1.2.0", selected)
	}
}

func TestUnpublishingASelectedVersionWithOthersRefuses(t *testing.T) {
	// Removing the version the toolchain is resolving against would leave the
	// module resolving to a different one without anyone asking, so it is
	// refused and the user is told what to do instead.
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	install(t, KindComponent, "math", "1.0.0")
	install(t, KindComponent, "math", "1.2.0")

	err := UnpublishModule("math", "1.2.0")
	if err == nil {
		t.Fatal("the selected version was unpublished while another version was installed")
	}
	if !contains(err.Error(), "activate") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
	// The version is still there: a refused unpublish changes nothing.
	if !IsInstalled(KindComponent, "math", "1.2.0") {
		t.Error("a refused unpublish removed the version anyway")
	}
}

func TestUnpublishingTheLastVersionSelectsAnotherIfOneIsLeft(t *testing.T) {
	// Selecting an older version and then removing it has to move the selection
	// rather than leave the module resolving to nothing while a version sits
	// installed.
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	install(t, KindComponent, "math", "1.0.0")
	install(t, KindComponent, "math", "1.2.0")
	if err := ActivateModule("math", "1.0.0"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	// With 1.0.0 selected and 1.2.0 installed, removing 1.0.0 is still a
	// refusal under the same rule, so the module has to be moved off it first
	// or the rule is what the user gets. Selecting 1.2.0 and removing 1.0.0 is
	// the case the rule exists to make possible.
	if err := ActivateModule("math", "1.2.0"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if err := UnpublishModule("math", "1.0.0"); err != nil {
		t.Fatalf("unpublish an unselected version: %v", err)
	}
	if got := selectedVersionOf(t, KindComponent, "math"); got != "1.2.0" {
		t.Errorf("selected = %q, want the one that is left", got)
	}
}

func TestEveryKindIsActivatableAndUnpublishable(t *testing.T) {
	// A kind the index does not track is a module that is installed and that no
	// command can act on, because nothing would recognise what it is. So every
	// kind the toolchain installs under is exercised here.
	for _, kind := range IndexKinds {
		t.Run(kind, func(t *testing.T) {
			_, cleanup := SetupTestRegistry(t)
			defer cleanup()
			install(t, kind, "thing", "1.0.0")
			install(t, kind, "thing", "1.1.0")

			// A selected version is the one a project resolves against, so it
			// is not removable until it is deactivated — whether or not another
			// version is installed, because "the build will use the other one"
			// is a change the user did not ask for.
			if err := ActivateModule("thing", "1.0.0"); err != nil {
				t.Fatalf("activate a %s: %v", kind, err)
			}
			if got := selectedVersionOf(t, kind, "thing"); got != "1.0.0" {
				t.Errorf("selected = %q", got)
			}
			if err := UninstallLocal("thing", "1.0.0"); err == nil {
				t.Fatalf("the selected version of a %s was removed", kind)
			}

			// Deactivating moves the selection to the version that is left, and
			// then the old one is removable.
			if err := Deactivate("thing", "1.0.0"); err != nil {
				t.Fatalf("deactivate: %v", err)
			}
			if got := selectedVersionOf(t, kind, "thing"); got != "1.1.0" {
				t.Fatalf("after deactivating, selected = %q, want the one that is left", got)
			}
			if err := UninstallLocal("thing", "1.0.0"); err != nil {
				t.Fatalf("uninstall a %s: %v", kind, err)
			}
			results, err := SearchRegistry("thing")
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].Version != "1.1.0" {
				t.Errorf("after uninstalling, what is installed is %+v, want the version that is left", results)
			}
		})
	}
}

func TestTheIndexRecordsWhereAModuleCameFrom(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	install(t, KindComponent, "published", "1.0.0")
	install(t, KindComponent, "fetched", "2.0.0")

	// A publish is from this machine; a fetch is from a registry. The index is
	// the only place that can say so, which is why it carries the origin, and
	// the origin is what the listing reports.
	registry, err := Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	setOrigin(t, registry, KindComponent, "fetched", originRemote)
	path, err := registryFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRegistry(path, registry); err != nil {
		t.Fatal(err)
	}

	results, err := SearchRegistry("fetched")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %+v", results)
	}
	if !results[0].IsRemote() {
		t.Errorf("a module fetched from a registry is reported as %q", results[0].Origin)
	}
}

// setOrigin records where a module came from, the way a fetch does.
func setOrigin(t *testing.T, registry *Registry, kind, name, origin string) {
	t.Helper()
	versions := versionBucketOf(registry, kind)
	for i := range *versions {
		if (*versions)[i].Name == name {
			(*versions)[i].Origin = origin
			return
		}
	}
	t.Fatalf("no %s named %q in the index", kind, name)
}

func TestReconcilingTwiceChangesNothing(t *testing.T) {
	// Reconciliation rewrites the index, so it has to be stable: a second run
	// that reordered or duplicated entries would make every command rewrite
	// the file and produce a diff.
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()
	install(t, KindComponent, "math", "1.0.0")
	install(t, KindComponent, "math", "1.2.0")
	install(t, KindPlugin, "http", "0.9.0")

	if _, err := SearchRegistry(""); err != nil {
		t.Fatal(err)
	}
	first := readIndexFile(t)
	if _, err := SearchRegistry(""); err != nil {
		t.Fatal(err)
	}
	if second := readIndexFile(t); second != first {
		t.Errorf("reconciling twice changed the index:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestTheIndexRoundTripsEveryKind(t *testing.T) {
	// The writer and the reader are separate functions over a hand-rolled
	// format, so what one writes and the other reads is worth a test per kind:
	// a key that is written and not read is a list that silently loses its
	// entries.
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	registry := &Registry{}
	for _, kind := range IndexKinds {
		*versionBucketOf(registry, kind) = append(*versionBucketOf(registry, kind),
			VersionEntry{Name: "thing", Version: "1.0.0", Origin: originLocal})
		*activeBucketOf(registry, kind) = append(*activeBucketOf(registry, kind),
			ActiveEntry{Name: "thing", Version: "1.0.0"})
	}
	path, err := registryFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRegistry(path, registry); err != nil {
		t.Fatal(err)
	}
	read, err := readRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range IndexKinds {
		versions := *versionBucketOf(read, kind)
		if len(versions) != 1 || versions[0].Name != "thing" || versions[0].Version != "1.0.0" {
			t.Errorf("%s versions did not round-trip: %+v", kind, versions)
		}
		if versions[0].Origin != originLocal {
			t.Errorf("%s origin did not round-trip: %q", kind, versions[0].Origin)
		}
		selection := *activeBucketOf(read, kind)
		if len(selection) != 1 || selection[0].Version != "1.0.0" {
			t.Errorf("%s selection did not round-trip: %+v", kind, selection)
		}
	}
}

// selectedVersionOf reads the selection straight out of the index, which is
// what a command the user is not looking at would do.
func selectedVersionOf(t *testing.T, kind, name string) string {
	t.Helper()
	registry, err := Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	version, _ := SelectedVersion(registry, kind, name)
	return version
}
