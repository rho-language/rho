# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-10-12

Fixed cross-reference links on nested pages: a `{ref}` now resolves relative to
the page it is on, so a link from `handbook/index` to a page in another section
reaches that page instead of looking for it inside `handbook/`.

The sidebar now opens only the section the reader is in and lists every other
section as a link to its first page, so the API reference is one line rather
than a list of every generated page on every page. A published site's link back
to the service that serves it is labelled by the service rather than "back".

## [0.1.2] - 2026-11-03

Images written as `![Description](./images/shot.png)` are rendered as images
and copied into the built site beside the page that shows them. A source that
names an image the tree does not carry is reported, so a broken screenshot is a
build diagnostic rather than a dead link. A page's images are scaled to the text
column.

## [0.1.3] - 2026-11-12

Built against the manifest reader that knows providers (`type: provider`),
interfaces and bindings. The 0.1.2 binary refused those manifests as unreadable,
so a tree's provider modules were left out of its documentation.

## [0.1.4] - 2026-11-14

Built against rho 0.7.1, whose manifest reader knows a module's `sql:` block: the six RoboHub modules that declare one (appapi, inventory, journal, rules, scheduler, widgets) are documented again instead of skipped as unreadable.

## [0.2.0] - 2026-11-14

A repository describes its site in `docs/site.yml`: the title, the page `index.html` opens on (`front`), where the API reference is read from (`api`, or `none`), and the order and names of the sections and of the pages within them. A tree without the file is built as before. A fenced block opened with four backticks holds a three-backtick block and names its language without the extra backtick. The API scan skips `testdata` folders, whose sources are fixtures rather than an interface. A tree with no root manifest is named by its site file's title rather than by its folder, and a project named like the site is not repeated beside the title. The "On this page" rail lists a page's sections and their parts (levels 2 and 3), not every small heading under them. A site served through the fragment interface (the docsite service) reads `docs/site.yml` the same way: its API reference comes from the directory the file names, and its sections are in the file's order.

## [0.2.1] - 2026-11-14

The API reference lists enums and their variants.

## [0.2.2] - 2026-11-15

API signatures show the constraints on type parameters.

## [0.2.3] - 2026-11-15

Each API page says how stable its module is, the index has a Stability column, and deprecated declarations and modules carry their deprecation note.

## [0.2.4] - 2026-11-19

A module that keeps process artefacts beside its pages keeps the pages in `docs/doc/`: when it exists, only it is read by the convention, and `docs/doc/` is left out of a page's address exactly as `docs/` is, so a page keeps its address and every reference to it when it moves there. `docs/requirements/` and `docs/architecture/` are not read as pages.

## [0.2.5] - 2026-11-19

A new hook, `sources` (contract `sources`): before any page is read, a plugin is given the modules the build found and answers with Markdown pages for them, which are read like the modules' own — parsed, cross-referenced, searched — and follow them in their section, in the order given; a page may be `unlisted` (out of the menu unless it is being read). This is how rho-doc-req puts requirements and architecture decisions on a site. A label qualified by its module (`robohub-gateway:ADR-0009`) is reached by its short name from that module's pages, and by either of a module's names — its manifest name or its directory. Decision records under `docs/architecture/adr/` are not read as pages even without `docs/doc/`. A tree that keeps requirements or decisions and has no plugin to show them gets one note in the build output; the site is built without them and `-strict` is not affected.
