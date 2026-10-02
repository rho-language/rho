# rho-test-gtest

Runs the GoogleTest suites a project declares.

```sh
rho test --kind gtest
```

## What it is for

A test runner that only knows how to run tests written in the language it
compiles is a test runner that can test one project. GoogleTest suites are C, built
by whatever build system their project uses, and reporting a per-case result means
reading something GoogleTest wrote rather than something this plugin decided.

So this plugin is the part of the test runner that knows GoogleTest: the runner asks
which suites a plugin knows how to run, and this one answers for the suites of kind
`gtest`, with the steps that build and run them.

## The manifest says how; this does not guess

```yaml
- name: error
  kind: gtest
  tags: [unit, host]
  gtest:
    binary: build/tests/unit
    configure: [cmake, -S, ., -B, build, -DBUILD_TESTING=ON]
    build: [cmake, --build, build, --target, unit]
```

`binary` is required and is never guessed — a plugin that worked out where a
project's test binary was would be a plugin that ran somebody else's tests. The
build steps are the project's to declare: a suite may build with CMake, Bazel, a
Makefile or a shell script, and a plugin that knew only about CMake would be a
plugin that could only run the projects it happened to suit. Omit `configure` and
`build` when the binary is already there.

Every other key on the suite entry reaches this plugin untouched, which is what
lets a manifest describe a suite for a plugin that is not installed yet.

## Reading the report, not the screen

Each suite is run in three steps: the build the project declared, the binary
itself with `--gtest_output=xml:<path>`, and this plugin again, asked to read that
file and print what it says.

The XML is the only place GoogleTest states its results unambiguously — which
test, inside which case, passed, failed or was skipped, and what it printed when it
failed. Its console output is a table written to be read by a person, with progress
lines through it, and a plugin that parsed that would be guessing at a format that
is free to change.

What comes out is one line per test in the form the runner already understands. The
run step produces a file; this reads it; the run sees lines. A binary that crashed
before writing its report is reported as a failure, because it did not pass.

## Settings

| key | |
|---|---|
| `binary` | the test executable, relative to the project root. Required |
| `configure` | how to produce it, as a list of words or one line |
| `build` | how to build it, same |
| `filter` | passed to the binary as `--gtest_filter` |
| `xml` | where the report goes; under `build/test-reports/` otherwise |
| `args` | extra arguments for the binary |

## Running it as its own program

It is dispatched to as a plugin, like any other module, and reads its request from
standard input. It is also the report step of every suite it resolves, which is why
it takes a report path:

```sh
build/tests/unit --gtest_output=xml:out.xml
rho-test-gtest --report out.xml
```