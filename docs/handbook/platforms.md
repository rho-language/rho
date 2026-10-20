# Platforms

A **platform** is a module that brings a program to a part: which compiler
builds it, where its memory is, how it is written to the board, and the code
that drives the part's peripherals.

Two things make a platform, and they are separate:

- A **target spec**, which says what the part is and how a program is laid out
  and flashed. That is {ref}`rho-target <documentation/components>`'s business,
  and the spec is data.
- The **platform's own code**, which is the library a program links and the
  board support package that registers the board's peripherals.

A program written against the platform's contracts builds for any board the
platform supports: the board is chosen by the preset a build is given, and the
program does not change.

## The target spec

A spec is JSON, and it refers to another: a board names the chip it carries, and
a chip names the architecture it is built as. Each says what it knows and leaves
the rest to what it refers to.

```json
{
  "kind": "mcu",
  "name": "atmega328p",
  "arch": "avr",
  "toolchain": { "compiler": "avr-gcc", "objcopy": "avr-objcopy" },
  "memory": {
    "flash": { "origin": "0x00000000", "length": "0x8000" },
    "ram":   { "origin": "0x00800100", "length": "0x800" }
  }
}
```

A board adds the facts a board has and a chip does not: the pin map, the
peripherals, the flash command, and the library that implements the contracts.

```sh
rho target list                 # what is known
rho target show atmega328p      # one resolved target
rho target generate . --target atmega328p   # the CMake for a build
```

### The linker script

Where a program's memory is laid out is the part's fact, and the script is
written from the part's memory map. Three things can say otherwise, and they
are tried in this order:

| What | Where | What it means |
|------|-------|---------------|
| `linker_script` | the project | The project's own script, for a layout the part's does not describe |
| `linker.script` | the spec | The spec's own script, absolute or beside the spec that named it |
| `linker.toolchain_script` | the spec | The toolchain brings its own, and none is written |

The last one is for the toolchains that know their part. `avr-gcc` knows where an
AVR's vector table belongs and how its runtime expects to be reached; a script
written here is that knowledge repeated less well. The memory map is still
declared — what a program is told about the part it runs on does not depend on
who writes the script.

## The runtime's two hooks

A program on a part has no console and no way out, and the runtime asks the
platform for both. A platform that has its own answers them by name, in its
build:

```cmake
target_compile_definitions(robohub-avr PUBLIC
    RHO_PROGRAM_WRITE=rho_avr_write
    RHO_PROGRAM_HALT=rho_avr_halt)
```

| Hook | What it is | What the runtime does without it |
|------|------------|----------------------------------|
| `RHO_PROGRAM_WRITE` | The function that writes to the platform's console | The C library's `stdout`, which on a part is nothing |
| `RHO_PROGRAM_HALT` | The function a panic stops in | `exit`, which on a part is a path nowhere to go |

Both are named rather than decided by architecture, because a part the runtime
has never heard of is exactly the one whose console and stop are the platform's
business. A platform that says nothing about them gets the C library's
behaviour, which is right for a program on a host and wrong for a program on a
part.

The functions are the platform's own:

```c
/* The console: what the runtime's print writes to. */
int rho_avr_write(int fd, const char* data, int len);

/* The stop: what a panic ends in. It does not return. */
void rho_avr_halt(void);
```

## Reaching the board

A board says how it is written to, and that is a command:

```json
"flash": {
  "tool": "avrdude",
  "command": "avrdude -p m328p -c arduino -P {env:AVRDUDE_PORT:-/dev/ttyUSB0} -U flash:w:{binary}"
}
```

`{binary}` is where the file that was just built goes, and a command that never
says so has it appended.

A command may also read the environment: `{env:NAME}` is the value of `NAME`, and
`{env:NAME:-default}` is that value or the default when it is not set. That is how
a spec says something the *machine* decides — which serial line the board is on,
which probe it is behind — without the spec being wrong on every machine but the
one it was written on:

```sh
AVRDUDE_PORT=/dev/ttyUSB1 rho flash -target uno-r3 build/output/main
```

A reference with no default and no value is an error rather than a hole: a
flash command with a hole in it is a command that would flash nothing.

It is a substitution and not a shell. There is no pipeline, no glob and no
command substitution, so a spec that could name one is still a spec that cannot
run whatever a machine could be talked into.

The reference is spelled `{env:…}` rather than `${…}` because the command
travels into a CMake file, and CMake reads `${…}` as its own: a reference
written that way is a configure error rather than a placeholder.

## The platform's code

The library a program links, and the board support package that registers the
board's peripherals, are ordinary C in the platform's own tree. A program asks
for a peripheral by name — `user_led`, `system_timer` — and the board says which
pins those are.

A platform is a module like any other: it has a manifest, it is depended on by
path or by git, and a project says which preset it is written for.

```yaml
# my-app/module.yml
build:
  presets:
    - uno-r3

depends:
  - name: robohub-avr
    type: c
    path: ../robohub-avr
```

```sh
rho build -target uno-r3 .
rho flash -target uno-r3 build/output/main
```
