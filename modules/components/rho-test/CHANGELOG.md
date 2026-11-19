# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.2] - 2026-10-21

A Rho test is built for the C backend, and for no target at all. The name the
runner used to pass was not a target a spec declares: a project that declares a
Go target and no C one had no such target, and the build was asked for a board
that does not exist. The runner wants the C the backend generates, because it
compiles that C itself and runs what it compiled.

## [0.1.3] - 2026-10-21

A suite manifest may include another. A project keeps its tests in folders —
the unit tests in one, the smoke tests in another — and the manifest that says
what the project has includes them, so one run covers both and a run of either
can be asked for on its own.

The included suites are the project's suites: they run in the project's own
root, not in the folder they were written in, because a path in a suite means
one thing and a test whose paths meant two would be a test that passes in one
place and fails in another.

## [0.1.4] - 2026-10-21

The results a run hands a plugin are now in the shape a plugin reads them in.
The fields carried no names of their own, so every plugin read empty results and
reported every case as a failure: a report that says a passing run failed is
worse than no report, because a CI system reads it.

## [0.1.5] - 2026-10-21

A suite may say `enabled: false`, which keeps it from being run. A suite that
is not enabled is not a suite that failed: it is there, and the reason it is
not run is that somebody said so. The listing shows it as skipped, with the
reason.

## [0.2.0] - 2026-11-15

Test programs are built under `build/test/` (through `rho build --emit-c`) and never overwrite the application's binaries. A suite that finds no test file, or no `test fn`, fails the run. Declared `test fn`s are the cases: a printed `[PASS]`/`[FAIL]` (or TAP) line reports the case of that name, a case nothing reported fails if the program stopped and is skipped with a hint if others reported; TAP `ok 3 - name` is the case `name`. `--echo` no longer repeats result lines. `rho test path/x_test.rho` runs only that suite. Flags take `--x=value`.

### Breaking

- A missing or empty suite fails the run instead of being skipped.

## [0.3.0] - 2026-11-19

Every run that runs something writes its own record, `build/test-reports/results.json` (or under `--report-dir`), replacing the one before it: the run time, the rho version, the runner's version, and per suite its name, kind, status and duration with every case's name, status (`pass`, `fail`, `skip`), message and file:line. A tool that reads the last run — `rho req trace` — reads it rather than the text a person reads, so evidence no longer depends on `--verbose`, a redirect or the JUnit plugin. What a run prints is unchanged.

## [0.3.1] - 2026-11-19

A skipped case is read from the runner's output — `[SKIP] name  (reason)`, GoogleTest's `[  SKIPPED ]`, TAP's `# SKIP` — and reported skipped with its reason; a suite whose cases were all skipped is skipped, where it was failed as a suite that "ran but reported no cases".

## [0.4.0] - 2026-11-19

A Rho test file is built as a test build, `rho build --emit-c --test-report <names>`, which makes every `test fn` print `[PASS] <name>` when it returns. A declared `test fn` nothing reported therefore did not run, and it fails: "never ran: main() does not call it" when the program exited 0, "did not run: the program stopped before it" when the program stopped. A printed `[FAIL]` still fails its case, and a failure still wins over a pass. With a rho older than `--test-report` the runner warns in one line and reads the cases the old way. A failed case prints its reason beside it: `[FAIL] suite: name  (reason)`.

### Breaking

- A `test fn` that `main` never calls fails. It used to pass when the program printed no result lines, and be skipped when it printed others — a test that never ran was reported green.
