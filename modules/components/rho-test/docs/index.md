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

A Rho test file declares its tests with `test fn` and calls them from `main`;
`main` must call every one, because a test it never calls fails:

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

The runner strips the `test` marker, translates the file to C as a test build
(`rho build --emit-c --test-report <the test fn names>`, which builds nothing
else), compiles it with `cc` under `build/test/` and runs it — on the C backend
only, whatever targets the project declares, and never over the application's
own binaries. In a test build the compiler makes each `test fn` print
`[PASS] <name>` every time it returns; a failing assertion stops the program
first, so it never does. The cases a report lists are the `test fn` declarations
with their lines: a `[PASS] name` / `[FAIL] name` line (or TAP's `ok N - name`)
reports the declared case of that name, and a failure wins over a pass. A case
nothing reported **fails** — "did not run: the program stopped before it" when the
program stopped, "never ran: main() does not call it" when it exited 0. A printed
name that is not a `test fn` never becomes a case (a `[FAIL]` under one still
fails the suite). With a `rho` older than `--test-report` the runner warns in one
line and falls back to the old reading, where an unreported case passed or was
skipped.

A runner may also report a case it did not run: `[SKIP] name  (reason)`,
GoogleTest's `[  SKIPPED ]`, or TAP's `ok N - name # SKIP reason`. The case is
reported skipped with that reason, and a suite whose cases were all skipped is
skipped — not failed as a suite that reported nothing — so a test that needs a
board or a binary the machine lacks says so on every run instead of passing.

The built-in kinds are `rho`, `command` and `build`. Any other kind is resolved
by a plugin, and a kind that no installed plugin resolves is reported as skipped
with a reason rather than dropped. A suite whose file cannot be found, or names
no `test fn`, fails the run.

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
