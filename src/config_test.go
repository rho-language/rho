//
// @file config_test.go
// @brief Tests for carrying a project's configuration into generation.
//
// The generator does not resolve a configuration; it carries what a project
// declared and writes the build that resolves it. These tests are about that
// carrying: the fragments arrive in the generated project, the values a
// manifest sets arrive as a defconfig the merge can read, and a project that
// declared nothing gets none of it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configuredProject is a project that declares a configuration bridge, and the
// directory it was written in.
func configuredProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"module.yml": `module:
  name: sensorhub
  type: rho
  version: 0.1.0

main: src/main.rho

config:
  dialect: freertos
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_UART_BAUD: "57600"
    CONFIG_SPI_MASTER: "y"
`,
		"kconfig/board.Kconfig": "config CONFIG_UART_BAUD\n\tint\n\tdefault 115200\n",
		"src/main.rho":          "module main\n\nfn main() void {\n    print(\"x\");\n}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTheFragmentsAndTheValuesReachTheGeneratedProject(t *testing.T) {
	// A generated project is built on its own and may be moved, so what it needs
	// is copied into it: a fragment referenced in place would be a build that
	// only works where it was generated.
	root := configuredProject(t)
	out := filepath.Join(t.TempDir(), "generated")

	bridge := configBridge(root, out)
	if bridge == nil {
		t.Fatal("expected the declared bridge to be carried")
	}
	if bridge.Dialect != "freertos" {
		t.Errorf("expected the dialect the project named, got %q", bridge.Dialect)
	}
	if len(bridge.Fragments) != 1 || bridge.Fragments[0] != filepath.Join("kconfig", "board.Kconfig") {
		t.Errorf("expected the fragment, got %+v", bridge.Fragments)
	}
	fragment, err := os.ReadFile(filepath.Join(out, "kconfig", "board.Kconfig"))
	if err != nil {
		t.Fatalf("expected the fragment to be copied into the project: %v", err)
	}
	if !strings.Contains(string(fragment), "CONFIG_UART_BAUD") {
		t.Fatalf("expected the fragment's contents, got:\n%s", fragment)
	}
	// The values a manifest sets are written as a defconfig, which is the format
	// every dialect already reads, so a project needs no syntax of its own.
	defconfig, err := os.ReadFile(filepath.Join(out, "kconfig", "rho-defconfig"))
	if err != nil {
		t.Fatalf("expected the values to be written as a defconfig: %v", err)
	}
	for _, want := range []string{"CONFIG_UART_BAUD=57600", "CONFIG_SPI_MASTER=y"} {
		if !strings.Contains(string(defconfig), want) {
			t.Errorf("expected %q in the defconfig, got:\n%s", want, defconfig)
		}
	}
}

func TestAProjectWithNoBridgeCarriesNone(t *testing.T) {
	// Most projects configure nothing, and a build that assumed otherwise would
	// copy fragments that are not there.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: plain\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bridge := configBridge(root, filepath.Join(root, "out")); bridge != nil {
		t.Fatalf("expected no bridge, got %+v", bridge)
	}
}

func TestAFragmentThatCannotBeReadIsLeftToTheMerge(t *testing.T) {
	// The error belongs where it is useful: in front of the build, naming the
	// fragment, rather than in front of a generation the project may not use.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nconfig:\n  kconfig_fragments:\n    - kconfig/absent.Kconfig\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	bridge := configBridge(root, filepath.Join(t.TempDir(), "out"))
	if bridge == nil {
		t.Fatal("expected the bridge to be carried")
	}
	if len(bridge.Fragments) != 0 {
		t.Fatalf("expected no fragment to be copied, got %+v", bridge.Fragments)
	}
	// A bridge with no fragment is a project that cannot resolve anything, and
	// the merge says so.
	if bridge.Dialect != "linux" {
		t.Fatalf("expected the default dialect, got %q", bridge.Dialect)
	}
}

func TestAModuleOfConstantsIsCarriedAsAFragment(t *testing.T) {
	// A Rho module of constants is read by the merge, so it is named to it
	// rather than copied: the project owns the file, and a project's own file
	// belongs where the project keeps it.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nconfig:\n  kconfig_fragments:\n    - kconfig/board.Kconfig\n  module: config.rho\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "kconfig"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kconfig", "board.Kconfig"), []byte("config CONFIG_ONE\n\tbool\n\tdefault y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bridge := configBridge(root, filepath.Join(t.TempDir(), "out"))
	if bridge == nil {
		t.Fatal("expected the bridge to be carried")
	}
	if len(bridge.Fragments) != 2 {
		t.Fatalf("expected the fragment and the module, got %+v", bridge.Fragments)
	}
	if !strings.HasSuffix(filepath.ToSlash(bridge.Fragments[1]), "config.rho") {
		t.Fatalf("expected the module of constants to be named, got %q", bridge.Fragments[1])
	}
}
