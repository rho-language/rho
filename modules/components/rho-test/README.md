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

## Plugins

The runner takes plugins, so a run can be reported in a form a system reads
without the runner knowing anything about that form. It offers two hooks:
`discover`, over the test files a run found, and `run`, over the results of a
finished run.

A plugin names the component it extends rather than being free-floating, and the
relation is checked before it is published:

```yaml
extends: rho-test
hooks: [run]
commands:
  - name: junit
    run: rho-test-junit
    hook: run
```

`rho-test-junit` is the worked example: it writes a JUnit report. `rho test
--list-plugins` names the plugins a run would answer to, and a plugin that fails
is reported rather than allowed to stop the run.
