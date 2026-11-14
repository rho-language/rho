# Flashing a board

`rho flash` puts a built program on a board.

```sh
rho flash -target nucleo-h753zi -project . build/output/hello_world
```

## Why it is a component

A cross build produces an ELF file. Nothing has put it on the board yet, and the
program that does is not the one that compiled it.

What a board needs in order to be flashed is not a property of the flasher.
OpenOCD, `st-flash`, `picotool` and `dfu-util` each want a different
invocation, and which is right depends on the part, the probe, and what the
board has soldered on it. So the platform says, and this component runs what it
said:

```yaml
# a board's target spec
flash:
  tool: openocd
  command: openocd
  interface: interface/stlink.cfg
  target_config: target/stm32h7x.cfg
```

Nothing here is a switch over part numbers. A board this component has never
heard of is flashed by its spec declaring how, which is what makes a new board
a spec file rather than a release.

## Where the command comes from

| Source | When it is used |
|---|---|
| The board's target spec | The usual case: the spec declares the tool and its arguments |
| The `platform` contract | A board reached in a way a spec cannot state. A plugin answers the `flash` hook with the command, and this component runs it |

The contract exists because a spec can state a command but cannot *choose* one:
a platform whose probe is whichever one is plugged in computes the command from
what is installed, and that is a program rather than a value. An
integration plugin fills it for its ecosystem; {ref}`rho-robohub <rho-robohub/index>`
is one, and the handbook's integrations page describes the contract.

## Options

| Flag | What it is |
|---|---|
| `-target NAME` | The board, as its target spec names it |
| `-project DIR` | The project, whose manifest names the platform that declares the board |
| The positional path | The binary to flash |
| `--production` | Also run the platform's production plan; see below |
| `--dry-run`, `-n` | Print what would be run, and run nothing |

A board whose target spec declares no way to be reached is refused with that
said, rather than being flashed with a guess.

## Related

- {ref}`rho-target <rho-target/index>` — the spec that declares how a board is
  reached, and the build that produces the binary.
- {ref}`rho-monitor <rho-monitor/index>` — reading the board's console once it
  is running.
- {ref}`rho-dbg <rho-dbg/index>` — debugging it.

## Signed artifacts

`rho flash` and `rho deploy` accept `-artifact` with `-pub` (or
`-allow-unsigned` for development). The artifact is verified and extracted
before the flasher or deployment command runs. Remote OTA capability and
firmware rollback are described in `rho-artifact`'s remaining-work notes.

## Production flashing

Some hardening cannot be undone: an ESP32 burns eFuses for flash encryption and
secure boot, an RP2350 writes its boot key hash into OTP. An ordinary
`rho flash` never does any of it. `--production` does, and only after two
things:

1. It prints the whole plan — every step in the order it runs, the exact
   command with every placeholder filled in, the full contents of any file a
   step burns, and `[IRREVERSIBLE]` against each step that cannot be undone,
   including the ones a production image performs itself at its first boot.
2. It asks for the board's name to be typed. Only the exact name continues;
   anything else, an empty line or the end of the input stops before the first
   step. There is no flag that answers for you.

```sh
rho flash -target pico2w build/output/app.elf --production --dry-run   # read it
rho flash -target pico2w build/output/app.elf --production             # then do it
```

The plan is the platform's, not this component's. The platform module ships
`production/<name>.json`, looked up by the target's name, then its preset, its
family, and finally `production.json`; a platform with none has no production
mode and says so. A plan is a list of steps:

```json
{
  "name": "rp2350 production",
  "build": "how the image must have been built",
  "steps": [
    {"description": "what it does to the part", "irreversible": true,
     "when": "before-flash", "command": "picotool otp load {env:RH_PICO_OTP_JSON}",
     "show": "{env:RH_PICO_OTP_JSON}"}
  ],
  "notes": ["what is reversible, and how"]
}
```

`when` is `before-flash`, `after-flash` or `first-boot` (printed only: the
firmware does it). A command is split like a flash command, never handed to a
shell, with `{binary}`, `{env:NAME}` and `{platform}` (the platform module's
directory) filled in. The first step that fails stops the rest.

