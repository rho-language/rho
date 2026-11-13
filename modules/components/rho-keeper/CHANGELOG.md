# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.7] - 2026-11-13

`sync`, `fix` and `install` take `--path DIR`, for a module in a folder that no
registry lists — an application in its checkout. A switch such as `--yes` is no
longer taken for the flag value of the name after it.

## [0.1.0] - 2026-10-01

Initial release: a local dashboard for the installation — every module, the
version in use, the contracts each component needs filled, and what could not be
read.

## [0.1.1] - 2026-10-12

Added a remote registry section: the modules the configured registry offers,
which of them are not installed here, and an Install button that runs
`rho module install` — the same command a terminal would.

Added a release history to every module: a `CHANGELOG.md` beside the manifest,
carried in the package, with the section for the published version recorded on
the registry against that release.

## [0.1.2] - 2026-10-12

Added uninstall: a module that is not in use can be removed from this machine,
with the same `rho module uninstall` a terminal would run. It is refused while
the module is in use, so the page asks for it to be turned off first, and once
removed the registry offers it again.

## [0.1.3] - 2026-10-12

Fixed the registry section listing a module once per published version; it is
one row per module now, showing the newest release.

The header reports the version that is installed and in use. A binary run from
outside its module directory used to find no manifest and show `0.0.0`; it now
reads the installed module's manifest instead.

## [0.1.4] - 2026-11-10

Security fix for toolchain archives. Tar and zip extraction now use `os.Root`,
which refuses a path outside the destination even after a link. The host-tar
fallback validates the listing before extraction. The previous post-extraction
cleanup, which could delete any path named in a malicious listing, is gone.
Tests cover escaping tar and zip members and the preflight.

## [0.1.5] - 2026-11-10

The module detail page now offers every installed version and can enable any of
them, not only the newest. Switching versions is the same `rho module enable
name@version` operation a terminal runs; the on/off switch remains separate.

## [0.1.6] - 2026-11-10

The version selector is now the Version field itself. Choosing a version updates
that field; the toggle enables the selected version, switches to it when another
is selected, or turns the module off when the active version is selected. The
separate version-enable button is gone. The detail page also reports the enabled
version rather than always showing the newest installed one.
