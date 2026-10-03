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
	"strings"
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

// A family spec's preset is its own name — it is what a program is built for — and
// that is not a chain that refers to itself. Reported as a cycle, every board on
// every platform that names its families this way would carry a diagnostic about a
// mistake nobody made.
func TestAFamilyNPresetIsNotAChainThatRefersToItself(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"family.json": `{
		  "kind": "mcu", "name": "family", "arch": "arch", "preset": "family",
		  "flags": ["-mthumb"]
		}`,
		"arch.json": `{"kind": "arch", "name": "arch", "toolchain": {"compiler": "arm-none-eabi-gcc"}}`,
		"board.json": `{
		  "kind": "board", "name": "board", "preset": "family",
		  "memory": {"flash": {"origin": "0x0", "length": "0x1000"},
		             "ram": {"origin": "0x20000000", "length": "0x1000"}}
		}`,
	})
	spec, diagnostics := library.Resolve("family")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if spec.Name != "family" || !containsString(strings.Join(spec.AllFlags(), " "), "-mthumb") {
		t.Errorf("expected the family resolved with its own flags, got %+v", spec)
	}
	spec, diagnostics = library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a board on that family to resolve cleanly, got %+v", diagnostics)
	}
	if !containsString(strings.Join(spec.AllFlags(), " "), "-mthumb") {
		t.Errorf("expected the board to inherit the family's flags, got %+v", spec.AllFlags())
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

/* -------------------------------------------------------------------------- */
/*  Debugging                                                                  */
/* -------------------------------------------------------------------------- */

// A board with a probe of its own keeps the whole block it declares, and a chip
// with none inherits its architecture's — which is the thing a spec is for: the
// same answer written once rather than by every board of a family.
func TestABoardInheritsHowToBeDebugged(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": `{
		  "kind": "arch", "name": "arm-cortex-m",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "debug": {
		    "tool": "arm-none-eabi-gdb",
		    "server": "openocd",
		    "server_args": ["-f", "interface/cmsis-dap.cfg"],
		    "adapter": "cortex-debug",
		    "svd": "cm0plus.svd"
		  }
		}`,
		"chip.json": `{"kind": "mcu", "name": "chip", "arch": "arm-cortex-m",
		  "memory": {"flash": {"origin": "0x08000000", "length": "0x100000"},
		             "ram": {"origin": "0x20000000", "length": "0x20000"}}}`,
	})
	spec, diagnostics := library.Resolve("chip")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if got := spec.Debug.Debugger(spec.Toolchain.Compiler); got != "arm-none-eabi-gdb" {
		t.Errorf("expected the architecture's debugger, got %q", got)
	}
	if got := spec.Debug.Adapter; got != "cortex-debug" {
		t.Errorf("expected the architecture's adapter, got %q", got)
	}
	if got := spec.Debug.SVD; got != "cm0plus.svd" {
		t.Errorf("expected the architecture's register map, got %q", got)
	}
	if !spec.Debug.Watcher() {
		t.Error("expected a board behind a probe server to need one running")
	}
}

// A board that declares its own debug block keeps it whole: the fields the board
// does not fill in are not inherited, because a probe server and a debugger that
// do not match are a session that does not start.
func TestABoardKeepsTheProbeItDeclares(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": `{
		  "kind": "arch", "name": "arch",
		  "debug": {"tool": "gdb", "adapter": "cortex-debug"}
		}`,
		"board.json": `{
		  "kind": "board", "name": "board", "arch": "arch",
		  "debug": {"server": "pyocd", "server_type": "pyocd", "svd": "board.svd"}
		}`,
	})
	spec, _ := library.Resolve("board")
	if spec.Debug.Tool != "" {
		t.Errorf("expected the board's block not to be filled in from its architecture, got %q", spec.Debug.Tool)
	}
	if spec.Debug.Adapter != "" {
		t.Errorf("expected the board's block not to be filled in from its architecture, got %q", spec.Debug.Adapter)
	}
	if spec.Debug.Server != "pyocd" || spec.Debug.SVD != "board.svd" {
		t.Errorf("expected the board's own probe, got %+v", spec.Debug)
	}
	if !spec.Debug.Watcher() {
		t.Error("expected a probe server to need running before a session")
	}
	// A board that names a probe and no debugger is still debuggable: the
	// debugger that goes with its compiler is what runs, which is what keeps the
	// board's block from needing to repeat what the toolchain already says.
	if got := spec.Debug.Debugger("arm-none-eabi-gcc"); got != "arm-none-eabi-gdb" {
		t.Errorf("expected the compiler's own debugger, got %q", got)
	}
}

// The port is the one thing about a session that is not a fact about the board:
// two of them at once is ordinary, so it is declared rather than guessed at, and
// the default is the port a debugger run by hand would have used.
func TestTheListeningPortIsDeclaredOrTheDefaultOne(t *testing.T) {
	declared := Debug{Server: "openocd", Port: "4444"}
	if got := declared.ListenPort(); got != "4444" {
		t.Errorf("expected the declared port, got %q", got)
	}
	if got := (Debug{Server: "openocd"}).ListenPort(); got != DebugPort {
		t.Errorf("expected the default port, got %q", got)
	}

	program, arguments, ok := declared.ServerInvocation()
	if !ok || program != "openocd" {
		t.Fatalf("expected openocd to be started, got %q %v (%v)", program, arguments, ok)
	}
	// The port is appended after the server's own arguments, so a config file
	// that also wanted a port would be overridden rather than fought with.
	if joined := strings.Join(arguments, " "); joined != "-s 4444" {
		t.Errorf("expected the port to be handed over, got %q", joined)
	}
	if _, _, started := (Debug{}).ServerInvocation(); started {
		t.Error("expected a target with no probe server to start nothing")
	}
}

// A spec may leave the debugger out and still be debuggable: every GNU
// toolchain ships the debugger that goes with its compiler.
func TestTheDebuggerIsTheOneThatGoesWithTheCompiler(t *testing.T) {
	for _, testCase := range []struct{ compiler, want string }{
		{"arm-none-eabi-gcc", "arm-none-eabi-gdb"},
		{"riscv32-unknown-elf-gcc", "riscv32-unknown-elf-gdb"},
		{"arm-none-eabi-g++", "arm-none-eabi-gdb"},
		{"/opt/gcc/bin/arm-none-eabi-gcc", "arm-none-eabi-gdb"},
		{"gcc", "gdb"},
		{"clang++", "gdb"},
		{"", ""},
	} {
		if got := DebuggerFor(testCase.compiler); got != testCase.want {
			t.Errorf("%q: expected %q, got %q", testCase.compiler, testCase.want, got)
		}
	}

	// What a spec says wins over what would have been derived, because a
	// debugger named by hand is a debugger that was looked up rather than
	// assumed.
	named := Debug{Tool: "lldb"}
	if got := named.Debugger("arm-none-eabi-gcc"); got != "lldb" {
		t.Errorf("expected the named debugger, got %q", got)
	}
}

// A boolean in a spec is written the way YAML writes one — bare — and read from
// both spellings, because a plain bool field refuses `load: true` in a file format
// whose scalars all arrive as strings. A spec that had to quote every boolean would
// be a rule nobody remembers.
func TestAYesOrNoIsReadFromEitherSpelling(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"board.yaml": "kind: board\nname: board\ndebug:\n  server: openocd\n  load: true\n",
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if !spec.Debug.LoadsProgram() {
		t.Error("expected the bare YAML boolean to be read")
	}

	library = libraryOf(t, map[string]string{
		"board.yaml": "kind: board\nname: board\ndebug:\n  server: openocd\n  load: \"true\"\n",
	})
	spec, _ = library.Resolve("board")
	if !spec.Debug.LoadsProgram() {
		t.Error("expected the quoted spelling to be read")
	}

	library = libraryOf(t, map[string]string{
		"board.json": `{"kind":"board","name":"board","debug":{"server":"openocd","load":true}}`,
	})
	spec, _ = library.Resolve("board")
	if !spec.Debug.LoadsProgram() {
		t.Error("expected a JSON boolean to be read")
	}
}

func TestAYesOrNoThatIsNotOneIsRefused(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"board.json": `{"kind":"board","name":"board","debug":{"load":"perhaps"}}`,
	})
	_, diagnostics := library.Resolve("board")
	if len(diagnostics) == 0 {
		t.Error("expected a flag that is not a yes or a no to be reported")
	}
}

// A board spec written before anything needed a debugger already says how the board
// is reached: the flash command names the probe, the interface configuration and the
// target configuration. That is the same connection a session needs, so it is read
// rather than asked for a second time.
func TestABoardThatCanBeFlashedCanBeDebuggedWithoutSayingSoTwice(t *testing.T) {
	// The shape a Nucleo-144 board's spec has: an ST-LINK on the board, and a flash
	// command that drives it.
	library := libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board", "core": "cortex-m7",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "flash": {
		    "tool": "openocd",
		    "command": "openocd -f interface/stlink.cfg -f target/stm32h7x.cfg -c \"program {binary} verify reset exit\""
		  }
		}`,
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if spec.Debug.Server != "openocd" {
		t.Fatalf("expected the probe the flash command starts, got %+v", spec.Debug)
	}
	if got := strings.Join(spec.Debug.Config, " "); got != "interface/stlink.cfg target/stm32h7x.cfg" {
		t.Errorf("expected both configuration files, in the order the command gave them, got %q", got)
	}
	if spec.Debug.Interface != "stlink" {
		t.Errorf("expected the probe to be named the way the server names it, got %q", spec.Debug.Interface)
	}
	// What the server would refuse is dropped: the `-c program ... exit` half is
	// how a flasher quits, and handing it to a server that is meant to keep
	// listening would end the session as it started.
	if joined := strings.Join(spec.Debug.ServerArgs, " "); strings.Contains(joined, "program") ||
		strings.Contains(joined, "{binary}") {
		t.Errorf("expected the flashing half to be dropped, got %q", joined)
	}
	if !spec.Debug.LoadsProgram() {
		t.Error("expected the program to be written to a board that was being flashed")
	}
	if spec.Debug.Tool != "arm-none-eabi-gdb" {
		t.Errorf("expected the compiler's own debugger, got %q", spec.Debug.Tool)
	}
	// The port is the one every server in this family defaults to, because the
	// flash command does not listen on one at all.
	if got := spec.Debug.ListenPort(); got != DebugPort {
		t.Errorf("expected the default port, got %q", got)
	}
	// And an editor has an adapter to drive a Cortex-M board with.
	if spec.Debug.Adapter != "cortex-debug" {
		t.Errorf("expected the Cortex-M adapter, got %q", spec.Debug.Adapter)
	}
}

// A block that says something is taken whole, and nothing is derived over it: a
// board that has been told how it is debugged has said so on purpose.
func TestADebugBlockIsNeverDerivedOver(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board", "core": "cortex-m7",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg"},
		  "debug": {"server": "pyocd", "server_type": "pyocd", "svd": "board.svd"}
		}`,
	})
	spec, _ := library.Resolve("board")
	if spec.Debug.Server != "pyocd" {
		t.Errorf("expected the declared probe, got %+v", spec.Debug)
	}
	if len(spec.Debug.Config) != 0 {
		t.Errorf("expected nothing derived over a declared block, got %v", spec.Debug.Config)
	}
	if spec.Debug.Adapter != "" {
		t.Errorf("expected no adapter to be added, got %q", spec.Debug.Adapter)
	}
}

// A flasher that is not a probe server is not turned into one. Starting a program
// that will never listen takes twenty seconds to report what was already wrong.
func TestAFlasherThatIsNotAProbeServerIsNotOne(t *testing.T) {
	for name, flash := range map[string]string{
		"a picotool":     `{"tool": "picotool", "command": "picotool load -f -x {binary}"}`,
		"a dfu":          `{"tool": "dfu-util", "command": "dfu-util -a 0 -s 0x08000000:leave -D app.bin"}`,
		"a wrapper":      `{"tool": "flash.sh", "command": "flash.sh openocd -f interface/cmsis-dap.cfg"}`,
		"no config file": `{"tool": "openocd", "command": "openocd -c \"init\""}`,
		"no command":     `{"tool": "openocd"}`,
	} {
		library := libraryOf(t, map[string]string{
			"board.json": `{"kind": "board", "name": "board", "toolchain": {"compiler": "gcc"}, "flash": ` + flash + `}`,
		})
		spec, _ := library.Resolve("board")
		if spec.Debug.Declared() {
			t.Errorf("%s: expected nothing derived, got %+v", name, spec.Debug)
		}
	}
}

// A command with an unbalanced quote is not read at all: half of it would be read,
// and a probe server started from half a command line fails in a way that names
// neither the spec nor the board.
func TestAMalformedFlashCommandIsNotRead(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board",
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg -c \"program {binary}"}
		}`,
	})
	spec, _ := library.Resolve("board")
	if spec.Debug.Declared() {
		t.Errorf("expected nothing derived from a command that is not closed, got %+v", spec.Debug)
	}
}

// A program built for a board is a program somebody will stop in. Without debug
// information the debugger resolves function names from the symbol table and no
// lines at all, every breakpoint lands on a function entry, and the session looks
// like a debugger that does not understand the program it is driving.
func TestATargetThatIsDebuggedIsBuiltWithDebugInformation(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board",
		  "toolchain": {"compiler": "arm-none-eabi-gcc", "flags": ["-mcpu=cortex-m7", "-mthumb"]},
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg"},
		  "flags": ["-nostartfiles"]
		}`,
	})
	spec, _ := library.Resolve("board")
	flags := strings.Join(spec.AllFlags(), " ")
	if !strings.Contains(flags, "-g") {
		t.Errorf("expected debug information for a target that can be debugged, got %q", flags)
	}
	// And nothing is added twice, or moved, when the spec says so itself.
	library = libraryOf(t, map[string]string{
		"board.json": `{
		  "kind": "board", "name": "board",
		  "toolchain": {"compiler": "arm-none-eabi-gcc", "flags": ["-g3"]},
		  "debug": {"server": "openocd"}
		}`,
	})
	spec, _ = library.Resolve("board")
	if got := strings.Join(spec.AllFlags(), " "); got != "-g3" {
		t.Errorf("expected the spec's own flag, got %q", got)
	}

	// And a host build is left alone: the editor's own compiler path and the
	// toolchain's own settings already say where that program is debugged.
	library = libraryOf(t, map[string]string{
		"host.json": `{"kind":"arch","name":"host","toolchain":{"compiler":"gcc","flags":["-O2"]}}`,
	})
	spec, _ = library.Resolve("host")
	if got := strings.Join(spec.AllFlags(), " "); strings.Contains(got, "-g") {
		t.Errorf("expected a host build's flags left as they are, got %q", got)
	}
}

// A board that names its family through its preset — which is how a platform's
// boards say which family's facts they inherit — folds that family in. It is the
// only link in the chain for a board that names a part no spec declares, so a
// family's flags and its register map are otherwise unreachable from it.
func TestABoardInheritsTheFamilyItsPresetNames(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": `{
		  "kind": "arch", "name": "arm-cortex-m",
		  "toolchain": {"compiler": "arm-none-eabi-gcc", "flags": ["-mcpu=cortex-m7", "-mthumb"]}
		}`,
		"family.json": `{
		  "kind": "mcu", "name": "family", "arch": "arm-cortex-m",
		  "flags": ["-nostartfiles"],
		  "debug": {"svd": "vendor/cmsis_svd/family.svd"}
		}`,
		"board.json": `{
		  "kind": "board", "name": "board",
		  "mcu": "PART-NO-SPEC-DECLARES", "preset": "family",
		  "memory": {"flash": {"origin": "0x08000000", "length": "0x200000"},
		             "ram": {"origin": "0x20000000", "length": "0x20000"}},
		  "flags": ["-Wl,--gc-sections"]
		}`,
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	// The family is nearer than the architecture, so its flags come after the
	// architecture's and before the board's — which is the order `AllFlags` says
	// they are given in. The leading `-g` is because a target whose family says how
	// it is debugged is built with debug information.
	joined := strings.Join(spec.AllFlags(), " ")
	want := "-g -mcpu=cortex-m7 -mthumb -nostartfiles -Wl,--gc-sections"
	if joined != want {
		t.Errorf("expected %q, got %q", want, joined)
	}
	if spec.Debug.SVD != "vendor/cmsis_svd/family.svd" {
		t.Errorf("expected the family's register map, got %q", spec.Debug.SVD)
	}
	// And the chain says what it was built from, in the order it was walked.
	if got := strings.Join(spec.ResolvedFrom, " "); got != "board family arm-cortex-m" {
		t.Errorf("expected the family in the chain, got %q", got)
	}
}

// A preset that names a build preset rather than a spec is not a link, and the
// architecture it also names is still the next relation to try.
func TestAPresetThatIsNotASpecIsNotALink(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": `{
		  "kind": "arch", "name": "arch",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "flags": ["-mthumb"]
		}`,
		"board.json": `{
		  "kind": "board", "name": "board", "arch": "arch", "preset": "some-build-preset",
		  "memory": {"flash": {"origin": "0x0", "length": "0x1000"},
		             "ram": {"origin": "0x20000000", "length": "0x1000"}}
		}`,
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if got := strings.Join(spec.AllFlags(), " "); got != "-mthumb" {
		t.Errorf("expected the architecture's flags, got %q", got)
	}
}

// A family that names its register map and says nothing about how its parts are
// reached must not cost the boards that inherit it their probe. A block that
// declares one thing and not the other is the ordinary case, and the derivation
// fills in what is missing while the block keeps what it set.
func TestAFamilyRegisterMapDoesNotCostItsBoardsTheirProbe(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"family.json": `{
		  "kind": "mcu", "name": "family", "arch": "arch", "preset": "family",
		  "debug": {"svd": "vendor/cmsis_svd/family.svd"}
		}`,
		"arch.json": `{
		  "kind": "arch", "name": "arch",
		  "toolchain": {"compiler": "arm-none-eabi-gcc", "flags": ["-mthumb", "-mcpu=cortex-m7"]}
		}`,
		"board.json": `{
		  "kind": "board", "name": "board", "preset": "family", "core": "cortex-m7",
		  "memory": {"flash": {"origin": "0x08000000", "length": "0x200000"},
		             "ram": {"origin": "0x20000000", "length": "0x20000"}},
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg -f target/family.cfg"}
		}`,
	})
	spec, diagnostics := library.Resolve("board")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean resolution, got %+v", diagnostics)
	}
	if spec.Debug.SVD != "vendor/cmsis_svd/family.svd" {
		t.Errorf("expected the family's register map, got %q", spec.Debug.SVD)
	}
	if !spec.Debug.Watcher() {
		t.Error("expected the board's probe, derived from the command that flashes it")
	}
	if spec.Debug.Server != "openocd" {
		t.Errorf("expected openocd, got %q", spec.Debug.Server)
	}
	if !spec.Debug.LoadsProgram() {
		t.Error("expected the program to be written to a board that was being flashed")
	}
	// And a board that says `load: false` keeps it, which is the whole reason the
	// field has three states rather than two.
	library = libraryOf(t, map[string]string{
		"family.json": `{"kind":"mcu","name":"family","debug":{"svd":"family.svd"}}`,
		"board.json": `{
		  "kind": "board", "name": "board", "core": "cortex-m7",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "memory": {"flash": {"origin": "0x0", "length": "0x1000"},
		             "ram": {"origin": "0x20000000", "length": "0x1000"}},
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg"},
		  "debug": {"load": false}
		}`,
	})
	spec, _ = library.Resolve("board")
	if !spec.Debug.Watcher() {
		t.Error("expected the probe still to be derived for a board that only says load")
	}
	if spec.Debug.LoadsProgram() {
		t.Error("expected a board that says it does not flash a program to be believed")
	}
}

// A board that names where its register map comes from and says nothing about its
// probe has declared a debug block, and a parent's must not replace it — the one
// thing it said is the thing it said for.
func TestABoardThatOnlyNamesItsRegisterMapIsStillDeclared(t *testing.T) {
	library := libraryOf(t, map[string]string{
		"arch.json": `{
		  "kind": "arch", "name": "arch",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "debug": {"tool": "arm-none-eabi-gdb", "adapter": "cortex-debug", "svd": "family.svd"}
		}`,
		"board.json": `{
		  "kind": "board", "name": "board", "arch": "arch",
		  "toolchain": {"compiler": "arm-none-eabi-gcc"},
		  "memory": {"flash": {"origin": "0x0", "length": "0x1000"},
		             "ram": {"origin": "0x20000000", "length": "0x1000"}},
		  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg"},
		  "debug": {"svd_source": "STMicro/Part.svd"}
		}`,
	})
	spec, _ := library.Resolve("board")
	if spec.Debug.SVDSource != "STMicro/Part.svd" {
		t.Errorf("expected the board's own published file, got %q", spec.Debug.SVDSource)
	}
	// The architecture's other facts are not inherited over it, because a declared
	// block is kept whole — but the probe the board did not say anything about is
	// still derived from the command that flashes it.
	if !spec.Debug.Watcher() {
		t.Error("expected the probe to be derived for a board that declares only its register map")
	}
}
