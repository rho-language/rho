# GoogleTest suites

A `rho-test` plugin. It runs the GoogleTest suites a project declares.

```sh
rho test --kind gtest
```

## Why it is a plugin

A test runner that only knows how to run tests written in the language it
compiles is a test runner that can test one project. GoogleTest suites are C,
built by whatever build system their project uses, and reporting a per-case
result means reading something GoogleTest wrote rather than something this
plugin decided.

So the runner asks which suites a plugin knows how to run, and this one answers
for the suites of kind `gtest`, with the steps that build and run them.

## The attachment

```yaml
extends: rho-test

implements:
  - discover

hooks:
  - suites
```

`rho-test` offers the hook `suites` once, over what the project declared. A
plugin answers with the suites it knows how to run, and the runner schedules
them like any other: the difference between a Rho test and a GoogleTest suite
is the plugin, not the runner.

## Enabling it

```sh
rho plugin enable rho-test-gtest
rho test . --kind gtest
```

## Related

- {ref}`rho-test <rho-test/index>` — the runner this extends, and the test
  contracts in full.
- {ref}`rho-test-junit <rho-test-junit/index>` — the other test plugin here,
  which reports what a run found rather than adding suites to it.
