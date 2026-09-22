package deps

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureOutput captures stdout during PrintTree
func captureOutput(f func()) string {
	var buf bytes.Buffer
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = stdout
	buf.ReadFrom(r)
	return buf.String()
}

func TestPrintTree(t *testing.T) {
	tmp := t.TempDir()

	// Create fake depA with depB inside
	createFakeDep := func(name, version string, deps []string) string {
		p := filepath.Join(tmp, name)
		if err := os.Mkdir(p, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := fmt.Sprintf("module %s\nversion %s\n\n", name, version)
		for _, d := range deps {
			content += fmt.Sprintf("dep %s {}\n", d)
		}

		if err := os.WriteFile(filepath.Join(p, "rho.mod"), []byte(content), 0644); err != nil {
			t.Fatalf("write rho.mod: %v", err)
		}

		return p
	}

	createFakeDep("depA", "1.0.0", []string{"depB"})
	createFakeDep("depB", "2.0.0", nil)

	manifest := &Manifest{
		ModuleName: "root",
		Version:    "0.1.0",
		Deps: []DepSpec{
			{
				Name:    "depA",
				Type:    DepTypeC,
				Version: VersionRange{Raw: ">=1.0.0 <2.0.0"},
				Path:    "depA",
			},
		},
	}

	loc := &DefaultLocator{
		ProjectRoot: tmp,
		GlobalCache: filepath.Join(tmp, "global"),
		LocalCache:  filepath.Join(tmp, "local"),
	}

	r := &Resolver{Locator: loc}

	graph, _, err := r.Resolve(manifest, tmp)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	out := captureOutput(func() {
		PrintTree(graph)
	})

	if !strings.Contains(out, "depA 1.0.0") {
		t.Fatalf("tree missing depA: %s", out)
	}
	if !strings.Contains(out, "-> depB") {
		t.Fatalf("tree missing depB: %s", out)
	}
}
