# Rho

Rho is a systems programming language that compiles to C and to Go. A program
declares its structure and its interface in the language, and the backend
supplies the parts that need a host: storage, logging, documentation, the
system calls it cannot make itself.

That split is the whole design. Rho has no heap, no globals and no file I/O, so
a program that needs them declares them as `extern fn` and asks for a port by
name. What a project needs from a host, it asks for, and what it does not, it
does not carry.

## Start here

- {ref}`handbook/index` — the handbook: how a project is built, published and
  used, and how the toolchain is installed.
- {ref}`language/index` — the language: syntax, types, and the standard
  library.
- The **API reference** in the sidebar is generated from the Rho sources in this
  tree, so it describes what the compiler actually sees.

## A first program

```rho
module hello

fn main() void {
    print("hello from rho");
}
```

A module is a file, and its name is the dotted name an import uses. `main` is
the entry point of a program, and a program that returns nothing says so with
`void`.

## Where things live

| Tree | What it holds |
|------|---------------|
| `src/` | The compiler: front end, intermediate representation, code generation, project generation |
| `std/` | The standard library, as Rho sources |
| `modules/components/` | Components: the formatter, the logger, the test runner, the documentation generator, the language server, the credential manager |
| `modules/plugins/` | Plugins: modules that extend a tool rather than adding a command |
| `applications/` | Applications: the module registry |
| `examples/` | Small programs that show how a project is put together |
| `cmd/` | The Rho CLI and the tools that are part of the core toolchain |

## The tool

`rho doc` builds this site. It is a component like any other, so it is
published, installed and dispatched to rather than compiled into the CLI:

```sh
rho doc . -out build/docs
```

Everything you are reading is doc-as-code: the pages above are Markdown sources
in this repository, and the reference beside them is generated from the Rho
sources. See {ref}`handbook/documentation` for how that fits together.
