# rho-dbg

A debugging session an editor can drive.

`rho dbg` is a debug adapter. VS Code, Neovim, and anything else that speaks the
Debug Adapter Protocol talks to it on standard input; it talks to a debugger on the
other side, and starts the probe that sits between the debugger and a board.

```
the editor  <-- Debug Adapter Protocol -->  rho dbg  <-- Machine Interface -->  gdb
                                                                       |
                                                                       +--> openocd --> the board
```

Three processes, and the correspondence between what the editor asks for and what
the debugger is told. The correspondence is the whole of it: a breakpoint is
`-break-insert`, threads are `-thread-info`, a frame is `-stack-list-frames`, and a
step is `-exec-next`, `-exec-step` or `-exec-finish` depending which way it goes.
None of that is negotiable, which is why it is written as one table rather than
spread through a session.

## Running one

An editor starts it, and everything an editor's launch configuration can say is a
flag, so the two are the same thing:

```sh
rho dbg --dap --program build/output/blinky --target pico
```

And what a machine can do, which a failed session cannot tell you:

```sh
rho dbg engines                     # what debuggers this machine has
rho dbg check --target pico         # which of them are actually installed
```

## The part that is not obvious

**The board decides.** Which debugger, which probe, which port and which register
map come from the target spec, not from a launch configuration somebody wrote once.
`rho ide vscode` writes the editor's half of the same facts, so the two agree about
the board rather than disagreeing about it.

**Which side starts the program.** A program running on this machine is started by
the debugger. A program on a board was started by whatever flashed it, so the
debugger connects to a probe, writes the program, and continues to the entry point.
A session that gets this backwards hangs on the first Continue.

**The probe is started, and waited for, before anything connects to it.** A
debugger that connects to a port nothing is listening on reports a connection
failure, which is a true sentence about the connection and a misleading one about
the board.

**Registers are read when they are looked at.** The chip's register map is shown as
a scope of its own, and the values are read from the board when a peripheral is
expanded and forgotten when the program moves. A register value read at handshake is
a value of where the program used to be.

The map itself comes from the project first — `debug.svd` in the application's own
manifest, a path per board — then from what a project or a platform vendors, then
from the vendor's published set, worked out from the part number against what that
set actually publishes. A board's registers are never approximated: a part nothing
publishes, or a name two vendors use, stops the session with the names it was
looked under and the line to write.

Where the maps are kept and where they are fetched from are this component's own
settings — `svd_repository`, `svd_branch`, `svd_cache`, `svd_fetch` — declared in
its manifest and read from the `config/dbg.yml` that travels with it. The default
repository is the CMSIS device data repository; the default cache is shared by every
project on the machine, because a vendor's register map is the same file whichever
program is being debugged.

```sh
rho dbg svd --target nucleo-h753zi    # put this board's map in the cache
rho dbg registers --target pico       # the map as JSON, which is what the panel reads
```

## Another debugger, as a plugin

GDB is the debugger this component ships with. It is not *the* debugger: a board
debugged with LLDB, or with a probe that speaks something else, is a session on a
different engine — and the session above does not know or care which. So an engine
is a plugin, and what it contributes is a program that speaks the Debug Adapter
Protocol itself, which is what LLDB and probe-rs already ship.

A plugin declares the contract, and answers with the debugger it provides:

```json
{"engines": [{
  "name": "probe-rs",
  "kind": "adapter",
  "targets": ["nrf52840"],
  "program": "probe-rs",
  "args": ["dap", "--chip", "${target}"]
}]}
```

An engine that names a board beats the built-in one for that board, so enabling
the plugin changes what that board's sessions do — and `rho dbg engines` says what
a machine has. See the component's README for the manifest a plugin declares.

## What a target spec says

```yaml
debug:
  tool: arm-none-eabi-gdb
  server: openocd
  server_args: ["-f", "interface/cmsis-dap.cfg", "-f", "target/rp2040.cfg"]
  adapter: cortex-debug
  server_type: gdb
  interface: cmsis-dap
  svd: rp2040.svd
  port: "3333"
  load: true
```

Everything except the compiler is optional, and what is left out is derived rather
than defaulted: the debugger is the one that goes with the compiler, and the port
is the one GDB itself uses. The block is folded like any other — a chip gives what
its architecture says, a board gives what its chip does not.