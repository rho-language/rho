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

## Language Notes

### String Escapes

String literals decode escape sequences, so a JSON payload can be written
directly:

```rho
fn module_json(name: string, version: string) string {
    return "{\"name\":\"" + name + "\",\"version\":\"" + version + "\"}";
}
```

Supported: `\n \r \t \a \b \f \v \0 \\ \" \'`, plus `\xNN` (one or two hex
digits), `\uNNNN` and `\UNNNNNNNN`. An unknown escape such as `\d` is a lexing
error rather than a silent surprise, matching C and Go. Escapes are decoded by
the lexer and re-encoded during lowering, so both backends emit the same bytes.

### Generics

Generic structs and functions are monomorphised: each concrete application is
expanded into its own specialised declaration.

```rho
import std.core.result

fn parse(ok: bool) Result[u32, u32] {
    if ok {
        return Ok[u32, u32](7)
    }
    return Err[u32, u32](1)
}

let good: Result[u32, u32] = parse(true)
if good.is_ok {
    print("ok");
}
```

`Result[u32, u32]` above becomes a struct named `Result_u32_u32`, and `Ok`
becomes `Ok_u32_u32`, so the C and Go backends emit one definition per
instantiation. A generic that is never applied emits nothing. `default(T)`
yields the zero value of `T`, which is what backs `Option[T]` and `Result[T, E]`.

Type arguments are deduced, so they only need writing when deduction cannot
resolve them. A type is taken from the field values, the call arguments, or
the expected type at the use site:

```rho
let a: Box[i32] = Box{ 1 }          // T from the field value
let b: Box[u32> = wrap(7)          // T from the argument
let ok: Result[u32, u32] = Ok(7)    // E only from the declared type
```

### Type-less bindings

A `let` may omit its annotation and take the type of its value:

```rho
let total = 1 + 2        // i32
let label = "hi"         // string
let wrapped = wrap(7)    // Box_u32, from the deduced specialisation
```

A binding with neither an annotation nor a value is rejected, since there is
nothing to infer from.

### Default type parameters

A type parameter can declare a default, used when deduction cannot resolve it:

```rho
struct Result[T, E = Unit] {
    is_ok: bool
    ok: T
    err: E
}

fn Ok[T, E = Unit](v: T) Result[T, E] { ... }
fn Err[T = Unit, E = Unit](e: E) Result[T, E] { ... }
```

With those defaults the standard library needs no type arguments at all:

```rho
let good = Ok(7)     // Result[i32, Unit]
let bad = Err(1)     // Result[Unit, i32]
let none = None()    // Option[Unit]
```

`Unit` is a zero-sized type. It is synthesised when a default names it and the
program does not define it; a program that declares its own `Unit` (or any
other type) uses that instead.

A partial application is completed with the defaults before the name is
mangled, so `Pair[u32]` and a deduced `Pair[u32, Unit]` are the same
specialisation.

A parameter with neither a binding nor a default is still an error, reported by
name.

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

### Multi-Target Generation

A Rho application can be generated as C, as a Go command, or as a Go web
service. Declare the backends in `module.yml`:

```yaml
build:
  language: c
  targets:
    - c
    - go
    - go-server
```

`rho build` then writes one project per target:

```
build/generated/c/          app.c app.h main.c runtime.c CMakeLists.txt
build/generated/go/         app.go main.go runtime.go go.mod
build/generated/go-server/  app.go server.go main.go runtime.go go.mod static/
```

Each generated directory is a self-contained project:

```sh
cd build/generated/go && go run .
cd build/generated/c   && cmake -B out && cmake --build out
```

`build.targets` defaults to `c` when omitted.

**Target kinds.** `go` is a command line program (`func main` calls the Rho
entry point). `go-server` starts an `net/http` service with a `/healthz`
route, a `static/` asset folder, request logging and graceful shutdown, for web
APIs and REST backends. Add project routes in `registerRoutes` in the generated
`server.go`.

**Project descriptors.** The file layout of every target is data, not code,
held in `src/projectgen/descriptor.default.yml`. A project overrides it by
adding its own `project.yml` at the root:

```yaml
version: 1
output_dir: build/generated   # where generated projects go
build_dir: build              # where compiled artefacts go

targets:
  - id: api
    emitter: go
    kind: server               # cli or server
    module_path: example.com/org/api
    requires:
      - github.com/go-chi/chi/v5
    output_dir: dist/api       # overrides the descriptor default
    files:
      - path: app.go
        role: implementation
      - path: server.go
        role: server
      - path: static
        kind: dir              # creates a folder
```

Roles are `implementation`, `interface`, `runtime`, `entrypoint`, `server` and
`build`. A new backend needs a descriptor entry and an emitter registered with
`projectgen.RegisterBackend`; the generator itself is unchanged.
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
