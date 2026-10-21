# Changelog

Notable changes to the Rho toolchain, one section per release. The section for
the version being published is what a registry shows against that version, so
this is the file the handbook's overview reads.

## [0.1.0] - 2026-10-12

First release of the Rho toolchain: the compiler, the C and Go backends, the
standard library, the module and package tools, and the component set — the
formatter, linter, documentation generator, test runner, debugger, target specs
and the keeper dashboard.

Added the module registry and its official deployment at
https://registry.rho-lang.org, where components and plugins are published and
installed from, and the handbook service that serves the documentation.

Modules gained a release history: a `CHANGELOG.md` beside the manifest, carried
in the package, whose section for a published version is recorded against that
release.

## [0.1.1] - 2026-10-20

The C runtime's symbols are callable from a program again. A fragment of the
runtime is emitted into the runtime's own translation unit and called from the
program's, and the prototypes the program was given said `static` — so a
program that called `panic()`, read an environment variable or loaded a YAML
file was compiled with a call to a symbol its own translation unit did not
define, and the link failed on a name that had been declared right there. The
prototypes and the definitions are external now, and the rule is a test.

A program's stop is the platform's, the way its console already was. A platform
names the function in `RHO_PROGRAM_HALT`, and a panic on a part ends there
rather than in the C library's `exit` — which is a firmware carrying an exit
path it has nowhere to go from. A platform that says nothing keeps the hosted
behaviour, and the platforms the runtime knows keep their own.

A pointer to a pointer is a pointer to the pointer: `Ptr_Ptr_Thing` is `Thing**`
in C, where it stopped at one star and named a different type than the program
meant.

Two different generic applications that mangle to the same name are reported by
the backend rather than one of them dropped: a program that compiles and means
something other than it says is worse than one that fails to compile.

The handbook has a page on platforms: what one is, the target spec that says
what the part is, and the console and the stop a program on a part is given.

A generated project names its program before the target file is read, so that
the target file can say how the board is reached in terms of the program it
builds: the `flash` target a build carries now names the file it wrote, and
splits the command into arguments rather than handing the tool the whole
command as one name it cannot parse.

## [0.1.2] - 2026-10-21

A loop can `continue`: the statement that leaves the innermost loop and goes
on with the next thing the loop has to do, which is what a guard inside a loop
body has always meant. A `continue` outside a loop is refused, as a `break`
outside a loop is.

The step of a `for` loop now comes before the body rather than after it, and it
is a real assignment rather than a piece of text. Two things follow. A `continue`
in a `for` no longer skips the step, so a loop whose body continues is a loop
that ends. And a `for` loop advances at all: the step was written in a form the
backends did not read, so the index never moved and the loop repeated its first
element for ever.

The native YAML reads more of a document: `yaml.value` takes a dotted path, so
`board.connectors` is the connectors of the board; `yaml.kind` says what a
value is — a list, a map, a string — because a program that reads a document
needs to know what it read; and `yaml.keys` says what a document holds.

A program can read a file and write one — `rho_read_text` and
`rho_write_text` — and the arguments of a program it runs are split on
whitespace honouring quotes, so a name with a space in it is one argument and
not two.

The C runtime answers the same as the Go one, which it did not. Its YAML
walked a document by its top-level keys and nothing else; it now walks a dotted
path the way the Go runtime does, and says what a value is and what a document
holds. A program's configuration is read the same on either backend too: the
name under the `user:` section, which the Go runtime was reading from the top
level instead.

The C runtime can run a program as well. The program is run directly rather
than through a shell, and what it said is written where the caller asked for
it, so a program that orchestrates other programs means the same thing on
either backend.
