# rho-doctor

What this machine has, against what it needs.

A session that fails and a machine that is missing something look the same from the
outside. The code is wrong, or the program it needed was never installed, and the
error names this component either way — because the program that was missing was
this component's to ask for.

```sh
rho doctor                      # every installed module's requirements, checked
rho doctor --json               # the same report, for a program to read
rho doctor --component rho-dbg  # one module
rho doctor --enabled            # only the modules that run
rho doctor --quiet              # one line, for a status bar
```

## It reports, and it does not change anything

That is the whole division. This command reads; `rho keeper sync` acts. A tool that
inspected and installed would be a tool whose report you could not tell from the
changes it had made — and what it would be changing is the machine, with root, on
the strength of a line in a module manifest.

So the exit status is the question a script asks: `0` when nothing is missing and
nothing is too old, `1` when something is, and the report is on standard output
either way.

## What a module declares

```yaml
system_deps:
  executables:
    - name: arm-none-eabi-gdb
      version: ">=10.0"
      version_flag: "--version"
      version_pattern: '^\n]*?\)\s*([0-9][^\s(]*)'
      description: which sessions drive a board through the Machine Interface
      install:
        apt: gdb-multiarch
        brew: arm-none-eabi-gcc
        manual: https://developer.arm.com/downloads/-/arm-gnu-toolchain-downloads

  env_vars:
    - name: ARM_TOOLCHAIN_PATH
      required: false
      description: an override for where the cross toolchain is

  udev_rules:
    - name: 60-rho-probes.rules
      target_path: /etc/udev/rules.d/60-rho-probes.rules
      description: a probe that is plugged in and not permitted is a probe that is not there
```

`system_deps` is a different kind of requirement from `deps`, and is declared apart
from it for that reason. Everything in `deps` is another module of this toolchain and
`rho-deps` resolves it. Everything in `system_deps` is a program or a permission on
the machine, and nothing this toolchain fetches will produce it.

`required` defaults to true: a system dependency a module does not need is not one,
and a block of optional programs nobody checks for is a block that reports nothing.

## The answers

Five, because "installed but too old" and "installed and right" want different
things done about them, and a report that reduced both to *ok* would make the older
one indistinguishable from the newer one.

| status | what it means |
| --- | --- |
| `OK` | installed, and the version wanted |
| `MISSING` | a required program, variable or rule that is not there |
| `OUTDATED` | installed at a version the requirement excludes |
| `WARNING` | an optional requirement that is not satisfied |
| `UNKNOWN` | a check that could not be made — its own answer, because "I could not tell" and "it is fine" are not the same thing |

A program that will not print its version is `UNKNOWN`, never `MISSING`: a debugger
that hangs on its version flag is installed, and reporting it as absent sends a
person to install a second copy of it.

## Versions

Constraints are `>=`, `>`, `<=`, `<`, `=`, `!=`, `~`, `^`, joined by a space for
*and* and by `||` for alternatives: `>=0.11 <0.13`, `~1.2`, `^2`, `>=10 || >=12 <14`.

`~1.2` is the same major and minor and a later patch; `^1.2` is the same major and a
later minor; and a pre-release is older than the release it leads to, so `>=12.1`
refuses a `12.1-rc1` rather than quietly accepting a release candidate.

This is written here rather than taken from a version library because the toolchain
has no dependencies outside itself, and a release you cannot build on a machine with
no network is not a release to depend on to find out whether you can debug. The
grammar is small and every rule in it is tested.

## What it scans

Every installed module that has declared anything — components, plugins, modules and
libraries — because a requirement on the machine is not something only a component
has. `--enabled` narrows it to the modules that run, because a machine usually has
modules installed and not enabled, and their requirements are not today's problem.