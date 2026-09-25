# Rho
Version: v0.1

Rho is a small, experimental language for the RoboHub ecosystem. It is
designed to compile down to portable C for host-side tooling and embedded
development workflows.

This repository contains the language compiler, runtime helpers, examples,
and the minimal infrastructure needed to build and test host-side Rho code.

## Overview

Rho currently focuses on a narrow, explicit subset of language features suited
for early experimentation and simple toolchain integration. The compiler is
written in Go and emits C for x86 host execution and embedded targets.

## Current Scope

Supported features include:

- `module`, `fn`, and basic type declarations
- `string`, `bool`, `u8`, `u32`, and `u64` primitives
- minimal YAML loading for simple nested maps and lists
- host-side example execution and basic console output

The language is intentionally small and the runtime is still in an early stage.

## Build

Requirements:

- Go 1.22 or newer
- CMake 3.16 or newer for the optional wrapper build
- A C compiler for generated C output

From the project root:

```sh
go test ./...
go build ./...
```

The CLI can compile the example project to `build/main.c` and run it natively
on x86:

```sh
go build -o ./bin/rho ./cmd/rho
./bin/rho build examples/hello-rho
./bin/rho run examples/hello-rho
./bin/rho test examples/hello-rho
```

For a project, `rho.mod` marks the project root. Imports resolve from
`src/`, while `include "file.rho"` resolves relative to the source file that
contains the directive. Generated C output is written to `build/`.

## Installation

Install Rho to `~/.rho` (or `$RHO_HOME` if set) with all tools, registry, cache,
and PATH integration:

```sh
./bin/rho install            # interactive install
./bin/rho install --force    # reinstall (removes existing ~/.rho)
```

The install command:
- Creates `~/.rho/{bin,modules,registry,cache,config}` directories
- Builds and installs `rho`, `rho-fmt`, `rho-test`, `rho-deps`, `rho-doc`
- Initializes local registry and package cache
- Adds `~/.rho/bin` to your shell PATH (bash, zsh, fish, or generic)

After install, restart your shell or run the suggested `source` command, then
`rho` is available globally:

```sh
rho help
rho pkg init my-project
```

## CLI Tools

The main `rho` command provides `build`, `run`, `test`, `lsp`, `install`, `pkg`, `module`, and `help`:

```sh
./bin/rho build <project-dir>
./bin/rho run <project-dir>
./bin/rho test <project-dir>
./bin/rho lsp
./bin/rho install [--force]
./bin/rho pkg <command> [args]
./bin/rho module <command> [args]
./bin/rho help
```

The language server can also be built as a standalone executable:

```sh
go build -o ./bin/rho-lsp ./cmd/rho-lsp
./bin/rho-lsp
```

For VS Code, install the `Rho` extension (includes built-in LSP client):

```sh
cd editors/rho-language
./package.sh
code --install-extension rho-0.2.0.vsix
```

The extension automatically launches `bin/rho lsp` from your workspace root. Configure the server path in VS Code settings if needed:

```json
{
  "rho.lsp.serverPath": "bin/rho"
}
```

No separate Generic LSP Client extension is required.

The server intentionally prints no startup banner. It waits for the client's
LSP `initialize` request. To smoke-test the integrated command manually:

If Generic LSP Client reports `activation (enabled: false)`, set the open
file's language mode to `Rho` in the bottom-right status bar, run
`Developer: Reload Window`, then run `Generic LSP Client: Restart server`.
Use `View: Toggle Output` and select `Generic LSP Client`; a successful launch
logs `Started language server`.

```sh
go build -o ./bin/rho ./cmd/rho
python3 -c 'import json,subprocess; p=json.dumps({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///"}}).encode(); print(subprocess.run(["./bin/rho","lsp"], input=(f"Content-Length: {len(p)}\\r\\n\\r\\n".encode()+p), stdout=subprocess.PIPE, check=True).stdout.decode())'
```

The repository also contains standalone tools. Build each tool from the
project root with `go build`.

### Documentation

`rho-doc` generates Markdown documentation for a Rho module, including its
structs, fields, functions, parameters, and return types:

```sh
go build -o ./rho-doc ./cmd/rho-doc
./rho-doc examples/hello-rho/main.rho
./rho-doc examples/hello-rho/main.rho > docs/hello.md
```

### Formatting

`rho-fmt` prints canonical formatting by default. Use `-w` to write the
formatted source back to the input file:

```sh
go build -o ./rho-fmt ./cmd/rho-fmt
./rho-fmt examples/hello-rho/main.rho
./rho-fmt -w examples/hello-rho/main.rho
```

The formatter currently covers the syntax represented by the parser AST. Full
statement and expression formatting will expand as the language does.

### Package Management

`rho pkg` manages Rho project scaffolding and `.rho-pkg` archives:

```sh
# Create a new project with module.yml, src/main.rho, README.md
rho pkg init [directory]

# Create a .rho-pkg archive from a project directory
rho pkg pack [directory]

# Inspect a .rho-pkg archive (shows name, type, version, main, features)
rho pkg inspect <file.rho-pkg>

# Unpack a .rho-pkg archive
rho pkg unpack <file.rho-pkg> [destination]
```

The `rho pkg` commands are built into the main `rho` binary - no separate
build step required.

### Module Management

`rho module` manages Rho module dependencies and publishing:

```sh
# Create a new module (library) or plugin
rho module init <name> <type>     # type: rho, module, or plugin

# Validate a module structure
rho module validate [directory]

# Show module metadata
rho module info [directory]

# Publish a module to local registry
rho module publish [directory]

# Unpublish a module version from registry
rho module unpublish <name@version>
rho module uninstall <name@version>   # alias for unpublish

# Activate a specific version (only one active at a time per module)
rho module activate <name@version>

# Download a module from registry
rho module download <name>[@version]

# Install all dependencies from module.yml
rho module install [directory]

# Remove a dependency
rho module remove <name>[@version]

# Update dependencies to latest compatible versions
rho module update [directory]

# Search registry for modules (shows active status)
rho module search [query]
rho module list [query]   # alias for search
```

**Module Versioning & Active Versions:**
- Multiple versions of the same module can be installed simultaneously
- Only one version per module is active at a time
- Publishing the first version of a module auto-activates it
- `rho module search` / `rho module list` shows `[ACTIVE]` marker for active versions
- `rho module unpublish` fails if trying to unpublish an active version that has other versions available (must switch active version first)
- If unpublishing the only version (even if active), it succeeds and clears from active list

**Remote Registry (Future):**
- Registry configuration stored in `~/.rho/config/registry.yaml`
- When a remote registry URL and auth token are configured, `rho module publish` uploads to remote
- `rho module download` fetches modules from the remote registry
- Remote results are shown with `[REMOTE]` marker in search results

The `rho module` commands are built into the main `rho` binary - no separate
build step required.

### Device Flashing

`rho-flash` is a standalone hardware tool for supported targets. It requires
the target-specific external tools and hardware:

```sh
go build -o ./rho-flash ./cmd/rho-flash
./rho-flash -mcu pico firmware.bin
./rho-flash -mcu stm32 firmware.bin
```

## Examples

The repository includes a minimal project in `examples/hello-rho` that loads a
name from `config/app.yml` and prints a greeting on the command line. Build and
run it with:

```sh
./bin/rho build examples/hello-rho
./bin/rho run examples/hello-rho
```

## License

Rho is released under the MIT License. See [LICENSE](LICENSE).
