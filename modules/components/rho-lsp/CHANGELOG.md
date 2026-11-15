# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.2.0] - 2026-11-14

Diagnostics are the compiler's: a file is checked as part of its project (its manifest, its dependencies, the unsaved buffer), every error is shown at its own line and column with the whole token underlined, and notes arrive as related information. Fixed: every diagnostic was one line low.

## [0.2.1] - 2026-11-14

Slice cuts are indexed: names used inside `s[a..b]` are found, hovered and renamed.

## [0.2.2] - 2026-11-15

Function literals are indexed: their parameters and the names they capture are found and hovered.

## [0.2.3] - 2026-11-15

Deprecation warnings carry the Deprecated tag, so an editor strikes the use through.
