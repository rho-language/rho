//
// @file spec_test.go
// @brief Tests for reading and resolving target specs.
//
// A spec is data another spec refers to, so these tests are about the chain: what
// a chip gives a board, what the board's own facts win, and what happens when a
// reference does not resolve. A board whose chip is missing is a board whose
// compiler is missing, and a build that finds that out at the compiler is a build
// that fails worse.
//
// @project Rho Programming Language
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

// libraryOf writes specs into a directory and reads them back.
func libraryOf(t *testing.T, specs map[string]string) *Library {
	t.Helper()
	root := t.TempDir()
	for name, content := range specs {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Load(root)
}

const archSpec = `{
  "kind": "arch", "name": "arm-cortex-m",
  "toolchain": {"compiler": "arm-none-eabi-gcc", "objcopy": "arm-none-eabi-objcopy"}
}`

const chipSpec = `{
  "kind": "mcu", "name": "chip", "arch": "arm-cortex-m", "core": "cortex-m4",
  "toolchain": {"flags": ["-mcpu=cortex-m4", "-mthumb"]},
  "flags": ["-Wl,--gc-sections"],
  "defines": ["CHIP=1"],
  "memory": {
    "flash": {"origin": "0x08000000", "length": "0x100000"},
    "ram": {"origin": "0x20000000", "length": "0x20000"},
    "extra": {"ccm": {"origin": "0x10000000", "length": "0x10000"}}
  }
}`

const boardSpec = `{
  "kind": "board", "name": "board", "mcu": "chip", "vendor_library": "robohub_board",
  "flags": ["-DBOARD=1"],
  "pins": {"gp25": {"name": "GP25", "role": "led"}},
  "peripherals": {"i2c0": {"instance": "i2c0", "pins": {"sda": "GP4"}}}
}`

/* -------------------------------------------------------------------------- */
/*  Reading                                                                    */
/* -------------------------------------------------------------------------- */

func TestSpecsAreReadByName(t *testing.T) {
	library := libraryOf(t, map[string]string{"chip.json": chipSpec, "arch.json": archSpec})
	if len(library.Diagnostics) != 0 {
		t.Fatalf("expected a clean read, got %+v", library.Diagnostics)
	}
	names := library.Names()
	if len(names) != 2 || names[0] != "arm-cortex-m" || names[1] != "chip" {
		t.Fatalf("expected both specs by name, got %v", names)
	}
	if spec, found := library.Get("chip"); !found || spec.Core != "cortex-m4" {
		t.Fatalf("expected the chip, got %+v (%v)", spec, found)
	}
}

func TestASpecThatNamesNothingIsReported(t *testing.T) {
	library := libraryOf(t, map[string]string{"bad.json": `{"kind": "mcu"}`})
	if len(library.Diagnostics) != 1 {
		t.Fatalf("expected the spec to be reported, got %+v", library.Diagnostics)
	}
}

func TestASpecThatIsNotJSONIsReported(t *testing.T) {
	library := libraryOf(t, map[string]string{"bad.json": `not json`})
	if len(library.Diagnostics) != 1 {
		t.Fatalf("expected the spec to be reported, got %+v", library.Diagnostics)
	}
}

func TestTwoSpecsOfOneNameAreReported(t *testing.T) {
	// A project that ships a board of a name a component already has should hear
	// that its board is not the one being built for.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	one := `{"kind": "mcu", "name": "chip"}`
	for _, name := range []string{"one.json", "nested/two.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(one), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	library := Load(root)
	if len(library.Diagnostics) != 1 {
		t.Fatalf("expected the repeated name to be reported, got %+v", library.Diagnostics)
	}
	if names := library.Names(); len(names) != 1 {
		t.Fatalf("expected one spec, got %v", names)
	}
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

func TestABoardInheritsFromItsChipAndArchitecture(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": archSpec, "chip.json": chipSpec, "board.json": boardSpec,
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}

	// What the architecture gives.
	if spec.Toolchain.Compiler != "arm-none-eabi-gcc" {
		t.Errorf("expected the compiler from the architecture, got %q", spec.Toolchain.Compiler)
	}
	if spec.Toolchain.Objcopy != "arm-none-eabi-objcopy" {
		t.Errorf("expected objcopy from the architecture, got %q", spec.Toolchain.Objcopy)
	}
	// What the chip gives.
	if spec.Core != "cortex-m4" {
		t.Errorf("expected the core from the chip, got %q", spec.Core)
	}
	if spec.Memory.Flash.Origin != "0x08000000" || spec.Memory.RAM.Length != "0x20000" {
		t.Errorf("expected the chip's memory, got %+v", spec.Memory)
	}
	// Both flags, the chip's first, because a compiler reads them in order.
	flags := spec.AllFlags()
	want := []string{"-mcpu=cortex-m4", "-mthumb", "-Wl,--gc-sections", "-DBOARD=1"}
	if len(flags) != len(want) {
		t.Fatalf("expected %v, got %v", want, flags)
	}
	for index := range want {
		if flags[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, flags)
		}
	}
	if spec.VendorLibrary != "robohub_board" {
		t.Errorf("expected the board's library, got %q", spec.VendorLibrary)
	}
	if pin, found := spec.Pin("led"); !found || pin.Name != "GP25" {
		t.Errorf("expected the board's led pin, got %+v (%v)", pin, found)
	}
}

func TestAChipResolvesOnItsOwn(t *testing.T) {
	library := libraryOf(t, map[string]string{"arch.json": archSpec, "chip.json": chipSpec})
	spec, diagnostics := library.Resolve("chip")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if spec.Name != "chip" || spec.Toolchain.Compiler != "arm-none-eabi-gcc" {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestAReferenceThatDoesNotResolveIsReported(t *testing.T) {
	// A board whose chip is missing has no compiler, and a build that finds that
	// out from a generator is better than one that finds it out from a compiler.
	library := libraryOf(t, map[string]string{"board.json": boardSpec})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 1 {
		t.Fatalf("expected the missing chip to be reported, got %+v", diagnostics)
	}
	if spec.Toolchain.Compiler != "" {
		t.Fatalf("expected no compiler to be invented, got %q", spec.Toolchain.Compiler)
	}
}

func TestAnUnknownTargetIsReportedWithTheOnesThatExist(t *testing.T) {
	library := libraryOf(t, map[string]string{"arch.json": archSpec})
	_, diagnostics := library.Resolve("teensy")
	if len(diagnostics) != 1 {
		t.Fatalf("expected the unknown target to be reported, got %+v", diagnostics)
	}
	if !containsString(diagnostics[0].Message, "arm-cortex-m") {
		t.Fatalf("expected the targets that exist to be named, got %q", diagnostics[0].Message)
	}
}

func TestAChainThatRefersToItselfIsReported(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"loop.json": `{"kind": "mcu", "name": "loop", "mcu": "loop"}`,
	})
	_, diagnostics := library.Resolve("loop")
	if len(diagnostics) == 0 {
		t.Fatal("expected a cycle to be reported")
	}
}

/* -------------------------------------------------------------------------- */
/*  Memory and pins                                                            */
/* -------------------------------------------------------------------------- */

func TestRegionsAreNamedTheWayALinkerScriptNeeds(t *testing.T) {
	library := libraryOf(t, map[string]string{"arch.json": archSpec, "chip.json": chipSpec})
	spec, _ := library.Resolve("chip")
	regions := spec.Regions()
	if len(regions) != 3 {
		t.Fatalf("expected flash, ram and the extra region, got %+v", regions)
	}
	if regions[0].Name != "FLASH" || regions[1].Name != "RAM" || regions[2].Name != "CCM" {
		t.Fatalf("expected named regions, got %+v", regions)
	}
	if regions[0].Kind != "flash" || regions[1].Kind != "ram" {
		t.Fatalf("expected the kinds a linker script uses, got %+v", regions)
	}
}

func TestARegionCanBeAFactRatherThanSomewhereToPutASection(t *testing.T) {
	// A window onto another region is not memory a linker script may use: an
	// ESP-IDF board's flash is reached through a cache, and the framework's own
	// script is what lays a program out.
	library := libraryOf(t, map[string]string{
		"chip.json": `{
		  "kind": "board", "name": "soc",
		  "memory": {
		    "flash": {"origin": "0x10000", "length": "0x100000", "link": false},
		    "ram": {"origin": "0x3FC80000", "length": "0x64000"}
		  }
		}`,
	})
	spec, _ := library.Resolve("soc")
	if len(spec.Regions()) != 2 {
		t.Fatalf("expected both regions as facts, got %+v", spec.Regions())
	}
	linkable := spec.LinkRegions()
	if len(linkable) != 1 || linkable[0].Name != "RAM" {
		t.Fatalf("expected only RAM to be linkable, got %+v", linkable)
	}
}

func TestPinsReadInTheBoardsOwnOrder(t *testing.T) {
	// A pin map is read by a person looking for a pin, so GP2 comes before GP16.
	library := libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board",
		  "pins": {
		    "gp16": {"name": "GP16"}, "gp2": {"name": "GP2"}, "gp25": {"name": "GP25"}
		  }
		}`,
	})
	spec, _ := library.Resolve("board")
	names := spec.PinNames()
	want := []string{"gp2", "gp16", "gp25"}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, names)
		}
	}
}

func containsString(text, want string) bool {
	for index := 0; index+len(want) <= len(text); index++ {
		if text[index:index+len(want)] == want {
			return true
		}
	}
	return false
}
