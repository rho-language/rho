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
