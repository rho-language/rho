package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A new source file declares the name its module's import name and its place
// give it, and its header carries what the manifest says.
func TestANewSourceFileKnowsItsModule(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(text), 0o644)
	}
	write("alert/module.yml", "module:\n  name: robohub-alert\n  maintainer: \"Ada Lovelace\"\n  license: \"MIT\"\nprovides:\n  import: robohub.alert\n")
	write("mesh/module.yml", "module:\n  name: mesh\n  version: 0.1.0\n")
	write("mesh/tests/module.yml", "module:\n  name: mesh-tests\n")
	write("mesh/tests/tests.yml", "name: mesh\nsuites:\n  - name: tests/other_test.rho\n    kind: rho\n")

	cases := []struct{ path, module string }{
		{"alert/src/rule.rho", "robohub.alert.rule"},
		{"alert/src/main.rho", "robohub.alert"},
		{"mesh/src/mesh/fleet.rho", "mesh.fleet"},
		{"mesh/src/mesh/domain/impact.rho", "mesh.domain.impact"},
		{"mesh/tests/fleet_test.rho", "mesh.fleet_test"},
	}
	for _, c := range cases {
		result, err := NewSource(SourceRequest{Path: filepath.Join(root, c.path), Description: "What it is.", Date: "2026-11-12"})
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		if result.Module != c.module {
			t.Errorf("%s declares %q, want %q", c.path, result.Module, c.module)
		}
		text, _ := os.ReadFile(result.Path)
		if !strings.Contains(string(text), "\nmodule "+c.module+"\n") {
			t.Errorf("%s does not declare its module:\n%s", c.path, text)
		}
	}

	rule, _ := os.ReadFile(filepath.Join(root, "alert/src/rule.rho"))
	for _, part := range []string{"// @file rule.rho\n", "// @brief What it is.\n", "// @module robohub.alert.rule\n",
		"// @project robohub-alert\n", "// @author Ada Lovelace\n", "// @date 2026-11-12\n", "// SPDX-License-Identifier: MIT\n"} {
		if !strings.Contains(string(rule), part) {
			t.Errorf("the header lacks %q:\n%s", part, rule)
		}
	}
	fleet, _ := os.ReadFile(filepath.Join(root, "mesh/src/mesh/fleet.rho"))
	if !strings.Contains(string(fleet), "SPDX-License-Identifier: NOASSERTION") {
		t.Errorf("a module that states no licence is said not to:\n%s", fleet)
	}

	test, _ := os.ReadFile(filepath.Join(root, "mesh/tests/fleet_test.rho"))
	if !strings.Contains(string(test), "fn main() void {") || !strings.Contains(string(test), "test fn ") {
		t.Errorf("a test file does not start as a suite:\n%s", test)
	}
	suites, _ := os.ReadFile(filepath.Join(root, "mesh/tests/tests.yml"))
	if !strings.Contains(string(suites), "  - name: tests/fleet_test.rho\n") || !strings.Contains(string(suites), "other_test.rho") {
		t.Errorf("the test file was not listed beside the others:\n%s", suites)
	}

	if _, err := NewSource(SourceRequest{Path: filepath.Join(root, "alert/src/rule.rho")}); err == nil {
		t.Error("an existing file was replaced without --force")
	}
}
