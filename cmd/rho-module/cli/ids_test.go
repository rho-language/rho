package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, id, extra string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "module:\n  name: m\n  id: " + id + "\n" + extra
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIdsPassesADistinctTree(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "a"), "acme.one", "")
	writeManifest(t, filepath.Join(root, "b"), "acme.two", "  previous_ids:\n    - acme.old\n")
	if err := Ids([]string{root}); err != nil {
		t.Fatal(err)
	}
}

func TestIdsFailsOnADuplicateAndSkipsBuildOutput(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "a"), "acme.one", "")
	writeManifest(t, filepath.Join(root, "b"), "acme.one", "")
	if err := Ids([]string{root}); err == nil {
		t.Fatal("a duplicate id passed")
	}
	other := t.TempDir()
	writeManifest(t, filepath.Join(other, "a"), "acme.one", "")
	writeManifest(t, filepath.Join(other, "build", "copy"), "acme.one", "")
	if err := Ids([]string{other}); err != nil {
		t.Fatalf("a copy under build/ counted: %v", err)
	}
}

func TestIdsFailsOnAMalformedID(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "a"), "robohub", "")
	if err := Ids([]string{root}); err == nil {
		t.Fatal("a one-segment id passed")
	}
}
