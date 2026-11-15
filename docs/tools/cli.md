# The rho command

`rho` is the CLI. Its own commands are inside the binary and are the same on
every machine. Every other command belongs to a **component** and exists only on
a machine where that component is installed *and* enabled. `rho help` reads both
lists rather than printing one written down, so it shows what this machine can
actually do:

```sh
rho help              # the CLI's commands, then the enabled components'
rho help build        # one command's own help, as `rho build --help`
```

`rho help <command>` runs `rho <command> --help`, for the CLI's own commands and
a component's alike, and asking for help exits 0. A flag may be written before
the path a command takes or after it — `rho build . --target go` and
`rho build --target go .` are the same — with one dash or two, and as
`--name value` or `--name=value`; `--` ends the flags.

`help`, `version`, `init`, `new` and `install` work before anything is installed,
and so does any of the CLI's own commands asked for its `--help`.
Every other command reads the local registry, and on a machine without one it
says `The local registry is not installed` and exits 1.

## The CLI's own commands

| Command | What it does |
|---|---|
| `rho run [<project-dir>\|<file.rho>] [args...]` | Build and run a project, or one file, with the program's arguments |
| `rho build [<project-dir>\|<file.rho>]` | Generate and build every target the project declares |
| `rho init [<generator>] <directory>` | Start a Rho application, or a project a module knows how to generate |
| `rho new <file.rho>` | Start a Rho source file with its header |
| `rho install [--update\|--force]` | Install the toolchain, refresh it, or reinstall it |
| `rho pkg <command>` | `.rho-pkg` archives: `init`, `pack`, `unpack`, `inspect` |
| `rho module <command>` | Modules: the local registry, the remote one, and a project's dependencies |
| `rho plugin <command>` | Plugins: `list`, `enable`, `disable` |
| `rho config get\|set` | The remote registry's address and token |
| `rho ide <editor> [dir]` | Write what an editor reads: launch configurations, tasks, a workspace file |
| `rho protocol generate` | Generate C, Go or Rho from protocol schemas |
| `rho env [dir]` | Show the Rho home and the workspace a build resolves paths against |
| `rho version` | Print the toolchain version (also `--version`, `-v`) |
| `rho help [<command>]` | Show the list, or one command's help |

## rho run

```sh
rho run hello.rho                 # one file
rho run .                         # the project in this directory (the default)
rho run . sample.csv --sum price  # the program's own arguments follow the path
rho run . -- --help               # `--` after the path is optional
rho run -- sample.csv             # the project here, with arguments
```

A project whose manifest declares a Go target (`go` or `go-server`) is generated,
built with Go and run; the first Go target the manifest names is the one that
runs. Otherwise the program is translated to C and compiled with `gcc`, with the
C sources the manifest lists, and run. The program's exit status is passed on.

Every argument after the path is the program's, flags included: `rho run . -v`
gives the program `-v`. A `--` straight after the path is dropped, and a `--`
before any path runs the project in the working directory with what follows.
The program runs in the directory `rho run` was started from, so a relative path
among its arguments means what it means to the shell.

## rho build

```sh
rho build                           # the project in this directory
rho build path/to/project
rho build . --target pico           # a board, by the name its target spec has
rho build . --target go             # only the generated Go project
rho build . --build-type Debug      # Release by default
rho build . --no-link-map           # do not write the linker map
rho build --clean                   # remove the build directory and stop
```

| Flag | Default | What it does |
|---|---|---|
| `--target NAME` | the manifest's `build.toolchain`, else `native` | A backend id (`c` or `go`) generates only that project; any other name is a board or target that the `rho-target` component resolves |
| `--build-type TYPE` | `Release` | `Release`, `Debug`, `RelWithDebInfo` or `MinSizeRel`, matched without regard to case |
| `--link-map` / `--no-link-map` | the manifest's `build.link_map`, else on | Whether the link writes its map |
| `-o FILE` | none | Also write the translated C of a Rho entry to this file, when the project generates a C target |
| `--emit-c` | off | With `-o`: write only the translation and stop — no project is generated and nothing is built (what `rho test` uses) |
| `--clean` | off | Remove the project's build directory and stop |

There is no `--output` flag: where a generated project and its binary go is the
project descriptor's (`output_dir`, `build_dir`), not a flag's. By default a Go
target's program is `build/<name>`, named after the manifest's `module.name`, and
the C program is `build/output/<name>`, named after the module `src/main.rho`
declares; the build's last line names it (`Built binary: build/output/csv_report`).

A build resolves the dependencies and writes `rho.lock`, generates a project per
declared target under `build/generated/<target>/`, builds each Go project with Go,
and hands the C project to the `rho-target` component, which configures and
builds it with CMake for the target. A project that declares a C connection runs
`rho-cimport` first, and one that declares `config:` runs `rho-kconfig merge`.
An unknown build type is refused before anything is generated.

## Diagnostics

A program that is wrong is reported the same way by every command that reads
it — `rho build`, `rho run`, `rho lint` and the editor through `rho lsp` — one
line per mistake, in the form compilers share, so a terminal and an editor's
problem matcher read it alike:

```text
src/helper.rho:4:18: error: cannot use a string as u32 in `let y`; parse it with int_from_str(...) or i64_from_str(...) and convert the result, or change the declared type
src/main.rho:18:22: error: area takes 1 argument, and this call passes 2; it is called with (Point)
src/main.rho:8:1: note: area is declared here
src/loop.rho:10:5: warning: loop.run: the loop at line 10 keeps a string it builds in "history", ...
```

- **The place is the mistake's.** The file is the one the mistake is written in —
  an included file, an imported module, a file of the standard library — not the
  file that brought it into the program. The column is that of the expression
  that is wrong: the value of a `let`, the argument of a call, the name nothing
  declares. Lines and columns count from one, and a column counts characters.
- **Every independent mistake is reported.** The checker keeps going after an
  error: each declaration is checked, and inside a function each statement is.
  What follows from a mistake already reported is not reported again — a name
  nothing declares is reported once in a function, and a `let` whose value is
  wrong does not make every later use of its name an error. A parse error is
  recovered from at the next declaration that begins a line.
- **Twenty errors are shown**, then `and N more errors`. Warnings are always
  shown.
- **A note** is a second place worth seeing: where a function called wrongly is
  declared (and, for a generic, what it was specialised with at that call), or
  the line that includes or imports a file that does not parse.
- **A use of something deprecated** is a warning at the use, naming what to
  use instead, with the declaration as a note (code `deprecated`;
  {ref}`language/syntax` has the `@deprecated` tag). A warning does not change
  the exit status of `rho build`; `rho lint --fail-on warning` fails a run on
  one, for a pipeline that wants deprecations gone.
- **Refusals after the check** use the same form: a C build refusing `len` of a
  `[T]`, a target that cannot supply an imported module or a built-in (at the
  `import` or the call), a board refusing a loop that keeps its strings (at the
  loop).

The structure behind each line is `Diagnostic` in the compiler package
(`File`, `Line`, `Column`, `EndLine`, `EndColumn`, `Severity`, `Code`, `Message`,
`Notes`); `rho lint` writes the same findings as SARIF (rule `rho-check` for the
checker, `rho-parse` for the parser), and `rho lsp` publishes them, counted from
zero as the protocol counts, with the notes as related information.

## rho init and rho new

`rho init <directory>` writes a new application: `module.yml`, `src/main.rho`, a
`README.md` and a `.gitignore`. The module name is the directory's, made into a
name (`csv-report` becomes `csv_report`, and `rho init` says so);
`--description` is the manifest's description and the `@brief` of both files;
`--target c,go` declares both targets and takes `build.language` from the first.
In a repository whose manifest declares generators, it writes what a generator
describes instead. Its flags and the generator format are in
{ref}`documentation/generating`.

`rho new src/storage/cache.rho --description "What the cache keeps."` writes a
source file that starts with what its module already knows:

```rho
//
// @file cache.rho
// @brief What the cache keeps.
//
// @module acme.storage.cache
// @project acme-storage
// @author Ada Example
// @date 2026-11-12
//
// SPDX-License-Identifier: MIT
//

module acme.storage.cache
```

The module name is the module's import name (`provides.import`, otherwise its
`import_path` or its name) followed by the file's folders and name under `src/`
or `tests/`; a folder that repeats the name's last part is not repeated, and
`main.rho` is the module itself. The author is the manifest's maintainer (or
author, or the git user), the date is today, and the licence is the manifest's
`license`, or `NOASSERTION` when it states none. A file named `*_test.rho`
starts as a test file — a `check`, one `test fn` and a `main` that calls it — and
is added to the module's `tests/tests.yml` when it has one. Its opening comment
names the module's sources under `src/` to `include`, never `src/main.rho`, whose
`main` would clash with the test's own. `--author`, `--date` and `--force`
(replace a file that is there) change what it writes.

## rho install

```sh
rho install             # first time: build the toolchain, create the registry
rho install --update    # rebuild the toolchain in place; nothing else is touched
rho install --force     # remove the toolchain's directories, then install again
```

Run from the root of a checkout. What each one does, and which directories
`--force` removes and which it keeps, is in {ref}`documentation/installing`.

## rho config

```sh
rho config get
rho config set remote.url https://registry.rho-lang.org
rho config set remote.token <token>
```

The two keys are `remote.url`, the remote registry `rho module` publishes to and
installs from, and `remote.token`. `get` shows whether a token is set but never
the token. A credential stored by `rho auth login` is presented in preference to
`remote.token`, so after logging in there is no token to set.

## rho ide

```sh
rho ide vscode .
rho ide neovim . -target pico
```

The editors are `vscode` and `neovim`. It writes `.vscode/launch.json`,
`.vscode/tasks.json` and `<project>.code-workspace` from what the project
declares — its target, or the one `-target` names — with the debug sessions the
board's spec calls for. Neovim's DAP client reads the same `launch.json`. A file
that is already there is merged rather than replaced, so a configuration a person
added is kept.

## rho module

```sh
rho module publish <dir>          # pack, install here, and upload when logged in
rho module publish --local <dir>  # pack and install here only
rho module install <name>         # fetch from the remote registry and install
rho module enable <name>          # the version that runs; name@version pins one
rho module list                   # what is installed here
```

| Command | What it does |
|---|---|
| **The local registry** | |
| `install <name>[@version]` | Fetch a module from the remote registry and install it here |
| `uninstall <name>[@version]` | Remove an installed module; `--all` removes every version |
| `enable <name>[@version]` | Make a component or plugin the version that runs; with no version, the highest installed |
| `disable <name>` | Keep it installed and stop using it |
| `activate <name>@<version>` | Select the version a project resolves against |
| `deactivate <name>@<version>` | Clear that selection |
| `list [--format tsv] [query]` | What is installed here; `tsv` prints one line per version for a program |
| `search [--format tsv] [query]` | What is installed here, then what the remote registry has |
| **The remote registry** | |
| `publish <dir> [--local]` | Pack a module, install it here, and upload it when a remote is configured and you are logged in; `--local` never uploads |
| `unpublish <name>@<version>` | Withdraw a version; only the module's owner may |
| `visibility <name> [public\|private]` | Show or change whether anybody may install it |
| `mine [query]` | The modules you published, and their state |
| `keygen` | Make the key you sign what you publish with (`~/.rho/config/signing.key`) |
| `trust <name\|*> <public-key>` | Pin the key a module must be signed by when it is installed |
| `download <name>[@version]` | Fetch a package into the cache without installing it |
| **A project's dependencies** | |
| `install [dir]` | Resolve and install a project's dependencies |
| `update [dir]` | Update them |
| `remove <name>` | Drop one from a project |
| **A module on disk** | |
| `init <name> <rho\|module\|plugin>` | Create a module directory |
| `validate [dir]` | Check a manifest against the schema |
| `info [dir]` | Print what a manifest says |
| `check [dir]` | Compare its exports with its previous release, and list the modules a breaking change would reach |
| `ids [dir...]` | Print the id of every module under the directories, and fail when two claim one id or an id is malformed |
| `build [dir] [-o output-dir]` | Build a component's command, without installing it |

`install` takes a name or a path: an argument that starts with `.` or contains a
`/` is a project directory, so a module whose name is also a directory here is
written `./name` to mean the directory.

Installing and enabling are separate decisions. A module can be on the machine
because a dependency wanted it; running it is something a person says yes to,
which is why a component that is installed and not enabled is refused with the
command that enables it, and is not in `rho help`.

## rho plugin

```sh
rho plugin list                  # what is installed, what each provides, what runs
rho plugin enable rho-test-junit # name@version pins one
rho plugin disable rho-test-junit
```

The same record `rho module enable` writes; `enable` also says which hooks the
plugin will now be asked for.

## rho pkg

```sh
rho pkg init [directory]          # a new project; the same generator as rho init
rho pkg pack [directory]          # a .rho-pkg archive of a project
rho pkg inspect <file.rho-pkg>    # what an archive holds
rho pkg unpack <file.rho-pkg> [destination]
```

## rho protocol

Turns a schema — enums, messages and rpcs — into the code both ends of a link
need: packed little-endian C for a device, Go with the same wire format and JSON
for a service, or Rho. It is a command of its own because the two ends are built
apart. See {ref}`language/protocol`.

```sh
rho protocol generate --target c  --out build/protocol/c  schema/
rho protocol generate --target go --out build/protocol/go --package sensors schema/
```

| Flag | Default | What it does |
|---|---|---|
| `--target c\|go\|rho` | `c` | The code to generate |
| `--out DIR` | `build/protocol/<target>` | Where to write it |
| `--package NAME` | `protocol` | The Go package name |
| `--bus FILE` | none | A bus binding file: also generate the event-bus files, for `c` |
| `--transport FILE` | none | A framing file: add the wire framing, for `rho` |
| `--platform NAME` | none | Take the bus and transport bindings from that platform's integration plugin |

## rho env

```sh
rho env
rho env path/to/project
```

Prints `RHO_HOME` and `RHO_WORKSPACE` — the directory a project and the modules
built beside it live in — with where each value came from, and any ecosystem
workspace variables that are set. A manifest writes paths relative to these, so
a build that cannot find a sibling module is answered here first. It exits 1
when the workspace is not a directory.

## Component commands

A command that is not in the table above is a component's: the component
publishes the command in its manifest, and `rho` runs the enabled version of it
with the rest of the arguments, and exits with the component's exit status. {ref}`tools/components` lists every one, and each
component has its own page:

| Command | Component |
|---|---|
| `rho test` | {ref}`rho-test <rho-test/index>` |
| `rho doc` | {ref}`rho-doc <rho-doc/index>` |
| `rho fmt` | {ref}`rho-fmt <rho-fmt/index>` |
| `rho lint` | {ref}`rho-lint <rho-lint/index>` |
| `rho target` | {ref}`rho-target <rho-target/index>` |
| `rho flash`, `rho deploy` | {ref}`rho-flash <rho-flash/index>`, {ref}`rho-deploy <rho-deploy/index>` |
| `rho dbg`, `rho monitor` | {ref}`rho-dbg <rho-dbg/index>`, {ref}`rho-monitor <rho-monitor/index>` |
| `rho mem`, `rho bench` | {ref}`rho-mem <rho-mem/index>`, {ref}`rho-bench <rho-bench/index>` |
| `rho auth` | {ref}`rho-auth <rho-auth/index>` |
| `rho lsp` | {ref}`rho-lsp <rho-lsp/index>` |
| `rho doctor`, `rho keeper` | {ref}`rho-doctor <rho-doctor/index>`, {ref}`rho-keeper <rho-keeper/index>` |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Done |
| 1 | It failed: the code did not build, a module could not be published, a registry refused |
| 2 | The command was used wrongly: an unknown flag, subcommand or build type, a missing argument |
| 77 | `rho build` only: this machine cannot try the build yet — a dependency is not fetched, a toolchain is not installed, a board's vendor tree is missing |

77 is the automake convention for "skipped", and the one CMake and CTest use. It
lets a suite that builds every example for every board say "cannot be tried
here" rather than "failed". A component's codes are on its own page; the
commands that check something — `rho lint`, and the checking subcommands of `rho
mem` and `rho bench` — use 1 for "the answer is no" (a finding, a budget
exceeded, a regression) and 2 for a command they cannot carry out, so a pipeline
can tell a failed check from a typo.
