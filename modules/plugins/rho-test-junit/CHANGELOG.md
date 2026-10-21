# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-10-21

A report reads the run's own verdict. The status the run concluded is what the
report reads, rather than a verdict of its own: a report that decided for
itself what passed would be a second opinion on the run, and a run that passed
would be reported as failed by a report that never read it.

## [0.1.2] - 2026-10-21

A test that is not run is reported as skipped, not as a failure. A run that
reports a passing test as a failure is worse than a run that reports nothing,
because a CI system reads it and stops a build that would have passed.
