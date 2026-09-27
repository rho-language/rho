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
3. Runs the generated binary for each test.
4. Reports each one, with the file and the line a failure is on.

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
| `c` | The native build |
| `go` | The Go backend, which is what most projects generate |

The default is the native one, because that is the language's own target. A
project that generates `go` passes `--target go`.

## Where the runner lives

The implementation is in `src/rhotest`, so `rho test` and the `rho-test` binary
are the same runner: one reached through the CLI, the other for a shell that
should not go through it. That is the shape every component takes, and it is why
adding the runner to the toolchain did not mean changing the CLI.
