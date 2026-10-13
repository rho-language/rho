# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.1] - 2026-10-13

A preset may name no dependencies. A preset fetches the vendor repositories a
board needs, and a hosted target needs none — the machine already has its
compiler and its libraries — so an empty list is a hosted build rather than a
preset that says too little. A preset that does fetch something is still checked
dependency by dependency.

## [0.1.0] - 2026-10-12

Initial release.
