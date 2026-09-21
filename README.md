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
go build -o ./rho ./cmd/rho
./rho build examples/hello-rho/src/main.rho
./rho run examples/hello-rho/src/main.rho
```

For a project, `rho.mod` marks the project root. Imports resolve from
`src/`, while `include "file.rho"` resolves relative to the source file that
contains the directive. Generated C output is written to `build/`.

## CLI Tools

The main `rho` command currently provides `build`, `run`, and `help`:

```sh
./rho build <file.rho>
./rho run <file.rho>
./rho help
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
name from `config/app.yml` and prints a greeting on the command line.

## License

Rho is released under the MIT License. See [LICENSE](LICENSE).
