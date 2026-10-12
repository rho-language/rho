# AGENTS.md

## Test Commands

The order matters. A component is a separate Go module that the toolchain runs as
a program, so a test that builds a program needs rho-target, rho-cimport and
rho-kconfig installed in the local registry and enabled. Running the tests before
that fails with `this build needs the rho-target component: module not installed`,
which is true of the environment and says nothing about the code.

```sh
# The whole procedure, in the order it has to happen. This is what CI runs and
# what a new machine runs: build, publish, install, test.
./scripts/check.sh

# Or one module at a time, once the toolchain is installed
cd modules/components/rho-doc && go test ./...

# Just the core module
go test ./...

# Build and install the core toolchain
go build -o ./bin/rho ./cmd/rho
./bin/rho install --update     # refresh; --force empties the registry first
```

`scripts/check.sh` publishes every module the tree carries, at the version its own
manifest declares, so there is no list of versions to keep and nothing to replace
by hand.

## Project Structure

- `cmd/rho` - Main CLI entry point
- `cmd/rho-deps` - Dependency manager
- `cmd/rho-pkg/` - Package management (init, pack, unpack, inspect)
- `cmd/rho-module/` - Module management (publish, unpublish, search, etc.)
- `src/` - Compiler, runtime, AST, parser, codegen
- `src/codegen/` - C and Go code generation backends, and the built-in ports
- `src/projectgen/` - Generated-project descriptors and backends
- `src/schema/` - YAML schema validation
- `src/plugins/` - How plugins are found and run for a component
- `src/yaml/` - Minimal YAML loader
- `internal/paths/` - Shared path utilities
- `docs/` - The language reference and the handbook, in doc-as-code
- `modules/components/` - Components: `rho-auth`, `rho-cimport`, `rho-dbg`, `rho-doc`, `rho-doctor`, `rho-flash`, `rho-fmt`, `rho-kconfig`, `rho-keeper`, `rho-lint`, `rho-log`, `rho-lsp`, `rho-target`, `rho-test`
- `src/ide/` - What an editor reads, rendered from what a project declares (`rho ide`)
- `src/sysdep/` - What a module needs from the machine: `system_deps`, versions, the fetched-toolchain layout
- `src/privilege/` - Saying something when a run has more authority than it needs
- `modules/plugins/` - Plugins: `rho-doc-toc`, `rho-test-gtest`, `rho-test-junit`

The core toolchain is only `rho` and `rho-deps`. Everything else in
`modules/components/` is a component: published, installed and dispatched to by
the command its manifest publishes, rather than compiled into the CLI.

## Components

A component is a module with a `command`, a `main` that is a binary in its own
root, and its own `go.mod`. To build one and make `rho <command>` reach it:

```sh
# Build the component binary into the module root, where the manifest's
# `main:` says it is
cd modules/components/rho-doc && go build -o rho-doc ./src

# Publish it, which installs it under the Rho home
./bin/rho module publish modules/components/rho-doc

# Now the command is reachable
rho doc . -out build/docs
```

## Releases

Every module carries a `CHANGELOG.md` beside its `module.yml`, with a section per
release. It is in the package, so it travels with the module, and the section for
the version being published is read at publish time and recorded by the registry
against that version:

```markdown
## [0.2.0] - 2026-02-01

What changed, in the module's own words.
```

A module's description says what it is and does not change between releases; the
release note is what tells two versions apart on a registry page. A version with
no section yet is published without a note rather than refused. The section
becomes valid JSON through `json_string`, never by pasting it into a document.

## Documentation

Documentation is doc-as-code: Markdown sources with the MyST constructs, plus an
API reference generated from the Rho sources. A module documents itself with a
`docs:` section in its `module.yml`; a `docs/` directory beside the manifest is
picked up without being declared; a repository documents itself with `docs/`.

```sh
# Build the whole site
rho doc . -out build/docs

# One module's pages only
rho doc applications/registry -section applications -out build/docs/registry

# List the plugins that extend the generator
rho doc -list-plugins

# Fail a build that reports a problem
rho doc . -strict
```

The reference is generated from the Rho sources in the tree, so it describes
what the compiler sees. What a cross-reference resolves to, and what a
generated page looks like, is decided in `modules/components/rho-doc/src/rhodoc/`.

## Build Targets

Rho applications generate C, Go, or Go web-service projects. The layout of
each target is data-driven by `src/projectgen/descriptor.default.yml`; a
project overrides it with its own `project.yml`. Applications pick backends via
`build.targets` in `module.yml`.

```sh
# Generate all declared targets (build/generated/<target>/)
./bin/rho build <project-dir>

# Build and run a generated Go project
cd <project-dir>/build/generated/go && go run .

# Build a generated C project
cd <project-dir>/build/generated/c && cmake -B out && cmake --build out
```

A target may ask for a port, which is how a project gets what only a host can
supply: `logemit` for logging, `fsstore` for a module store, `docemit` for the
documentation generator, `kconfig` for a resolved configuration.

## Debugging

`rho dbg` is a debug adapter: the Debug Adapter Protocol on one side, the GDB
Machine Interface on the other, and the probe that reaches a board in between.
The board's own facts come from its target spec, and the editor's from the same
place — `rho ide vscode` writes them, so a launch configuration and the session
it produces cannot disagree about the board.

```sh
rho dbg engines                       # what a machine can debug a board with
rho dbg check -target pico            # which of it is installed
rho dbg registers -target pico        # the chip's register map, as JSON
rho dbg svd --target pico             # put that map in the machine's cache
rho dbg --dap --program build/output/app --target pico
```

A debugger this component has never heard of is a plugin: a module in
`modules/plugins` declaring `extends: rho-dbg` with the `engine` contract names
the debugger's own adapter, and `rho dbg` runs it and relays to it. GDB is the one
it carries, because every cross toolchain has one.

A register map comes from the application first — `debug.svd` in its manifest, a
path per board — then from what the project or a platform vendors, then from the
vendor's published set, which `rho-dbg` is configured with (`svd_repository`,
`svd_branch`, `svd_cache`, `svd_fetch`, declared in its own manifest and read from
the `config/dbg.yml` that travels with it). A part that the published set does not
have stops the session rather than being approximated.

## Targets

A build for a dev board needs facts that belong to the board: which compiler,
where the flash is, and which pin the LED is on. Those live in specs the
`rho-target` component carries, and it writes the CMake and the linker script a
build reads from them:

```sh
rho target list                          # what the component knows
rho target show pico                     # one resolved target
rho target generate . --target pico      # write cmake/ into the project
```

The generated project includes `cmake/toolchain.cmake` before `project()` and
`cmake/target.cmake` after it, when they are there. A project that targets the
host has neither and builds as it did.

## System dependencies

A module says what it needs from the machine in `system_deps`, which is a
different kind of requirement from `deps` and is declared apart from it:

```yaml
system_deps:
  executables:
    - name: arm-none-eabi-gdb
      version: ">=10.0"            # the same constraint grammar `deps` uses
      version_flag: "--version"
      version_pattern: '...'       # which version in the output is meant
      fetch: {url: ..., archive: tar.xz, binary: bin/arm-none-eabi-gdb}
      install: {apt: gdb-multiarch, brew: arm-none-eabi-gcc}
  env_vars: [...]
  udev_rules: [...]
```

Everything in `deps` is another module and `rho-deps` resolves it. Everything in
`system_deps` is a program or a permission on the machine, and nothing this
toolchain fetches produces it.

`rho doctor` reads the blocks and reports; it changes nothing and exits 1 when
something is missing. `rho keeper` acts on that report — `sync`, `fix`, `install` —
and prefers a fetch into `~/.rho/toolchains/` over a package manager, asking for
root only for a udev rule and only for that one file. Both commands read the
doctor's JSON, so a terminal and the dashboard cannot disagree.

## Static analysis

`rho lint` runs every static analyzer a project has over one compilation database and
writes what they find as SARIF. It reads `compile_commands.json` when a build wrote
one — which the generated C project makes happen by setting
`CMAKE_EXPORT_COMPILE_COMMANDS` — and derives one from `module.yml` and the board's
target spec when nothing has, saying which of the two it used.

The analyzers are plugins (`extends: rho-lint`, the `engine` contract) that name a
*program* and how its output is read: `gcc`, `cppcheck`, `sarif` or this component's
own shape. The two it carries itself are the rules over Rho sources, through the
compiler's own parser, and over the C a build compiles — which is why one command
covers a Rho application and a hand-written C program alike.

```sh
rho lint                              # the run
rho lint engines                      # what this machine can lint with
rho lint check                        # which of it is installed, and where the database is
rho lint compile-commands             # put a compilation database in place
rho lint . --tree-plugins             # also run the plugins this project's tree carries
```

`rho` warns when it is run as root and never refuses: a container is root.

## Configuration

A project declares its configuration once, in a top-level `config:` section, and the
`rho-kconfig` component writes it in the form the toolchain it targets reads:

```sh
rho build <project>            # generates a project whose CMake resolves it
rho kconfig list --root <dir>  # every symbol, its value, and where it came from
```

The generator copies the declared fragments into the generated project and emits
the CMake that runs the merge, so the configuration is resolved by the build
rather than baked into a generated file. The dialects are `linux` (`.config` and
`autoconf.h`), `espidf` (`sdkconfig` and `sdkconfig.h`) and `freertos`
(`FreeRTOSConfig.h`).
