# rho-doctor

What this machine has, against what the installed modules need from it.

A session fails and a machine is missing something look the same from outside: the
code is wrong, or the program it needed was never installed. `rho doctor` tells them
apart.

```sh
rho doctor
rho doctor --json               # for rho keeper, and for an editor
rho doctor --component rho-dbg  # one module
rho doctor --enabled            # only what runs today
rho doctor --quiet              # one line
```

## The division

`rho doctor` reads and reports. `rho keeper sync` and `rho keeper fix` act on that
report. A tool that both would have a report you could not tell from the changes it
had made — and it would be changing the machine, with root, on the strength of a
line in a module manifest.

## The contract

A module says what it needs from the machine in its `system_deps` block, which is a
different kind of requirement from `deps`:

```yaml
deps:
  rho-log: ">=0.1.0"          # other modules — rho-deps resolves these

system_deps:
  executables:
    - name: arm-none-eabi-gdb  # programs — somebody installs these
      version: ">=10.0"
      version_flag: "--version"
      version_pattern: '[^\\n]*?\\)\\s*([0-9][^\\s(]*)'
      install:
        apt: gdb-multiarch
        brew: arm-none-eabi-gcc
        manual: https://developer.arm.com/downloads/-/arm-gnu-toolchain-downloads
  env_vars:
    - name: ARM_TOOLCHAIN_PATH
      required: false
  udev_rules:
    - name: 60-rho-probes.rules
      target_path: /etc/udev/rules.d/60-rho-probes.rules
      optional: true
```

Everything in `deps` is another module of this toolchain. Everything in
`system_deps` is a program or a permission on the machine, and no amount of
resolving this toolchain's own graph produces it.

## What it says

`OK`, `MISSING`, `OUTDATED`, `WARNING`, `UNKNOWN` — five rather than two, because
"installed but too old" and "installed and right" want different things done about
them, and a program that will not print its version is installed and must not be
reported as absent.

The report is grouped by module and ordered by how much each item stops, so what a
person reads first is what is broken rather than what sorts first. Every entry
carries the recipe that would fix it, because a report that says what is missing and
leaves the reader to work out how to get it has done half the job.