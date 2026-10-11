//
// @file scaffold_test.go
// @brief Tests for starting a project: the Rho application, and the generators
// a module declares.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/deps"
)

func TestApplicationWritesAProjectThatReadsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-app")
	result, err := Application(Options{Dir: dir, Description: "A test app."})
	if err != nil {
		t.Fatalf("Application: %v", err)
	}
	if result.Name != "my_app" {
		t.Errorf("name = %q, want my_app (the directory, made a name)", result.Name)
	}
	manifest, err := deps.LoadManifest(filepath.Join(dir, "module.yml"))
	if err != nil {
		t.Fatalf("the generated module.yml does not read back: %v", err)
	}
	if manifest.ModuleName != "my_app" || manifest.Main != "src/main.rho" {
		t.Errorf("manifest = %+v", manifest)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "main.rho")); err != nil {
		t.Errorf("src/main.rho missing: %v", err)
	}

	// A directory that already holds a manifest is refused rather than written
	// over: an init that overwrote a project would be the one command that
	// destroys work.
	if _, err := Application(Options{Dir: dir}); err == nil {
		t.Errorf("a second Application into the same directory should be refused")
	}
}

func TestGeneratorsReadInlineAndFromAFile(t *testing.T) {
	inline := t.TempDir()
	write(t, filepath.Join(inline, "module.yml"), `module:
  name: inline
  type: rho
  version: 0.1.0

main: src/main.rho

generate:
  - name: driver
    description: A driver
    variables:
      - name: category
        description: its category
    outputs:
      - path: drivers/{{category}}/{{name}}
        template: templates/driver
`)
	generators, err := Generators(inline)
	if err != nil {
		t.Fatalf("Generators(inline): %v", err)
	}
	if len(generators) != 1 || generators[0].Name != "driver" {
		t.Fatalf("generators = %+v", generators)
	}
	if len(generators[0].Variables) != 1 || generators[0].Variables[0].Name != "category" {
		t.Errorf("variables = %+v", generators[0].Variables)
	}
	if len(generators[0].Outputs) != 1 || generators[0].Outputs[0].Path != "drivers/{{category}}/{{name}}" {
		t.Errorf("outputs = %+v", generators[0].Outputs)
	}

	// The same definitions from a file named by the manifest.
	referenced := t.TempDir()
	write(t, filepath.Join(referenced, "module.yml"), `module:
  name: referenced
  type: rho
  version: 0.1.0

main: src/main.rho

generate: generate.yml
`)
	write(t, filepath.Join(referenced, "generate.yml"), `generators:
  - name: example
    output: yes
    outputs:
      - path: examples/{{name}}
        template: t
`)
	generators, err = Generators(referenced)
	if err != nil {
		t.Fatalf("Generators(referenced): %v", err)
	}
	if len(generators) != 1 || generators[0].Name != "example" {
		t.Fatalf("generators from file = %+v", generators)
	}
}

func TestRenderWritesEveryOutputWithSubstitutions(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "module.yml"), `module:
  name: gen
  type: rho
  version: 0.1.0

main: src/main.rho

generate:
  - name: driver
    description: A driver and its example
    variables:
      - name: category
        description: its category
      - name: name
        description: its name
    outputs:
      - path: drivers/{{category}}/{{name}}
        template: templates/driver
      - path: examples/{{category}}/{{name}}
        template: templates/example
`)
	write(t, filepath.Join(root, "templates", "driver", "{{name}}.c"), "// {{name}} in {{category}}\n")
	write(t, filepath.Join(root, "templates", "example", "main.c"), "// uses {{name}}\n")

	generator, ok, err := Find(root, "driver")
	if err != nil || !ok {
		t.Fatalf("Find: ok=%v err=%v", ok, err)
	}

	dest := t.TempDir()
	results, err := Render(root, generator, dest, map[string]string{"category": "temperature", "name": "ds18b20"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want two outputs, got %d", len(results))
	}

	driver := filepath.Join(dest, "drivers", "temperature", "ds18b20", "ds18b20.c")
	content, err := os.ReadFile(driver)
	if err != nil {
		t.Fatalf("the driver output was not named from the variables: %v", err)
	}
	if string(content) != "// ds18b20 in temperature\n" {
		t.Errorf("driver content = %q", content)
	}

	example := filepath.Join(dest, "examples", "temperature", "ds18b20", "main.c")
	if got, err := os.ReadFile(example); err != nil || string(got) != "// uses ds18b20\n" {
		t.Errorf("example = %q err=%v", got, err)
	}

	// A name that is not a variable is left as written, so a typo is visible.
	if !strings.Contains(substitute("{{nope}}", map[string]string{"name": "x"}), "nope") {
		t.Errorf("an unknown placeholder should be left in place")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
