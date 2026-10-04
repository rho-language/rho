//
// @file elf.go
// @brief Reading a program, when there is no map beside it.
//
// A build that was asked for a linker map has one, and the map is the better
// source: the linker wrote it with the whole picture in hand, and it names the
// memory regions exactly as the linker script does. The ELF is what is there when
// a build was made without one, and it is what a flash is checked against, so it
// is read rather than declared optional.
//
// What it can answer without a linker script is where the sections are, which is
// most of a budget: a section's address and size say which region it is in, as
// long as somebody says what the regions are. Where nobody does, the regions are
// named after the sections they were inferred from, which is honest about being
// an inference — "FLASH" is a name this component made up, and a budget written
// against it is a budget against an assumption.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"debug/elf"
	"fmt"
	"sort"
	"strings"
)

// Program is what an ELF file says about where a program is.
type Program struct {
	// Sections are the allocated sections, with the addresses they were linked at.
	Sections []PlacedSection
	// Symbols are the sized symbols the symbol table holds.
	Symbols []PlacedSymbol
	// Entry is the entry point, which is reported because a program whose entry
	// point is outside every region it was budgeted for is a fact worth knowing.
	Entry uint64
	// ZeroInit are the sections that occupy memory without storing anything.
	//
	// This is the one thing the map cannot say and the ELF can. A linker script
	// gives `.bss` a load address — the place its initialiser image would go, had
	// it one — and a linker map prints it, so a reader that charges every load
	// address to flash counts `.bss` twice: once as RAM it occupies and once as
	// flash it does not. `SHT_NOBITS` is the section type that says which sections
	// those are, and it is the answer the map has no way of expressing.
	ZeroInit map[string]bool
}

// ParseELF reads a linked program.
//
// A program with no symbol table is read as far as it goes: its sections are
// there whatever the link was told to keep, and a firmware linked with `-s` has
// no symbols and still has a size. The missing half is reported rather than
// silently empty, because "nothing is over budget" and "nothing could be
// measured" are different answers and a check that cannot tell them apart is a
// check that passes.
func ParseELF(path string) (*Program, error) {
	file, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := &Program{Entry: file.Entry, ZeroInit: map[string]bool{}}
	for _, section := range file.Sections {
		// Only what occupies memory: a debug section is in the file and not on the
		// board, and counting it would make a `-g` build look like an overflowing
		// one.
		if section.Flags&elf.SHF_ALLOC == 0 || section.Addr == 0 || section.Size == 0 {
			continue
		}
		if section.Type == elf.SHT_NOBITS {
			out.ZeroInit[section.Name] = true
		}
		out.Sections = append(out.Sections, PlacedSection{
			Name:    section.Name,
			Address: section.Addr,
			Size:    section.Size,
			// The ELF has no separate load address: a section's contents are in
			// the file at the same place it runs, and `.data` is the one case where
			// that is not true of where the linker put it — which is exactly why
			// the map is the better source when there is one.
			LoadAddress: section.Addr,
		})
	}
	sort.Slice(out.Sections, func(i, j int) bool { return out.Sections[i].Address < out.Sections[j].Address })

	symbols, err := file.Symbols()
	if err != nil {
		// A stripped program has no `.symtab`, which is a fact about the build
		// rather than a failure to read it.
		return out, fmt.Errorf("%s carries no symbol table, so only its sections could be read: %w", path, err)
	}
	for _, symbol := range symbols {
		if symbol.Name == "" || symbol.Size == 0 || symbol.Value == 0 {
			continue
		}
		if elf.ST_TYPE(symbol.Info) != elf.STT_FUNC && elf.ST_TYPE(symbol.Info) != elf.STT_OBJECT {
			continue
		}
		out.Symbols = append(out.Symbols, PlacedSymbol{
			Name:    symbol.Name,
			Address: symbol.Value,
			Size:    symbol.Size,
			Object:  sectionNameOf(file, symbol),
		})
	}
	sort.Slice(out.Symbols, func(i, j int) bool { return out.Symbols[i].Size > out.Symbols[j].Size })
	return out, nil
}

// sectionNameOf is the section a symbol belongs to, which is how an ELF says
// which object it came from when there is no map to say so.
func sectionNameOf(file *elf.File, symbol elf.Symbol) string {
	for _, section := range file.Sections {
		if section.Type == elf.SHT_SYMTAB && section.Addr != 0 &&
			symbol.Value >= section.Addr && symbol.Value < section.Addr+section.Size {
			return section.Name
		}
	}
	return ""
}

// looksLikeELF reports whether a file is an ELF object.
//
// Read from the magic rather than from the extension, because a linked program is
// routinely called `main`, `firmware` or the target's name, and an analysis that
// looked for `.elf` would find nothing in a build tree that has one program in
// it.
func looksLikeELF(path string) bool {
	file, err := openLimited(path)
	if err != nil {
		return false
	}
	defer file.Close()
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return false
	}
	return string(magic) == "\x7fELF"
}

// regionNameFromSection is the region name a section implies when no linker
// script named one.
//
// Named for what the section holds rather than for where it is, because a
// component that cannot read the memory map has no address to name a region
// after, and `FLASH` is at least true of what it is.
func regionNameFromSection(section string) string {
	switch {
	case strings.HasPrefix(section, ".text"), strings.HasPrefix(section, ".rodata"),
		strings.HasPrefix(section, ".isr"), section == ".init" || section == ".fini",
		strings.Contains(section, "vectors"):
		return "FLASH"
	case section == ".data", section == ".bss", section == "._user_heap_stack",
		strings.HasSuffix(section, "_init") || strings.HasSuffix(section, "_fini"):
		return "RAM"
	default:
		return "OTHER"
	}
}
