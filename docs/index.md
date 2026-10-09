# Rho

Rho is a systems programming language that compiles to C and to Go. A program
declares its structure and its interface in the language, and the backend
supplies the parts that need a host: storage, logging, documentation, the
system calls it cannot make itself.

That split is the whole design. Rho has no heap, no globals and no file I/O, so
a program that needs them declares them as `extern fn` and asks for a port by
name. What a project needs from a host, it asks for, and what it does not, it
does not carry.

```rho
module hello

fn main() void {
    print("hello from rho");
}
```

## Start here

- {ref}`getting-started <documentation/getting-started>` — install the toolchain, build a program, run it. Start
  here if you have not used Rho before.
- {ref}`language/index` — the language: types, modules, generics, and what a
  program can say.
- {ref}`cli <documentation/cli>` — every command the `rho` tool answers.
- {ref}`components <documentation/components>` — every component and plugin, and what each one does.
- {ref}`handbook/index` — the handbook: how a project is built, published and
  installed, and how the toolchain is laid out.
- The **API reference** in the sidebar is generated from the Rho sources in this
  tree, so it describes what the compiler actually sees rather than what
  somebody remembered to write down.

## The shape of a project

A project is a directory with a manifest. `module.yml` is the file the whole
toolchain reads: it says what the module is, what it depends on, how it is
built, and what its configuration and its memory budget are.

```
my-service/
  module.yml        what this is, what it needs, what it generates and builds
  src/main.rho      the program
  tests/            Rho tests
  config/           declared configuration, if it has any
  docs/             its documentation, if it has any
```

```sh
rho build .          # generate and build every declared target
rho run .            # build it if needed, then run it
rho test .           # discover and run its tests
rho doc . -out build/docs   # build its documentation
```

## What Rho is not

Stated plainly, because a language that implies otherwise is worse than one
that says so:

- **No hidden allocation.** There is no heap and no garbage collector. A value
  lives where it is bound, and a pointer is a pointer.
- **No globals.** State belongs to a value, which is what makes a function
  testable without a fixture.
- **No closure, and no higher-order function.**
- **No file I/O, time or network** unless a port supplies it.

## Where things live

| Tree | What it holds |
|------|---------------|
| `src/` | The compiler: front end, intermediate representation, code generation, project generation |
| `std/` | The standard library, as Rho sources |
| `modules/components/` | Components: tools that publish a command, such as the formatter, the test runner and the language server |
| `modules/plugins/` | Plugins: modules that extend a tool rather than adding a command |
| `applications/` | Applications written in Rho, such as the module registry |
| `cmd/` | The Rho CLI and the tools that are part of the core toolchain |
| `editors/` | The editor extension and the language server's client |
| `docs/` | These pages |

## The tool

`rho` is a small CLI. Everything beyond `build`, `run`, `install` and `module`
is a component: a program published with a manifest, installed into the local
registry and dispatched to by the command it publishes. `rho doc` — which built
this site — is one of them, so the CLI does not have to change for a tool to be
added.

Everything you are reading is doc-as-code: these pages are Markdown sources in
this repository and the reference beside them is generated from the Rho
sources. {ref}`handbook/documentation` describes how that fits together.
