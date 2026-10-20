# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-10-20

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

## [0.1.3] - 2026-10-20

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
