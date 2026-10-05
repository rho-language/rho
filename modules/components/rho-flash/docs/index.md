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
what is installed, and that is a program rather than a value. See
{ref}`rho-robohub <rho-robohub/index>` for the shape of such a plugin.

## Options

| Flag | What it is |
|---|---|
| `-target NAME` | The board, as its target spec names it |
| `-project DIR` | The project, whose manifest names the platform that declares the board |
| The positional path | The binary to flash |

A board whose target spec declares no way to be reached is refused with that
said, rather than being flashed with a guess.

## Related

- {ref}`rho-target <rho-target/index>` — the spec that declares how a board is
  reached, and the build that produces the binary.
- {ref}`rho-monitor <rho-monitor/index>` — reading the board's console once it
  is running.
- {ref}`rho-dbg <rho-dbg/index>` — debugging it.
