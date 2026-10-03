# rho-dbg

A debugging session an editor can drive.

One program and two protocols. On one side the Debug Adapter Protocol, which is
what VS Code, Neovim and anything else with a debug client speaks. On the other
the GDB Machine Interface, which is what every debugger worth using on a
microcontroller speaks. In between it starts the probe — OpenOCD, pyOCD, probe-rs
— because nothing else does: an editor cannot attach to a board, and a debugger
cannot find one.

```
the editor  <-- Debug Adapter Protocol -->  rho dbg  <-- Machine Interface -->  gdb
                                                                       |
                                                                       +--> openocd --> the board
```

## What it is for

A session's facts come from the board's target spec rather than from a launch
configuration somebody wrote once: which debugger, which probe, which port, and
which register map. `rho ide vscode` writes the editor's half of those same facts,
so the two agree about the board instead of disagreeing about it.

## Running one

```sh
# A session an editor starts. Everything an editor's configuration can say is a
# flag here, so a launch configuration and a command line are the same thing.
rho dbg --dap --program build/output/blinky --target pico

# What this machine can debug a board with, and which of it is installed.
rho dbg engines
rho dbg check --target pico
```

`--dap` is a flag rather than a subcommand because that is how an editor starts a
debug adapter: it runs the program and speaks to its standard input.

## The translation

Every request an editor makes becomes one or two debugger commands. None of the
mapping is negotiable, which is why it is written as a table rather than spread
through a session:

| the editor asks for | the debugger is told |
| --- | --- |
| `initialize` | `-gdb-set mi-async`, and the capabilities the engine really has |
| `launch` | `-exec-run`, or a continue to the entry point on a board |
| `attach` | `-exec-interrupt` — it halts where it is rather than continuing |
| `setBreakpoints` | `-break-delete` for the old ones, then `-break-insert` for each |
| `threads` | `-thread-info` |
| `stackTrace` | `-stack-list-frames` |
| `scopes` | `-data-list-register-names` + `-data-list-register-values`, `-stack-list-arguments`, `-stack-list-locals` |
| `variables` | what the scopes already read |
| `evaluate` | `-var-create` — which gives back a handle as well as a value |
| `continue` | `-exec-continue` |
| `next` / `stepIn` / `stepOut` | `-exec-next` / `-exec-step` / `-exec-finish` |
| `pause` | `-exec-interrupt` |
| memory for a register | `-data-read-memory-bytes` |

Which side of the connection starts the program is the part that is easy to get
wrong, and it is decided in the engine rather than by the caller: a program on this
machine is started by the debugger, a program on a board was started by whatever
flashed it, so the debugger connects to a probe, writes the program if the session
says to, and continues to the entry point.

## The register map

A chip's registers are shown as a scope of its own next to the frame's, read from
a CMSIS-SVD file. Values are read from the board when a peripheral is expanded and
forgotten when the program moves, because a register value read at handshake is a
value of where the program used to be.

Where the file comes from, in this order:

1. **the project**, in its own manifest — `debug.svd`, a path per board
2. **a repository**: a file the project carries, or one a platform it depends on
   carries in its vendor tree
3. **a plugin** that knows where this vendor keeps them
4. **the published set**, worked out from the part number

### Saying it yourself

```yaml
# the project's own module.yml
debug:
  svd:
    nucleo-h753zi: vendor/cmsis_svd/stm32h7xx.svd
    nucleo-f446ze: vendor/cmsis_svd/stm32f4.svd
```

A path **per board**, because a project built for six boards has six chips and one
path would be one board's registers standing in for all of them — on the five it is
wrong for, silently. What is named here is read as written: a session consults
nothing else, not the board's spec and not the published set.

### Working the part number out

A board's spec names a part (`STM32H753ZI`), and a vendor publishes its register
map per device under the vendor's own name (`STM32H753x`, one file for four
parts). So `rho dbg` works the part number out against **what the vendor actually
publishes** — the CMSIS device data repository's own file list, fetched once and
cached:

```
STM32H753ZI  ->  stm32h753zi  ->  stm32h753  ->  stm32h753x  ✓ STMicro/STM32H753x.svd
```

Most specific first, and a less specific name is only reached when every more
specific one published nothing — so a part that is published on its own never gets
its line's file, and a line never gets a family's.

**Nothing is fetched on a guess.** A name no file matches is reported, and a name
two vendors use is refused with both named, because choosing between them would be
a guess about which chip is on the desk. And when neither settles it, the session
stops rather than starting with every register view quietly wrong:

```
Cannot identify svd for target under debug: the published set has no register map
for stm32h723xx, under any of the names it is written as.
  the board is nucleo-h723zg
  the part is stm32h723xx, and the published set was searched under:
    stm32h723xx
    stm32h723
    stm32h723x
    stm32h7xx

  Point at the file for this board, in the project's own module.yml:

    debug:
      svd:
        nucleo-h723zg: vendor/cmsis_svd/stm32h7xx.svd
```

### The cache

Fetched once per **machine**, not per project: a vendor's register map is the same
file whichever program is being debugged, so a machine holds one of each.

```
~/.rho/cache/svd/STMicro/STM32H753x.svd    the map
~/.rho/cache/svd/index.json               the repository's file list, for the search
```

### What this component is set to

Four settings, declared in its own manifest and read from `config/dbg.yml` — the
file travels with the module, so a rho-dbg installed from a registry is configured
by the same file as one run from a checkout.

| setting | default | what it is for |
| --- | --- | --- |
| `svd_repository` | `https://github.com/cmsis-svd/cmsis-svd-data` | where a map is fetched from; empty means never reach for the network |
| `svd_branch` | `main` | which ref — a branch moves, a tag does not |
| `svd_cache` | `cache/svd` | where it is kept, relative to the Rho home |
| `svd_fetch` | `true` | whether to fetch at all |

```sh
rho dbg svd --target nucleo-h753zi    # put this board's map in the cache, now
rho dbg svd --source STMicro/STM32H753x.svd --refresh
```

## Another debugger

GDB is not the debugger. It is the one this component ships with, which is the one
every cross toolchain has. A board debugged with something else is a session on a
different engine, and the session above it does not know or care which.

So an engine is a plugin:

```yaml
module:
  name: rho-dbg-probe-rs
  type: plugin
extends:
  component: rho-dbg
  implements: [engine]
commands:
  - name: engine
    run: rho-dbg-probe-rs
    hook: engine
```

and the program it names, run and relayed to:

```json
{"engines": [{
  "name": "probe-rs",
  "kind": "adapter",
  "targets": ["nrf52840", "stm32f4"],
  "program": "probe-rs",
  "args": ["dap", "--chip", "${target}"],
  "comment": "the debug adapter probe-rs ships, driven directly"
}]}
```

Only an `adapter` — a program that speaks the Debug Adapter Protocol itself — is
what a plugin provides, because a debugger with an adapter of its own needs no
wrapper. Wrapping one in a protocol of ours would be a second implementation of
something that works, and the one that would rot first.

An engine that names a board beats the built-in one for that board, so enabling the
plugin changes what that board's sessions do. `rho dbg engines` shows what a
machine has; `rho dbg engines -target nrf52840` shows what it would choose.

## What a target spec has to say

```yaml
debug:
  tool: arm-none-eabi-gdb      # or left out, and derived from the compiler
  server: openocd              # the probe; omit for a board debugged in place
  server_args: ["-f", "interface/cmsis-dap.cfg"]
  adapter: cortex-debug        # what an editor drives it with
  server_type: gdb
  svd: rp2040.svd
  port: "3333"
  load: true                   # write the program to the board at the start
```

Everything except the compiler it builds with is optional, and what is left out is
derived rather than defaulted: the debugger is the one that goes with the compiler,
and the port is the one GDB itself uses.

## What a failure says

The three ways a session fails to start look identical from inside it: a board
that is not connected, a debugger that is not installed, and a probe server that
is not installed all end as a connection that was refused. So `rho dbg check` says
which of them applies, before anything has been started, and a probe that says why
it cannot find a board has that said in the failure — because it is not in any error
raised anywhere else.