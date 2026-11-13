# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-11-13

A named parameter written more than once in a statement (`:level = '' OR level =
:level`) is one value: it is rendered as `?N`, its ordinal, so every mention binds
the same argument rather than leaving the later ones unbound.
