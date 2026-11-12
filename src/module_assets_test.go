package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A module ships its own assets and locales: a program that includes it carries
// them embedded and reads the assets by name, or as files when a reader wants a
// path, with nothing read from the
// module's folder at run time and no path from the program into the module.
func TestModuleAssetsTravelWithTheProgram(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("modules/greeter/module.yml", `module:
  name: greeter
  type: module
  version: 0.1.0
  description: greets

main: src/main.rho

assets:
  - templates
  - db/schema.sql

locales:
  - locale: en
    file: locales/en.json
`)
	write("modules/greeter/src/main.rho", `module greeter

extern fn rho_asset(module: string, path: string) string
extern fn rho_asset_dir(module: string) string
extern fn rho_read_text(path: string) string

fn greeting() string {
    return rho_asset("greeter", "templates/hello.txt") + "|" + rho_asset("greeter", "db/schema.sql") +
        "|" + rho_read_text(rho_asset_dir("greeter") + "/templates/hello.txt")
}
`)
	write("modules/greeter/templates/hello.txt", "hello from the module")
	write("modules/greeter/db/schema.sql", "CREATE TABLE t (x INTEGER);")
	write("modules/greeter/locales/en.json", `{"greeter.title": "Greeter"}`)
	write("app/module.yml", `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

deps:
  greeter: ">=0.1.0"

build:
  language: go
  targets:
    - go
`)
	write("app/src/main.rho", `module app

include "greeter"

fn main() void {
    print(greeting())
}
`)
	entry := filepath.Join(root, "app", "src", "main.rho")
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets: %v", err)
	}
	goDir := dirs[0]
	generated, _ := os.ReadFile(filepath.Join(goDir, "rho_assets.go"))
	if !strings.Contains(string(generated), `"greeter/templates/hello.txt"`) {
		t.Fatalf("the module's asset was not embedded:\n%s", generated)
	}
	var catalog string
	entries, _ := os.ReadDir(goDir)
	for _, entry := range entries {
		data, _ := os.ReadFile(filepath.Join(goDir, entry.Name()))
		if strings.Contains(string(data), "greeter.title") {
			catalog = entry.Name()
		}
	}
	if catalog == "" {
		t.Error("the module's locale was not merged into the program's catalogs")
	}
	binary := filepath.Join(t.TempDir(), "app")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go build: %v\n%s", err, out)
	}
	// Run from an empty directory: nothing on disk is there to be read.
	run := exec.Command(binary)
	run.Dir = t.TempDir()
	run.Env = append(os.Environ(), "XDG_CACHE_HOME="+t.TempDir(), "HOME="+t.TempDir())
	out, err := run.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "hello from the module|CREATE TABLE t (x INTEGER);|hello from the module" {
		t.Errorf("output = %q", out)
	}
}
