# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.2.0] - 2026-11-11

The plugin now answers `rho-module`'s `contract` hook with the module contract
RoboHub publishes in `robohub-common/contracts/`. The toolchain validates a
RoboHub module against the ecosystem's own types and schemas instead of carrying
them, which is what keeps Rho a language for more than one ecosystem. A device
is the first type this makes possible: it is a `device`, not a `driver`.

## [0.1.0] - 2026-10-12

Initial release.
