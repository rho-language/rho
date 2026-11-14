# A JUnit report

A `rho-test` plugin. It is handed the results of a run and answers with a JUnit
report, which is the document a CI system reads.

```sh
rho plugin enable rho-test-junit
rho test --report-dir build/test-reports
```

## The attachment

```yaml
extends: rho-test

implements:
  - report

hooks:
  - run
```

`report` is the contract and `run` is the hook: `rho-test` asks it once, over
the results of a finished run. The plugin answers with the XML and a note
(`junit report: N cases, M passed`); it does not write a file itself.

## Where the report goes

The runner writes each plugin's report to `report-<n>.txt` in
`build/test-reports/` under the project, or in the directory `--report-dir`
names, numbered in the order the plugins answered, and prints the path. With
this as the only reporting plugin, the JUnit XML is `report-1.txt`; a CI job
that wants a `.xml` name copies it.

## What it writes

```xml
<testsuites>
  <testsuite name="option_test.rho" tests="2" failures="1" errors="0" skipped="0" time="0" package="tests/option_test.rho">
    <testcase name="some_holds_a_value" classname="tests/option_test.rho" time="0"></testcase>
    <testcase name="none_holds_nothing" classname="tests/option_test.rho" time="0">
      <failure message="test failed" type="failure">…the output…</failure>
    </testcase>
  </testsuite>
</testsuites>
```

One `testsuite` per source file, one `testcase` per result. A skipped result is
a `<skipped>` element with its reason, never a pass. Paths are relative to the
project, with forward slashes.

## Related

- {ref}`rho-test <rho-test/index>` — the runner this extends.
- {ref}`tools/testing` — reports and plugins in full.
- {ref}`rho-test-gtest <rho-test-gtest/index>` — the other test plugin here,
  which adds suites to a run rather than reporting one.
