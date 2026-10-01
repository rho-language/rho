//
// @file specloader_test.go
// @brief Tests for reading target specs: both formats, and which root wins.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhtarget

import (
	"os"
	"path/filepath"
	"testing"
)

// A spec written as YAML is read, and read the same way as one written as JSON.
// The two formats exist so that a target a person writes by hand can be written
// as YAML — which is mostly comments saying what a board is — while the specs
// that ship stay as they were. They mean the same thing, and this asserts it
// rather than leaving it to whichever one a build happened to try.
func TestASpecIsReadFromEitherJSONOrYAML(t *testing.T) {
	dir := t.TempDir()

	writeSpec(t, dir, "chip.json", `{
      "kind": "mcu",
      "name": "from-json",
      "toolchain": { "compiler": "arm-none-eabi-gcc" }
    }`)

	writeSpec(t, dir, "board.yaml", `
kind: board
name: from-yaml
core: cortex-m7
toolchain:
  compiler: arm-none-eabi-gcc
  flags:
    - -O2
memory:
  flash:
    origin: 0x08000000
    length: 0x100000
`)

	library := Load(dir)

	for _, name := range []string{"from-json", "from-yaml"} {
		if _, ok := library.Get(name); !ok {
			t.Errorf("%s was not read; the library has %v", name, library.Names())
		}
	}

	// The nesting has to survive the conversion, or a YAML board would load and
	// then have no memory map and no flags.
	board, ok := library.Get("from-yaml")
	if !ok {
		t.Fatalf("from-yaml was not read")
	}
	if board.Core != "cortex-m7" {
		t.Errorf("core is %q, want cortex-m7", board.Core)
	}
	if len(board.Toolchain.Flags) != 1 || board.Toolchain.Flags[0] != "-O2" {
		t.Errorf("flags are %v, want [-O2]", board.Toolchain.Flags)
	}
	if board.Memory.Flash.Origin != "0x08000000" || board.Memory.Flash.Length != "0x100000" {
		t.Errorf("flash is %+v, want 0x08000000/0x100000", board.Memory.Flash)
	}
}

// A target declared in the Rho home is an override, not a second opinion: it is
// the one that stands, and the one it shadows is reported so that the reader can
// see that there are two and which one won.
func TestARootReadFirstOverridesOneReadLater(t *testing.T) {
	override := t.TempDir()
	shipped := t.TempDir()

	writeSpec(t, override, "native.yaml", `
kind: arch
name: native
toolchain:
  compiler: clang
`)
	writeSpec(t, shipped, "native.yaml", `
kind: arch
name: native
toolchain:
  compiler: gcc
`)

	library := LoadFrom([]string{override, shipped})

	native, ok := library.Get("native")
	if !ok {
		t.Fatalf("native was not read at all")
	}
	if native.Toolchain.Compiler != "clang" {
		t.Errorf("compiler is %q, want clang: the first root read is the one that stands", native.Toolchain.Compiler)
	}

	if len(library.Diagnostics) == 0 {
		t.Errorf("the shadowed spec was not reported, so a reader cannot tell that two exist")
	}
}

// The other order gives the other answer, which is what makes the order the
// thing that decides rather than the file.
func TestARootReadLastIsTheOneThatIsShadowed(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	writeSpec(t, first, "native.yaml", "kind: arch\nname: native\ntoolchain:\n  compiler: clang\n")
	writeSpec(t, second, "native.yaml", "kind: arch\nname: native\ntoolchain:\n  compiler: gcc\n")

	native, ok := LoadFrom([]string{first, second}).Get("native")
	if !ok {
		t.Fatalf("native was not read at all")
	}
	if native.Toolchain.Compiler != "clang" {
		t.Errorf("compiler is %q, want clang", native.Toolchain.Compiler)
	}
}

// A root that does not exist is not a failure. The Rho home is read before the
// directory this component ships, and most machines have no targets in it, so a
// build that treated its absence as an error would fail on every machine.
func TestARootThatDoesNotExistIsNotAProblem(t *testing.T) {
	present := t.TempDir()
	writeSpec(t, present, "chip.json", `{"kind":"mcu","name":"chip"}`)

	library := LoadFrom([]string{filepath.Join(present, "no-such-directory"), present})

	if _, ok := library.Get("chip"); !ok {
		t.Errorf("the root that does exist was not read; the library has %v", library.Names())
	}
}

// writeSpec writes a spec file, creating the directory it goes in.
func writeSpec(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}
}
