# GoogleTest suites

A `rho-test` plugin. It runs the GoogleTest suites a project declares.

```sh
rho plugin enable rho-test-gtest
rho test --kind gtest
```

## The attachment

```yaml
extends: rho-test

implements:
  - discover

hooks:
  - discover
```

`rho-test` asks the `discover` hook once, over the suites the project declared.
This plugin answers for the suites of kind `gtest` and leaves every other kind
alone. For each one it returns the steps that build and run it, and the runner
schedules them like any other suite.

## Declaring a suite

```yaml
# tests/tests.yml
suites:
  - name: unit
    kind: gtest
    tags: [unit]
    gtest:
      binary: build/tests/unit
      configure: [cmake, -S, ., -B, build, -DBUILD_TESTING=ON]
      build: [cmake, --build, build, --target, unit]
```

| Key | What it means |
|-----|---------------|
| `binary` | The test executable, relative to the project. Required: a suite without it is skipped, never guessed |
| `configure` | How to configure the build, as a list of words or one line |
| `build` | How to build the binary, the same way |
| `filter` | Passed to the binary as `--gtest_filter` |
| `xml` | Where GoogleTest writes its report; `build/test-reports/<suite>.gtest.xml` otherwise |
| `args` | Extra arguments for the binary |

When a suite declares `configure` or `build`, the first step clones GoogleTest
into `vendor/googletest` if it is not there already, and the build steps see
its path as `GTEST_ROOT`. A suite whose binary is already built needs neither.

The binary is run with `--gtest_output=xml:<path>`, and the plugin then reads
that XML report back to give the runner one result per test. A binary that
crashes before writing its report is a failure.

## Related

- {ref}`rho-test <rho-test/index>` — the runner this extends.
- {ref}`tools/testing` — the suite manifest and the hooks in full.
- {ref}`rho-test-junit <rho-test-junit/index>` — the other test plugin here,
  which reports a run rather than adding suites to it.
