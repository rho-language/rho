//
// @file map.go
// @brief Reading what the linker wrote.
//
// A GNU linker map is a text file with no version marker and no fixed grammar: it
// is whatever the linker printed, in a layout that has grown fields over thirty
// years and differs between linkers. So it is read by shape rather than by
// position — a line that starts with a section name and carries an address and a
// size is a section, a line that carries an address, a size and a file is a
// placed symbol, and the line after it names the symbol.
//
// What makes it worth reading anyway is the one table in it that is unambiguous:
// `Memory Configuration`, which is the linker script's own MEMORY block echoed
// back, with every region's name, origin and length as the linker saw them. That
// table is why this component can attribute a byte to a region without knowing
// anything about the board.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LinkMap is what a linker map says, read into a shape this component can reason
// about.
type LinkMap struct {
	// Regions are the memory regions, as the linker script declared them.
	//
	// Named exactly as the script names them — `FLASH`, `DTCM`, `SRAM1` — because
	// a budget is written against those names and a component that renamed them
	// would make every budget in every project refer to something else.
	Regions []Region
	// Sections are the output sections the link placed, in link order.
	Sections []PlacedSection
	// Symbols are the symbols that were placed, with the object each came from.
	Symbols []PlacedSymbol
	// ReferencedBy is the cross-reference table: which object referred to which
	// symbol. Only present when the link was asked for one.
	ReferencedBy map[string][]string
	// Diagnostics is everything that could not be read.
	Diagnostics []string
}

// Region is one memory region as the linker script declared it.
type Region struct {
	Name   string
	Origin uint64
	Length uint64
}

// End is the first address past the region.
func (r Region) End() uint64 { return r.Origin + r.Length }

// Contains reports whether an address is in the region.
//
// Half-open, because a region's length is how many addresses it starts at: the
// region at 0x08000000 of length 0x200000 ends at 0x0A000000, which is one past
// its last byte and not one of its own. An inclusive test would credit the first
// byte of the next region to this one.
func (r Region) Contains(address uint64) bool {
	return address >= r.Origin && address < r.End()
}

// PlacedSection is one output section the link placed.
type PlacedSection struct {
	Name string
	// Address is where the section runs, which is the region it occupies.
	Address uint64
	// LoadAddress is where its initializer image is stored, which is a different
	// region for a section whose variables are not where its contents are.
	LoadAddress uint64
	// HasLoad says whether the map said anything about a load address. A section
	// with none is stored where it runs, which is true of code and of `.bss`.
	HasLoad bool
	Size    uint64
}

// PlacedSymbol is one symbol the link placed, and the object file it came from.
//
// The object is what turns a list of sizes into something actionable: a symbol
// that costs 12 kB is a fact, and "12 kB in `stm32h7xx_hal_eth.c`, which the
// linker pulled in for one call" is an action.
type PlacedSymbol struct {
	Name    string
	Region  string
	Address uint64
	Size    uint64
	// Object is the object file or archive member the symbol came from.
	Object string
	// Section is the output section it was placed in, which is what its address
	// alone cannot say once two sections alias.
	Section string
}

// ParseMap reads a linker map.
//
// Tolerant by construction: a line it does not recognise is skipped rather than
// reported, because a map is a human-readable dump and refusing to read one
// because of a line format added in a newer linker would make this useless on
// exactly the toolchains it is most needed for. What it cannot do is reported —
// a map with no `Memory Configuration` has no regions, and a caller that silently
// reported zero usage for a board with 2 MB of flash would be worse than an
// error.
func ParseMap(path string) (*LinkMap, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := &LinkMap{ReferencedBy: map[string][]string{}}
	scanner := bufio.NewScanner(file)
	// A map has lines of thousands of characters: the cross-reference table writes
	// one line per referencing object, and a path can be long. The default buffer
	// is 64 kB, which a large build reaches.
	scanner.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)

	inMemory, inCref, placed := false, false, false
	current := ""
	// pending is the placed symbol a name line belongs to. The map writes an
	// object's contribution first — address, size, file — and the symbol's own name
	// on the line after, so the size and the object are known before the name is.
	var pending *PlacedSymbol
	flush := func() {
		if pending != nil && pending.Name != "" {
			out.Symbols = append(out.Symbols, *pending)
		}
		pending = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "Memory Configuration"):
			inMemory, inCref = true, false
			continue
		case strings.HasPrefix(trimmed, "Cross Reference Table"):
			inCref, inMemory = true, false
			continue
		case strings.HasPrefix(trimmed, "Linker script and memory map"):
			inMemory, inCref, placed = false, false, true
			continue
		}

		if inMemory {
			if region, ok := parseMemoryRow(trimmed); ok {
				out.Regions = append(out.Regions, region)
			}
			continue
		}
		if inCref {
			// `Symbol<spaces>File`, with the file last so a path containing
			// spaces does not shift it.
			name, object := splitCrefRow(trimmed)
			if name != "" && object != "" {
				out.ReferencedBy[name] = append(out.ReferencedBy[name], object)
			}
			continue
		}
		if !placed {
			// Before the linker gets to laying the program out, a map carries two
			// more lists that look like the real one: the archive members that were
			// pulled in, and the input sections that were *discarded*. A discarded
			// section is written with an address of zero and its size still stated,
			// because the size is what it would have been — so reading it as placed
			// would charge the board for code the linker threw away, in an address
			// no region covers.
			continue
		}

		if trimmed == "" {
			continue
		}

		// An output section header: a name, an address, a size, and optionally the
		// address its image is stored at.
		if section, ok := parseSectionHeader(trimmed); ok {
			// A section that is not on the board is not part of the board's
			// memory, whatever size the linker recorded for it. Debug and comment
			// sections are in the file and not in the flash, and a `-g` build
			// carries several hundred kilobytes of them — counting them would make
			// a debug build look like one that does not fit.
			if !onBoard(section.Name) {
				continue
			}
			flush()
			current = section.Name
			out.Sections = append(out.Sections, section)
			continue
		}

		// A line indented under a section that is an address, a size and a file:
		// one object's contribution.
		if fields := splitAddressSizeObject(trimmed); fields != nil {
			flush()
			pending = &PlacedSymbol{
				Address: fields.address,
				Size:    fields.size,
				Object:  fields.object,
				Section: current,
			}
			continue
		}

		// A line that is an address and a name: the symbol that contribution is.
		if address, name, ok := parseAddressName(trimmed); ok {
			if pending != nil && pending.Address == address {
				pending.Name = name
				flush()
				continue
			}
			// A symbol with no size line before it is one the linker resolved but
			// did not place a size for — an alias, or an address-only entry such as
			// `_sdata = .`. Kept, because the name is worth having even at no size.
			flush()
			pending = &PlacedSymbol{Address: address, Name: name, Section: current}
			flush()
			continue
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return out, err
	}
	if len(out.Regions) == 0 {
		out.Diagnostics = append(out.Diagnostics, fmt.Sprintf(
			"%s carries no Memory Configuration, so it names no memory regions; "+
				"build with a linker map that has one, or point `mem.regions` at the regions yourself",
			path))
	}
	return out, nil
}

// onBoard reports whether a section occupies memory.
//
// The test is the one ELF and linker script both agree on: a section that is
// `SHF_ALLOC` is in memory, and the ones that are not are the things that make a
// linked file larger than what is in the board. `-g` is the case that matters,
// because a debug build's file is several times its flash image and every one of
// those bytes would otherwise be charged to a firmware that has none of them.
func onBoard(name string) bool {
	switch {
	case strings.HasPrefix(name, ".debug"), strings.HasPrefix(name, ".comment"),
		strings.HasPrefix(name, ".stab"), strings.HasPrefix(name, ".symtab"),
		strings.HasPrefix(name, ".strtab"), strings.HasPrefix(name, ".shstrtab"),
		strings.HasPrefix(name, ".line"), strings.HasPrefix(name, ".ARM.attributes"),
		strings.HasPrefix(name, ".gnu.attributes"), strings.HasPrefix(name, ".note"):
		return false
	}
	return true
}

// parseMemoryRow is one row of the MEMORY table.
//
//	Name             Origin             Length             Attributes
//	FLASH            0x0000000008000000 0x0000000000200000 xr
//
// The `*default*` row is a catch-all for an address in no region and is not a
// region: it has no capacity, so budgeting against it would divide by nothing.
func parseMemoryRow(line string) (Region, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return Region{}, false
	}
	name := fields[0]
	if name == "*default*" || name == "Name" {
		return Region{}, false
	}
	origin, err := parseAddress(fields[1])
	if err != nil {
		return Region{}, false
	}
	length, err := parseAddress(fields[2])
	if err != nil {
		return Region{}, false
	}
	return Region{Name: name, Origin: origin, Length: length}, true
}

// splitCrefRow is one row of the cross-reference table: a symbol name, spaces,
// and the object that referenced it.
func splitCrefRow(line string) (string, string) {
	if line == "Symbol File" || strings.HasPrefix(line, "Symbol ") && strings.Contains(line, "File") && !strings.Contains(line, ".o") {
		return "", ""
	}
	index := strings.Index(line, " ")
	if index <= 0 {
		return "", ""
	}
	name := strings.TrimSpace(line[:index])
	object := strings.TrimSpace(line[index+1:])
	if name == "" || object == "" || !looksLikeObject(object) {
		return "", ""
	}
	return name, object
}

// parseSectionHeader is an output section header.
//
//	.text           0x00000000080002c0     0x74a0
//	.data           0x0000000020000000       0x88 load address 0x0000000008007768
//
// The distinction that matters is in the second line: a section with a load
// address is stored in one place and runs in another, and it costs a region for
// each.
//
// Exactly three fields, or six with a load address. That is not pedantry: a
// *placed input section* is written on one line in almost the same shape —
// `.text._init 0x080002d0 0x4 app.c.obj` — and reading one of those as a header
// would count the same bytes once as a section and again as its symbol. The
// field count is what tells the two apart, and nothing else in the line does.
func parseSectionHeader(line string) (PlacedSection, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return PlacedSection{}, false
	}
	// Three fields is a header. Six is a header with a load address. Anything
	// else is a placed input section and belongs to the symbol reader.
	loadForm := len(fields) >= 6 && fields[3] == "load" && fields[4] == "address"
	if len(fields) != 3 && !loadForm {
		return PlacedSection{}, false
	}
	address, err := parseAddress(fields[1])
	if err != nil {
		return PlacedSection{}, false
	}
	size, err := parseAddress(fields[2])
	if err != nil {
		return PlacedSection{}, false
	}
	section := PlacedSection{Name: fields[0], Address: address, Size: size, LoadAddress: address}
	if loadForm {
		if load, loadErr := parseAddress(fields[5]); loadErr == nil {
			section.LoadAddress = load
			section.HasLoad = true
		}
	}
	return section, true
}

// placedFields is one object's contribution inside a section.
type placedFields struct {
	address uint64
	size    uint64
	object  string
}

// splitAddressSizeObject is one object's contribution inside a section, written
// in either of the two shapes the linker uses.
//
//	.text.startup.main
//	                0x00000000080002c0       0x10 CMakeFiles/main.dir/main.c.obj
//	.text._init    0x00000000080002d0        0x4 CMakeFiles/main.dir/app.c.obj
//
// The first is the common one: a name on its own line, then the contribution, then
// the symbol's own name. The second is the same three facts on one line, which is
// what the linker writes when the section name is the symbol name — every small
// function comes out this way, so a reader that only understood the first would
// lose most of the image's attribution.
func splitAddressSizeObject(line string) *placedFields {
	fields := strings.Fields(line)
	offset := 0
	// A leading name is not an address, and the address is the one field the
	// shape cannot be written without.
	if len(fields) > 0 {
		if _, err := parseAddress(fields[0]); err != nil {
			offset = 1
		}
	}
	rest := fields[offset:]
	if len(rest) < 3 {
		return nil
	}
	address, err := parseAddress(rest[0])
	if err != nil {
		return nil
	}
	size, err := parseAddress(rest[1])
	if err != nil {
		return nil
	}
	object := strings.Join(rest[2:], " ")
	if !looksLikeObject(object) {
		return nil
	}
	return &placedFields{address: address, size: size, object: object}
}

// parseAddressName is a line of `address name`, which is how the map names the
// symbol a contribution belongs to.
//
// A name here is one word: the linker writes it unquoted and undecorated, and a
// word carrying `=` is an assignment (`_sdata = .`) rather than a symbol, which
// is worth skipping because an assignment is not a thing that occupies memory.
func parseAddressName(line string) (uint64, string, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return 0, "", false
	}
	address, err := parseAddress(fields[0])
	if err != nil {
		return 0, "", false
	}
	name := fields[1]
	if strings.ContainsAny(name, "=*") {
		return 0, "", false
	}
	return address, name, true
}

// parseAddress reads an address or a size, in any of the forms a linker writes
// one: hexadecimal with or without `0x`, and with underscores in it.
//
// Hexadecimal in every case, including a word of nothing but digits. That is the
// linker's convention — the map writes `0x74a0` for a size and `8000000` for an
// address — and it is worth being explicit about because `size`, which prints the
// same quantities for the same program, writes them in decimal. Reading a map's
// bare digits as decimal would make `8000000` eight million and put every symbol
// in a region no board has.
func parseAddress(raw string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "_", "")
	if raw == "" {
		return 0, fmt.Errorf("no number")
	}
	base := 16
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		base = 0
	}
	if value, err := strconv.ParseUint(raw, base, 64); err == nil {
		return value, nil
	}
	return 0, fmt.Errorf("not a number: %s", raw)
}

// looksLikeObject reports whether a field names an object file or an archive.
//
// Checked rather than assumed because the same shape is used for lines that are
// not object references — a `load address` phrase, a `PROVIDE` expression — and
// reading one of those as a path would put an attribution on a symbol that came
// from nowhere.
func looksLikeObject(value string) bool {
	return strings.Contains(value, ".o") || strings.Contains(value, ".obj") ||
		strings.Contains(value, ".a)") || strings.HasSuffix(value, ")")
}
