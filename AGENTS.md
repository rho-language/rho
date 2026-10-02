# AGENTS.md

## Test Commands

```sh
# Run all Go tests in the core module
go test ./...

# Run a module's tests: components and plugins are separate Go modules
cd modules/components/rho-doc && go test ./...

# Build every module in the repository, core included
for m in modules/components/*/ modules/plugins/*/; do
  [ -f "$m/go.mod" ] && (cd "$m" && go build ./... && go test ./...)
done

# Build the rho CLI
go build -o ./bin/rho ./cmd/rho

# Build and install the core toolchain
./bin/rho install --force
```

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
- `modules/components/` - Components: `rho-auth`, `rho-cimport`, `rho-doc`, `rho-flash`, `rho-fmt`, `rho-kconfig`, `rho-log`, `rho-lsp`, `rho-target`, `rho-test`
- `modules/plugins/` - Plugins: `rho-doc-toc`, `rho-test-junit`

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
