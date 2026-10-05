# A JUnit report

A `rho-test` plugin. It is handed the results of a run and answers with a JUnit
report, which is the document a CI system reads.

## Why it is a plugin

A test run is the one piece of a project's work everything has an opinion
about: a CI system wants a document it can parse rather than a line it has to
read, a dashboard wants a JSON payload, a person wants a line on a terminal.
None of that belongs in the runner, which knows how to run a test and nothing
about how a result is reported. So the runner offers a hook, and a plugin
answers the one it was written for.

## The attachment

```yaml
extends: rho-test

implements:
  - report

hooks:
  - report
```

`rho-test` offers the hook `report` once, over the results of a finished run.
The plugin writes the document and says where it wrote it; the runner does not
read it.

## Enabling it

```sh
rho plugin enable rho-test-junit
rho test . --report build/junit.xml
```

## Related

- {ref}`rho-test <rho-test/index>` — the runner this extends, and the `report`
  contract in full.
- {ref}`rho-test-gtest <rho-test-gtest/index>` — the other test plugin here,
  which adds suites to a run rather than reporting one.
