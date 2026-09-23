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

## CLI Tools

The main `rho` command provides `build`, `run`, `test`, `lsp`, and `help`:

```sh
./bin/rho build <project-dir>
./bin/rho run <project-dir>
./bin/rho test <project-dir>
./bin/rho lsp
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

### Packages

`rho-pkg` manages the minimal `rho.yaml` project manifest. It does not yet
download, resolve, or install dependencies:

```sh
go build -o ./rho-pkg ./cmd/rho-pkg
./rho-pkg init
./rho-pkg add common ../robohub-common
./rho-pkg list
```

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
