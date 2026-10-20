//
// @file flash_test.go
// @brief Rho Flash tests.
//
// How a command from a target spec is rendered, and what is refused. No
// hardware is involved: the flashing itself is whatever the spec named, and a
// test that reached for a board would be testing the board.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"reflect"
	"strings"
	"testing"
)

// A command names the program and its arguments, and the binary goes where the
// command said it does. This is the whole of what the flasher contributes: the
// board said how it is reached, and this runs it.
func TestInvocationSubstitutesTheBinary(t *testing.T) {
	flash := Flash{
		Command: `openocd -f interface/stlink.cfg -f target/stm32h7x.cfg -c "program {binary} verify reset exit"`,
	}

	program, args, err := flash.invocation("build/firmware.elf")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if program != "openocd" {
		t.Errorf("expected openocd, got %q", program)
	}
	want := []string{
		"-f", "interface/stlink.cfg",
		"-f", "target/stm32h7x.cfg",
		"-c", "program build/firmware.elf verify reset exit",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// A quoted argument stays one argument. A path with a space in it is a path, and
// a flasher that split it would report a file that does not exist.
func TestInvocationKeepsQuotedArgumentWhole(t *testing.T) {
	flash := Flash{Command: `st-flash write "{binary}" {offset}`}

	_, args, err := flash.invocation("build/my firmware.bin")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	whole := false
	for _, arg := range args {
		if arg == "build/my firmware.bin" {
			whole = true
		}
	}
	if !whole {
		t.Errorf("expected the binary path kept whole, got %q", args)
	}
}

// A command that never says where the file goes has it appended, because a tool
// left to its own default will write whatever that default is rather than the
// binary that was just built.
func TestInvocationAppendsBinaryWhenCommandOmitsIt(t *testing.T) {
	flash := Flash{Command: "st-flash write"}

	_, args, err := flash.invocation("firmware.bin")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if args[len(args)-1] != "firmware.bin" {
		t.Errorf("expected the binary appended, got %q", args)
	}
}

// The offset belongs where the target spec says it goes, and a tool that takes
// the address after the file gets it there.
func TestInvocationPlacesOffsetFromTheSpec(t *testing.T) {
	flash := Flash{Command: "st-flash write", Offset: "0x08000000", PostArgs: []string{"{offset}"}}

	_, args, err := flash.invocation("firmware.bin")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if args[len(args)-1] != "0x08000000" {
		t.Errorf("expected the spec's offset last, got %q", args)
	}
}

// A platform may give the invocation as a list rather than a string, for a board
// whose command has nothing in it needing quoting.
func TestInvocationAcceptsArgumentList(t *testing.T) {
	flash := Flash{Arguments: []string{"picotool", "load", "{binary}"}}

	program, args, err := flash.invocation("firmware.uf2")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if program != "picotool" {
		t.Errorf("expected picotool, got %q", program)
	}
	if !reflect.DeepEqual(args, []string{"load", "firmware.uf2"}) {
		t.Errorf("expected load and the binary, got %q", args)
	}
}

// A target that says nothing about reaching a board is a target this program
// cannot flash, and it is told so. Guessing would be worse than refusing: the
// old answer was a guessed address, and a wrong one is a part that no longer
// works.
func TestInvocationRefusesAnUndeclaredCommand(t *testing.T) {
	flash := Flash{}
	if flash.declared() {
		t.Fatal("a flash block with neither command nor arguments declares nothing")
	}
	if _, _, err := flash.invocation("firmware.bin"); err == nil {
		t.Error("expected a refusal, got an invocation")
	}
}

// The command is split, not handed to a shell. A spec is data a platform ships,
// and one that could name a pipeline would be one that could run whatever a
// machine could be talked into.
//
// What the splitter does with a separator is the test: it arrives as an ordinary
// argument to the program that was named. Nothing runs `rm`, and nothing runs at
// all except the one program, which decides for itself what to do with a word
// that looks like punctuation.
func TestInvocationDoesNotInterpretShellSyntax(t *testing.T) {
	flash := Flash{Command: "flasher --file {binary} ; rm -rf /"}

	program, args, err := flash.invocation("firmware.bin")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if program != "flasher" {
		t.Errorf("expected the named program, got %q", program)
	}
	if len(args) == 0 || args[0] != "--file" {
		t.Fatalf("expected the program's own arguments, got %q", args)
	}
	if args[1] != "firmware.bin" {
		t.Errorf("expected the binary as the program's argument, got %q", args)
	}
	// The separator is passed through rather than acted on: it is the last thing
	// that shows a shell was not involved.
	if args[len(args)-1] != "/" {
		t.Errorf("expected the trailing words passed through as arguments, got %q", args)
	}
}

// A board whose flasher takes the image as a plain argument — Arduino's
// arduino-flasher-cli does — renders the same way: the program, its options,
// and the binary where the command put it.
func TestInvocationForArduinoFlasherCli(t *testing.T) {
	flash := Flash{Command: `arduino-flasher-cli upload --board uno-q {binary}`}

	program, args, err := flash.invocation("build/firmware.hex")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if program != "arduino-flasher-cli" {
		t.Errorf("expected arduino-flasher-cli, got %q", program)
	}
	want := []string{"upload", "--board", "uno-q", "build/firmware.hex"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// A target that says nothing about reaching a board cannot be flashed, and
// saying so is better than running a tool with no target.
func TestNoFlashBlockIsRefused(t *testing.T) {
	if _, _, err := (Flash{}).invocation("app.bin"); err == nil {
		t.Fatal("expected a target with no flash command to be refused")
	}
}

// A command may read the environment: which serial line the board is on is a
// fact about the machine, and a spec that hard-codes it is a spec that is
// wrong on every machine but the one it was written on.
func TestInvocationReadsTheEnvironment(t *testing.T) {
	t.Setenv("RHO_TEST_PORT", "/dev/ttyUSB1")

	flash := Flash{Command: `avrdude -P {env:RHO_TEST_PORT} -U flash:w:{binary}`}

	program, args, err := flash.invocation("build/firmware.elf")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if program != "avrdude" {
		t.Errorf("expected avrdude, got %q", program)
	}
	want := []string{"-P", "/dev/ttyUSB1", "-U", "flash:w:build/firmware.elf"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// A reference with a default is the value when there is one and the default
// when there is not, which is how a spec says "this machine's choice, and this
// one if it says nothing".
func TestInvocationFallsBackToTheDefault(t *testing.T) {
	t.Setenv("RHO_TEST_PORT", "")

	flash := Flash{Command: `avrdude -P {env:RHO_TEST_PORT:-/dev/ttyUSB0} -U flash:w:{binary}`}

	_, args, err := flash.invocation("build/firmware.elf")
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	want := []string{"-P", "/dev/ttyUSB0", "-U", "flash:w:build/firmware.elf"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// A reference with no default and no value is a hole in a command that would
// flash nothing. Saying so is better than doing it.
func TestInvocationReportsAMissingEnvironment(t *testing.T) {
	t.Setenv("RHO_TEST_PORT", "")

	flash := Flash{Command: `avrdude -P {env:RHO_TEST_PORT} -U flash:w:{binary}`}

	_, _, err := flash.invocation("build/firmware.elf")
	if err == nil {
		t.Fatal("expected the missing variable to be reported")
	}
	if !strings.Contains(err.Error(), "RHO_TEST_PORT") {
		t.Errorf("expected the variable to be named, got %q", err)
	}
}
