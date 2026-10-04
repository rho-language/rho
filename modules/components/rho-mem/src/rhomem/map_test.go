//
// @file map_test.go
// @brief Reading a linker map, against the shapes real linkers write.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMap writes a linker map and returns its path.
func writeMap(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "firmware.map")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A memory configuration is the one table in a map that is unambiguous: the
// linker script's own MEMORY block, echoed back. It is what lets a byte be
// attributed to a region without knowing anything about the board.
func TestTheMemoryConfigurationIsRead(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr
RAM              0x0000000020000000 0x0000000000020000 rw
*default*        0x0000000000000000 0xffffffffffffffff

Linker script and memory map
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Regions) != 2 {
		t.Fatalf("expected two regions, got %+v", parsed.Regions)
	}
	flash := parsed.Regions[0]
	if flash.Name != "FLASH" || flash.Origin != 0x08000000 || flash.Length != 0x200000 {
		t.Fatalf("unexpected FLASH region: %+v", flash)
	}
	if flash.End() != 0x08200000 {
		t.Errorf("expected the region to end one past its last byte, got %#x", flash.End())
	}
	// `*default*` is a catch-all for an address in no region, not a region: it has
	// no capacity, so budgeting against it would divide by nothing.
	if parsed.Regions[1].Name != "RAM" {
		t.Fatalf("the default row was read as a region: %+v", parsed.Regions[1])
	}
}

// A region's end is one past its last address, which is what makes an address
// belong to exactly one of two regions that touch.
func TestARegionContainsEveryAddressButTheOneAfterIt(t *testing.T) {
	region := Region{Name: "FLASH", Origin: 0x08000000, Length: 0x200}
	if !region.Contains(0x08000000) || !region.Contains(0x080001FF) {
		t.Error("expected the region's own addresses to be in it")
	}
	if region.Contains(0x08000200) {
		t.Error("the first address past the region was counted as in it")
	}
}

// An output section is three fields, and a placed input section is four with a
// file name at the end. Reading one as the other counts the same bytes twice,
// which is the difference between a number and a number that happens to be right.
func TestAPlacedInputSectionIsNotReadAsAnOutputSection(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000     0x40
 *(.text*)
 .text._init    0x0000000008000000        0x4 CMakeFiles/main.dir/app.c.obj
                0x0000000008000000                _init
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sections) != 1 {
		t.Fatalf("expected the one output section, got %d: %+v", len(parsed.Sections), parsed.Sections)
	}
	if parsed.Sections[0].Name != ".text" || parsed.Sections[0].Size != 0x40 {
		t.Fatalf("unexpected section: %+v", parsed.Sections[0])
	}
	// And it became a symbol, which is what it is.
	if len(parsed.Symbols) != 1 || parsed.Symbols[0].Name != "_init" {
		t.Fatalf("expected the placed section to be read as a symbol, got %+v", parsed.Symbols)
	}
	if parsed.Symbols[0].Size != 4 || parsed.Symbols[0].Object != "CMakeFiles/main.dir/app.c.obj" {
		t.Errorf("unexpected symbol: %+v", parsed.Symbols[0])
	}
}

// The common shape: a name on its own line, then the contribution, then the
// symbol's own name.
func TestAPlacedSymbolIsReadWithItsObject(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000     0x100
 .text.app_main
                0x0000000008000000      0x148 main-dep0/libfoo.a(app.c.obj)
                0x0000000008000000                app_main
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Symbols) != 1 {
		t.Fatalf("expected one symbol, got %+v", parsed.Symbols)
	}
	symbol := parsed.Symbols[0]
	if symbol.Name != "app_main" || symbol.Size != 0x148 {
		t.Errorf("unexpected symbol: %+v", symbol)
	}
	if symbol.Object != "main-dep0/libfoo.a(app.c.obj)" {
		t.Errorf("expected the archive member it came from, got %q", symbol.Object)
	}
}

// A discarded input section is written with an address of zero and its size still
// stated. Reading it as placed would charge the board for code the linker threw
// away, in an address no region covers.
func TestDiscardedInputSectionsAreNotCounted(t *testing.T) {
	path := writeMap(t, `Archive member included to satisfy reference by file (symbol)

main-dep0/libfoo.a(main.c.obj)

.text.std_core_option_some
                0x0000000000000000        0x8 main-dep0/libfoo.a(main.c.obj)

Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x10
 .text.kept
                0x0000000008000000       0x10 main-dep0/libfoo.a(main.c.obj)
                0x0000000008000000               kept
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range parsed.Symbols {
		if symbol.Address == 0 {
			t.Fatalf("a discarded section was read as placed: %+v", symbol)
		}
	}
	if len(parsed.Symbols) != 1 || parsed.Symbols[0].Name != "kept" {
		t.Fatalf("expected only the placed symbol, got %+v", parsed.Symbols)
	}
}

// Debug and comment sections are in the file and not on the board. A `-g` build
// carries hundreds of kilobytes of them, and counting those would make a debug
// build look like one that does not fit.
func TestDebugSectionsAreNotCounted(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x10
.debug_info     0x0000000000000000    0x2d939
.comment        0x0000000000000000       0x33
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sections) != 1 || parsed.Sections[0].Name != ".text" {
		t.Fatalf("expected only .text, got %+v", parsed.Sections)
	}
}

// A load address is a section stored in one place and running in another, and it
// is read as such.
func TestALoadAddressIsRead(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr
RAM              0x0000000020000000 0x0000000000010000 rw

Linker script and memory map

.data           0x0000000020000000       0x88 load address 0x0000000008001000
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	section := parsed.Sections[0]
	if !section.HasLoad || section.LoadAddress != 0x08001000 || section.Address != 0x20000000 {
		t.Fatalf("unexpected section: %+v", section)
	}
}

// A cross-reference table says which object referred to which symbol, which is
// what turns a size into a reason: a driver that was not meant to be linked is a
// different problem from one that is simply large.
func TestTheCrossReferenceTableIsRead(t *testing.T) {
	path := writeMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x10

Cross Reference Table

Symbol                                            File
HAL_GPIO_Init                                    CMakeFiles/main.dir/gpio.c.obj
HAL_GPIO_Init                                    CMakeFiles/main.dir/app.c.obj
`)
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	referrers := parsed.ReferencedBy["HAL_GPIO_Init"]
	if len(referrers) != 2 {
		t.Fatalf("expected both referrers, got %+v", referrers)
	}
}

// A map with no memory table names no regions, and that is said rather than
// reported as an image that occupies nothing: a check that cannot tell those two
// apart is a check that passes.
func TestAMapWithNoMemoryTableSaysSo(t *testing.T) {
	path := writeMap(t, "Linker script and memory map\n\n.text 0x1000 0x10\n")
	parsed, err := ParseMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Diagnostics) == 0 {
		t.Fatal("expected a diagnostic naming the missing memory configuration")
	}
}

// An address is read whichever way the linker wrote it, and a word of nothing
// but digits is hexadecimal — the map's convention, and the one `size` differs
// from it on.
func TestAnAddressIsReadInEveryFormALinkerWritesIt(t *testing.T) {
	cases := map[string]uint64{
		"0x08000000": 0x08000000,
		"8000000":    0x08000000,
		"1_000":      0x1000,
		"4096":       0x4096,
	}
	for raw, want := range cases {
		got, err := parseAddress(raw)
		if err != nil {
			t.Errorf("%s: %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("%s: expected %#x, got %#x", raw, want, got)
		}
	}
}
