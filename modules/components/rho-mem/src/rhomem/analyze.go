//
// @file analyze.go
// @brief Attributing a byte to the memory region it costs.
//
// This is the whole component. Everything else is a format to read or a report to
// print; the one thing that has to be right is which region a byte is charged to,
// because a total per region that is off by a section is worse than no total —
// it is a number that looks like an answer.
//
// Two things make it more than a lookup of an address in a range:
//
// **A section can cost two regions.** `.data` has variables in RAM and its
// initializer image in flash, and both are paid for: the flash holds the bytes
// that are copied at startup, and the RAM holds them afterwards. So a section with
// a load address different from where it runs is charged to both regions, which is
// what makes these totals add up to the text/data/bss that `size` prints while
// being distributed correctly among the regions.
//
// **Regions can overlap.** A linker script that declares `RAM` and `DTCM` at the
// same origin — which an H7 script does, because the general-purpose RAM alias is
// declared for a board whose data really lives in TCM — means an address is in
// both, and "the region containing this address" is not a question with one
// answer. The most specific region that fits wins: the smallest, because a larger
// region containing a smaller one is a region described in more general terms, and
// the specific one is the more useful answer. When two fit equally well the first
// declared wins, and the ambiguity is reported rather than resolved silently — a
// budget written against the wrong one of two aliased regions is a budget that
// measures the wrong thing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"fmt"
	"sort"
	"strings"
)

// Usage is what one memory region is being used for.
type Usage struct {
	// Name is the region's name as the linker script gave it.
	Name string
	// Origin and Length are the region itself, or zero when nothing declared one.
	Origin uint64
	Length uint64
	// Used is the bytes charged to the region.
	Used uint64
	// FromLoad is how much of that is a section's initializer image rather than
	// storage the program reads at run time. Reported because the two answer
	// different questions: a project that can shrink its initializers has a
	// different lever from one that can shrink its runtime state.
	FromLoad uint64
	// Declared says whether the region's geometry came from a linker script. A
	// region without it is named after what was placed in it, which is an
	// inference and is labelled as one in the report.
	Declared bool
	// Ambiguous says more than one region contains this one's addresses.
	Ambiguous bool
	// Symbols is the region's symbols, largest first.
	Symbols []PlacedSymbol
}

// Analysis is a program's memory, by region.
type Analysis struct {
	// Target is the board or chip this was built for, when it is known.
	Target string
	// Usage is every region, ordered as the linker script declared them so that a
	// report reads in the same order as the script.
	Usage []Usage
	// Artifacts is what was read to produce this.
	Artifacts Artifacts
	// Diagnostics is everything that could not be established.
	Diagnostics []string
	// Source is `map`, `elf` or `map+elf`.
	Source string
}

// Artifacts is which build outputs were read, and where from.
//
// Reported because a total without its provenance is a number a reader has to
// trust: whether a figure came from a linker map or from inferred section names
// changes what it is worth.
type Artifacts struct {
	MapFile string
	ELFFile string
}

// Total is the sum of every region's usage.
func (a Analysis) Total() uint64 {
	var total uint64
	for _, one := range a.Usage {
		total += one.Used
	}
	return total
}

// Region is one region's usage by name, and whether there is one.
func (a Analysis) Region(name string) (Usage, bool) {
	for _, one := range a.Usage {
		if strings.EqualFold(one.Name, name) {
			return one, true
		}
	}
	return Usage{}, false
}

// Analyze attributes a linked program to memory regions.
//
// The map is read when there is one, because it is what the linker wrote and it
// names the regions. The ELF is read when there is one, because it is what will
// be flashed and its symbols are the same symbols the map lists — reading both is
// how a symbol the map places but the ELF omits is still counted.
func Analyze(mapFile, elfFile, target string) (*Analysis, error) {
	out := &Analysis{Target: target, Source: "none"}
	var linked *LinkMap
	var program *Program

	if mapFile != "" {
		parsed, err := ParseMap(mapFile)
		if err != nil {
			return nil, fmt.Errorf("read the linker map: %w", err)
		}
		linked = parsed
		out.Artifacts.MapFile = mapFile
		out.Source = "map"
		out.Diagnostics = append(out.Diagnostics, parsed.Diagnostics...)
	}
	if elfFile != "" {
		parsed, err := ParseELF(elfFile)
		if err != nil {
			// A program that cannot be read is reported and the map is used on its
			// own, because a map alone still answers the budget question.
			out.Diagnostics = append(out.Diagnostics, err.Error())
		} else {
			program = parsed
			out.Artifacts.ELFFile = elfFile
			if out.Source == "map" {
				out.Source = "map+elf"
			} else {
				out.Source = "elf"
			}
		}
	}
	if linked == nil && program == nil {
		return nil, fmt.Errorf("neither a linker map nor a program was given, so there is nothing to attribute")
	}

	regions := regionsOf(linked, program)
	if len(regions) == 0 {
		out.Diagnostics = append(out.Diagnostics,
			"no memory regions are known: the build produced no linker map, and an ELF names sections rather than regions. "+
				"Declare them under `mem.regions` to have anything checked against a budget.")
	}

	usage := newLedger(regions)
	attributeSections(out, usage, linked, program)
	attributeSymbols(out, usage, linked, program)
	return out, nil
}

// regionsOf is the memory map, from the linker script if there was one and
// inferred from the sections if there was not.
func regionsOf(linked *LinkMap, program *Program) []Region {
	if linked != nil && len(linked.Regions) > 0 {
		return linked.Regions
	}
	if program == nil {
		return nil
	}
	// Grouped by address, so that the sections that run together are one region
	// rather than one each: a firmware's `.text` and `.rodata` are one flash
	// budget, and reporting them separately would make the bar meaningless.
	type span struct {
		origin uint64
		length uint64
	}
	spans := map[string]*span{}
	for _, section := range program.Sections {
		name := regionNameFromSection(section.Name)
		existing, seen := spans[name]
		if !seen {
			spans[name] = &span{origin: section.Address, length: section.Size}
			continue
		}
		if section.Address < existing.origin {
			existing.length += existing.origin - section.Address
			existing.origin = section.Address
		}
		if end := section.Address + section.Size; end > existing.origin+existing.length {
			existing.length = end - existing.origin
		}
	}
	regions := []Region{}
	for name, one := range spans {
		regions = append(regions, Region{Name: name, Origin: one.origin, Length: one.length})
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].Origin < regions[j].Origin })
	return regions
}

// ledger is the per-region accumulator: the regions by name, and the order they
// were declared in.
//
// The order is kept because two rules depend on it — a report reads in the order
// the linker script declared the regions, and a tie between two overlapping ones
// is broken by declaration order. Both would be accidents of map iteration
// otherwise, and a report that reordered itself between two runs of the same
// binary is one nobody can read twice.
//
// Usage is a span rather than a sum, which is the one decision that makes these
// figures add up to what is on the board. A link pads: between two sections, and
// inside one to align a member. Those bytes are in the image, and the linker says
// so — as `*fill*` entries inside a section that already counts them, and as gaps
// between sections that nothing counts at all. Summing section sizes therefore
// double-counts the first and drops the second, and the result is a number that is
// neither the file nor the board.
//
// The span — from the lowest address charged to a region to the highest — is right
// by construction: it counts every byte between them once, which is exactly the
// bytes that region holds.
type ledger struct {
	order  []string
	byName map[string]*Usage
	spans  map[string]span
}

// span is the address range a region is charged.
type span struct {
	start uint64
	end   uint64
	has   bool
}

// newLedger is an accumulator over the regions, in declaration order.
func newLedger(regions []Region) *ledger {
	out := &ledger{byName: map[string]*Usage{}, spans: map[string]span{}}
	for _, region := range regions {
		out.put(&Usage{
			Name: region.Name, Origin: region.Origin, Length: region.Length, Declared: true,
		})
	}
	return out
}

// put adds a region, keeping declaration order and not replacing one already
// there.
func (l *ledger) put(region *Usage) *Usage {
	if existing, seen := l.byName[region.Name]; seen {
		return existing
	}
	l.byName[region.Name] = region
	l.order = append(l.order, region.Name)
	return region
}

// charge is `size` bytes at `start`, extending the region's span.
func (l *ledger) charge(region *Usage, start, size uint64) {
	if region == nil || size == 0 {
		return
	}
	current := l.spans[region.Name]
	if !current.has {
		current.start, current.has = start, true
	}
	if end := start + size; end > current.end {
		current.end = end
	}
	l.spans[region.Name] = current
}

// names is every region name in declaration order.
func (l *ledger) names() []string { return l.order }

// finalize turns the spans into the reported usage.
func (l *ledger) finalize() []Usage {
	out := make([]Usage, 0, len(l.order))
	for _, name := range l.order {
		region := l.byName[name]
		if charged, ok := l.spans[name]; ok {
			region.Used = charged.end - charged.start
		}
		out = append(out, *region)
	}
	return out
}

// attributeSections charges each placed section to the regions it occupies.
func attributeSections(out *Analysis, usage *ledger, linked *LinkMap, program *Program) {
	if linked != nil {
		zeroInit := zeroInitialised(program)
		for _, section := range linked.Sections {
			if section.Size == 0 {
				continue
			}
			// Where it runs.
			usage.charge(regionFor(usage, section.Address, out), section.Address, section.Size)
			// Where its initializer image is stored, which is a different region for
			// a section whose variables do not live where its contents do — unless
			// it stores nothing at all, in which case it has no initializer image
			// and the load address is only where one would have gone.
			if section.HasLoad && section.LoadAddress != section.Address && !zeroInit[section.Name] {
				usage.charge(regionFor(usage, section.LoadAddress, out), section.LoadAddress, section.Size)
			}
		}
		return
	}
	if program == nil {
		return
	}
	for _, section := range program.Sections {
		if section.Size == 0 {
			continue
		}
		usage.charge(regionFor(usage, section.Address, out), section.Address, section.Size)
	}
}

// zeroInitialised is which sections occupy memory without storing anything.
//
// From the ELF where there is one, because `SHT_NOBITS` is exactly the question
// and the ELF answers it. From the names a linker script always uses where there
// is not, because a program built without one still has a `.bss` — and charging it
// to flash would overstate the image by however large it is, which for a firmware
// with a big zero-initialised table is a large part of a flash budget.
func zeroInitialised(program *Program) map[string]bool {
	if program != nil && len(program.Sections) > 0 {
		return program.ZeroInit
	}
	return map[string]bool{
		".bss": true, ".noinit": true, "._user_heap_stack": true, "._user_heap": true,
	}
}

// attributeSymbols places each symbol, so that a region's usage can be read as a
// list of the things in it rather than as one number.
//
// The map is preferred where both have a symbol, because the map knows which
// object each came from and which section it was placed in.
func attributeSymbols(out *Analysis, usage *ledger, linked *LinkMap, program *Program) {
	place := func(symbol PlacedSymbol) {
		if symbol.Size == 0 {
			return
		}
		if one := regionFor(usage, symbol.Address, out); one != nil {
			symbol.Region = one.Name
			one.Symbols = append(one.Symbols, symbol)
		}
	}
	if linked != nil {
		seen := map[string]bool{}
		for _, symbol := range linked.Symbols {
			seen[symbolKey(symbol)] = true
			seen[symbol.Name] = true
			place(symbol)
		}
		if program != nil {
			// The same symbols, from the program the link produced. Counted once:
			// the map's answer is the better one because it knows which object each
			// came from, and adding the ELF's would double the whole image while
			// changing nothing about it.
			//
			// Matched by name alone, because an ELF has no object attribution: a
			// symbol the map placed under one object and the ELF lists under
			// another is one symbol, and the two names agreeing is the evidence.
			for _, symbol := range program.Symbols {
				if seen[symbol.Name] || seen[symbolKey(symbol)] {
					continue
				}
				place(symbol)
			}
		}
	} else if program != nil {
		for _, symbol := range program.Symbols {
			place(symbol)
		}
	}

	// Ordered largest first, which is the order the report prints and the order a
	// reader wants them in.
	for _, one := range usage.byName {
		sort.Slice(one.Symbols, func(i, j int) bool {
			if one.Symbols[i].Size != one.Symbols[j].Size {
				return one.Symbols[i].Size > one.Symbols[j].Size
			}
			return one.Symbols[i].Name < one.Symbols[j].Name
		})
	}
	out.Usage = usage.finalize()
}

// symbolKey is what makes two descriptions of one symbol recognisable as one.
func symbolKey(symbol PlacedSymbol) string {
	return symbol.Name + "\x00" + symbol.Object
}

// regionFor is the region an address is charged to.
//
// The most specific region that contains the address, and the first declared
// among equals. Both rules exist for the same reason: a linker script may declare
// overlapping regions, and picking one arbitrarily would make a budget measure a
// region nobody intended.
func regionFor(usage *ledger, address uint64, out *Analysis) *Usage {
	best := (*Usage)(nil)
	var bestLength uint64
	var alsoFit []string
	for _, name := range usage.names() {
		region := usage.byName[name]
		if region.Length == 0 || address < region.Origin || address >= region.Origin+region.Length {
			continue
		}
		if best == nil || region.Length < bestLength {
			// A smaller region that fits is the more specific one, so it is the
			// more useful answer: a general-purpose region declared across a
			// specific one is a description in general terms.
			if best != nil {
				alsoFit = append(alsoFit, best.Name)
			}
			best, bestLength = region, region.Length
			continue
		}
		if region.Length == bestLength {
			alsoFit = append(alsoFit, region.Name)
		}
	}
	if best == nil {
		// An address in no region is still an address somebody spent. It is
		// reported as its own region rather than dropped, because a byte that is in
		// no budgeted region is how an image outgrows one without any budget
		// noticing.
		return usage.put(&Usage{Name: "UNMAPPED"})
	}
	// One note per pair, naming which one was used and why — a reader who has to
	// work it out from a per-address rule cannot tell whether the number in the
	// report is the region they thought.
	for _, other := range alsoFit {
		noteOverlap(out, best.Name, other)
	}
	return best
}

// noteOverlap records that two regions cover the same addresses.
func noteOverlap(out *Analysis, used, other string) {
	message := fmt.Sprintf(
		"%q and %q cover the same addresses, so a byte here belongs to both; "+
			"it is charged to %q, which the linker script declared first",
		used, other, used)
	for _, existing := range out.Diagnostics {
		if existing == message {
			return
		}
	}
	out.Diagnostics = append(out.Diagnostics, message)
	if one, ok := findUsage(out, other); ok {
		one.Ambiguous = true
	}
	if one, ok := findUsage(out, used); ok {
		one.Ambiguous = true
	}
}

// findUsage is a region's usage by name, among the ones reported.
func findUsage(out *Analysis, name string) (*Usage, bool) {
	for index := range out.Usage {
		if out.Usage[index].Name == name {
			return &out.Usage[index], true
		}
	}
	return nil, false
}
