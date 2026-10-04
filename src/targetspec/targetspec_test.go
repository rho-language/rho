//
// @file targetspec_test.go
// @brief Tests for reading what a target spec says.
//
// A shared reader, which makes these worth having of their own: the editor
// configuration and the debugging session both read through it, and a field one of
// them needs that the other forgot is a field one of them silently does without.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package targetspec

import "testing"

// What `rho target show -json` prints for a board: the resolved spec, with the
// chip's and the architecture's facts already folded in.
const boardJSON = `{
  "kind": "board",
  "name": "nucleo-h753zi",
  "core": "cortex-m7",
  "toolchain": {"compiler": "arm-none-eabi-gcc", "objcopy": "arm-none-eabi-objcopy"},
  "defines": ["STM32H753xx", "USE_HAL_DRIVER"],
  "memory": {
    "flash": {"origin": "0x08000000", "length": "0x200000"},
    "ram": {"origin": "0x20000000", "length": "0x20000"}
  },
  "flash": {"tool": "openocd", "command": "openocd -f interface/stlink.cfg -f target/stm32h7x.cfg"},
  "debug": {
    "tool": "arm-none-eabi-gdb",
    "server": "openocd",
    "server_args": ["-f", "interface/stlink.cfg", "-f", "target/stm32h7x.cfg"],
    "adapter": "cortex-debug",
    "server_type": "gdb",
    "interface": "stlink",
    "config": ["interface/stlink.cfg", "target/stm32h7x.cfg"],
    "port": "3333",
    "load": true
  }
}`

func TestWhatABoardSaysIsRead(t *testing.T) {
	board, err := ParseTarget([]byte(boardJSON))
	if err != nil {
		t.Fatal(err)
	}
	if board.Name != "nucleo-h753zi" {
		t.Errorf("expected the board, got %q", board.Name)
	}
	if board.Toolchain.Compiler != "arm-none-eabi-gcc" {
		t.Errorf("expected the compiler, got %q", board.Toolchain.Compiler)
	}
	if len(board.Defines) != 2 || board.Memory.Flash.Origin != "0x08000000" {
		t.Errorf("expected the macros and the memory map, got %+v", board)
	}

	debug := board.Debug
	if !debug.Declared() {
		t.Fatalf("expected the board to say how it is debugged, got %+v", debug)
	}
	if !debug.Watcher() {
		t.Error("expected a board behind a probe server to need one running")
	}
	if debug.ListenPort() != "3333" {
		t.Errorf("expected the declared port, got %q", debug.ListenPort())
	}
	// The `load` flag is a boolean in the JSON, which it is because a spec reader
	// elsewhere made it one out of both spellings. Here it is a plain bool, and a
	// reader that got it wrong would load a program onto a board that did not ask
	// for it.
	if !debug.Load {
		t.Error("expected the program to be written to the board")
	}
}

// A program on the machine it is built on is a target of its own, and the one thing
// it says is that it has no probe.
func TestTheHostCarriesNoProbe(t *testing.T) {
	host, err := ParseTarget([]byte(`{"kind":"arch","name":"native","toolchain":{"compiler":"gcc"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if host.Debug.Declared() {
		t.Errorf("expected nothing said about debugging, got %+v", host.Debug)
	}
	if host.Debug.Watcher() {
		t.Error("expected no probe server to be needed")
	}
	// And no memory map of its own, which is what tells a generated editor
	// configuration to leave the editor's compiler settings alone.
	if host.Memory.Flash.Origin != "" {
		t.Errorf("expected no memory map, got %+v", host.Memory)
	}
}

// Something that is not a target spec is reported rather than read as an empty one:
// a caller that treated it as empty would write a configuration for a board it
// knows nothing about, which looks right and does nothing.
func TestSomethingThatIsNotATargetSpecIsRefused(t *testing.T) {
	for name, given := range map[string]string{
		"not json":  "not json at all",
		"no name":   `{"kind":"board"}`,
		"an array":  `[{"name":"pico"}]`,
		"a failure": `rho target: no spec named "pico"`,
	} {
		if _, err := ParseTarget([]byte(given)); err == nil {
			t.Errorf("expected %s to be refused", name)
		}
	}
}

// A toolchain's debugger is named after its compiler, which is why a spec may leave
// it out and still be debuggable.
func TestTheDebuggerGoesWithTheCompiler(t *testing.T) {
	for compiler, want := range map[string]string{
		"arm-none-eabi-gcc":       "arm-none-eabi-gdb",
		"riscv32-unknown-elf-gcc": "riscv32-unknown-elf-gdb",
		"arm-none-eabi-g++":       "arm-none-eabi-gdb",
		"gcc":                     "gdb",
		"clang":                   "gdb",
		"":                        "",
	} {
		if got := DebuggerFor(compiler); got != want {
			t.Errorf("%q: expected %q, got %q", compiler, want, got)
		}
	}
	// What a spec names wins over what would have been derived.
	board, err := ParseTarget([]byte(boardJSON))
	if err != nil {
		t.Fatal(err)
	}
	if got := board.Debug.Tool; got != "arm-none-eabi-gdb" {
		t.Errorf("expected the board's own debugger, got %q", got)
	}
}
