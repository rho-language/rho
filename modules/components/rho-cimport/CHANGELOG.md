# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-10-13

A preset may name no dependencies. A preset fetches the vendor repositories a
board needs, and a hosted target needs none — the machine already has its
compiler and its libraries — so an empty list is a hosted build rather than a
preset that says too little. A preset that does fetch something is still checked
dependency by dependency.

## [0.1.2] - 2026-11-12

Republished so an installation picks up what changed since 0.1.1 without a new
number: paths in a manifest are expanded (`${ROBOHUB_WORKSPACE}`, `${RHO_HOME}`)
before the platform's headers are looked for, so a project that names its
platform by the workspace builds.

## [0.1.3] - 2026-11-21

A foreign interface generated for a preset (`--preset`) uses that preset's variant library (`c_connection.variants.<preset>.library`) in place of `c_connection.library`, adds the variant's `include_dirs` after the project's, and neither reads headers from nor looks for the preset in a dependency declared for other presets (`depends[].presets`). `rho cimport headers` takes `--preset` too.

A build for a preset whose board declares no sources of its own removes the board-sources record a previous build for another preset left, rather than compiling that board's sources into this one.

## [0.1.4] - 2026-11-21

A vendor repository already checked out sparsely is widened with the paths the board being built names, rather than used as the first board left it. A repository is shared by every board of a family and was checked out with what the first board named, so on a fresh machine a second board of the same family (nucleo-f446ze after nucleo-f412zg) found no startup file of its own and linked without `Reset_Handler`. Nothing is run when every path is already checked out, so a build that needs nothing new still works offline; a checkout that is not sparse, or a vendor folder that is not a repository of its own, is left as it is.
