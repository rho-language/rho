# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.2] - 2026-11-03

Images written as `![Description](./images/shot.png)` are rendered as images
and copied into the built site beside the page that shows them. A source that
names an image the tree does not carry is reported, so a broken screenshot is a
build diagnostic rather than a dead link. A page's images are scaled to the text
column.

## [0.1.1] - 2026-10-12

Fixed cross-reference links on nested pages: a `{ref}` now resolves relative to
the page it is on, so a link from `handbook/index` to a page in another section
reaches that page instead of looking for it inside `handbook/`.

The sidebar now opens only the section the reader is in and lists every other
section as a link to its first page, so the API reference is one line rather
than a list of every generated page on every page. A published site's link back
to the service that serves it is labelled by the service rather than "back".

## [0.1.0] - 2026-10-12

Initial release.
