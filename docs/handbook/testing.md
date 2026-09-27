# Testing

A test is an ordinary Rho program, so it runs on one backend: whatever the
project generates.

## Running them

```sh
rho test <project-dir>
```

The runner discovers the tests under `tests/`, builds the project, and runs the
generated binary for each one. `--target go` runs them on the Go backend, which
is the one most projects generate.

## What a test file looks like

```rho
module tests.option_test

import std.core.option

test "Some holds a value" {
    let value = Some(7)
    assert(is_some(value))
}

test "None holds nothing" {
    let value = None[u8]()
    assert(is_none(value))
}
```

The file is a module, so it imports what it exercises. A test that cannot import
the thing it is about is a test about something else.

## Assertions

`std.assert` holds the assertions:

| Assertion | What it says |
|-----------|--------------|
| `assert(condition)` | The condition holds |
| `assert_eq(left, right)` | Two values are equal, and a failure reports both |
| `assert_ne(left, right)` | Two values differ |
| `assert_true(value)` / `assert_false(value)` | A boolean is what it is |
| `assert_err(result)` | A `Result` is an error |
| `assert_ok(result)` | A `Result` is a value |

A failure reports the file, the line and what it expected, because a test that
says only "failed" is a test nobody acts on.

## Testing the toolchain itself

The Go side of the repository is tested with `go test`:

```sh
go test ./...
```

The components and plugins are separate Go modules, so they are tested from their
own trees:

```sh
cd modules/components/rho-fmt && go test ./...
```

The formatter is checked for round-trip safety: formatting a file and parsing the
result yields the same tree it started from. A formatter that loses code is
worse than no formatter, so anything it does not understand is emitted as a
visible marker rather than dropped.

## The documentation plugin tests

A plugin is another process, so the documentation plugin is tested over the
protocol itself: a request is written to its standard input, and its response is
read back. That covers the contract rather than the markup, which is what a
plugin owns.
