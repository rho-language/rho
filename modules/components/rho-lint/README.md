# rho-lint

One run of every static analyzer over one compilation database, told the same thing,
and the findings written out as SARIF.

```sh
rho lint                                  # every engine the settings name
rho lint . --target nucleo-h753zi         # a specific board
rho lint . --format sarif -o build/lint.sarif
rho lint . --tree-plugins --engine cppcheck   # a plugin from this project's own tree
rho lint engines                          # what this machine can lint with
rho lint check                            # which of it is installed
rho lint compile-commands -out build/compile_commands.json
```

## The one thing this insists on

An embedded C static analysis is only as good as the flags it was given. Without
`-DSTM32H753xx` the analyzer parses out the branches a build excludes — which are
usually the ones that pick the board — and reports nothing about the ones it is
actually there for. Without the vendor's include paths it cannot see a peripheral's
header at all. Without `-std=c11` it reads `_Bool` as a typo.

So every engine is handed a compilation database: the list of arguments the compiler
is really invoked with. Two ways to have one, in this order.

1. **One a build already wrote.** A generated C project sets
   `CMAKE_EXPORT_COMPILE_COMMANDS`, so configuring it produces
   `build/compile_commands.json` — containing the flags that came out of the
   toolchain file, the Kconfig merge, and each dependency's exported include paths.
   Nothing derived from a manifest knows any of that.
2. **One derived from `module.yml`.** The board's compiler and defines from
   `rho-target`, the project's sources and include directories from the manifest, and
   `-std=c11` because that is what the generated project uses. A best guess, and
   labelled as one everywhere it is reported.

The distinction is carried all the way into the output. A finding from an analyzer
that was given a guessed command line is a weaker claim than one given the real one,
and a report that flattens the two is a report that will be believed once too often.

A project written for several boards has one database per board, because one board's
`-DSTM32F446xx` and another's `-DSTM32H753xx` produce two different sets of branches
in the same headers. `--target` picks the one; reading the newest would be an analysis
of whichever board happened to be built last.

## Both kinds of project, one component

A Rho application and a hand-written C program differ in the language a person wrote
them in and in nothing else a lint run can see. After codegen both are a set of C
files and a compilation database, and the `c` engine reads both by the same rules. So
does the `rho` engine cover both, because for a Rho project it is the only one that
can see the program *before* it became C — a shadowed `let` in the Rho is a finding
about a line somebody wrote, where the same shadow in the generated C is a finding
about the generator.

```
$ cd examples/rho/hello_world && rho lint .
engine: rho read 1 source, found 0 findings
engine: c read 6 sources, found 0 findings
No findings. The compilation database was derived from the project's module.yml and
the nucleo-h753zi target spec rather than read from a build, so the flags these
findings rest on are the manifest's and the target spec's.

$ cd examples/c/hello_world && rho lint .
engine: rho reads rho and there was none here
engine: c read 1 source, found 0 findings
```

## The engines this component carries

Two, and they exist so that `rho lint` says something on a machine with nothing
installed rather than only reporting that nothing is. Everything else is a plugin.

| engine | reads | checks |
| --- | --- | --- |
| `rho` | `.rho`, through the compiler's own parser | parse, module name, unused import, shadowed binding, unreachable, empty block, TODO |
| `c` | the C a build compiles, plus the headers beside it | unbalanced braces, definition in a header, missing include guard, static in a header, empty body, TODO |

Every rule in both is decidable from the text alone — no preprocessor, no symbol
table, no type inference. That is a constraint rather than a limitation: a rule
needing those is a rule that would be wrong sometimes, and a lint tool that is wrong
sometimes is one people learn to distrust wholesale. The checks that need a front end
are clang-tidy's and cppcheck's job.

A Rho program and a C program that carry nothing the rules can act on are the test
material for the two rows, and both must produce nothing.

### `rho-unused-import`, and why it knows about generated modules

```rho
import ffi

fn main() void {
    if device_init() != 0 { return }
}
```

`ffi` is not named by anything, and it is not unused: `rho-cimport` generated its
declarations from the platform's headers into `build/generated/modules/ffi.rho`, and
they are in scope by virtue of the import. So an import whose module the *build*
generated is never reported — a rule that reported it would fire on every Rho
application with a C interface, which is most of them.

## Plugins: adding an analyzer

Which static analyzer a project runs is a fact about the project rather than about
this component. clang-tidy and cppcheck both exist and are both better than anything
written here, and a project's own convention checker is better than both. So an engine
is a *description of a program*, not an implementation of a checker.

```yaml
module:
  name: rho-lint-cppcheck
  type: plugin
  version: 0.1.0

main: rho-lint-cppcheck

extends: rho-lint
implements: [engine]
hooks: [engine]

commands:
  - name: engine
    run: rho-lint-cppcheck
    hook: engine
```

The hook receives the request as JSON on standard input and answers with the engines
it provides:

```json
{
  "version": 1,
  "payload": {
    "engines": [
      {
        "name": "cppcheck",
        "kind": "program",
        "languages": ["c"],
        "program": "cppcheck",
        "args": ["--enable=all", "--xml", "--xml-version=2",
                 "--template=gcc", "-p", "${database}", "${files}"],
        "format": "cppcheck",
        "targets": ["nucleo-h753zi", "nucleo-f446ze"],
        "version": "2.10",
        "tool": "https://cppcheck.sourceforge.io/",
        "rules": [
          {"id": "unusedVariable", "severity": "warning",
           "shortDescription": "A variable that is never read"}
        ]
      }
    ]
  }
}
```

A third reporter is the same shape with the `report` hook, which is asked how to
render a run's findings for a reader this component does not write for.

### Plugins in the project's own tree

A plugin in `modules/plugins` is a program that came from whoever wrote the tree, and
for a repository that was just cloned that is a stranger. So a run does **not** load
them by default, and `--tree-plugins` is the flag that says to — the same decision
and the same flag `rho-doc` and `rho-target` make. Installed plugins are loaded
always, because somebody chose to install them.

```sh
rho lint . --tree-plugins          # also run what this project's tree carries
rho lint engines --tree-plugins    # and see what it offers
```

### The fields, and the three that matter

| field | meaning |
| --- | --- |
| `program` | the executable. `${database}`, `${root}` and `${files}` are filled in from the run. |
| `args` | its arguments. `${files}` expands to *as many arguments as there are files*, so an analyzer that takes its sources separately gets them. |
| `format` | how its output is read. `gcc` (the default), `cppcheck`, `sarif` or `json`. |
| `languages` | which source languages it reads, so a run hands a C analyzer no `.rho`. Empty means `c`. |
| `targets` | the boards it is for. Empty means any — which is what an analyzer of C is, since the flags come from the database. |
| `rules` | the checks it carries. Optional: an engine that says nothing gets them assembled from what it found. |
| `qualified` | whether a finding's rule name carries the engine's name in front of it. On by default. |
| `format` as a plugin might also mean | a SARIF file. If the analyzer already emits SARIF — most of them now — it is a format name and nothing else. |

`${files}` expanding to a list is why it cannot be written inside a larger argument,
and `rho lint` refuses a spec that tries: building one argument out of several paths
produces a single argument holding a filename that does not exist.

`qualified` is on because `unusedVariable` and `unusedFunction` are different checks in
different analyzers, and a project that suppresses one has said nothing about the
other. An unqualified name would let it.

A spec that names a format this component cannot read is **refused**, not guessed at.
An analyzer whose output silently vanishes is indistinguishable from an analyzer that
found nothing, and a lint tool must not have that failure.

### What a plugin may and may not be

A plugin may only provide a **program**. An in-process driver would have to be
compiled into this component, which is the thing being avoided, so a spec claiming to
be one is refused rather than quietly run as something else — the same rule rho-dbg
applies to a debugger it cannot drive in process.

The one thing a plugin's program is looked for in, before `PATH`, is the plugin's own
directory. A plugin is a module with a directory, a name is unambiguous inside it, and
a project with two versions of an analyzer installed should get the one its plugin
shipped with.

## Normalized output

Every analyzer formats errors differently. clang-tidy writes gcc-shaped lines,
cppcheck writes XML, a bespoke rule engine writes JSON of an own inventing, and SARIF
is the one format all of them have somewhere to go. So every analyzer's output is read
into one shape and written out as SARIF — **one run per analyzer**, because that is
what a SARIF run is and because a consumer that merged them would lose the attribution
that makes "clang-tidy and cppcheck disagree about this line" answerable.

Two details in the writer are about a pipeline rather than about a person:

- **No timestamps.** Every field a run does not need is left out, and a clock reading
  is the one that guarantees two runs of the same tree differ. A pipeline that diffs
  its own output cannot do that if every run carries a time.
- **Relative paths against a declared base.** A report that says
  `/home/alice/...` says nothing about the code; a SARIF `uri` that differs per machine
  cannot be grouped by code scanning.

Two analyzers that report the same defect with the same wording produce one finding.
Two that report the same *place* differently produce two, because they are two claims
and somebody has to decide which to act on.

## Exit codes

| code | meaning |
| --- | --- |
| 0 | nothing at or above the threshold |
| 1 | findings at or above it, **or** no engine could run at all |
| 2 | the command could not happen — a project that declares nothing, a directory that is not one |

The second half of 1 is the load-bearing part. An analyzer that is not installed, one
that refused the compilation database, and a clean tree all come out of a run as
silence. So a run where *no* engine worked is a failure, and one engine failing out of
four is not — the other three ran, and their findings are real.

`rho lint check` is the command that tells those cases apart, which is why it exists
rather than being folded into a run.

## Settings

`config/lint.yml`, declared in this component's `module.yml` so that what a person
reads to find out what a run can be asked to do is the thing the component runs on.

```yaml
default_engines: rho,c,clang-tidy,cppcheck
fail_on: error
database: ""            # empty means the places this component looks
exclude: vendor/
outside: exclude        # `include` for a platform analyzed as a project of its own
```

Every finding is printed whatever `fail_on` says. The setting moves the exit code and
nothing else, because a threshold that also hid findings would hide the ones nobody
had decided about yet.

`outside` is the one worth understanding. An application's generated project compiles
its platform's sources beside its own — the HAL, the startup assembly, the vendor's
drivers — and those live in another checkout, are that checkout's to fix, and their
findings belong to whoever runs a lint over that checkout. A platform analyzed as a
project of its own has the opposite answer. Setting `include` on the platform and
leaving `exclude` in the application is what makes the same code linted once by
whoever owns it rather than once by everybody.

`build/generated/` is **not** excluded by default. For a Rho application it holds the
only C there is, and running clang-tidy over it with the build's own flags is the most
valuable analysis in the run. The `rho` engine reads the sources that produced it, so
the two do not report the same thing twice.

## Related

- `rho target` — which compiler a board is compiled with, and which defines it always
  has. Every one of those facts comes from here rather than from a copy of the specs.
- `rho doctor` / `rho keeper` — `clang-tidy` and `cppcheck` are declared in this
  component's `system_deps`, so the doctor reports on them and the keeper can install
  them.
- `rho dbg` — the same plugin engine contract for a debugger, with `one: true` because
  a debugging session is driven by exactly one. A lint run is *several* analyzers over
  one database, which is why both contracts are `one: false` here and what makes the
  normalized output worth having at all.