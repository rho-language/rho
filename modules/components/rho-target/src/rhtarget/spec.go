//
// @file spec.go
// @brief Board and architecture specifications, and how they compose.
//
// A build for a dev board needs facts that belong to the board and not to the
// project: which compiler, which flags, where the flash is, how much RAM there
// is, and which pin is the LED. Those facts are the same for every project that
// targets the board, so they live in a spec rather than in every project.
//
// A spec is written once and refers to another: a board names the chip it
// carries, and a chip names the architecture it is built as.
//
//	stm32f4        the chip: core, toolchain, memory map
//	pico           the board: the chip, the vendor library, the pin map
//	esp32c3        a chip that is also its own board
//
// Resolution walks that chain and folds it into one spec, so a caller reads a
// single answer: a board with the toolchain of its chip and the flags of its
// architecture.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhtarget

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/yaml"
)

// Kind is what a spec describes.
type Kind string

const (
	// KindArch is an architecture: the flags every core of it is built with.
	KindArch Kind = "arch"
	// KindMCU is a chip: a core, a toolchain and a memory map.
	KindMCU Kind = "mcu"
	// KindBoard is a board: a chip, the library that implements its interfaces,
	// and the pins a program names.
	KindBoard Kind = "board"
)

// Spec is one target: an architecture, a chip or a board.
type Spec struct {
	Kind    Kind   `json:"kind"`
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
	// Arch is the architecture a chip is built as, and the spec it inherits its
	// flags from.
	Arch string `json:"arch,omitempty"`
	// Preset is the family spec this one is built for, when a board names its family
	// through its preset rather than through `mcu` or `arch`.
	//
	// Read as a link in the chain when it names a spec, and as the build preset it
	// otherwise is. Those are the same string for a platform whose presets are named
	// after their families, which is what lets one field carry both meanings without
	// either of them being wrong about the other.
	Preset string `json:"preset,omitempty"`
	// MCU is the chip a board carries.
	MCU string `json:"mcu,omitempty"`
	// ResolvedFrom names the specs this one was resolved from, nearest first, so
	// a report can say what a value actually came from.
	ResolvedFrom []string `json:"-"`

	// Family is the family of parts this spec is for, as the vendor names it —
	// `STM32H7`. A platform that selects a family at compile time maps it to a
	// macro through the platform hook, so a spec states which family it is once
	// and the platform's naming lives with the platform rather than here.
	Family string `json:"family,omitempty"`
	// HALHeader is the header a vendor's HAL umbrella includes by name, such as
	// `stm32f4xx_hal.h`, or empty when this target has no such vendor.
	//
	// It is the one fact that says which family's configuration header a build
	// has to produce from the vendor's template, and the vendor's naming is not
	// derivable from a part number without guessing — so the platform states it.
	HALHeader string `json:"hal_header,omitempty"`
	// LinkMap asks the link to write a linker map beside the program it links.
	//
	// The map is what says where every byte went: which region it landed in, and
	// which object file put it there. Without it a build reports a size and
	// nothing else, which is the number a budget is written against and no way to
	// find out what to do about it when the budget moves.
	//
	// Not carried by a spec file, because a map is a property of how this build
	// was linked rather than of the part: the same board linked twice produces
	// two maps. It is set by whoever asked for the link.
	LinkMap bool `json:"-"`
	// Core is the processor core, which is a fact about the chip that a program
	// may want to know.
	Core string `json:"core,omitempty"`
	// Toolchain is how the chip is compiled and linked.
	Toolchain Toolchain `json:"toolchain,omitempty"`
	// Memory is the address space a program is laid out in.
	Memory Memory `json:"memory,omitempty"`
	// Defines are the macros a build of this target always has.
	Defines []string `json:"defines,omitempty"`
	// Flags are added to the compiler's, after the architecture's.
	Flags []string `json:"flags,omitempty"`
	// LinkerScript names the script a project supplies, when the target needs one
	// that is not generated from the memory map.
	LinkerScript string `json:"linker_script,omitempty"`
	// Linker is how this target's program is laid out in memory: where its regions
	// are, how much stack and heap it gets, and anything the generated script does
	// not place on its own.
	//
	// Declarative rather than a script, because the common reasons to want a
	// different linker script are not "a different linker script" — they are a
	// bigger stack, a region in tightly coupled memory, a section of one's own.
	// Each of those is a value here, and a program that needs something this cannot
	// express writes a `.ld` and names it in `Script` instead.
	Linker Linker `json:"linker,omitempty"`
	// VendorLibrary is the library that implements this board's interfaces, which
	// is what a build links.
	VendorLibrary string `json:"vendor_library,omitempty"`
	// Pins are the names a program uses for the pins of a board.
	Pins map[string]Pin `json:"pins,omitempty"`
	// Peripherals are the interfaces a board brings out, and the pins they are on.
	Peripherals map[string]Peripheral `json:"peripherals,omitempty"`
	// Flash is how a built program is written to the board.
	Flash Flash `json:"flash,omitempty"`
	// Debug is how a program's execution is inspected on this target.
	Debug Debug `json:"debug,omitempty"`
	// Program is a CMake fragment to place beside the generated build and
	// include once the program target exists, relative to the file that
	// declared it.
	//
	// It is where a platform says what a program needs said about itself —
	// which of a framework's stdio drivers it registers, say — without the
	// generator having to know which framework, or that there is one.
	Program string `json:"program,omitempty"`
	// Layout is how a framework arranges a program's sources, when that is not
	// the plain arrangement of one executable in the project root.
	//
	// A framework that builds by component — ESP-IDF, Zephyr — expects a
	// program to be a directory of sources with a build file that registers
	// them, not an `add_executable` at the top. The platform says so here, and
	// the generator emits that shape without knowing which framework asked for
	// it.
	Layout *Layout `json:"layout,omitempty"`
	// Source is the file the spec was read from, so a report can say where it
	// came from.
	Source string `json:"-"`
}

// Layout is the shape a program's sources take in a framework's project.
type Layout struct {
	// Dir is the directory the program's sources are built from, relative to
	// the generated project — the directory a component-based framework
	// expects a program to be.
	Dir string `json:"dir,omitempty"`
	// Register is a CMake file beside the spec that is copied into Dir and
	// included there, once the sources have been listed in
	// `RHO_PROGRAM_SOURCES` and their include directories in
	// `RHO_PROGRAM_INCLUDE_DIRS`.
	//
	// It is what turns a list of sources into whatever the framework calls a
	// buildable unit — `idf_component_register` for ESP-IDF — so that name
	// belongs to the platform and not to Rho.
	Register string `json:"register,omitempty"`
	// Sources are files beside the spec copied into Dir, for a component that
	// needs code the program does not bring — the bridge from a framework's
	// entry name to a program's, say. They travel with the register file
	// because the build file names them by a path in Dir.
	Sources []string `json:"sources,omitempty"`
}

// Toolchain is the programs and flags a chip is built with.
type Toolchain struct {
	// Compiler, Linker, Objcopy and Size are the programs. A program named
	// without a path is found on PATH.
	Compiler string `json:"compiler,omitempty"`
	Linker   string `json:"linker,omitempty"`
	Objcopy  string `json:"objcopy,omitempty"`
	Size     string `json:"size,omitempty"`
	// Flags are the architecture's own: the ones that say what the core is.
	Flags []string `json:"flags,omitempty"`
	// Prelude is a CMake file to include before `project()`, relative to the
	// file that declared it.
	//
	// It exists for a target whose toolchain is a framework's: the vendor's
	// import has to run before `project()` chooses a compiler, and it brings
	// its own from there. When it is set the rest of this block is left out —
	// the prelude owns the toolchain, and two of them setting a compiler is
	// one of them being ignored without saying so.
	Prelude string `json:"prelude,omitempty"`
}

// Linker is how a target's program is laid out in memory.
//
// Every field is optional and every one of them converges onto the same generated
// script a target with no `linker` block gets. That is the point: a project that
// wants a larger stack says so and gets the script it would otherwise have had,
// with that number changed — rather than a mechanism that replaces the script and
// so silently loses every improvement made to it since.
type Linker struct {
	// Script names a `.ld` file to use instead of the generated one, for the
	// layouts this cannot express.
	//
	// Absolute, or relative to the file that declared it. When it is set nothing
	// else here is used: a script that replaces the generated one also replaces
	// what it would have been generated with, and merging the two would produce a
	// script that is neither.
	Script string `json:"script,omitempty"`
	// StackBytes is how much stack the program gets. Zero means "as it would
	// otherwise have been" — which, for a Rho program, is the bound the compiler
	// computed from the call graph rather than a number somebody guessed.
	StackBytes uint64 `json:"stack_bytes,omitempty"`
	// HeapBytes is how much heap the program gets, and zero means as before.
	HeapBytes uint64 `json:"heap_bytes,omitempty"`
	// Entry is the symbol the program starts at. Empty is `Reset_Handler`, or the
	// entry the startup assembly declares.
	Entry string `json:"entry,omitempty"`
	// Regions are further places a program may be put, keyed by name. They are
	// declared, not placed: the generated script emits a `MEMORY` entry for each,
	// so a program may address them, and anything that wants to live in one names
	// it in `Sections`.
	Regions map[string]Region `json:"regions,omitempty"`
	// Sections are placements the generated script does not make on its own.
	Sections []LinkerSection `json:"sections,omitempty"`
	// Symbols are extra values the script defines, for a program or a library
	// that reads them. `name: value`, both written as they are in the script.
	Symbols map[string]string `json:"symbols,omitempty"`
	// ForeignFrames is what a call into this target's own C costs, by name.
	//
	// This is what makes a computed stack a bound rather than an estimate. The
	// compiler can see every frame it emitted and every call between them; it
	// cannot see inside a library written in C, because nothing holds that call
	// graph. So a target declares what its own functions cost, the analysis adds
	// those to what it read, and the number stops being a guess.
	//
	// Declared here rather than left as a single "external calls cost N" because
	// the functions are not alike: a platform's clock bring-up is deep and its
	// getter is not, and one number for both is the same guess wearing a
	// declaration's clothes.
	ForeignFrames map[string]uint64 `json:"foreign_frames,omitempty"`
}

// LinkerSection is one section placed in one region.
type LinkerSection struct {
	// Name is the output section, as the script spells it.
	Name string `json:"name"`
	// Region is where it goes, by the name a `MEMORY` entry has.
	Region string `json:"region"`
	// Sources are the input patterns it collects, as the script spells them:
	// `*(.mydata)`. Written out rather than derived because which sections exist
	// is a fact about the program, not about the linker.
	Sources []string `json:"sources,omitempty"`
	// Flags are what the script says about it: `KEEP`, `ALIGN(4)`. Left as words
	// because they are the script's own, and a tool that validated them would
	// reject the next one the linker grows.
	Flags []string `json:"flags,omitempty"`
	// Align is the alignment the section starts at, as a number. Zero is the
	// linker's own choice.
	Align uint64 `json:"align,omitempty"`
	// At is a load region for a section whose contents are stored somewhere other
	// than where they run, as `.data` is. Empty is "wherever it runs".
	At string `json:"at,omitempty"`
	// Noload marks a section that reserves address space without storing anything
	// there, which is what a stack or a heap is.
	Noload bool `json:"noload,omitempty"`
}

// Memory is the address space of a chip.
type Memory struct {
	// Flash is where a program is stored and executed from.
	Flash Region `json:"flash,omitempty"`
	// RAM is where a program's data lives.
	RAM Region `json:"ram,omitempty"`
	// Extra are the further regions a chip has, such as the core-coupled memory
	// of an STM32 or the second bank of an RP2040, keyed by the name a program
	// refers to them by.
	Extra map[string]Region `json:"extra,omitempty"`
}

// Region is one region of memory: where it starts and how long it is, as the
// numbers a linker script is written with.
type Region struct {
	Origin string `json:"origin"`
	Length string `json:"length"`
	// Comment says what the region is for, when its name does not.
	Comment string `json:"comment,omitempty"`
	// Link says whether a program may be laid out in this region. A region that
	// is real memory is, and a window onto another region is not: a cache that
	// shows the flash at another address is not somewhere to put a section.
	// Absent means yes, which is what a region of memory is.
	Link *bool `json:"link,omitempty"`
}

// Linkable reports whether a program may be laid out in this region.
func (r Region) Linkable() bool { return r.Link == nil || *r.Link }

// Pin is one pin of a board.
type Pin struct {
	// Name is the board's own name for it, such as GP25, which is what a program
	// writes.
	Name string `json:"name,omitempty"`
	// Role is what the board wires it to, such as led or uart_tx. A pin with no
	// role is one the board brings out and does not use.
	Role string `json:"role,omitempty"`
	// Comment says what is on the board at that pin, such as an LED.
	Comment string `json:"comment,omitempty"`
}

// Peripheral is an interface a board brings out.
type Peripheral struct {
	// Instance is the hardware instance, such as i2c0, which is the name a board
	// registers with the platform.
	Instance string `json:"instance,omitempty"`
	// Pins are the pins it is on, by their role in the interface: sda, scl, tx,
	// rx, sck, mosi, miso.
	Pins map[string]string `json:"pins,omitempty"`
	// Comment says what it is connected to, such as an on-board sensor.
	Comment string `json:"comment,omitempty"`
}

// Flash is how a program reaches a board.
type Flash struct {
	// Tool is the program that writes a built binary: openocd, picotool,
	// esptool.py.
	Tool string `json:"tool,omitempty"`
	// Command is how the tool is invoked, with the binary named as {binary} and
	// the spec's own values available as {name} where a caller has one.
	Command string `json:"command,omitempty"`
	// Offset is where a program is written, for a tool that writes at an address
	// rather than into a named region.
	Offset string `json:"offset,omitempty"`
	// Comment says what has to be true for flashing to work, such as a boot mode.
	Comment string `json:"comment,omitempty"`
	// Arguments is the invocation as a list, for a platform that would rather
	// not write a command string with quoting inside it. It is the same thing:
	// a command is used when there is one, and arguments when there is not.
	Arguments []string `json:"arguments,omitempty"`
	// PostArgs are the arguments the offset goes between, for a tool that takes
	// the address after the file rather than before it. Written out, because a
	// tool's argument order is the one thing about flashing that is not
	// interchangeable, and a reader guessing it would flash to the wrong place.
	PostArgs []string `json:"post_args,omitempty"`
}

// Declared reports whether the target says anything about reaching a board.
func (f Flash) Declared() bool {
	return strings.TrimSpace(f.Command) != "" || len(f.Arguments) > 0
}

// Invocation renders the flash command for a binary, as the program to run and
// the arguments to run it with.
//
// The command is split on whitespace rather than handed to a shell. A spec is
// data that a platform ships, and a spec that could name a pipeline would be a
// spec that could run whatever a machine could be talked into; the tools that
// need quoting are served by honouring quotes here instead. `{binary}` is where
// the file goes, and a command that never says so has it appended — otherwise a
// tool with a default would flash whatever that default is rather than the file
// that was just built.
func (f Flash) Invocation(binary string) (string, []string, error) {
	argv := []string{}
	switch {
	case strings.TrimSpace(f.Command) != "":
		argv = SplitCommand(f.Command)
	case len(f.Arguments) > 0:
		argv = append(argv, f.Arguments...)
	default:
		return "", nil, fmt.Errorf("the target declares no way to reach the board: neither a command nor a list of arguments")
	}
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("the target's flash command is empty")
	}

	out := make([]string, 0, len(argv)+len(f.PostArgs)+1)
	named := false
	for _, arg := range argv {
		if strings.Contains(arg, "{binary}") {
			arg = strings.ReplaceAll(arg, "{binary}", binary)
			named = true
		}
		out = append(out, arg)
	}
	if !named {
		out = append(out, binary)
	}
	for _, arg := range f.PostArgs {
		if arg == "{offset}" && f.Offset != "" {
			arg = f.Offset
		}
		out = append(out, arg)
	}
	return out[0], out[1:], nil
}

// SplitCommand splits a command into arguments, honouring quotes so that a path
// containing a space survives as one argument.
func SplitCommand(command string) []string {
	var (
		args    []string
		current strings.Builder
		quote   rune
		started bool
	)
	for _, r := range command {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t' || r == '\n':
			if started || current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if started || current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

/* -------------------------------------------------------------------------- */
/*  Debugging                                                                  */
/* -------------------------------------------------------------------------- */

// Debug is how a program's execution is inspected on a target.
//
// It is a block in a spec for the same reason `flash` is: which debugger, which
// probe, and where the register map lives are facts about the board rather than
// about the project, and they are the same for every project built for it. A
// build can therefore hand an editor a debug configuration that is already
// correct instead of one the person has to know how to write.
type Debug struct {
	// Tool is the debugger program, such as arm-none-eabi-gdb. Empty means the
	// debugger that goes with the compiler, which is what a cross toolchain
	// always has.
	Tool string `json:"tool,omitempty"`
	// Server is the program that sits between the debugger and the board when a
	// probe is in the way: openocd, pyocd, probe-rs, JLinkGDBServer. A target
	// that is debugged in place, with no probe between, declares none and the
	// debugger starts the program itself.
	Server string `json:"server,omitempty"`
	// ServerArgs are the arguments that server is started with, before the port
	// it is told to listen on. Written as a list rather than a command string:
	// these are arguments, and a list is what a process is started with.
	ServerArgs []string `json:"server_args,omitempty"`
	// Adapter names the debug adapter an editor drives this target with, which is
	// `cortex-debug` for a Cortex-M board. It is a name an editor resolves rather
	// than a program to run, so it is said here and left to the editor.
	Adapter string `json:"adapter,omitempty"`
	// ServerType is what the debugger connects to, which is what distinguishes
	// one probe server from another at the far end of the port: `gdb` for
	// OpenOCD and GDB server, `pyocd`, `jlink`.
	ServerType string `json:"server_type,omitempty"`
	// Interface is the probe, as the server names it: cmsis-dap, stlink.
	Interface string `json:"interface,omitempty"`
	// Config are the configuration files the server is started with, as the
	// server spells them: interface/stlink.cfg, target/stm32f4x.cfg.
	Config []string `json:"config,omitempty"`
	// Device is the probe as the vendor names it, for a server that wants that
	// rather than an interface and a board: J-Link, ST-LINK/V2.
	Device string `json:"device,omitempty"`
	// SVD is the CMSIS-SVD file that describes this chip's peripherals, which is
	// what turns a register address into a name a person recognises. It is a path,
	// looked for beside the program and in the vendor tree of each platform the
	// project depends on.
	SVD string `json:"svd,omitempty"`
	// SVDSource is the same file in the repository `rho dbg` fetches from when no
	// project and no platform carries one: a vendor directory and a file, as the
	// CMSIS device data repository writes it — `STMicro/STM32H7x3.svd`.
	//
	// Separate from `svd` because the two say different things and neither is
	// derived from the other. `svd` is where a file a repository carries is; this is
	// which file in a vendor's published set it is, and a path cannot say that
	// because the file's name in the vendor's set is the vendor's, not ours —
	// `STM32H7x3.svd` covers a whole device line where a project would have named
	// one part. Naming it is therefore a fact about the part, which only the spec
	// that knows the part can state.
	SVDSource string `json:"svd_source,omitempty"`
	// Port is the TCP port the server listens on for the debugger. Empty means
	// 3333, which is what every GDB server in this family defaults to.
	Port string `json:"port,omitempty"`
	// Reset is how the board is reset before a session starts, which is the one
	// thing that differs between a board whose bootloader watches a pin and one
	// that does not. Empty means halt-and-load, which is what most do.
	Reset string `json:"reset,omitempty"`
	// Load is whether the program is written to the board at the start of a session.
	// A launch on a board that is already flashed says false; a launch that flashes
	// first, or reloads a rebuilt binary, says true; and a block that says nothing
	// leaves it to be worked out, which is why this is a pointer rather than a flag.
	//
	// Three states, not two, and the third is the one that matters here: a family
	// that names its register map and says nothing about loading leaves this to the
	// board's flash command, and a bool could not tell that from a board that said
	// `load: false`.
	Load *Tri `json:"load,omitempty"`
	// Comment says what has to be true for a session to work at all: a boot mode,
	// a jumper, a probe that has to be plugged in a particular way.
	Comment string `json:"comment,omitempty"`
}

// LoadsProgram is whether a session on this target writes the program to it.
func (d Debug) LoadsProgram() bool { return d.Load != nil && d.Load.Value }

// Tri is a yes/no a spec may leave out, read from either spelling of one.
//
// Two things about it, both from the same source. A YAML scalar arrives as a
// string — `load: true` and `load: "true"` are the same to the reader, and the
// first is what a person writes — so a plain bool refuses the ordinary spelling of
// the thing it means, in one of the two formats a spec may be written in. And a
// bool cannot say "nobody said", which is the third state that matters for a field
// one spec may set and another leaves to be worked out.
type Tri struct {
	// Value is what it says.
	Value bool
	// Stated says whether it said anything at all.
	Stated bool
}

// UnmarshalJSON reads a yes/no, or the absence of one.
func (t *Tri) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	switch strings.ToLower(text) {
	case "null", "":
		*t = Tri{}
	case "true", "yes", "1", "on":
		*t = Tri{Value: true, Stated: true}
	case "false", "no", "0", "off":
		*t = Tri{Value: false, Stated: true}
	default:
		return fmt.Errorf("%q is not a yes or a no", text)
	}
	return nil
}

// MarshalJSON writes what it says, and nothing when it said nothing.
func (t Tri) MarshalJSON() ([]byte, error) {
	if !t.Stated {
		return []byte("null"), nil
	}
	if t.Value {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

// DebugPort is the port the default server is told to listen on. It is the port
// GDB itself defaults to, so a spec that names none lands where a debugger run
// by hand would have connected.
const DebugPort = "3333"

// Declared reports whether the target says anything about debugging.
func (d Debug) Declared() bool {
	// Every field, because a block that declares one thing is declared: replacing it
	// with a parent's would lose the one thing it said. A board that names where its
	// register map is and says nothing about its probe still keeps that, and gets
	// the probe filled in beside it.
	return strings.TrimSpace(d.Tool) != "" ||
		strings.TrimSpace(d.Server) != "" ||
		strings.TrimSpace(d.Adapter) != "" ||
		strings.TrimSpace(d.SVD) != "" ||
		strings.TrimSpace(d.SVDSource) != ""
}

// Watcher is whether a probe has to be running before a debugger may connect.
//
// It is the difference between debugging a board and debugging a process, and
// getting it wrong is why a launch works on a laptop and does nothing on the
// bench: a program running on the host is started by the debugger, and a program
// on a board is started by whatever flashed it.
func (d Debug) Watcher() bool {
	return strings.TrimSpace(d.Server) != ""
}

// ListenPort is the port the server is told to listen on, which is the declared
// one or the default.
func (d Debug) ListenPort() string {
	if port := strings.TrimSpace(d.Port); port != "" {
		return port
	}
	return DebugPort
}

// ServerInvocation is how the server is started, as the program to run and the
// arguments to run it with.
//
// The listening port is appended rather than written into a spec, because a spec
// that named one would have to be edited the moment two sessions were wanted at
// once. A target that declares no server answers false: there is nothing to
// start, and the debugger starts the program itself.
func (d Debug) ServerInvocation() (string, []string, bool) {
	server := strings.TrimSpace(d.Server)
	if server == "" {
		return "", nil, false
	}
	argv := append([]string{}, d.ServerArgs...)
	return server, append(argv, "-s", d.ListenPort()), true
}

// Debugger is the debugger a session on this target is run with, given the
// compiler this target is built with.
//
// A spec that names a tool has said which one, and that answer stands. One that
// does not has said nothing, which is not the same as having no debugger: every
// GNU toolchain ships the debugger that goes with its compiler, and the name of
// one is the name of the other with the compiler replaced. So a spec may leave
// this out and still be debuggable, and `arm-none-eabi-gcc` becomes
// `arm-none-eabi-gdb` without anybody writing that down.
func (d Debug) Debugger(compiler string) string {
	if tool := strings.TrimSpace(d.Tool); tool != "" {
		return tool
	}
	return DebuggerFor(compiler)
}

// DebuggerFor is the debugger that goes with a compiler.
//
// The compiler's own name decides it: the last component of a prefixed
// toolchain is dropped and `gdb` appended, so `arm-none-eabi-gcc` gives
// `arm-none-eabi-gdb` and `riscv32-unknown-elf-gcc` gives
// `riscv32-unknown-elf-gdb`. A compiler with no prefix is the host's own, and
// the host's debugger is `gdb`.
func DebuggerFor(compiler string) string {
	compiler = strings.TrimSpace(compiler)
	if compiler == "" {
		return ""
	}
	if slash := strings.LastIndexAny(compiler, "/\\"); slash >= 0 {
		compiler = compiler[slash+1:]
	}
	// The suffixes a compiler is named with, longest first so that `g++` is not
	// read as a `g`.
	for _, suffix := range []string{"-gcc", "-g++", "-clang++", "-clang", "-cc"} {
		if trimmed := strings.TrimSuffix(compiler, suffix); trimmed != compiler {
			return trimmed + "-gdb"
		}
	}
	switch compiler {
	case "gcc", "g++", "cc", "c++", "clang", "clang++":
		return "gdb"
	}
	return compiler + "-gdb"
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                    */
/* -------------------------------------------------------------------------- */

// Library is where the specs a component ships live.
type Library struct {
	// Root is the directory the specs are read from.
	Root string
	// specs are the specs that were read, by name.
	specs map[string]Spec
	// Diagnostics is what was wrong with them.
	Diagnostics []Diagnostic
}

// Diagnostic is a spec that could not be read, or a reference that does not
// resolve.
type Diagnostic struct {
	Path    string
	Message string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

// isSpecFile reports whether a path names a target spec.
//
// Both JSON and YAML are read. They say the same thing, and a target a person
// writes by hand is far more often written as YAML: a spec is mostly comments
// saying what a board is, and YAML lets those be written as YAML comments
// rather than as a quoted block that a JSON parser will not keep.
func isSpecFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	}
	return false
}

// decodeSpec reads a spec, in whichever of the two formats it was written.
func decodeSpec(data []byte, path string, spec *Spec) error {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return json.Unmarshal(data, spec)
	}
	// YAML is parsed and then handed to the same JSON decoder the JSON specs go
	// through, rather than being decoded field by field here. One decoder means
	// one set of rules about what a field is called and how it is spelled, so a
	// field cannot mean one thing in a .json spec and another in a .yaml one.
	document, err := yaml.Load(string(data))
	if err != nil {
		return err
	}
	converted, err := json.Marshal(yaml.ToAny(document))
	if err != nil {
		return err
	}
	return json.Unmarshal(converted, spec)
}

// Load reads every spec in a directory tree.
//
// A spec that cannot be read is reported and the rest are read: one board with a
// mistake in it must not cost a project the boards that are right.
func Load(root string) *Library {
	return LoadFrom([]string{root})
}

// LoadFrom reads every spec in each of several directory trees, in order.
//
// The first tree to declare a name is the one that stands. That is what makes an
// override an override: a target declared in the Rho home is read before the one
// the component ships, so a machine that needs a different compiler or a
// different memory map says so in its own directory and the shipped spec is
// reported as shadowed rather than silently fighting it.
//
// One tree is the ordinary case, and a project that keeps its own targets needs
// nothing else: it puts them in its own directory and names them.
func LoadFrom(roots []string) *Library {
	library := &Library{specs: map[string]Spec{}}
	seenRoot := false
	for _, root := range roots {
		if root == "" {
			continue
		}
		if !seenRoot {
			library.Root = root
			seenRoot = true
		}
		readInto(library, root)
	}
	return library
}

// AddFile reads one declared spec file into the library.
//
// A module names the spec file it carries — a board names its own, because its
// memory map and how it is reached are that board's — and that one file is read.
//
// Its directory is not searched. A board's folder also holds the board's
// manifest, its presets and its support package, and walking one reads all of
// them: the manifest arrives as a spec that has no compiler flags in it and is
// reported as naming nothing, which names neither the file nor the contract that
// put it there.
func (l *Library) AddFile(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: err.Error()})
		return
	}
	spec := Spec{}
	if err := decodeSpec(data, path, &spec); err != nil {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: "cannot read the spec: " + err.Error()})
		return
	}
	if spec.Name == "" {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: "a spec names nothing"})
		return
	}
	spec.Source = path
	if existing, seen := l.specs[spec.Name]; seen && existing.Source != path {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{
			Path:    existing.Source,
			Message: fmt.Sprintf("a spec named %q is already declared; the first one is used", spec.Name),
		})
		return
	}
	l.specs[spec.Name] = spec
}

// readInto reads one declared directory of specs into a library.
func readInto(library *Library, root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !isSpecFile(path) {
			return nil
		}
		library.AddFile(path)
		return nil
	})
}

// Effective are every spec, resolved, in name order: what a listing shows is
// what each target amounts to rather than what its own file happens to say.
func (l *Library) Effective() []Spec {
	resolved := make([]Spec, 0, len(l.specs))
	for _, name := range l.Names() {
		spec, diagnostics := l.Resolve(name)
		if len(diagnostics) > 0 {
			continue
		}
		resolved = append(resolved, spec)
	}
	return resolved
}

// Names are the specs that were read, sorted, which is what a listing shows.
func (l *Library) Names() []string {
	names := make([]string, 0, len(l.specs))
	for name := range l.specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns a spec by name.
func (l *Library) Get(name string) (Spec, bool) {
	spec, found := l.specs[name]
	return spec, found
}

// Has reports whether a spec of this name is declared.
//
// Reported separately from Get because a caller walking a chain of parents has to
// ask whether the name it holds is a spec before treating it as one, and Get
// answers with a zero spec — which is indistinguishable from a spec that declares
// nothing at all.
func (l *Library) Has(name string) bool {
	if name == "" {
		return false
	}
	_, found := l.Get(name)
	return found
}

// Boards are the specs that describe a board.
func (l *Library) Boards() []Spec {
	boards := []Spec{}
	for _, name := range l.Names() {
		if l.specs[name].Kind == KindBoard {
			boards = append(boards, l.specs[name])
		}
	}
	return boards
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

// Resolve folds a board, the chip it carries and the architecture it is built as
// into one spec, which is what a generator reads.
//
// The chain is followed by name, and a name that does not resolve is reported
// rather than ignored: a board whose chip is missing is a board whose toolchain
// is missing, and a build with no compiler named is a build that fails later
// with a worse error.
func (l *Library) Resolve(name string) (Spec, []Diagnostic) {
	diagnostics := []Diagnostic{}
	spec, found := l.Get(name)
	if !found {
		return Spec{}, []Diagnostic{{Message: fmt.Sprintf("no spec named %q; this component knows %s",
			name, strings.Join(l.Names(), ", "))}}
	}

	chain := []Spec{}
	seen := map[string]bool{}
	// unresolved is the name the chain ended on, if it ended on one.
	unresolved := ""
	for current := spec; ; {
		if seen[current.Name] {
			diagnostics = append(diagnostics, Diagnostic{
				Path: current.Source, Message: fmt.Sprintf("%q refers to itself; the chain is broken here", current.Name)})
			break
		}
		seen[current.Name] = true
		chain = append(chain, current)

		// Each relation is tried until one names a spec, because a relation that names
		// nothing is the ordinary case rather than a fault: no spec is named after a
		// part, and most presets name a build preset rather than a family. Stopping at
		// the first name that names nothing would cut the chain short on every board
		// that says its family through a preset, which is every board on such a
		// platform.
		next := ""
		for _, relation := range relationsOf(current) {
			if relation.name == "" || !l.Has(relation.name) {
				continue
			}
			// A preset that names its own spec is not a link to itself: it is a family
			// spec saying which preset a program is built for, which is every one of
			// them and is not a cycle. An `mcu` or an `arch` that names the spec it is
			// on genuinely is, and is reported.
			if relation.field == "preset" && relation.name == current.Name {
				continue
			}
			next = relation.name
			break
		}
		if next == "" {
			// Nothing more to fold. Which relation was named and named nothing is
			// said, because that is what a person reading a diagnostic for a board
			// needs: not that a name is missing, but which one and why the rest of the
			// chain was not enough.
			for _, relation := range relationsOf(current) {
				if relation.name != "" && !(relation.field == "preset" && relation.name == current.Name) {
					unresolved = relation.name
					break
				}
			}
			break
		}
		parent, _ := l.Get(next)
		current = parent
	}

	// The chain named something that no spec declares — almost always a part, since
	// no spec is named after one: a board is `nucleo-h753zi` and carries an
	// `STM32H753ZI`, which is the chip rather than another spec. That is not a
	// problem for a board, which carries its own memory map and flash command; it is
	// a problem for a spec that cannot stand on its own, and the diagnostic says
	// which one rather than naming the part again.
	if unresolved != "" && !specIsSelfContained(chain[0]) {
		diagnostics = append(diagnostics, Diagnostic{
			Path:    chain[0].Source,
			Message: fmt.Sprintf("%q is for %q, which no spec declares, and it does not carry a toolchain and memory map of its own to stand without one", chain[0].Name, unresolved),
		})
	}

	// The chain is written child first, and a child's facts win: a board's flags
	// are added after its chip's, and the board's memory map is the one a program
	// is laid out in.
	resolved := chain[0]
	for _, parent := range chain[1:] {
		resolved = merge(parent, resolved)
	}
	resolved.ResolvedFrom = nil
	for _, one := range chain {
		resolved.ResolvedFrom = append(resolved.ResolvedFrom, one.Name)
	}
	resolved.Kind = spec.Kind
	resolved.Name = spec.Name
	resolved.Source = spec.Source

	// A spec that says nothing about debugging is not necessarily a board that
	// cannot be: it is more often a board that already says how it is reached.
	// Derived after the chain is folded, so that a chip's flash block and a board's
	// are both available and a board's still wins.
	resolved.Debug = resolved.Debugged()
	return resolved, diagnostics
}

// Debugged is how this target is debugged, from its own `debug` block when it
// declares one and from its `flash` block when it does not.
//
// The derivation exists because the two blocks describe the same thing from the two
// ends. Reaching a board to write to its flash and reaching it to read a register
// are the same connection: the same probe, the same interface configuration, the
// same target configuration. A board spec written before anything needed a debugger
// already carries every fact a session needs in the command that flashes it, and
// requiring a second block to be written by hand would leave those boards
// undebuggable for no reason.
//
// What is taken from the flash command is what a probe server needs and nothing
// else: the program, and the `-f` configuration files. What is dropped is what the
// server would refuse — `-c program ... exit`, which writes the flash and quits,
// and the binary it names. The port is the one every server in this family defaults
// to, because the flash command does not listen on one at all.
func (s Spec) Debugged() Debug {
	debug := s.Debug
	if !debug.Declared() {
		return s.derivedDebug(debug)
	}

	// A block that declares one thing and not the other is the ordinary case, not
	// an oversight: a family says which register map its parts have and says nothing
	// about how they are reached, because how they are reached is the board's. So the
	// derivation fills in the fields the block left out and the block keeps the ones
	// it set — which is the same rule every other fact in a spec follows, and the
	// reason a family can add a register map without a board's probe going missing.
	if !debug.Watcher() && strings.TrimSpace(debug.Tool) == "" {
		return s.derivedDebug(debug)
	}
	return debug
}

// derivedDebug is the block a flash command implies, with whatever a declared block
// already said kept.
func (s Spec) derivedDebug(declared Debug) Debug {
	server, config, interfaceName, ok := probeFromFlashCommand(s.Flash)
	if !ok {
		return declared
	}
	writes := Tri{Value: true, Stated: true}
	derived := Debug{
		Server:     server,
		ServerArgs: configArguments(config),
		Config:     config,
		Interface:  interfaceName,
		Port:       DebugPort,
		// The program is written to the board at the start of a session, because the
		// board was being flashed by the very command this was read from.
		Load: &writes,
	}
	if strings.HasPrefix(s.Core, "cortex-m") {
		derived.Adapter = "cortex-debug"
	}
	if derived.Tool == "" {
		derived.Tool = DebuggerFor(s.Toolchain.Compiler)
	}
	return fillDebug(declared, derived)
}

// fillDebug is the declared block over the derived one: a field the block set is the
// answer, and a field it left out is the derived block's.
func fillDebug(declared Debug, derived Debug) Debug {
	out := declared
	if out.Tool == "" {
		out.Tool = derived.Tool
	}
	if out.Server == "" {
		out.Server = derived.Server
	}
	if len(out.ServerArgs) == 0 {
		out.ServerArgs = derived.ServerArgs
	}
	if len(out.Config) == 0 {
		out.Config = derived.Config
	}
	if out.Interface == "" {
		out.Interface = derived.Interface
	}
	if out.Adapter == "" {
		out.Adapter = derived.Adapter
	}
	if out.Port == "" {
		out.Port = derived.Port
	}
	if out.Load == nil {
		out.Load = derived.Load
	}
	return out
}

// probeFromFlashCommand is the probe server a flash command starts, and the
// configuration files it was given.
//
// False for a command that is not a probe server: a flasher this cannot tell from a
// command line is not turned into one, because a session that starts the wrong
// program and waits for it to listen would take twenty seconds to say what was
// already wrong here.
func probeFromFlashCommand(flash Flash) (server string, config []string, interfaceName string, ok bool) {
	tool := strings.TrimSpace(flash.Tool)
	command := strings.TrimSpace(flash.Command)
	if tool == "" || command == "" {
		return "", nil, "", false
	}
	if !isProbeServer(tool) {
		return "", nil, "", false
	}
	arguments, wellFormed := commandFields(command)
	if !wellFormed {
		return "", nil, "", false
	}
	if len(arguments) == 0 || arguments[0] != tool {
		// The command names a different program than the block's tool. Left alone:
		// a wrapper is a thing a flasher may do, and a wrapper's arguments are not
		// a server's arguments.
		return "", nil, "", false
	}
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] != "-f" && arguments[index] != "--file" {
			continue
		}
		file := arguments[index+1]
		config = append(config, file)
		if interfaceName == "" && strings.Contains(file, "interface/") {
			interfaceName = strings.TrimSuffix(strings.TrimPrefix(path.Base(file), "interface/"), ".cfg")
		}
	}
	if len(config) == 0 {
		// A server with no configuration file is not one this can say how to reach
		// anything with, and starting it would leave it listening on a port with no
		// target behind it.
		return "", nil, "", false
	}
	return tool, config, interfaceName, true
}

// isProbeServer is whether a program's name is one this treats as a probe server
// rather than as a flasher.
//
// By name, because that is all a spec says: the tools in this family are named for
// what they are, and a spec that names something else has said what its flash needs
// are, not what its debug needs.
func isProbeServer(tool string) bool {
	base := path.Base(tool)
	base = strings.TrimSuffix(base, ".exe")
	for _, known := range []string{"openocd", "pyocd", "probe-rs", "JLinkGDBServer", "st-util", "pyocd-gdbserver"} {
		if base == known {
			return true
		}
	}
	return false
}

// configArguments are the configuration files as arguments, in the order the
// command gave them.
//
// Order kept because it is the order the server wants: an interface configuration
// before a target one, which is what nearly every invocation writes and what the
// target configuration refers to.
func configArguments(config []string) []string {
	out := make([]string, 0, len(config)*2)
	for _, file := range config {
		out = append(out, "-f", file)
	}
	return out
}

// commandFields splits a command line, honouring quotes.
//
// Honoured because a flash command routinely contains a `-c "program {binary} exit"`
// and the quotes are what keep that one argument — a naive split would hand the
// server half of it and the session would fail on a command that is in the spec.
func commandFields(command string) ([]string, bool) {
	fields := []string{}
	current := &strings.Builder{}
	quote := rune(0)
	started := false
	for _, c := range command {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			current.WriteRune(c)
		case c == '\'' || c == '"':
			quote = c
			started = true
		case c == ' ' || c == '\t':
			if current.Len() > 0 || started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(c)
		}
	}
	if quote != 0 {
		return nil, false
	}
	if current.Len() > 0 || started {
		fields = append(fields, current.String())
	}
	return fields, true
}

// relation is one of the names a spec holds that may point at another spec.
type relation struct{ field, name string }

// relationsOf are the relations a spec declares, in the order they are tried: the
// part it carries, the family it is built for, the architecture it is built as.
//
// The family comes before the architecture because its facts are the part's — its
// flags, its register map — while the architecture's are what every part of the
// family shares.
func relationsOf(spec Spec) []relation {
	return []relation{
		{"mcu", spec.MCU},
		{"preset", spec.Preset},
		{"arch", spec.Arch},
	}
}

// merge folds a parent spec into a child, with the child's own facts winning.
func merge(parent, child Spec) Spec {
	if child.Core == "" {
		child.Core = parent.Core
	}
	if child.Arch == "" {
		child.Arch = parent.Arch
	}
	if child.MCU == "" {
		child.MCU = parent.MCU
	}
	if child.Family == "" {
		child.Family = parent.Family
	}
	if child.HALHeader == "" {
		child.HALHeader = parent.HALHeader
	}
	if child.Toolchain.Compiler == "" {
		child.Toolchain.Compiler = parent.Toolchain.Compiler
	}
	if child.Toolchain.Linker == "" {
		child.Toolchain.Linker = parent.Toolchain.Linker
	}
	if child.Toolchain.Objcopy == "" {
		child.Toolchain.Objcopy = parent.Toolchain.Objcopy
	}
	if child.Toolchain.Size == "" {
		child.Toolchain.Size = parent.Toolchain.Size
	}
	if len(child.Toolchain.Flags) == 0 {
		child.Toolchain.Flags = parent.Toolchain.Flags
	} else {
		child.Toolchain.Flags = append(append([]string{}, parent.Toolchain.Flags...), child.Toolchain.Flags...)
	}
	if child.Memory.Flash.Origin == "" {
		child.Memory.Flash = parent.Memory.Flash
	}
	if child.Memory.RAM.Origin == "" {
		child.Memory.RAM = parent.Memory.RAM
	}
	if len(parent.Memory.Extra) > 0 {
		extra := map[string]Region{}
		for name, region := range parent.Memory.Extra {
			extra[name] = region
		}
		for name, region := range child.Memory.Extra {
			extra[name] = region
		}
		child.Memory.Extra = extra
	}
	child.Defines = append(append([]string{}, parent.Defines...), child.Defines...)
	child.Flags = append(append([]string{}, parent.Flags...), child.Flags...)
	if child.Flash.Tool == "" {
		child.Flash = parent.Flash
	}
	// Debug is folded whole rather than field by field, like Flash: a board that
	// names a probe server has named a debugger that matches it, and a child's
	// debug block with half of it filled in is not a board overriding one field of
	// its chip's, it is a board with a broken block in it. A board that means to
	// override one value writes the whole block.
	if !child.Debug.Declared() {
		child.Debug = parent.Debug
	}
	return child
}

// AllFlags are every flag a target is built with, in the order a compiler reads
// them: the architecture's, then the chip's.
func (s Spec) AllFlags() []string {
	flags := append(append([]string{}, s.Toolchain.Flags...), s.Flags...)
	// Debug information, for a target that is debugged.
	//
	// Without it a bare-metal program has a symbol table and nothing else: the
	// debugger resolves `HAL_Delay` but reports no line, every breakpoint lands on
	// a function entry, and a session on a board looks like a debugger that does not
	// understand the program. It is added here rather than left to the person
	// building for a board, because a target that declares how it is debugged is
	// saying that this is a thing to be done here.
	if s.Debug.Declared() && !hasDebugInformation(flags) {
		flags = append([]string{"-g"}, flags...)
	}
	return flags
}

// hasDebugInformation is whether a list of flags already asks for it.
//
// Any of the flags that produce it, not only `-g`: a spec that says `-g3` or
// `-gdwarf-5` has asked, and adding `-g` beside it is at best redundant and at
// worst a second thing in the command line for somebody to explain.
func hasDebugInformation(flags []string) bool {
	for _, flag := range flags {
		if flag == "-g" || strings.HasPrefix(flag, "-g") {
			return true
		}
	}
	return false
}

// Regions are every memory region a target has, in the order a linker script
// declares them: flash, then RAM, then the extra regions, sorted.
func (s Spec) Regions() []NamedRegion {
	regions := []NamedRegion{}
	if s.Memory.Flash.Origin != "" {
		regions = append(regions, NamedRegion{Name: "FLASH", Region: s.Memory.Flash, Kind: "flash"})
	}
	if s.Memory.RAM.Origin != "" {
		regions = append(regions, NamedRegion{Name: "RAM", Region: s.Memory.RAM, Kind: "ram"})
	}
	names := make([]string, 0, len(s.Memory.Extra))
	for name := range s.Memory.Extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		regions = append(regions, NamedRegion{
			Name: strings.ToUpper(name), Region: s.Memory.Extra[name], Kind: "extra"})
	}
	return regions
}

// NamedRegion is one memory region with the name a linker script gives it.
type NamedRegion struct {
	Name   string
	Region Region
	Kind   string
}

// LinkRegions are the regions a linker script may use, in the order a script
// declares them.
func (s Spec) LinkRegions() []NamedRegion {
	regions := []NamedRegion{}
	for _, region := range s.Regions() {
		if region.Region.Linkable() {
			regions = append(regions, region)
		}
	}
	return regions
}

// PinNames are a board's pins in the order a person reads them: by the number
// the board gives them, so GP2 comes before GP16.
func (s Spec) PinNames() []string {
	names := make([]string, 0, len(s.Pins))
	for name := range s.Pins {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		_, leftNumber := trailingNumber(names[i])
		_, rightNumber := trailingNumber(names[j])
		// Two pins of the same board are read by their numbers: a board names
		// them GP2 and GP16, and a person reads the second after the first.
		if leftNumber >= 0 && rightNumber >= 0 && leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
		return names[i] < names[j]
	})
	return names
}

// trailingNumber is the number a pin name ends in, and where the name before it
// stops, or -1 when it does not end in one.
func trailingNumber(name string) (prefix string, number int) {
	index := len(name)
	for index > 0 && name[index-1] >= '0' && name[index-1] <= '9' {
		index--
	}
	if index == len(name) {
		return name, -1
	}
	value := 0
	for _, digit := range name[index:] {
		value = value*10 + int(digit-'0')
	}
	return name[:index], value
}

// Pin is the pin a board wires to a role, such as the one it calls led.
func (s Spec) Pin(role string) (Pin, bool) {
	for _, pin := range s.Pins {
		if pin.Role == role {
			return pin, true
		}
	}
	return Pin{}, false
}

// specIsSelfContained reports whether a spec carries what a build needs without a
// parent: how to compile it, and where it lives in memory.
func specIsSelfContained(spec Spec) bool {
	return spec.Toolchain.Compiler != "" && spec.Memory.RAM.Origin != "" && spec.Memory.Flash.Origin != ""
}
