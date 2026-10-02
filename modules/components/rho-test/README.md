# rho-test

Test runner for Rho projects.

```sh
rho test                       # the project in the working directory
rho test path/to/project       # a project
rho test --list                # what the project declares
rho test --dry-run             # what this run would do, without doing it
```

## A project declares its tests

`tests/tests.yml` says what the project has:

```yaml
name: my-platform
suites:
  - name: error
    kind: gtest
    tags: [unit, host]
    gtest:
      binary: build/tests/unit
      build: [cmake, --build, build, --target, unit]

  - name: sw-smoke
    kind: build
    tags: [smoke]
    matrix:
      example: [examples/hello]
      board:
        from: my-platform
        key: hardware.supported
```

Declared rather than discovered, which is the whole argument for the file: a
project's tests are a claim about what it promises, the claim is checkable, and a
runner that discovered tests by walking a directory would run whatever somebody
left in it.

## Two words describe a suite

They answer different questions, and keeping them apart is what lets one command
select across frameworks:

| | |
|---|---|
| **kind** | who knows how to run it — `gtest`, `command`, `build`, or a Rho test file |
| **tags** | what class of test it is — `unit`, `smoke`, `slow`; the project's own vocabulary |

So `--tag unit` runs every unit test whether it is a Rho file, a GoogleTest suite
or a command, and `--kind gtest` runs only what the gtest plugin answers for.

A suite's *options* — the `gtest:` mapping above — belong to its kind. The runner
reads the keys every kind shares and hands the rest to whichever plugin claims
that kind, untouched. That is what lets a manifest describe a suite for a plugin
that is not installed yet.

## A matrix is one suite over several axes

A suite with a `matrix` is one cell per combination, named after the axes:

```
sw-smoke!board=nucleo-h753zi!example=examples/hello
```

The values are usually paths, which is why the axes are joined with `!` and not
with `/`: a name that cannot be read back is a name a runner cannot act on.

An axis may read its values from a contract rather than restating it:

```yaml
board:
  from: my-platform
  key: hardware.supported
```

Two lists would be equal on the day they were written and after that one of them
would change alone. A board added to the contract is tested by the next run with
nothing else edited — and a board that stops building cannot go on being claimed
by a test that no longer runs it.

## Running without tests

A Rho test file is `*_test.rho`. Each declares its cases as `test fn name()`; the
runner strips the marker, builds the file and runs the result, so a test is
checked exactly as a program would be.

A project with test files and no manifest is still described — by what it has. A
project with neither is an error, because "nothing to run" and "nothing declared"
are different facts and only one of them is true.

## Plugins

Two hooks, because finding a project's tests and reporting a finished run are
unrelated jobs:

- **discover** — over what the manifest declared. A plugin answers with the suites
  it knows how to run, each with the steps that build and run it. This is how a
  framework the runner does not implement is reached.
- **run** — over the results, with the suites that were considered and what was
  skipped, so a reporter can say a suite did not run rather than guess.

A plugin may also add test files, for a project whose own layout it knows.

```yaml
extends: rho-test
implements: [discover]
hooks: [discover]
commands:
  - name: gtest
    run: rho-test-gtest
    hook: discover
```

`rho-test-gtest` runs GoogleTest suites and `rho-test-junit` writes a JUnit
report; both are worked examples. `rho test --list-plugins` names the plugins a
run would answer to, and a plugin that fails is reported rather than allowed to
stop the run.

## Three outcomes, and why there are three

A suite **passes**, **fails**, or is **skipped**, and a step that was not tried is
neither of the first two:

- A **build** exits `77` — the automake convention, and the number every build
  system already agrees on — to say the code is not the problem and this machine
  cannot try it yet. A suite over fifty boards on a machine with one toolchain is
  mostly this case, and reporting it as a failure is why smoke suites usually get
  turned off.
- A suite **declared `status: skip`** is a suite somebody wanted to write down
  before it could be written. It appears in every summary as a suite that did not
  run, so the gap is visible rather than absent.
- A suite whose kind has no runner installed is **skipped with the reason**, not
  dropped. A run that passed while a declared test went unrun is the outcome this
  runner exists to prevent.

Suites and cases are counted separately: one cell of a smoke matrix that did not
build is one failing suite, not one failure per case it would have had.

## Selecting

```sh
rho test --tag unit              # every unit test, whatever its kind
rho test --without slow          # everything but the slow ones
rho test --suite sw-smoke        # by name; a matrix's base name runs every cell
rho test --kind gtest            # only what the gtest plugin answers for
rho test --suite 'sw-smoke!board=nucleo-h753zi'   # one cell of a matrix
rho test --list-plugins
```