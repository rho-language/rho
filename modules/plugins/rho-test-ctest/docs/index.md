# CTest suites

A {ref}`rho-test <rho-test/index>` plugin. It runs the CTest suites a project
declares, so a C library's host tests — registered with CMake's `add_test` — run
with `rho test` rather than with `cmake` and `ctest` typed by hand or written into
a CI file.

```yaml
# tests/tests.yml
name: acme-drivers
suites:
  - name: unit
    kind: ctest
    tags: [unit, host]
    ctest:
      source: .
      defines: [ACME_WITH_MOCKS=ON]
```

```sh
rho plugin enable rho-test-ctest
rho test --kind ctest
```

## What it runs

The plugin answers the runner's `discover` hook for every suite of kind `ctest`,
and resolves each into four steps:

1. **Configure** — `cmake -S <source> -B <build_dir> -DCMAKE_BUILD_TYPE=<type>
   -DBUILD_TESTING=ON`, plus `-DRHO_BUILD_HOST_TESTS=ON` and the suite's
   `defines`.
2. **Build** — `cmake --build <build_dir> --parallel`.
3. **Run** — `ctest --test-dir <build_dir> --output-on-failure --no-tests=error
   --output-junit <build_dir>/ctest-report.xml`.
4. **Read the report** — the plugin reads the JUnit file CTest wrote and reports
   each test as a case.

A run that found no tests fails. CTest's own exit status calls "no tests were
found" a pass, and a suite that verified nothing would read as coverage.

## Settings

Every key under `ctest:` is optional:

| Key | Default | What it is |
|---|---|---|
| `source` | `.` | The directory with the `CMakeLists.txt`, inside the project |
| `build_dir` | `build/tests/<suite>` | Where the tests are built, inside the project |
| `defines` | none | Extra configure definitions, `NAME` or `NAME=VALUE` |
| `host_tests` | `true` | `false` leaves out `-DRHO_BUILD_HOST_TESTS=ON` |
| `build_type` | `Debug` | The CMake build type |

A path that is absolute or leaves the project is refused, and the suite is
skipped with the reason rather than run somewhere else.

## Related

- {ref}`tools/testing` — suite manifests, kinds and selection.
- {ref}`rho-test-gtest <rho-test-gtest/index>` — the same idea for GoogleTest
  binaries.
- {ref}`rho-test-junit <rho-test-junit/index>` — a JUnit report of the whole run.
