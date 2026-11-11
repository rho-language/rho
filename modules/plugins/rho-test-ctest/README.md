# rho-test-ctest

Runs the CTest suites a project declares, through `rho test`.

```yaml
# tests/tests.yml
name: my-platform
suites:
  - name: unit
    kind: ctest
    tags: [unit, host]
    ctest: {}
```

```sh
rho test --kind ctest
```

It resolves the suite into four steps and nothing else: configure with CMake
(asking for the host tests a build normally leaves out), build, run `ctest` with a
JUnit report and `--no-tests=error`, and read the report. A run that found no
tests fails, because "no tests were found" is a pass as far as CTest's exit status
goes and a suite that verified nothing is read as coverage.

Every key of `ctest:` is optional:

| key | default | |
|---|---|---|
| `source` | `.` | the directory with the `CMakeLists.txt`, inside the project |
| `build_dir` | `build/tests/<suite>` | where it is built, inside the project |
| `defines` | none | extra configure definitions, `NAME` or `NAME=VALUE` |
| `host_tests` | `true` | `false` leaves out `-DRHO_BUILD_HOST_TESTS=ON` |
| `build_type` | `Debug` | the CMake build type |

A path that is absolute or leaves the project is refused, and the suite is
skipped with the reason.
