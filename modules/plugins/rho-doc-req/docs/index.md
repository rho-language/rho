# Requirements on documentation sites

This plugin adds a module's requirements, its architecture decisions and their
trace to its documentation site. The pages are written when the site is built,
from the same files `rho req` reads — `docs/requirements/**/*.yml` and
`docs/architecture/adr/NNNN-slug.md` — so the site and `rho req check` cannot
disagree, and nothing generated is committed.

## What a module gets

A module gets pages only when it is a repository of the requirements process:
its root keeps `docs/requirements/` or `docs/architecture/adr/`. A module with
neither gets nothing, and no empty page.

| Address | Title | When |
|---|---|---|
| `<module>/requirements/index` | *module* requirements | the module declares requirements |
| `<module>/requirements/trace` | Requirements trace | the module is the root of its family (it keeps `docs/requirements/levels.yml`) |
| `<module>/adr/index` | *module* architecture decisions | the module records decisions |
| `<module>/adr/NNNN-slug` | ADR-NNNN: *title* | one per decision; listed by the index rather than in the menu |

They follow the module's own pages in its section of the menu, in that order.

### Requirements

A section per level, highest first, with a table of the level's requirements and
then each in full: its statement and rationale, status, verification, what it
satisfies and what satisfies it, the decisions about it, the code tagged `@req`,
the tests tagged `@verifies`, its owner, tags, the date it changed and the file
and line it is declared on.

What a requirement satisfies, or is satisfied by, may be in another repository of
the family. It is a link when that repository is on the same site and its id in
plain text when it is not, so a page never carries a reference that resolves to
nothing.

**Evidence** — whether the tests that verify a requirement passed — is shown when
the repository has results in `build/test-reports/` at build time: `results.json`,
which every `rho test` writes, or the other reports `rho req trace` reads. The
page says which run it is from. Without results the page says so and shows no
evidence column, rather than calling everything untested.

### The trace

On the root of a family (the repository that keeps `levels.yml`, such as
`robohub`): the levels with how many requirements each has and in which
repositories, every requirement of the family by level, the trace matrix — each
requirement's children, decisions, code, tests and evidence — and the gaps
`rho req trace` reports, saying which block a release.

### Decisions

The index lists every decision by number with its status, date and requirements.
A decision's page starts with its front matter as a header — status, date,
requirements, what it supersedes and what supersedes it, each a link — followed by
its text.

## References

| Write | On | Reaches |
|---|---|---|
| `{ref}`GW-0012`` | any page | the requirement, on its module's requirements page |
| `{ref}`robohub-gateway:ADR-0009`` | any page | the decision of the repository `robohub-gateway` |
| `{ref}`ADR-0009`` | a page of the module that records it | the module's own decision |

A decision is named by its repository's directory, as `rho req` names it — or,
for a module nested in a repository with a `requirements:` block of its own, by
its module name (`robohub-provider-mesh:ADR-0001`), which is the name `rho req`
gives it. A page
may also use the module's name in its place (`{ref}`gateway:ADR-0009``): rho-doc
treats a module's name and its directory as the same module in a qualified label.

## How it works

The plugin answers rho-doc's `sources` hook, which runs once before any page is
read, with the modules the build found. It answers with Markdown pages for some of
them, and rho-doc reads them as the modules' own pages: parsed, cross-referenced,
searched, and checked — a reference on them that names nothing is reported like
any other.

The repositories of a family are found beside each other in the tree being
documented (a repository's `parent` is its sibling directory, or a repository
that encloses it), not through
`ROBOHUB_WORKSPACE` or `RHO_WORKSPACE`, so a site shows exactly the tree it was
given. A module whose requirements have errors still gets its pages, with a note
in the build output to run `rho req check`.

Without this plugin, `rho doc` builds the site without these pages and says once
which modules keep requirements or decisions it could not show.
