//
// @file cmake.go
// @brief Writing what a build needs from a target spec.
//
// A spec is data, and a build reads files. This is the step between: the files a
// CMake build needs to cross-compile for a board, laid out the way CMake and the
// linker expect them.
//
//	toolchain.cmake  which compiler, and that the system is not the host's
//	target.cmake     the defines, the flags, the memory map, and how to flash
//	linker.ld        the sections and regions the linker lays a program out in
//
// The toolchain file is written to be included before `project()`, and the target
// file after: CMake reads the compiler from the first and the flags from the
// second, and a file that mixed them would have to be included twice.
//
// A linker script is generated from the memory map when the target does not name
// its own. That is the whole point of writing the boundaries down: the numbers a
// chip has are in one place, and the script that uses them is derived.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhtarget

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// File is one file a target writes.
type File struct {
	// Path is where it goes, relative to the project.
	Path string
	// Content is the file.
	Content string
}

// Files are the files a build needs for a target, in the order they are written.
func Files(spec Spec, directory string) []File {
	prefix := strings.TrimSuffix(directory, "/")
	if prefix != "" {
		prefix += "/"
	}
	files := []File{
		{Path: prefix + "toolchain.cmake", Content: ToolchainCMake(spec)},
		{Path: prefix + "target.cmake", Content: TargetCMake(spec, prefix+"linker.ld")},
	}
	if spec.LinkerScript == "" && len(spec.LinkRegions()) > 0 {
		files = append(files, File{Path: prefix + "linker.ld", Content: LinkerScript(spec)})
	}
	return files
}

/* -------------------------------------------------------------------------- */
/*  toolchain.cmake                                                            */
/* -------------------------------------------------------------------------- */

// ToolchainCMake is the file that makes CMake cross-compile.
//
// CMake has to be told that the system it builds for is not the system it runs
// on, or it compiles for the host and the flags for a Cortex-M reach a compiler
// that cannot use them.
func ToolchainCMake(spec Spec) string {
	var b strings.Builder
	banner(&b, "toolchain.cmake", spec, "What makes CMake cross-compile for this target.")
	b.WriteString("# The system is not the one CMake runs on, so nothing is looked for in the\n")
	b.WriteString("# host's libraries and nothing is run to test a feature.\n")
	b.WriteString("set(CMAKE_SYSTEM_NAME Generic)\n")
	b.WriteString("set(CMAKE_SYSTEM_PROCESSOR " + cmakeQuote(processor(spec)) + ")\n\n")
	b.WriteString("set(CMAKE_TRY_COMPILE_TARGET_TYPE STATIC_LIBRARY)\n\n")
	if spec.Toolchain.Compiler != "" {
		b.WriteString("set(CMAKE_C_COMPILER " + cmakeQuote(spec.Toolchain.Compiler) + ")\n")
	}
	b.WriteString("set(CMAKE_C_STANDARD 11)\n")
	b.WriteString("set(CMAKE_C_STANDARD_REQUIRED ON)\n")
	// A freestanding build has no C library to look for, and CMake's default
	// search finds the host's, which then fails to link.
	b.WriteString("set(CMAKE_C_FLAGS_INIT \"-ffreestanding\")\n")
	return b.String()
}

// processor is what CMake calls the processor: the core, which is the fact that
// decides which compiler is right.
func processor(spec Spec) string {
	if spec.Core != "" {
		return spec.Core
	}
	return "generic"
}

/* -------------------------------------------------------------------------- */
/*  target.cmake                                                               */
/* -------------------------------------------------------------------------- */

// TargetCMake is the file that gives a build this target's definitions, flags,
// memory and flashing.
func TargetCMake(spec Spec, linkerScript string) string {
	var b strings.Builder
	banner(&b, "target.cmake", spec, "The flags, memory map and flashing of this target.")

	defines := append([]string{}, spec.Defines...)
	sort.Strings(defines)
	if len(defines) > 0 {
		b.WriteString("# The macros a program is compiled with: which chip this is, and how\n")
		b.WriteString("# fast its clock runs before anything configures it.\n")
		for _, define := range defines {
			fmt.Fprintf(&b, "add_compile_definitions(%s)\n", define)
		}
		b.WriteString("\n")
	}

	flags := spec.AllFlags()
	if len(flags) > 0 {
		b.WriteString("# The flags the core is built with, after the ones the architecture gives.\n")
		b.WriteString("#\n")
		b.WriteString("# They are given to the link as well as to the compile, because not every\n")
		b.WriteString("# flag is only a compile flag. `-specs=nosys.specs` is what replaces the hosted C\n")
		b.WriteString("# library's syscall calls with ones that return an error rather than call, and\n")
		b.WriteString("# a build that gave it only to the compiler linked the hosted library anyway\n")
		b.WriteString("# and failed on `_getpid` — a symbol whose absence says nothing about the flag\n")
		b.WriteString("# that was in the wrong place.\n")
		b.WriteString("add_compile_options(\n")
		for _, flag := range flags {
			fmt.Fprintf(&b, "    %s\n", flag)
		}
		b.WriteString(")\n")
		b.WriteString("add_link_options(\n")
		for _, flag := range flags {
			fmt.Fprintf(&b, "    %s\n", flag)
		}
		b.WriteString(")\n\n")
	}

	regions := spec.Regions()
	if len(regions) > 0 {
		b.WriteString("# The memory map, which the linker script is generated from and a program\n")
		b.WriteString("# may read: where it is stored, and how much room it has.\n")
		for _, region := range regions {
			fmt.Fprintf(&b, "set(RHO_%s_ORIGIN %s)\n", region.Name, region.Region.Origin)
			fmt.Fprintf(&b, "set(RHO_%s_LENGTH %s)\n", region.Name, region.Region.Length)
		}
		fmt.Fprintf(&b, "set(RHO_FLASH_ORIGIN %s)\n", spec.Memory.Flash.Origin)
		fmt.Fprintf(&b, "set(RHO_FLASH_LENGTH %s)\n\n", spec.Memory.Flash.Length)
	}

	// The script comes from the target's own file when it names one, from the
	// `linker:` block when that names one, and from the memory map when neither
	// does.
	//
	// Three ways to get a script, in the order a person reaches for them: the
	// generated one, which is right for most programs; the `linker:` block, which
	// changes numbers in it; and a `.ld` of one's own, for the layouts that cannot
	// be reached by changing numbers at all.
	switch {
	case spec.LinkerScript != "":
		fmt.Fprintf(&b, "set(RHO_LINKER_SCRIPT ${CMAKE_CURRENT_SOURCE_DIR}/%s)\n", spec.LinkerScript)
	case spec.Linker.Script != "":
		// A path outside the project is taken as written, so that a platform can
		// ship one script and every project that uses it points at the same file.
		if filepath.IsAbs(spec.Linker.Script) {
			fmt.Fprintf(&b, "set(RHO_LINKER_SCRIPT %s)\n", spec.Linker.Script)
		} else {
			fmt.Fprintf(&b, "set(RHO_LINKER_SCRIPT ${CMAKE_CURRENT_SOURCE_DIR}/%s)\n", spec.Linker.Script)
		}
	case len(spec.LinkRegions()) > 0:
		fmt.Fprintf(&b, "set(RHO_LINKER_SCRIPT ${CMAKE_CURRENT_SOURCE_DIR}/%s)\n", linkerScript)
	}
	b.WriteString("if(DEFINED RHO_LINKER_SCRIPT)\n")
	b.WriteString("  add_link_options(-T${RHO_LINKER_SCRIPT})\n")
	b.WriteString("endif()\n\n")

	if spec.LinkMap {
		// The map, and the cross-reference table beside it.
		//
		// `-Map` says where every byte of the image went: which region it landed
		// in, which object file put it there, and how much of it there is. That is
		// the difference between a size and an answer — a build that reports
		// "image is 14 kB too large" and no way to find out what is in it has
		// given half the information.
		//
		// `--cref` is what turns the map into an attribution: which object refers
		// to which symbol. A symbol's size says how much it costs; the cross
		// reference says who pulled it in, which is the question a budget overrun
		// actually raises — a driver that was not meant to be linked at all is a
		// different problem from one that is simply large.
		b.WriteString("# The linker map, and the cross-reference table that attributes each\n")
		b.WriteString("# symbol to the object that pulled it in. This is what a memory\n")
		b.WriteString("# analysis reads: without it a build reports a size and nothing about\n")
		b.WriteString("# what is in it.\n")
		fmt.Fprintf(&b, "add_link_options(-Wl,-Map=${CMAKE_BINARY_DIR}/%s.map -Wl,--cref)\n", cmakeMapName(spec.Name))
		b.WriteString("\n")
	}

	if spec.VendorLibrary != "" {
		b.WriteString("# The library that implements this board's interfaces: the platform-agnostic\n")
		b.WriteString("# calls a program makes resolve to it.\n")
		fmt.Fprintf(&b, "set(RHO_VENDOR_LIBRARY %s)\n\n", spec.VendorLibrary)
	}
	if spec.Flash.Tool != "" {
		b.WriteString("# How a built program reaches the board. The binary is named as {binary}.\n")
		fmt.Fprintf(&b, "set(RHO_FLASH_TOOL %s)\n", spec.Flash.Tool)
		fmt.Fprintf(&b, "set(RHO_FLASH_COMMAND %s)\n", cmakeQuote(spec.Flash.Command))
		if spec.Flash.Offset != "" {
			fmt.Fprintf(&b, "set(RHO_FLASH_OFFSET %s)\n", spec.Flash.Offset)
		}
		b.WriteString("add_custom_target(flash\n")
		b.WriteString("  COMMAND ${RHO_FLASH_TOOL} ${RHO_FLASH_COMMAND}\n")
		b.WriteString("  COMMENT \"Writing the program to the board\"\n")
		b.WriteString("  VERBATIM\n")
		b.WriteString(")\n")
	}
	return b.String()
}

/* -------------------------------------------------------------------------- */
// cmakeMapName is the file a link writes its map to, named after the target.
//
// Named after the target rather than after the program because the program's
// name is a project's choice and the target's is not, and a build tree holds one
// build per target: `nucleo-h753zi.map` is found by knowing which board was
// built, where `<whatever-the-cmake-project-is-called>.map` is not.
func cmakeMapName(target string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(target))
	if name == "" {
		return "linker"
	}
	return name
}

/*  linker.ld                                                                  */
/* -------------------------------------------------------------------------- */

// LinkerScript renders the memory map and the sections a program is laid out in.
//
// It is a script for a freestanding program: the vector table the platform puts
// at the start of flash, then the code, then the data that is copied out of the
// flash into RAM before `main` runs, and then the space the stack grows down
// into. The address the stack starts at is the top of RAM, which is why the
// RAM region has to be the one the stack lives in.
func LinkerScript(spec Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/* Generated by rho-target from the %s target spec. DO NOT EDIT.\n", spec.Name)
	if spec.Comment != "" {
		fmt.Fprintf(&b, " *\n * %s\n", firstSentence(spec.Comment))
	}
	fmt.Fprintf(&b, " *\n * Regenerate with:\n *\n *     rho target generate <project> --target %s\n */\n\n", spec.Name)

	regions := convergeRegions(spec)
	b.WriteString("MEMORY\n{\n")
	for _, region := range regions {
		suffix := ""
		if region.Region.Comment != "" {
			suffix = " /* " + firstSentence(region.Region.Comment) + " */"
		}
		if region.Declared {
			suffix += " /* set by the target's linker block */"
		}
		fmt.Fprintf(&b, "    %s (r%s): ORIGIN = %s, LENGTH = %s%s\n",
			region.Name, access(region.Kind), region.Region.Origin, region.Region.Length, suffix)
	}
	b.WriteString("}\n\n")

	named := make([]NamedRegion, 0, len(regions))
	for _, region := range regions {
		named = append(named, region.NamedRegion)
	}
	flash, hasFlash := flashRegion(named)
	ram, hasRAM := ramRegion(named)
	if hasFlash {
		fmt.Fprintf(&b, "/* The stack grows down from the end of %s. */\n", ram.Name)
		fmt.Fprintf(&b, "_estack = ORIGIN(%s) + LENGTH(%s);\n", ram.Name, ram.Name)
		// The handlers the core may vector to, demanded from the libraries.
		//
		// A vendor startup assembly declares each of these weak and assigns the
		// address of its own default handler — an instruction to spin. That is a
		// *definition*, so by the time the linker sweeps a static library looking
		// for a strong one it has no unresolved symbol left to look for, and the
		// object that implements the handler is discarded whole.
		//
		// The symptom is the worst kind: the board boots, runs, and stops a
		// millisecond later when the first tick arrives, having recorded no fault
		// of any kind, because none occurred. Naming the symbols here makes them
		// undefined references, which is what makes the linker look.
		//
		// A handler a platform does not implement resolves to the default one,
		// exactly as before — the demand is not a claim that each one exists.
		b.WriteString("/* Demand the handlers the core vectors to, so a library that implements one\n")
		b.WriteString("   is not discarded for looking unnecessary. See the note on each. */\n")
		for _, handler := range []string{
			"Reset_Handler",
			"NMI_Handler",
			"HardFault_Handler",
			"MemManage_Handler",
			"BusFault_Handler",
			"UsageFault_Handler",
			"SVC_Handler",
			"DebugMon_Handler",
			"PendSV_Handler",
			"SysTick_Handler",
		} {
			fmt.Fprintf(&b, "EXTERN(%s)\n", handler)
		}
		b.WriteString("\n")
		entry := strings.TrimSpace(spec.Linker.Entry)
		if entry == "" {
			entry = "Reset_Handler"
		}
		fmt.Fprintf(&b, "ENTRY(%s)\n\n", entry)
	}
	if hasRAM {
		// The stack and the heap share what is left of RAM after the static data,
		// and these are how much of it each takes. They are defined here rather
		// than inside SECTIONS, because an assignment there is a section-local
		// symbol and the linker rejects one that is not inside a section — which
		// is a syntax error in the generated script rather than anything to do
		// with the memory map.
		fmt.Fprintf(&b, "_Min_Heap_Size  = %s;\n_Min_Stack_Size = %s;\n\n",
			hexBytes(spec.Linker.HeapBytes, DefaultHeapBytes), hexBytes(spec.Linker.StackBytes, DefaultStackBytes))
	}

	b.WriteString("SECTIONS\n{\n")
	if hasFlash {
		fmt.Fprintf(&b, "    .isr_vector :\n    {\n        KEEP(*(.isr_vector))\n        . = ALIGN(4);\n    } > %s\n\n", flash.Name)
		fmt.Fprintf(&b, "    .text :\n    {\n        KEEP(*(.init))\n        KEEP(*(.fini))\n        *(.text*)\n        *(.glue_7*)\n        *(.glue_7t*)\n        *(.eh_frame*)\n        *(.rodata*)\n        . = ALIGN(4);\n        _etext = .;\n    } > %s\n\n", flash.Name)
		// The C runtime walks four tables of function pointers, and a script that
		// does not place them leaves the linker to scatter them wherever it
		// likes: __libc_init_array then calls through whatever it finds, which is
		// how a program ends up jumping into an address no section covers. They
		// are placed here with the bounds the runtime is given, and the glue
		// sections the ARM toolchain emits beside .text go with them.
		b.WriteString("    .preinit_array :\n    {\n        PROVIDE_HIDDEN(__preinit_array_start = .);\n        KEEP(*(.preinit_array*))\n        PROVIDE_HIDDEN(__preinit_array_end = .);\n    } > " + flash.Name + "\n")
		b.WriteString("    .init_array :\n    {\n        PROVIDE_HIDDEN(__init_array_start = .);\n        KEEP(*(SORT(.init_array.*)))\n        KEEP(*(.init_array*))\n        PROVIDE_HIDDEN(__init_array_end = .);\n    } > " + flash.Name + "\n")
		b.WriteString("    .fini_array :\n    {\n        PROVIDE_HIDDEN(__fini_array_start = .);\n        KEEP(*(SORT(.fini_array.*)))\n        KEEP(*(.fini_array*))\n        PROVIDE_HIDDEN(__fini_array_end = .);\n    } > " + flash.Name + "\n\n")
		fmt.Fprintf(&b, "    .ARM.extab :\n    { *(.ARM.extab* .gnu.linkonce.armextab.*) } > %s\n", flash.Name)
		fmt.Fprintf(&b, "    .ARM.exidx :\n    { __exidx_start = .; *(.ARM.exidx* .gnu.linkonce.armexidx.*) __exidx_end = .; } > %s\n\n", flash.Name)
	}
	// Sections the project declared, placed where it said.
	//
	// After the sections the toolchain owns and before the heap and stack, so
	// that a declared section lands above the automatic ones: a project adding a
	// region of its own means it, not the C library's `.bss`, that is at risk of
	// running out of room, and a section placed below the stack would be laid out
	// in memory the stack can reach.
	for _, section := range spec.Linker.Sections {
		writeDeclaredSection(&b, section)
	}
	if hasFlash && hasRAM {
		// Initialised data lives in RAM and is copied out of the flash before
		// `main` runs, which is what the load address is for.
		//
		// `_sidata` is the symbol that load address, and every CMSIS startup
		// assembly references it — the vector table carries it as a word, and
		// Reset_Handler loads it to perform the copy. A script that omits it
		// links everything else and then fails on one undefined reference, which
		// is the point at which the generated project stops building for want of
		// a symbol the vendor's own startup cannot do without.
		fmt.Fprintf(&b, "    .data :\n    {\n        . = ALIGN(4);\n        _sdata = .;\n        *(.data*)\n        . = ALIGN(4);\n        _edata = .;\n    } > %s AT> %s\n\n", ram.Name, flash.Name)
		fmt.Fprintf(&b, "    /* The address in flash the initialised data is copied from. */\n    _sidata = LOADADDR(.data);\n\n")

		// Zeroed data is cleared between _sbss and _ebss. The double-underscore
		// names are the same two addresses: some CMSIS startups use one spelling
		// and some the other, and defining both costs nothing and saves a link
		// failure on whichever family turns out to want the other.
		fmt.Fprintf(&b, "    .bss :\n    {\n        . = ALIGN(4);\n        _sbss = .;\n        __bss_start__ = _sbss;\n        *(.bss*)\n        *(COMMON)\n        . = ALIGN(4);\n        _ebss = .;\n        __bss_end__ = _ebss;\n    } > %s\n\n", ram.Name)

		// A stack and a heap need somewhere to grow, and the stack is only as big
		// as the region left after the heap. ST's own scripts reserve both after
		// the static data; a script that does not lets the stack collide with
		// the heap, which is a fault that only appears once the program is big
		// enough to reach it.
		//
		// Both `end` and `_end`, with `=` and not a comma. newlib's `_sbrk` looks
		// for `end` and the C library's own allocator for `_end`, so a script
		// that provides only one of them links a firmware whose `malloc` cannot
		// find the heap. A comma there is a syntax error in the generated file
		// rather than anything to do with the memory map.
		fmt.Fprintf(&b, "    ._user_heap_stack :\n    {\n        . = ALIGN(8);\n        PROVIDE(end = .);\n        PROVIDE(_end = .);\n        . = . + _Min_Heap_Size;\n        . = . + _Min_Stack_Size;\n        . = ALIGN(8);\n    } > %s\n\n", ram.Name)
	}
	b.WriteString("}\n")
	for _, name := range sortedKeys(spec.Linker.Symbols) {
		fmt.Fprintf(&b, "\n/* Declared by the target's linker block. */\n%s = %s;\n", name, spec.Linker.Symbols[name])
	}
	return b.String()
}

// writeDeclaredSection is one section a project placed for itself.
//
// Written the way the generated sections are written rather than through a
// template of its own, so that a declared section and a generated one are
// indistinguishable in the script — which is what keeps a reader of a generated
// linker script able to read it.
func writeDeclaredSection(b *strings.Builder, section LinkerSection) {
	fmt.Fprintf(b, "\n    %s :\n    {\n", section.Name)
	for _, flag := range section.Flags {
		fmt.Fprintf(b, "        %s\n", flag)
	}
	for _, source := range section.Sources {
		fmt.Fprintf(b, "        %s\n", source)
	}
	if section.Align > 0 {
		fmt.Fprintf(b, "        . = ALIGN(%s);\n", hexBytes(section.Align, 0))
	}
	fmt.Fprintf(b, "    } > %s", section.Region)
	if section.At != "" {
		fmt.Fprintf(b, " AT> %s", section.At)
	}
	if section.Noload {
		// A section that reserves address space and stores nothing there, which
		// is what a stack is. Without this the linker would write zeroes into it.
		b.WriteString(" (NOLOAD)")
	}
	b.WriteString("\n\n")
}

// The stack and heap a target gets when it says nothing.
//
// A guess, and the one this project exists to stop making: 1 kB of stack is
// enough for most programs and silently too little for a deep call graph or a
// large local array, and a program that overruns it does not fail — it corrupts
// whatever is below it and carries on. For a Rho program the stack is computed
// from the call graph instead; these are what a target with no such analysis
// falls back to.
const (
	// DefaultStackBytes is the stack a target gets when nothing says otherwise.
	DefaultStackBytes = 0x400
	// DefaultHeapBytes is the heap a target gets when nothing says otherwise.
	DefaultHeapBytes = 0x200
)

// hexBytes is a size as the linker script writes one: a hexadecimal number.
func hexBytes(value, fallback uint64) string {
	if value == 0 {
		value = fallback
	}
	return fmt.Sprintf("0x%x", value)
}

// sortedKeys is a map's keys in order, so that a generated script is the same
// file on every machine.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// convergedRegion is one `MEMORY` entry and where it came from.
//
// A `NamedRegion` with one extra bit: `Declared` says the target's `linker:`
// block supplied or changed this entry, which the script says in a comment
// because a reader of a generated linker script is entitled to know which numbers
// came from the chip and which from the project.
type convergedRegion struct {
	NamedRegion
	Declared bool
}

// convergeRegions is the chip's own memory map with the target's `linker:` block
// applied to it.
//
// Converged rather than appended, and that is the whole of the rule: a block that
// names a region the chip already has *changes* it, and a block that names one it
// does not *adds* it. Appending either would emit two `MEMORY` entries of the same
// name, which is a linker syntax error naming a line of a generated file — and
// would do so for the most ordinary thing a project could write, which is "this
// board's second bank is half the size the chip datasheet says".
//
// Order is the chip's own, with anything new appended, so that a script generated
// with a block and one generated without it differ only where the block says.
func convergeRegions(spec Spec) []convergedRegion {
	out := []convergedRegion{}
	byName := map[string]int{}
	for _, region := range spec.LinkRegions() {
		byName[region.Name] = len(out)
		out = append(out, convergedRegion{NamedRegion: region})
	}
	for _, name := range sortedKeys(spec.Linker.Regions) {
		declared := spec.Linker.Regions[name]
		if at, existing := byName[name]; existing {
			// A region the chip has, restated. A length of zero means "leave the
			// chip's figure alone", so a block can change one number without having
			// to restate the other.
			if strings.TrimSpace(declared.Length) != "" && declared.Length != "0" {
				out[at].Region.Length = declared.Length
			}
			if strings.TrimSpace(declared.Origin) != "" && declared.Origin != "0" {
				out[at].Region.Origin = declared.Origin
			}
			out[at].Declared = true
			continue
		}
		byName[name] = len(out)
		out = append(out, convergedRegion{
			NamedRegion: NamedRegion{Name: name, Kind: "rw", Region: declared},
			Declared:    true,
		})
	}
	return out
}

// access is the linker's letter for what a region may be used for: a program is
// read from flash and read and written in RAM.
func access(kind string) string {
	if kind == "flash" {
		return "x"
	}
	return "w"
}

func flashRegion(regions []NamedRegion) (NamedRegion, bool) {
	for _, region := range regions {
		if region.Kind == "flash" {
			return region, true
		}
	}
	return NamedRegion{}, false
}

func ramRegion(regions []NamedRegion) (NamedRegion, bool) {
	for _, region := range regions {
		if region.Kind == "ram" {
			return region, true
		}
	}
	return NamedRegion{}, false
}

/* -------------------------------------------------------------------------- */
/*  Text                                                                       */
/* -------------------------------------------------------------------------- */

// banner is the top of every file this component writes, so a person who opens
// one knows what wrote it and what to change instead.
func banner(b *strings.Builder, name string, spec Spec, what string) {
	fmt.Fprintf(b, "# Generated by rho-target from the %s target spec. DO NOT EDIT.\n", spec.Name)
	if spec.Core != "" {
		fmt.Fprintf(b, "# %s\n", spec.Core)
	}
	fmt.Fprintf(b, "# %s\n", what)
	b.WriteString("#\n")
	b.WriteString("# Regenerate with:\n#\n#     rho target generate <project> --target " + spec.Name + "\n#\n\n")
}

// cmakeQuote is a value CMake reads as one argument.
func cmakeQuote(value string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"") + "\""
}

// firstSentence is a comment cut to what fits on one line of a generated file.
func firstSentence(text string) string {
	trimmed := strings.Join(strings.Fields(text), " ")
	if index := strings.Index(trimmed, ". "); index >= 0 {
		return trimmed[:index+1]
	}
	return trimmed
}
