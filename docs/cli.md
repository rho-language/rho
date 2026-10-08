# The rho command

`rho` is the CLI. Everything it answers is listed by `rho help`, which reads the
list rather than printing one written down: the toolchain's own commands are in
the binary, and a component's command exists only on a machine where that
component is installed *and* has been enabled.

```sh
rho help
```

## The toolchain's own commands

These are inside the binary and are the same everywhere.

| Command | What it does |
|---|---|
| `rho run <file.rho\|dir>` | Run a Rho program |
| `rho build [dir]` | Generate and build every target the project declares |
| `rho install [--update] [--force]` | Install the toolchain and create the local registry |
| `rho pkg <command>` | Package management: `init`, `pack`, `unpack`, `inspect` |
| `rho module <command>` | Modules in the local registry: publish, install, enable, list |
| `rho plugin <command>` | Plugins: list, enable, disable |
| `rho config <command>` | Read and set configuration |
| `rho ide <editor> [dir]` | Write what an editor reads: `launch.json`, `tasks.json` |
| `rho help` | Show the list |

## rho build

```sh
rho build .                          # build the project in this directory
rho build . --target nucleo-h753zi   # a board, by its target spec name
rho build . --build-type Debug       # Release by default
rho build . --no-link-map            # do not write the linker map
rho build -clean                     # remove the build directory and stop
```

| Flag | Default | What it does |
|---|---|---|
| `--target NAME` | the project's `build.toolchain` | The board or chip, as a rho-target spec names it |
| `--build-type TYPE` | `Release` | `Release`, `Debug`, `RelWithDebInfo` or `MinSizeRel` |
| `--link-map` | on | Write the linker map beside the program |
| `--output DIR` | the project's `build.output_dir` | Where the built program is copied to |
| `-clean` | off | Remove the project's build directory and stop |

The build type is `Release` unless it is asked for otherwise: a build is
optimised by default and a `Debug` build is something a person chooses while
chasing something. An unknown type is refused before any work is done.

## rho module

```sh
rho module publish <dir>        # publish a module from a directory into the local registry
rho module install <name>       # copy an installed module into this project's store
rho module enable <name>        # a component or plugin's command becomes runnable
rho module disable <name>
rho module list                 # what is in the local registry
rho module search <query>       # ask the configured remote registries
```

Installing and enabling are separate decisions. A module can be on the machine
because a dependency wanted it; using it is something a person has to have said
yes to. That is why `rho module enable` exists and why a command from a
component that has not been enabled is not in `rho help`.

## rho install

```sh
rho install             # first time: build the toolchain, create the registry
rho install --update    # rebuild the toolchain and refresh installed modules from the tree
rho install --force     # empty the local registry first
```

## rho protocol

Turns a schema — enums, messages and rpcs — into the code both ends of a link
need: packed little-endian C for a node, Go with the same wire and JSON for a
service. It is a command of its own because the two ends are built apart by
different toolchains. See {ref}`language/protocol`.

```sh
rho protocol generate --target c  --out build/protocol/c  schema/
rho protocol generate --target go --out build/protocol/go --package telemetry schema/
```

## Everything else

A command that is not in the list above is a component: `rho doc`, `rho mem`,
`rho lint`, `rho target`, and the rest. Each one is a program published with a
manifest and dispatched to by the command its manifest declares, so adding a
tool does not mean changing the CLI. {ref}`components <documentation/components>` is the index.

```sh
rho doc . -out build/docs      # this site
rho mem .                      # where a program's memory went
rho bench .                    # how long it takes
rho monitor                    # a board's console
rho lint .                     # static analysis, as SARIF
rho test .                     # discover and run tests
rho target list                # the boards this toolchain knows
```

## Exit codes

A command that can fail a pipeline uses three codes, so that "the thing you
asked about failed" and "the command could not run" are distinguishable:

| Code | Meaning |
|---|---|
| 0 | The thing was done |
| 1 | The thing was done, and the answer is no: a budget exceeded, a regression over its limit, a lint finding |
| 2 | The command was asked for something it cannot do: an unknown flag, an unknown build type |

A pipeline that treats the second and third alike has its gate switched off by
a typo.
