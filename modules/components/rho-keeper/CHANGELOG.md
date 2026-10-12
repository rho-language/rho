# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.2] - 2026-10-12

Added uninstall: a module that is not in use can be removed from this machine,
with the same `rho module uninstall` a terminal would run. It is refused while
the module is in use, so the page asks for it to be turned off first, and once
removed the registry offers it again.

## [0.1.1] - 2026-10-12

Added a remote registry section: the modules the configured registry offers,
which of them are not installed here, and an Install button that runs
`rho module install` — the same command a terminal would.

Added a release history to every module: a `CHANGELOG.md` beside the manifest,
carried in the package, with the section for the published version recorded on
the registry against that release.

## [0.1.0] - 2026-10-01

Initial release: a local dashboard for the installation — every module, the
version in use, the contracts each component needs filled, and what could not be
read.
