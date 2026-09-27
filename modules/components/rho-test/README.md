# rho-test

Test runner for Rho projects.

```sh
rho test                       # the project in the working directory
rho test path/to/project       # a project
rho test path go               # on a chosen target
```

Test files are `*_test.rho`. Each declares its cases as `test fn name()`, and a
`main` that calls them; the runner strips the `test` marker, compiles the file
with `rho build` and runs the result, so a test is compiled and checked exactly
as a program would be.

The runner logs through `rho-log`, so a test run is identifiable in a shared log
without every line having to say so.
