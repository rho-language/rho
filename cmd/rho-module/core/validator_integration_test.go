package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A plugin that only provides an ecosystem's data (ports for the platforms it
// serves, no component) is held to the integration contract too: one that does
// not name its ecosystem is refused, one that does is valid.
func TestADataOnlyIntegrationPluginIsHeldToTheContract(t *testing.T) {
	write := func(dir, manifest string) string {
		os.MkdirAll(filepath.Join(dir, "ports"), 0o755)
		os.WriteFile(filepath.Join(dir, "ports", "store.go"), []byte("package main\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "ports", "go.mod"), []byte("module acme/store\n\ngo 1.21\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0o644)
		return dir
	}
	head := "module:\n  name: acme-store\n  id: acme.store\n  type: plugin\n  version: 0.1.0\n  description: A store.\nmain: ports/store.go\nserves:\n  - acme\n"
	bad := write(t.TempDir(), head+"integration:\n  provides:\n    ports: ports/\n")
	if err := ValidateModule(bad); err == nil || !strings.Contains(err.Error(), "ecosystem") {
		t.Fatalf("a data-only plugin without its ecosystem was accepted: %v", err)
	}
	good := write(t.TempDir(), head+"integration:\n  ecosystem: acme\n  provides:\n    ports: ports/\n")
	if err := ValidateModule(good); err != nil {
		t.Fatalf("a complete data-only plugin was refused: %v", err)
	}
}
