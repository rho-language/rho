# rho-flash

Puts a built binary on a board.

```sh
rho flash -target nucleo-h753zi build/output/hello_world
```

## What it is for

A cross build produces an ELF file. Nothing has put it on the board yet, and the
program that does is not the one that compiled it.

What a board needs in order to be flashed is not a property of the flasher.
OpenOCD, `st-flash`, `picotool` and `dfu-util` each want a different
invocation, and which one is right depends on the part, the probe, and what the
board has soldered on it. So the platform says, and this program runs what it
said.

## The board describes itself

A target spec carries a `flash` block:

```json
"flash": {
  "tool": "openocd",
  "command": "openocd -f interface/stlink.cfg -f target/stm32h7x.cfg -c \"program {binary} verify reset exit\"",
  "comment": "Flashing reaches the part through the ST-LINK, which every Nucleo-144 board carries"
}
```

That is the only place a programmer is named. Adding a board that needs another
one is a change to its spec, not to this program, and a board that needs none of
the supported ones is a spec this program reports rather than one it guesses at.

## Where the spec comes from

`rho target` owns target specs, and it is what resolves them — including a board
that inherits from a family and did not write the `flash` block itself. This
program asks it (`rho target show -json <board>`) rather than reading the JSON
here, so the answer a flash acts on is the same answer a build compiled for.

It used to be otherwise, and that was the bug worth remembering: the flasher
carried a switch over MCU names, with the tool and the load address written into
the program. The address in particular was a constant in the flasher that the
memory map already knew, and the two descriptions of a board disagreed the first
time a platform reached its board some other way.

## Usage

```
rho flash -target <board> [-project dir] <file.bin>
```

`-target` is required and names the board. It is the board, not the preset: a
preset says which part family a board is, and the spec is where the programmer
that reaches it is written down. The binary is normally the one a build wrote to
`build/output/`.
