# Static analysis

`rho lint` runs every static analyzer a project has over one compilation database,
tells them all the flags the compiler is really invoked with, and writes what they
find as SARIF.

```sh
rho lint                                  # every engine the settings name
rho lint . --target nucleo-h753zi         # a specific board
rho lint . --format sarif -o build/lint.sarif
rho lint . --tree-plugins                 # also run what this project's tree carries
rho lint engines                          # what this machine can lint with
rho lint check                            # which of it is installed, and where the database is
rho lint compile-commands                 # put a compilation database in place
```

## Why a compilation database comes first

An embedded C static analysis is only as good as the flags it was given. Without
`-DSTM32H753xx` an analyzer parses out the branches a build excludes — usually the
ones that pick the board — and reports nothing about the ones it exists for. Without
the vendor's include paths it cannot see a peripheral's header at all.

So a run reads the compilation database a build wrote, and derives one from the
project's own `module.yml` when nothing has. A generated C project sets
`CMAKE_EXPORT_COMPILE_COMMANDS`, so configuring it is enough; and the derived one
carries the board's compiler and defines from `rho-target`.

Every run says which of the two it used, because a finding from an analyzer given a
guessed command line is a weaker claim than one given the real one.

## Both kinds of project

A Rho application and a hand-written C program differ in the language a person wrote
them in and in nothing else a lint run can see. After codegen both are C files and a
compilation database, so one engine reads both — and for a Rho project the `rho`
engine reads the program *before* it became C, which is where a finding about a line
somebody wrote comes from.

What the build wrote under the project's build directory — the generated C, its
runtime — is never read unless `--include-generated` asks for it: a finding there
is not the project's to fix. A test file is read as the compiler is given it, with
the test runner's `test` marker removed, so `test fn` is not a parse error and the
lines and columns stay the file's.

`rho-module-name` holds a file to the name `rho new` writes: the module's import
name, then the file's folders and name under `src/` or `tests/`, and the import name
alone for `src/main.rho`. A file whose name ends in its file's stem is also right.
`rho-unused-import` counts a bare call of a function the imported module declares
as a use (`import std.assert.assert` then `assert_eq(...)`), as the compiler's
loader resolves it. `rho-shadowed-binding` reads a function in order, so a binding
written after a loop shadows nothing the loop bound.

Two of its rules are the compiler's own reading. `rho-parse` reports each place the
parser stops — it recovers at the next declaration, so two broken functions are two
findings — and `rho-check` runs the checker `rho build` runs, over the program the
file belongs to, and reports every error it finds in that file. Both report at the
line and column the build prints, with the build's message, so a lint run and a
build cannot disagree about a Rho file (the format is in {ref}`tools/cli`).

## The analyzers are plugins

Which analyzer a project runs is a fact about the project rather than about the
component. clang-tidy and cppcheck both exist and are both better than anything
written here, and a project's own convention checker is better than both. So an engine
is a description of a program, not an implementation of a checker:

```yaml
extends: rho-lint
implements: [engine]
```

The hook is handed the request as JSON and answers with the analyzers it provides —
a path, some arguments, and which of the component's output formats to read them as.
An analyzer that already emits SARIF is a format name and nothing else.

A plugin in `modules/plugins` came from whoever wrote the tree, so it is loaded only
when `--tree-plugins` says to — the same decision `rho-doc` and `rho-target` make.
Installed plugins are loaded always.

`rho lint engines` lists what this machine has. `rho lint -list-plugins` lists what
was found and which component each one extends.

## What the exit code means

| code | meaning |
| --- | --- |
| 0 | nothing at or above the threshold |
| 1 | findings at or above it, or no engine could run at all |
| 2 | the command could not happen |

The second half of 1 is the point. An analyzer that is not installed, one that refused
the compilation database, and a clean tree all come out of a run as silence — so a
run where nothing worked is a failure, and `rho lint check` is the command that tells
those cases apart before anything is started.

## Settings

Which engines run, how loudly they have to be, and what is never read are decided
once per project and live in `config/lint.yml`, declared in the component's manifest.

See the component's `README.md` for the plugin contract, the output formats, and what
`outside: include` is for.