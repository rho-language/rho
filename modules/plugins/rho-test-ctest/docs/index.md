# CTest suites

A `rho-test` plugin. It runs the CTest suites a project declares, so a C
platform's host tests are run with `rho test` and not with `cmake` and `ctest`
typed by hand or written into a CI file.

See the module's README for the `ctest:` settings. The suite kind is `ctest`; the
plugin attaches to `rho-test` through its `discover` hook, as `rho-test-gtest`
does, and reads the JUnit report `ctest --output-junit` writes.
