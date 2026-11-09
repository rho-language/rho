# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.1] - 2026-10-12

Initial release.

## [0.1.2] - 2026-11-09

The Rho surface now configures the logger: `log_configure` sets the minimum
level, text or JSON format, the file path, rotation size and retention, and
whether records also go to standard error. `fatal` is available alongside the
other levels; the Go logger now has `Fatal` as well, without terminating the
process.
