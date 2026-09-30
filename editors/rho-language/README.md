# Rho for VS Code

Syntax highlighting, and a language server that reads Rho the way the compiler
reads it.

The server is not a second opinion about the language. It uses the compiler's own
lexer, parser and type checker, so a squiggle in the editor and a build failure
are the same finding, said twice by the same reading, at the same position in the
file. When the language gains a keyword or a rule, the editor gains it with it —
there is no keyword list here to keep in step.

## Requirements

A Rho toolchain. The extension does not bundle one, and does not need the sources
it was built from: `rho install` puts the binary in the Rho home, and that is
where the extension looks for it.

```sh
rho install
```

## Finding the toolchain

Nothing needs configuring. The extension looks, in this order, and says which
one it used in the status bar's tooltip:

1. **`rho.lsp.serverPath`** — only when the toolchain is somewhere unusual. An
   absolute path is used as given; a relative one is read from the Rho home, which
   is where a deployed toolchain puts itself.
2. **`RHO_BIN`** — set by the toolchain for the processes it launches, so a
   session started through `rho` uses that toolchain rather than a copy found by
   searching.
3. **The Rho home** — `~/.rho/bin/rho`, or whatever `RHO_HOME` names.
4. **The path** — where it is once the install instructions have been followed.

If none of those find it, the extension offers to install the toolchain rather
than failing with a path nobody chose. If the server is installed but has not
been enabled — Rho components are enabled deliberately — a failed start offers
the one command that fixes it.

## Commands

Under **Rho** in the command palette, in the editor title bar, and in the context
menu of a `.rho` file.

| Command | What it does |
|---------|--------------|
| `Rho: Show Toolchain Status` | which binary it found, where, and the client's state |
| `Rho: CLI Reference` | the toolchain's own `rho help`, read from the binary |
| `Rho: Build Project` | `rho build .` |
| `Rho: Run File` | `rho run <file>` on the active editor |
| `Rho: Run Tests` | `rho test` |
| `Rho: Generate Documentation` | `rho doc . -out build/docs` |
| `Rho: Manage Modules` | `rho module list` |

`Ctrl+Alt+B` builds; `F5` runs the active file. The rest run in a terminal, so a
build that says something can be read in full.

## Settings

| Setting | Default | |
|---------|---------|---|
| `rho.lsp.serverPath` | `""` | where the toolchain is, when it is not found by itself |

## The language

Rho is a systems language that compiles to C and to Go. A program declares its
structure and its interface in the language, and the backend supplies the parts
that need a host: storage, logging, documentation, the system calls it cannot make
itself.

```rho
module hello

fn main() void {
    print("hello from rho");
}
```

A module is a file, and its name is the dotted name an import uses. `main` is the
entry point; a function that returns nothing says `void`.

The design has a consequence worth knowing before reading code: **Rho has no
heap, no globals and no file I/O.** A program that needs them declares them as
`extern fn` and asks for a *port* by name — which is what a component provides.
That is why a Rho program has no `open` call, and why a project that needs a
database declares a driver and gets one.


```rho
module geo

struct Point {
    x: i32
    y: i32
}

fn manhattan(a: Point, b: Point) i32 {
    let dx = a.x - b.x
    let dy = a.y - b.y
    if dx < 0 { return 0 - dx }
    if dy < 0 { return 0 - dy }
    return dx + dy
}
```

`struct` declares a shape, `fn` a function, `let` a binding, and an `if` returns
from the branch it is in. Structs are passed by value, so `a.x` reaches the copy a
function was handed rather than something it may change in its caller.

A failure is carried in the type rather than in a convention: `Option[T]` for
something that may be absent, and `Result[T, E]` for something that may fail, so
a caller is told at the call that a failure is possible.

`extern fn` names something the host supplies, and is how a program asks for a
port without knowing which one it will get.

## Where the rest is

The toolchain carries the documentation, and the dashboard serves it:

```sh
rho module enable rho-keeper
rho keeper            # http://127.0.0.1:8477
```

The **handbook** covers how a project is built, published and used, and how the
toolchain is installed. The **language reference** covers syntax, types and the
standard library. The API reference beside them is generated from the Rho sources
in the tree, so it describes what the compiler actually sees.

## Building the extension

```sh
cd editors/rho-language
npm install          # only needed once
npm run compile
./package.sh         # writes rho-<version>.vsix
code --install-extension rho-<version>.vsix
```

`package.sh` uses Python's standard library and needs neither npm nor `vsce` —
npm is used only to compile the TypeScript client. The version, publisher and
engine requirement are read from `package.json`, so they cannot drift from it.
