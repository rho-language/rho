# Changelog

Notable changes to the Rho toolchain, one section per release. The section for
the version being published is what a registry shows against that version, so
this is the file the handbook's overview reads.

## [0.1.0] - 2026-10-12

First release of the Rho toolchain: the compiler, the C and Go backends, the
standard library, the module and package tools, and the component set — the
formatter, linter, documentation generator, test runner, debugger, target specs
and the keeper dashboard.

Added the module registry and its official deployment at
https://registry.rho-lang.org, where components and plugins are published and
installed from, and the handbook service that serves the documentation.

Modules gained a release history: a `CHANGELOG.md` beside the manifest, carried
in the package, whose section for a published version is recorded against that
release.
