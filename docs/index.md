# Rho

Rho is a small systems programming language that compiles to C and to Go: C for
host tools and microcontrollers, Go for command-line programs and HTTP services.
The compiler, its standard library and the tools around it are one toolchain,
built from this repository.

A Rho program has no `new`, no objects and no hidden I/O. What only a host can
do — keep a database, write a log file, open a socket — the program declares as
an `extern fn`, and the project asks the backend for a **port** that implements
it. What a project needs from a host, it asks for; what it does not ask for, it
does not carry. (Strings a program builds are allocated by the runtime; see
{ref}`language/index` for what that means on each backend.)

```rho
module hello

fn main() void {
    print("hello from rho\n");
}
```

```sh
rho run hello.rho
```

## If you are new

Read these in order; each one assumes the one before it.

1. {ref}`Installing <documentation/installing>` — build the toolchain from a
   checkout and put `rho` on your `PATH`.
2. {ref}`Getting started <documentation/getting-started>` — run a file, then
   start a project.
3. {ref}`Starting a project <documentation/generating>` — what `rho init`
   writes, and the manifest every tool reads.
4. {ref}`The language <language/index>` — types, modules, generics, and what a
   program can say.
5. {ref}`Building <building/targets>` — the C and Go backends, services, ports
   and boards.
6. {ref}`Modules <modules/modules>` — applications, libraries, components and
   plugins, and how they are published and installed.

## The sections of this site

| Section | What it covers |
|---|---|
| **Start** | Installing, a first program, starting a project |
| **Language** | The syntax, the type system, the built-in functions and the standard library |
| **Building** | Targets and what each one generates, HTTP services, ports, platforms and boards |
| **Modules** | Manifests, registries, plugins, interfaces, and how an ecosystem integrates with the toolchain |
| **Tools** | The `rho` command, the index of components and plugins, testing, and the documentation generator |
| **Components** | One page per component: the tools that add a command, such as `rho test` or `rho doc` |
| **Plugins** | One page per plugin: modules that extend a component rather than adding a command |
| **Applications** | The applications written in Rho in this repository: the module registry and this site |
| **API reference** | Generated from the standard library's Rho sources, so it describes what the compiler sees |

## A project

A project is a directory with a `module.yml`. Every tool reads that one file: what
the module is, what it depends on, which targets it builds, its configuration and
its memory budget.

```
my-app/
  module.yml        what this is, what it needs, what it builds
  src/main.rho      the program
  tests/            its tests, and tests/tests.yml listing them
  config/           declared configuration, if it has any
  docs/             its documentation, if it has any
```

```sh
rho build .                  # generate and build every declared target
rho run .                    # build if needed, then run
rho test .                   # run its tests (the rho-test component)
rho doc . -out build/docs    # build its documentation (the rho-doc component)
```

## The toolchain

`rho` is a small CLI: building, running, starting a project, installing, and
managing modules are its own commands. Everything else — the test runner, the
formatter, the documentation generator, the target specs, the debugger — is a
**component**: a separate program published as a module, installed into the
local registry, enabled, and reached by the command its manifest publishes.
Adding a tool does not mean changing the CLI. {ref}`tools/cli` lists the CLI's
commands and {ref}`tools/components` lists every component and plugin.

## Where things live

| Directory | What it holds |
|---|---|
| `src/` | The compiler: front end, intermediate representation, the C and Go code generators, project generation |
| `std/` | The standard library, as Rho sources |
| `cmd/` | The `rho` CLI and `rho-deps`, the core toolchain |
| `modules/components/` | Components: tools that publish a command |
| `modules/plugins/` | Plugins: modules that extend a component |
| `applications/` | Applications written in Rho: the module registry and the documentation site |
| `editors/` | The VS Code extension |
| `examples/` | Small example projects |
| `docs/` | These pages |

These pages are doc-as-code: Markdown in this repository, built by `rho doc`, with
the API reference generated from the Rho sources beside them.
{ref}`tools/documentation` describes how that fits together.
