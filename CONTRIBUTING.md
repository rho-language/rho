# Contributing

## Building and testing

The whole procedure, in the order it has to happen, is one script:

```sh
./scripts/check.sh
```

It builds the toolchain, publishes every component and plugin into a local
registry, installs, and then runs `go vet` and `go test` in every module. Running
`go test ./...` first fails with `this build needs the rho-target component:
module not installed`, which is true of the environment and says nothing about the
code. See `AGENTS.md` for the module-by-module commands.

## Changes

- A change that fixes a bug comes with a test that fails without it. Revert the fix
  locally and see the test fail before you rely on it.
- A component or plugin change is tested in its own module first. Only then its
  version is bumped, its `CHANGELOG.md` section written, and it is published — once
  per change, not once per commit.
- A user-visible change to the toolchain goes in the root `CHANGELOG.md`.
- Where the C and Go backends could differ, add a program to
  `src/parity_cases_test.go`. A known difference is listed there with what differs.

## Commits

`type(scope): summary`, then a body that says what was wrong and why the change is
the fix. Types in use: `feat`, `fix`, `build`, `docs`, `test`.

## Security

Report vulnerabilities privately; see `SECURITY.md`.
