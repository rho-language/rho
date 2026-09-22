package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func captureGraphOutput(t *testing.T, f func()) string {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = stdout

	out, _ := io.ReadAll(r)
	return string(out)
}

func createFakeDep(t *testing.T, root, name, version string, children ...string) {
	p := filepath.Join(root, name)
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := "module " + name + "\nversion " + version + "\n\n"
	for _, c := range children {
		content += "dep " + c + " {}\n"
	}

	if err := os.WriteFile(filepath.Join(p, "rho.mod"), []byte(content), 0644); err != nil {
		t.Fatalf("write rho.mod: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests                                                                      */
/* -------------------------------------------------------------------------- */

func TestDepsGraphJSON(t *testing.T) {
	tmp := t.TempDir()

	// depB
	createFakeDep(t, tmp, "depB", "2.0.0")

	// depA → depB
	createFakeDep(t, tmp, "depA", "1.0.0", []string{"depB"}...)

	// root rho.mod
	rootMod := `
module root
version 0.1.0

dep depA {
    type = "c"
    version = ">=1.0.0 <2.0.0"
    path = "depA"
}
`
	os.WriteFile(filepath.Join(tmp, "rho.mod"), []byte(rootMod), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureGraphOutput(t, func() {
		runDepsGraph([]string{"--json"})
	})

	var parsed struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(parsed.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(parsed.Nodes))
	}
}
