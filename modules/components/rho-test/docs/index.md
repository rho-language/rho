# rho-test

The test runner, reached as `rho test`. {ref}`tools/testing` is the full
reference; this page is the short version.

```sh
rho test                    # the project in this directory
rho test path/to/project
rho test tests/math_test.rho
rho test --list             # what the project declares, and stop
```

## What it runs

A project declares its suites in `tests/tests.yml` (or `tests.yml` at its root,
never both), and the runner runs what that file declares and nothing else. A
project with no suite manifest still runs: every `*_test.rho` under it, outside
`build/`, is a suite.

```yaml
name: acme-sensors
suites:
  - name: tests/option          # tests/option_test.rho
    kind: rho
    tags: [unit]
  - name: smoke
    kind: command
    command: python3 tests/smoke.py
```

A Rho test file declares its tests with `test fn` and calls them from `main`:

```rho
module option_test

import std.assert.assert

test fn some_holds_a_value() void {
    assert_true(true)
}

fn main() void {
    some_holds_a_value()
}
```

The runner strips the `test` marker, builds the file to C, compiles it with `cc`
and runs it. The exit status is the verdict, and the cases a report lists are
the `test fn` declarations with their lines.

The built-in kinds are `rho`, `command` and `build`. Any other kind is resolved
by a plugin, and a kind that no installed plugin resolves is reported as skipped
with a reason rather than dropped.

## Plugins

The runner offers two hooks, declared as two contracts in its manifest:

| Contract | Hook | When it runs | What a plugin answers with |
|----------|------|--------------|----------------------------|
| `discover` | `discover` | Once, over the declared suites and the test files found | Suites of a kind it knows, with the steps that build and run them, or test files to add |
| `report` | `run` | Once, over the results of a finished run | A report to write and notes to print |

```yaml
extends: rho-test
implements: [report]
hooks: [run]
commands:
  - name: junit
    run: rho-test-junit
    hook: run
```

```sh
rho test --list-plugins     # the plugins a run would answer to
rho test --no-plugins       # a run with none
```

{ref}`rho-test-gtest <rho-test-gtest/index>` answers `discover` for suites of
kind `gtest`; {ref}`rho-test-junit <rho-test-junit/index>` answers `run` with a
JUnit report. A plugin that fails is reported on standard error and the run
carries on.

## Reports

A plugin's report is written to a file and its notes are printed. The files are
`report-1.txt`, `report-2.txt` and so on, one per plugin that answered, in
`build/test-reports/` under the project unless `--report-dir` names another
directory. The run prints the path of each.

```sh
rho test --report-dir build/test-reports
```
