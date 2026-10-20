# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-10-20

A board's own linker script is now found where the board keeps it. A script
named by a relative path in a target spec is resolved beside that spec, rather
than beside the project that happens to be built for it — so a platform can
ship one script for its boards and every project that uses it points at the
same file.

## [0.1.2] - 2026-10-20

A part whose toolchain brings its own linker script now gets the toolchain's,
and no script is written. The toolchain knows where the part's vector table
belongs and how its runtime expects to be reached, and a generated script
guesses both.

A board that says nothing about its linker inherits its part's, so that where a
program's memory is laid out stays the part's fact.

A board whose chip resolves to a chip that stands on its own is no longer
reported as a board that cannot stand on its own.

## [0.1.3] - 2026-10-20

A flash command may read the environment. `{env:NAME}` is the value of `NAME`,
and `{env:NAME:-default}` is that value or the default when it is not set, so a
spec can say something the machine decides — which serial line the board is on,
which probe it is behind — without being wrong on every machine but the one it
was written on.

A reference with no default and no value is an error rather than a hole: a flash
command with a hole in it is a command that would flash nothing.

It is a substitution and not a shell. There is no pipeline, no glob and no
command substitution, so a spec that could name one is still a spec that cannot
run whatever a machine could be talked into.

The reference is spelled `{env:…}` rather than `${…}` because the command
travels into a CMake file, and CMake reads `${…}` as its own: a reference
written that way is a configure error rather than a placeholder.

## [0.1.5] - 2026-10-20

A flash command may read the environment: `{env:NAME}` is the value of `NAME`,
and `{env:NAME:-default}` is that value or the default when it is not set, so a
spec can say something the machine decides — which serial line the board is on,
which probe it is behind — without being wrong on every machine but the one it
was written on.

A reference with no default and no value is an error rather than a hole: a flash
command with a hole in it is a command that would flash nothing.

It is a substitution and not a shell. There is no pipeline, no glob and no
command substitution, so a spec that could name one is still a spec that cannot
run whatever a machine could be talked into.

The reference is spelled `{env:…}` rather than `${…}` because the command
travels into a CMake file, and CMake reads `${…}` as its own: a reference
written that way is a configure error rather than a placeholder.

## [0.1.6] - 2026-10-20

The generated build reads the environment too. A flash command that names one
is resolved in the CMake the build writes, from the environment CMake was
started in, and the value it falls back to is what the command is run with when
the environment says nothing — so a build and a flash agree about which serial
line the board is on.

## [0.1.7] - 2026-10-20

The generated build can flash what it built. The `flash` target now names the
program's own file rather than the placeholder the command carries, and the
command is split into arguments rather than handed to the tool as one: a tool
given the whole command as one argument is a tool given a name it cannot parse.
