# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.2.0] - 2026-11-14

A new `rho-check` rule reports what the type checker refuses; `rho-parse` reports each broken declaration where it is.

## [0.2.1] - 2026-11-14

Rules walk into slice cuts (`s[a..b]`).

## [0.2.2] - 2026-11-15

Rules read function types and literals; an import used only in a type is not reported as unused.

## [0.2.3] - 2026-11-15

Rules read constraints and explicit type arguments.

## [0.3.0] - 2026-11-15

Test files (`test fn`) are read. Files under the build and output directories are skipped; `--include-generated` reads them. `rho-module-name` accepts the name `rho init` writes and says what the name should be; `rho-unused-import` counts a bare call of what the import declares; `rho-shadowed-binding` ignores a binding declared after the inner scope.

### Breaking

- Generated files are no longer linted by default: pass `--include-generated`.
