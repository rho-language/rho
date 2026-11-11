package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeepSourcesShipsGoAsDataButNotTests(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"module.yml":      "x",
		"src/main.go":     "package main",
		"ports/a.go":      "package main",
		"ports/a_test.go": "package main",
		"ports/go.mod":    "module x",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	layout := PackageLayout{Include: []string{"module.yml", "src", "ports"}, KeepSources: []string{"ports"}}
	files, err := CollectLayoutFiles(dir, layout)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		got = append(got, filepath.ToSlash(rel))
	}
	joined := strings.Join(got, ",")
	for _, want := range []string{"ports/a.go", "ports/go.mod", "module.yml"} {
		if !strings.Contains(joined, want) {
			t.Errorf("package is missing %s: %s", want, joined)
		}
	}
	for _, unwanted := range []string{"src/main.go", "a_test.go"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("package carries %s: %s", unwanted, joined)
		}
	}
}
