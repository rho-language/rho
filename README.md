# Rho

[rho-lang.org](https://rho-lang.org) · [documentation](docs/index.md) · [changelog](CHANGELOG.md)

Rho is a small systems programming language. It compiles to C and to Go: C for
host tools and embedded targets, Go for command-line programs and HTTP services.

This repository holds the compiler, the standard library, the toolchain's
components and plugins, the official module registry, the documentation site and
the documentation itself. The version lives in [`cmd/rho/VERSION`](cmd/rho/VERSION),
beside the command it describes, and `rho version` prints it — one number rather
than several that have to agree.

## Overview

Rho is deliberately narrow and explicit. There is no `new`, no objects and no
hidden I/O: what only a host can do, a program declares as an `extern fn`
and a project asks the backend for a **port** that implements it. The compiler is
written in Go.

Supported today:

- `module`, `fn`, `struct`, `enum`, `type`, generics with deduction and default
  type parameters, and `match`
- the integer types `u8`–`u64` and `i8`–`i64`, `f32`/`f64`, `bool` and `string`
- `if`, `while`, `for` (with `step` and `reverse`), `break` and `continue`
- modules with declared exports and dependencies, and interfaces bound to
  providers in the manifest
- YAML loading with dotted paths, and reading and writing a text file
- C and Go backends, and Go web services through routes, middleware and ports

The two backends differ in a few places where C leaves behaviour undefined —
signed overflow, division by zero, an over-wide shift, an index past the end of
an array. They are listed, with what differs, in `src/parity_cases_test.go`.
See [`SECURITY.md`](SECURITY.md) for what the toolchain trusts and what it does
not.

## Quick start

Requirements: Go 1.26.10 or newer and a C compiler; CMake for C targets.

```sh
go build -o ./bin/rho ./cmd/rho
./bin/rho install                   # build the toolchain into ~/.rho, add it to PATH
./scripts/check.sh                  # build and install every component, then run the tests

rho init my-app && cd my-app
rho run .
```

[`docs/installing.md`](docs/installing.md) and
[`docs/getting-started.md`](docs/getting-started.md) go through it step by step.

## Installation

`rho install`, run from the root of a checkout, installs into `~/.rho` (or
`$RHO_HOME`):

- creates the Rho home: `bin`, `modules`, `registry`, `cache`, `config`, `auth`, `pkg`;
- copies the standard library and the documentation into it;
- builds `rho`, `rho-deps` and the language server `rho-lsp`, installs them in
  `~/.rho/bin`, and registers and enables `rho-lsp` as a component;
- creates the local registry, the package cache and the configuration;
- adds `~/.rho/bin` to your shell's `PATH` (bash, zsh, fish, or `.profile`).

```sh
./bin/rho install            # first install
./bin/rho install --update   # rebuild the toolchain in place; nothing else is touched
./bin/rho install --force    # remove the toolchain and install it again
```

`--force` removes only `bin`, `cache`, `docs`, `modules`, `pkg`, `plugins`,
`registry`, `std` and `targets` under the Rho home — so every installed component
and plugin goes and has to be installed again — and keeps the credentials, the
configuration, application state and the fetched cross toolchains.

The other tools — `rho test`, `rho doc`, `rho fmt`, `rho target` and the rest —
are components. A first install does not have them; `./scripts/check.sh` builds
and installs all of them from the checkout, or, one at a time:

```sh
cd modules/components/rho-test && go build -o rho-test ./src && cd -
./bin/rho module publish --local modules/components/rho-test
./bin/rho module enable rho-test
```

A C build needs the `rho-target` component, which configures and builds the
generated CMake project.

## A project

A project is a directory with a `module.yml`, which marks its root and is the file
every tool reads. `rho init` writes one:

```sh
rho init my-app
```

```
my-app/
  module.yml      what it is, what it depends on, which targets it builds
  src/main.rho    the program
  README.md
  .gitignore
```

Imports resolve from `src/` and from the declared dependencies, while
`include "file.rho"` resolves relative to the file that contains it. Generated
projects are written under `build/generated/<target>/`.

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
let b: Box[u32] = wrap(7)          // T from the argument
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

### HTTP Routes

A module can bind a Rho function to an HTTP endpoint with a route
declaration. Paths use `{name}` to capture a segment:

```rho
route POST "/modules/{name}/{version}/rho-pkg" => publish_module
route GET  "/search"                             => search_modules

fn publish_module(req: Request) Response {
    let name: string = param(req, "name")
    return json(200, "{\"name\":\"" + name + "\"}")
}
```

A handler takes a single `Request` and returns a `Response`; both types and the
helpers a service needs are provided by the server target, so no import is
required. `Request` and `Response` are synthesised for targets without a server,
so a module still compiles as C.

The server target generates the wiring: one `net/http` wrapper per route, a
small path router that matches `{name}` segments, and a `/healthz` endpoint. The
router is generated rather than relying on the Go 1.22 `ServeMux` wildcards, so
a generated project builds with any Go toolchain.

**Ports.** A module declares what it cannot yet implement itself as `extern fn`,
and a descriptor asks the backend to generate that implementation:

```yaml
targets:
  - id: go-server
    emitter: go
    kind: server
    ports:
      - fsstore
```

The built-in ports are `fsstore` (a filesystem-backed module store), `logemit`
(structured logging), `docemit` and `docstore` (the documentation generator),
`sql` (a database), `httproute` and `openapiemit` (routing and API
descriptions). This keeps a Rho project pure Rho: nothing outside `src/` is
hand-written host code. A new built-in port is a new generator registered in
`portGenerators`; a plugin in the project's own tree may provide more.

## Multi-target generation

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
cd build/generated/c  && cmake -B out && cmake --build out
```

`build.targets` defaults to `c` when omitted.

**Target kinds.** `go` is a command-line program. `go-server` starts a `net/http`
service with a `/healthz` route, a `static/` asset folder, request logging and
graceful shutdown. It listens on `127.0.0.1` at the port the manifest names
(`module.port`, else `deploy.port`, else 8080) unless `RHO_HTTP_ADDR` says
otherwise.

**Project descriptors.** The file layout of every target is data, not code, held
in `src/projectgen/descriptor.default.yml`. A project writes its targets in
`module.yml` under `build.targets`: an entry is a name that selects a descriptor
target, or a map that defines one (replacing a target of the same id). A project
that keeps its descriptor apart puts it in `project.yml`, which says only what
differs from the built-in descriptor:

```yaml
# project.yml
targets:
  - id: go-server
    templates:
      - dir: templates          # every file, named by its file name
    default_locale: en
    locales:                    # listed, so an unfinished translation stays out
      - {locale: en, file: locales/en.json}
      - {locale: de, file: locales/de.json}
```

Every template a program asks for by a literal name, `template_get("page")`, has
to be carried by the target, or the build stops and names it.

An `auth` middleware takes what it is not told from the application: the
audience is the service it deploys as (`deploy.service`, or the module name), the
public key is `~/.rho/auth/public.pem`, where `rho auth keygen` puts it, and the
login route is `/auth/login?audience=<audience>`. `RHO_AUTH_URL` names the issuer
when the application starts; an application with no key file fetches the
issuer's published key from `<RHO_AUTH_URL>/auth/public-key` and keeps a copy.
`RHO_AUTH_STANDALONE=1` runs an application without an issuer: it answers
requests from its own machine only.

Roles are `implementation`, `interface`, `runtime`, `entrypoint`, `server` and
`build`. A new backend needs a descriptor entry and an emitter registered with
`projectgen.RegisterBackend`; the generator itself is unchanged.

## Applications

| Application | Description |
|-------------|-------------|
| [`applications/registry`](applications/registry) | The remote module registry, written in Rho and compiled to a Go HTTP service |
| [`applications/docsite`](applications/docsite) | Serves the Rho documentation as a site — an index, a menu, the pages and the release notes |

The registry speaks the endpoints the module client uses, so `rho module
publish`, `download` and `search` work against it. Rho's official registry runs
this application at **https://registry.rho-lang.org**. See its
[README](applications/registry/README.md) for the endpoints and configuration,
and [docs/deployment.md](applications/registry/docs/deployment.md) for the
deployment options.

## The CLI

`rho` itself answers `run`, `build`, `init`, `new`, `install`, `pkg`, `module`,
`plugin`, `config`, `ide`, `protocol`, `env`, `version` and `help`. Every other
command is a component's, reached once the component is installed and enabled;
`rho help` lists both. [`docs/tools/cli.md`](docs/tools/cli.md) is the full
reference.

```sh
rho build [<project-dir>] [--target c|go|<board>] [--build-type Debug]
rho run [<project-dir>|<file.rho>]
rho init <directory>
rho new src/storage/cache.rho --description "What the cache keeps."
rho module publish [--local] <dir>
rho module install <name>
rho module enable <name>
rho config set remote.url https://registry.rho-lang.org
```

Installing and enabling are separate decisions: a module can be on the machine
because something depended on it, and running it is something a person says yes
to.

### Components and plugins

| Group | Components |
|---|---|
| Building for a target | `rho-target`, `rho-cimport`, `rho-kconfig` |
| Putting a program somewhere | `rho-flash`, `rho-deploy`, `rho-artifact`, `rho-bootstrap` |
| Running and debugging | `rho-test`, `rho-dbg`, `rho-monitor` |
| Measuring and checking | `rho-mem`, `rho-bench`, `rho-lint` |
| Writing code | `rho-fmt`, `rho-lsp`, `rho-doc`, `rho-log` |
| The machine and the installation | `rho-doctor`, `rho-keeper`, `rho-auth` |
| Services | `rho-router`, `rho-sql`, `rho-openapi` |

Plugins: `rho-doc-api` and `rho-doc-toc` (documentation), `rho-test-gtest`,
`rho-test-ctest` and `rho-test-junit` (testing), `rho-sql-sqlite` (a SQL engine),
and `rho-robohub`, an example integration plugin that maps one ecosystem's
platform onto the toolchain's contracts.
[`docs/tools/components.md`](docs/tools/components.md) has one line and a link
for each.

### Editors

For VS Code, package and install the extension in `editors/rho-language`; the
version is the one in its `package.json`, currently 0.4.0:

```sh
cd editors/rho-language
./package.sh
code --install-extension rho-0.4.0.vsix
```

It starts the language server as `rho lsp`, finding `rho` through the
`rho.lsp.serverPath` setting, `$RHO_BIN`, the Rho home or `PATH`, so nothing needs
configuring after `rho install`. `rho ide vscode .` (or `rho ide neovim .`)
writes the launch configurations, the tasks and a workspace file from what the
project declares, so a debug session and the board's target spec cannot
disagree.

### Documentation

`rho doc` builds a documentation site from the tree: the doc-as-code sources each
module ships, an API reference generated from the Rho sources, and what the
installed plugins add. The output is static HTML.

```sh
rho doc . -out build/docs
```

The sources are MyST-style Markdown. A module documents itself with a `docs:`
section in its `module.yml` or a `docs/` directory; a repository documents itself
with `docs/`, and `docs/site.yml` gives the site its title, front page and
sections. See [`docs/tools/documentation.md`](docs/tools/documentation.md).

### Packages and modules

```sh
rho pkg pack [directory]                 # a .rho-pkg archive of a project
rho pkg inspect <file.rho-pkg>           # name, type, version, main, features
rho pkg unpack <file.rho-pkg> [dest]

rho module validate [directory]          # check a manifest
rho module info [directory]              # what a manifest says
rho module check [directory]             # compare exports with the previous release
rho module list [query]                  # what is installed here
rho module search [query]                # here, and on the remote registry
rho module activate <name>@<version>     # the version a project resolves against
rho module uninstall <name>[@version]    # remove an installed module
rho module unpublish <name>@<version>    # withdraw a version from the remote registry
```

Several versions of a module can be installed at once; `enable` picks the one a
command runs and `activate` the one a project resolves against. Publishing checks
the module's exports against its previous release and refuses a breaking change
without a new major version.

**The remote registry** — the official one is **https://registry.rho-lang.org**:

- `rho config set remote.url https://registry.rho-lang.org`
- `rho auth login` obtains a token; publishing needs one, reading does not
- `rho module publish` uploads when you are logged in; `--local` never uploads
- `rho module search`, `install` and `download` fetch, and report remote results
  apart from what is installed

### Flashing and deploying

`rho flash` writes a built binary to a board, with the tool and command its target
spec declares — OpenOCD, `st-flash`, `picotool`, `dfu-util`. `rho deploy` copies a
program or a container image to a host over SSH, from the spec's `deploy` block.

```sh
rho flash  -target pico build/output/blink
rho deploy -target my-sbc -run build/output/sensor-api
```

`rho artifact` packs and verifies signed artifacts — a manifest, a SHA-256 payload
and an Ed25519 signature — and `rho flash -artifact` and `rho deploy -artifact`
verify one before touching a board or a host:

```sh
rho artifact pack -name sensor-api -version 1.2.0 -kind service \
  -target linux-arm64 -entrypoint sensor-api -service sensor-api \
  -key release.key build/output/
rho artifact verify -pub release.pub sensor-api-1.2.0.tar.gz
```

`rho bootstrap` gives a machine its own identity from a provisioning contract —
an instance UUID and an Ed25519 key, kept owner-only — and can enroll it with an
enrollment server using a one-time token.

### Debugging

`rho dbg` is a debug adapter: the Debug Adapter Protocol to an editor, the GDB
Machine Interface to a debugger, and the probe in between.

```sh
rho dbg engines                                   # what this machine can debug with
rho dbg check -target pico                        # which of it is installed
rho dbg --dap --program build/output/blink --target pico
rho dbg registers -target pico                    # the chip's register map, as JSON
rho dbg svd --target pico                         # put that map in the machine's cache
```

The register map comes from the project first — `debug.svd` in the application's
manifest, a path per board — then from what the project or a platform vendors,
and only then from the vendor's published set. A part nothing publishes stops the
session rather than showing registers it guessed at. Another debugger is a plugin
declaring `extends: rho-dbg`; see `modules/components/rho-dbg/README.md`.

### What this machine needs

A module says what it needs from the machine in `system_deps` — a different kind
of requirement from `deps`, declared apart from it. Everything in `deps` is
another module and `rho-deps` resolves it; everything in `system_deps` is a
program or a permission somebody installs.

```yaml
system_deps:
  executables:
    - name: arm-none-eabi-gdb
      version: ">=10.0"          # the same constraint grammar `deps` uses
      version_flag: "--version"
      version_pattern: '[^\n]*?\)\s*([0-9][^\s(]*)'
      fetch:
        url: https://developer.arm.com/-/media/.../arm-gnu-toolchain-14.2.rel1-x86_64-arm-none-eabi.tar.xz
        archive: tar.xz
        binary: bin/arm-none-eabi-gdb
        version: "14.2"
      install: {apt: gdb-multiarch, brew: arm-none-eabi-gcc, pacman: arm-none-eabi-gdb}
  udev_rules:
    - name: 60-rho-probes.rules
      target_path: /etc/udev/rules.d/60-rho-probes.rules
      source: probe/60-rho-probes.rules
```

`rho doctor` reads those blocks and reports what the machine has (`OK`,
`MISSING`, `OUTDATED`, `WARNING`, `UNKNOWN`); it changes nothing. `rho keeper`
acts on the report:

```sh
rho keeper sync --dry-run    # what it would do, and none of it
rho keeper sync              # do it, after showing it and asking
rho keeper fix               # one thing at a time, asking about each
rho keeper install rho-dbg arm-none-eabi-gdb
```

A fetch into `~/.rho/toolchains/` is preferred over a package manager, and root
is asked for only to install a udev rule, for that one file.

## Examples

`examples/` holds small projects. `examples/hello-rho` loads a name from
`config/app.yml` and prints a greeting:

```sh
rho build examples/hello-rho
rho run examples/hello-rho
rho test examples/hello-rho
```

## License

Rho is released under the Apache License, Version 2.0. See [LICENSE](LICENSE).
