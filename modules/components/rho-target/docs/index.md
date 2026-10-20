# Board and architecture specs

Board and architecture specifications, and the CMake that builds for them. A
build for a dev board needs facts that belong to the board and not to the
project — which compiler, which flags, where the flash is, how much RAM, which
pin the LED is on — and those facts are the same for every project that targets
the board.

```sh
rho target list                    # what this component knows
rho target show pico               # one resolved spec
rho target pins pico               # the pin map of a board
rho target generate . --target pico   # write the CMake and the linker script
```

## The specs

A spec is JSON, written once, and refers to another: a board names the chip it
carries, and a chip names the architecture it is built as.

| Spec | Kind | What it says |
|------|------|--------------|
| `arm-cortex-m` | arch | The toolchain every Cortex-M chip is built with |
| `stm32f4` | mcu | Cortex-M4F, arm-none-eabi, 1 MB flash, 128 KB + 64 KB CCM RAM, an ST-Link to flash through |
| `rp2040` | mcu | Cortex-M0+, 2 MB of XIP flash, 264 KB SRAM, the bootrom's USB protocol |
| `pico` | board | An RP2040 on the Raspberry Pi Pico, with the pin map |
| `esp32c3` | board | A RISC-V ESP32-C3 on its development board, built as an ESP-IDF project |

A spec resolves through that chain, so `rho target show pico` reports one target
with the compiler of the chip's architecture, the chips's memory, and the board's
pin map.

## What it writes

```sh
rho target generate . --target pico
```

| File | What it is |
|------|------------|
| `cmake/toolchain.cmake` | Which compiler, and that the system is not the host's |
| `cmake/target.cmake` | Defines, flags, the memory map, the vendor library, and a `flash` target |
| `cmake/linker.ld` | Sections and regions, generated from the memory map |

The generated `CMakeLists.txt` includes the toolchain file before `project()` and
the target file after it, when they are there. A project that targets the host
has neither and builds as it did.

:::{note}
The linker script is generated from the memory map when the target does not name
its own, and a target that is built by a framework (an ESP-IDF board) marks its
regions as facts rather than as somewhere a program is laid out. A board whose
platform links with its own script names it in `linker_script` instead.
:::

### The `linker` block

The `linker` block says how a program's memory is laid out, and it is the block
a part uses to say that the toolchain already knows:

| Field | What it means |
|-------|---------------|
| `script` | A `.ld` of the spec's own, absolute or beside the spec that named it |
| `toolchain_script` | The toolchain brings its own, and none is written |
| `stack_bytes`, `heap_bytes` | How much stack and heap the program gets |
| `entry` | The symbol the program starts at |
| `regions`, `sections`, `symbols` | Further places a program may be put, and what the script defines |

`toolchain_script` is for the toolchains that know their part: `avr-gcc` knows
where an AVR's vector table belongs and how its runtime expects to be reached,
and a script written here is that knowledge repeated less well. The memory map
is still declared — what a program is told about the part it runs on does not
depend on who writes the script.

```json
{
  "kind": "mcu",
  "name": "atmega328p",
  "memory": { "flash": { "origin": "0x0", "length": "0x8000" } },
  "linker": { "toolchain_script": true }
}
```

A board that says nothing about its linker inherits its part's, so that where a
program's memory is laid out stays the part's fact.

## The flash command

A board says how it is written to, and that is a command:

```json
"flash": {
  "tool": "avrdude",
  "command": "avrdude -P {env:AVRDUDE_PORT:-/dev/ttyUSB0} -U flash:w:{binary}"
}
```

`{binary}` is where the file that was just built goes, and a command that never
says so has it appended.

A command may read the environment: `{env:NAME}` is the value of `NAME`, and
`{env:NAME:-default}` is that value or the default when it is not set. That is how
a spec says something the machine decides — which serial line the board is on,
which probe it is behind — without the spec being wrong on every machine but the
one it was written on. A reference with no default and no value is an error
rather than a hole.

It is a substitution and not a shell: there is no pipeline, no glob and no
command substitution, so a spec that could name one is still a spec that cannot
run whatever a machine could be talked into. The reference is spelled
`{env:…}` rather than `${…}` because the command travels into a CMake file, and
CMake reads `${…}` as its own.

## The pin map

A board says what each pin is for, and which interface it is on:

```json
"peripherals": {
  "i2c0": { "instance": "i2c0", "pins": { "sda": "GP4", "scl": "GP5" } }
}
```

so a program asks for a role rather than a number, and the number is the board's
business.

## Extending it

A board this component has never heard of is a spec a plugin supplies. The hook
is `spec`: it runs once, with the targets that were found, and answers with the
specs it knows.

```yaml
extends: rho-target
hooks: [spec]
commands:
  - name: my-board
    run: my-board-spec
    hook: spec
```

A spec a plugin declares is used when this component does not carry one of that
name, and a plugin that fails is reported and left out.

## Building it

```sh
go build -o rho-target ./src
go test ./...
```

## The Rho library

`src/target.rho` declares what a program can read about the board it was built
for: the target's name, its core, its flash and RAM sizes, and the pin a role is
wired to. The externs behind it need a `target` port, which does not exist yet —
until then the library compiles and does not link, which is why nothing in this
tree imports it.
