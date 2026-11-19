# Testing

`rho test` runs a project's tests. It is the {ref}`rho-test <rho-test/index>`
component, so it has to be installed and enabled first:

```sh
rho test                    # the project in this directory
rho test path/to/project
rho test src/math_test.rho  # one file
```

The runner asks four questions in turn: what the project says it has, which of
it this run asked for, who can run each suite — the runner itself or a plugin —
and what came of it.

## A test file

A test is a function declared with `test fn` instead of `fn`, in a file whose
name ends in `_test.rho`. Apart from the marker it is an ordinary Rho program,
with a `main` that calls its tests. **`main` must call every `test fn`**: a test
it never calls fails.

```rho
module greeting_test

include "../src/greeting.rho"

import std.assert.assert

test fn prefix_is_hello() void {
    assert_eq(prefix(), "Hello ")
}

test fn greeting_names_the_person() void {
    assert_true(str_eq(greet("Ada"), "Hello Ada"))
}

fn main() void {
    prefix_is_hello()
    greeting_names_the_person()
}
```

The code under test is usually `include`d rather than imported, so the test sees
the module's private functions as well as its exports.

To run a file, the runner removes the `test` marker, translates what is left to
C with `rho build --emit-c --test-report <names> -o`, compiles it with `cc`, and
runs it; the C file
and the program are written under `build/test/` and removed afterwards. Nothing
else is built: the project's generated trees and its binaries (`build/<name>`,
`build/output/<name>`) are left as they are, so a test run never replaces the
application's program.

**Rho test files run on the C backend only**, whatever targets the project
declares. A difference between the C and the Go program is not something
`rho test` can see; a `command` suite that builds both and compares their output
is how a project checks that.

`--test-report` makes it a test build: the compiler wraps each `test fn` it names
so that, every time the function returns, it prints `[PASS] <name>` on standard
output (unbuffered, so a later test that stops the program cannot lose it). The
function's body, arguments and return value are unchanged; it moves to a function
of its own (`<name>__rho_test_body` in the C) and the wrapper keeps the name. A
test does not print its own `[PASS]` line; it may, and a second pass line changes
nothing. An ordinary `rho build` never does this.

A test fails by stopping the program: an assertion that fails panics, so the
wrapper never reaches its `[PASS]`. The cases a report lists are the `test fn`
declarations in the file, with their lines, and only those:

- A `[PASS] name` or `[FAIL] name` line (or TAP's `ok N - name` /
  `not ok N - name`, read as `name`) reports the result of the declared case
  called `name`. A failure wins over a pass for the same name, so a test that
  prints `[FAIL] name` and returns still fails.
- A case nothing reported **fails**. When the program stopped, the reason is
  "did not run: the program stopped before it" — it is the test that stopped the
  program, or one after it. When the program exited 0, the reason is "never ran:
  main() does not call it". A test that never ran asserted nothing, so it is never
  a pass.
- With a `rho` older than `--test-report` the runner prints a one-line warning
  and reads the cases the old way: a case nothing reported passes if the program
  exited 0 and reported nothing, and is skipped if it reported other cases.
- A printed name that is not a `test fn` never becomes a case. A `[FAIL]` under
  such a name still fails the suite, so a failure the program reported is not
  lost; a `[PASS]` under one is ignored.

A file with no `test fn` in it is an error, not an empty pass.

`rho new tests/greeting_test.rho` writes such a file with a `check` helper, one
`test fn` and the `main` that calls it.

## Assertions

`std.assert.assert` holds the assertions. Each one prints
`ASSERTION FAILED: <what>` and aborts when it does not hold:

| Function | What it checks |
|---|---|
| `assert(cond: bool)` | The condition holds |
| `assert_true(x: bool)` | `x` is true |
| `assert_false(x: bool)` | `x` is false |
| `assert_eq[T](a: T, b: T)` | `a == b` |
| `assert_ne[T](a: T, b: T)` | `a != b` |
| `fail(msg: string)` | Fails unconditionally, with the message |

`assert_eq` does not print the two values, so a test that needs to say what it
got prints it before asserting.

## Which tests a project has

A project says which suites it has in a **suite manifest**, `tests/tests.yml`
(or `tests.yml` at the project root; having both is an error). The runner runs
what the manifest declares and nothing else:

```yaml
# tests/tests.yml
name: acme-sensors
suites:
  - name: tests/greeting          # tests/greeting_test.rho (a comment, not part of the name)
    kind: rho
    tags: [unit, rho]

  - name: smoke
    kind: command
    command: python3 tests/smoke.py --fast
    tags: [smoke]

  - name: examples
    kind: build
    matrix:
      project: [examples/blink, examples/sensor]
      board: [native, pico]

  - name: on-hardware
    kind: command
    status: skip
    reason: needs a board on the bench
```

Every suite has a `name` and a `kind`. The keys the runner reads itself:

| Key | What it means |
|---|---|
| `name` | The suite's name; for a `rho` suite, the test file relative to the project, with or without `_test.rho` |
| `kind` | Who runs it: `rho`, `command` or `build` (built in), or a kind a plugin resolves, such as `gtest` or `ctest` |
| `tags` | What `--tag` and `--without` select on |
| `command` | For `command`: the program to run, as one string or a list of words |
| `matrix` | Axes the suite expands over, one suite per combination |
| `status: skip`, `reason` | A suite declared but not run yet; it is reported as skipped with the reason, never as a pass |
| `enabled: false` | A suite that is not run; it is reported as skipped, `disabled` |

A `#` after whitespace starts a comment, after a value or a list item alike; a
`#` inside quotes is part of the value.

A suite its runner cannot find **fails the run**: a `rho` suite whose file is not
there or declares no `test fn`, a `command` suite with no command and no script.
Only a suite whose *kind* has no runner installed is reported as skipped, so a
manifest can describe a suite for a plugin that is not installed yet.

Any other key on a suite is passed untouched to the plugin that runs its kind,
so the `ctest:` or `gtest:` settings belong to those plugins.

The built-in kinds:

- **`rho`** — a Rho test file, run as described above.
- **`command`** — any program the project can run from its root: a Python smoke
  test, a shell script, a `make` target. With no `command:`, the runner runs
  `tests/<name>` if that file exists. The exit status is the verdict.
- **`build`** — `rho build` over a directory, for a target. Exit 0 is a pass, 77
  (the build cannot be tried on this machine) is a skip, anything else a failure.

A matrix axis is a list of values, or `{from: <module>, key: <dotted.key>}` to
read the list from the project's own manifest or a dependency's, so a smoke suite
over a platform's boards cannot fall behind the platform. A `command` can use
`{axis}` placeholders, and a `build` suite reads its directory from the matrix
and its target from an axis named `target`, `board` or `preset`.

A manifest can `include:` other manifests — a file or a list — so unit and smoke
suites can each keep a file of their own.

A project with no suite manifest is not an error as long as it has test files:
every `*_test.rho` under the project (outside `build/`) is a `rho` suite.

## Choosing what runs

```sh
rho test --list                   # what the project declares, and stop
rho test --dry-run                # what this run would do, without doing it
rho test --tag unit               # suites carrying a tag; repeatable
rho test --without slow           # skip suites carrying a tag; repeatable
rho test --kind command           # suites of one kind; repeatable
rho test --suite examples         # one suite; a matrix's base name runs every cell
rho test --verbose --echo         # each step as it runs, and every suite's output
```

`--rho PATH` builds with another `rho` (by default `$RHO_BIN`, else the one on
`PATH`), which is how a run uses a toolchain in progress. `--target` names the
backend a run reports to its plugins; Rho test files are always built through the
C backend. Options may come before or after the path, as `--name value` or
`--name=value`.

`--verbose` prints each step and each case as it is reported; `--echo` prints
what each suite printed, without the result lines the report already shows.

The run exits 0 when nothing failed and 1 when a suite failed or the run itself
could not happen (an unreadable manifest, a project with no tests); a flag
without its value exits 2.

## Plugins

The runner offers two hooks. A plugin answers `discover` to resolve the suites of
a kind the runner does not know, and `run` to report a finished run:

| Plugin | Hook | What it adds |
|---|---|---|
| {ref}`rho-test-gtest <rho-test-gtest/index>` | `discover` | Suites of kind `gtest`: GoogleTest binaries, built and run, their cases read from GoogleTest's own output |
| {ref}`rho-test-ctest <rho-test-ctest/index>` | `discover` | Suites of kind `ctest`: configure with CMake, build, run `ctest`, read its JUnit report |
| {ref}`rho-test-junit <rho-test-junit/index>` | `run` | A JUnit report of the run, which is what a CI system reads |

```sh
rho plugin enable rho-test-junit
rho test --list-plugins           # what would answer this run
rho test --no-plugins             # a run with no plugins
```

A suite whose kind no installed plugin resolves is reported as having no runner,
so a manifest can describe a suite for a plugin that is not installed yet. A
plugin that fails is reported on standard error and the run carries on.

## Reports

A report a plugin writes is put in a file, not printed: by default
`build/test-reports/report-1.txt`, `report-2.txt` and so on, in the order the
plugins answered, and `--report-dir DIR` puts them elsewhere. The path of each is
printed at the end of the run, which is where a CI job picks it up.

```sh
rho plugin enable rho-test-junit
rho test --report-dir build/test-reports
```

## The run's own results

Every run that runs something also writes its own record,
`build/test-reports/results.json` (under `--report-dir` when one is given),
replacing the previous one, whatever plugins are enabled. What the run prints is
unchanged; the file is for a tool that asks what the last run found, such as
`rho req trace`, which reads it before any other report.

```json
{
  "schema": 1,
  "runner": "rho-test", "runner_version": "0.3.0",
  "rho": "0.19.1",
  "run": "2026-11-19T18:30:00Z", "seconds": 4.2,
  "project": "gateway", "target": "c",
  "summary": {"suites": 2, "cases": 3, "passed": 2, "failed": 1, "skipped": 0},
  "suites": [
    {"name": "tests/session", "kind": "rho", "status": "fail", "seconds": 1.3,
     "cases": [
       {"name": "sessions_survive_restart", "status": "pass", "file": "tests/session_test.rho", "line": 12},
       {"name": "table_is_bounded", "status": "fail", "message": "...", "file": "tests/session_test.rho", "line": 20}]},
    {"name": "smoke", "kind": "command", "status": "pass", "seconds": 2.9,
     "cases": [{"name": "smoke", "status": "pass"}]}
  ]
}
```

A case's `status` is `pass`, `fail` or `skip`; `message` says why a case failed
(its reason, or the end of what it printed) or was skipped; `file` is relative to
the project. A Rho `test fn` is its case, by name; a suite whose verdict is its
exit status — a `command` suite — has one case named for the suite; a suite
expanded from a matrix is named `base!axis=value`. A suite's `status` is `fail`
when a case failed, `skip` when it was not run, `pass` otherwise. A list or a dry
run writes nothing. `schema` grows only by adding fields.

## Testing the toolchain itself

The Go side of the repository is tested with `go test`, after the components a
build needs are installed — `./scripts/check.sh` does both, in that order:

```sh
./scripts/check.sh               # build, publish, install, then test everything
go test ./...                    # the core module, once the components are in place
cd modules/components/rho-fmt && go test ./...   # one component
```

Each component and plugin is its own Go module, so `go test ./...` at the root
does not reach into them. The formatter is checked for round-trip safety over
every source in the repository, and the two backends are checked against each
other by the parity suite (`src/parity_cases_test.go`), which runs the same
programs on C and Go and compares what they print.
