# rho-test

The test runner. A test is an ordinary Rho program, so it runs on one backend:
whatever the project generates.

```sh
rho test <project-dir>
rho test --target go <project-dir>
```

## What it does

1. Discovers the tests under `tests/`.
2. Builds the project.
3. Runs the generated binary for each one.
4. Reports each test, with the file and the line a failure is on.

```rho
module tests.option_test

import std.core.option

test "Some holds a value" {
    let value = Some(7)
    assert(is_some(value))
}
```

A failure reports the file, the line and what it expected. A test that says only
"failed" is a test nobody acts on.

## Targets

| Target | What it runs |
|--------|--------------|
| `c` | The native build, which is the default |
| `go` | The Go backend, which is what most projects generate |

## Plugins

A test run is read by machines. A CI system wants a document it can parse rather
than a line it has to read, and none of that belongs in a runner that knows how
to run a test. So the runner offers two hooks and lets a plugin answer them:

| Hook | When it runs | What a plugin answers with |
|------|--------------|---------------------------|
| `discover` | Once, over the test files the run found | Files to add, so a plugin that knows a project's own test layout takes part in the run |
| `run` | Once, over the results of a finished run | A report to write and notes to print |

A plugin is attached to the runner rather than free-floating: it names the
component it extends and the hooks of it it answers, and validation checks that
relation before it is published.

```yaml
extends: rho-test
hooks: [run]
commands:
  - name: junit
    run: rho-test-junit
    hook: run
```

```sh
rho test --list-plugins <project>   # what a run would answer to
rho test --no-plugins <project>     # a run with no extensions
```

`modules/plugins/rho-test-junit` is the worked example: it writes a JUnit report,
which is what a CI system reads. A plugin that fails is reported on standard
error and the run carries on, because one broken extension must not cost a
project its tests.

## Reports

A plugin's report is written to a file and its notes are printed, because a
report is a document a system reads and a note is a line a person reads:

```sh
rho test <project> --report-dir build/test-reports
```

The default is `build/test-reports` beside the project, which is where a build
already keeps what a run produced.
