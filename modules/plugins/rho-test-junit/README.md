# rho-test-junit

A test-run plugin. It is handed the results of a `rho test` run and answers
with a JUnit report, which is the document a CI system reads.

A test run is the one piece of a project's work that everything has an opinion
about: a CI system wants a document it can parse rather than a line it has to
read, a dashboard wants a JSON payload, a person wants a line on a terminal.
None of that belongs in the runner, which knows how to run a test and nothing
about how a result is reported. So the runner offers two hooks, and a plugin
answers the one it was written for.

## The attachment

The plugin names the component it plugs into and the hook of that component it
answers:

```yaml
extends: rho-test

hooks:
  - run

commands:
  - name: junit
    run: rho-test-junit
    hook: run
```

`rho module validate` checks the relation before a plugin is published: the
component has to exist, the hook has to be one the component offers, and the
command has to answer a hook declared here. A test run loads the plugins that
name `rho-test`, and nothing else.

## The protocol

The runner writes a request to the plugin's standard input as JSON:

```json
{
  "version": 1,
  "hook": "run",
  "command": "junit",
  "component": "rho-test",
  "payload": {
    "root": "/project",
    "target": "go",
    "passed": 3,
    "total": 4,
    "results": [
      {"case": {"name": "Some holds a value", "file": "/project/tests/option.rho", "line": 7},
       "passed": true, "output": ""}
    ]
  }
}
```

and reads the answer from its standard output:

```json
{
  "version": 1,
  "payload": {
    "report": "<testsuites>…</testsuites>",
    "notes": ["junit report: 4 cases, 3 passed"]
  }
}
```

The envelope is the toolchain's and is the same for every component that takes
plugins, so a plugin is written the same way whether it extends a test run or a
documentation build. The payload is the component's own, which is why this
plugin reads `results` and a documentation plugin reads `pages`.

The runner writes `report` to a file and prints `notes`, because a report is a
document a system reads and a note is a line a person reads.

## What it writes

```xml
<testsuites>
  <testsuite name="option.rho" package="tests/option.rho" tests="2" failures="1">
    <testcase name="Some holds a value" classname="tests/option.rho" time="0"/>
    <testcase name="None holds nothing" classname="tests/option.rho" time="0">
      <failure message="test failed" type="failure">…the output…</failure>
    </testcase>
  </testsuite>
</testsuites>
```

A suite is a test file and a class is the test function, because that is the
shape a reader of the format expects. Paths are relative to the project, with
forward slashes, so a report means the same thing wherever it is read.

## Building it

```sh
go build -o rho-test-junit ./src
go test ./...
```

To have the runner call it, publish it and put the command where the runner
looks for a plugin's program:

```sh
rho module publish modules/plugins/rho-test-junit
rho test --list-plugins <project>
rho test <project>
```

A plugin that fails is reported on standard error and the run carries on: one
broken extension does not cost a project its tests.
