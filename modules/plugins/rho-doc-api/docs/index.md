# HTTP API reference for documentation sites

This plugin adds one page to a documentation site: the HTTP API of the module
the site documents. It is a plugin rather than a generator of its own because
the point is that there is one site.

(api:what)=
## What a reader gets

A page in the site's `api` section, beside the module reference, listing:

- every endpoint, grouped the way the service declares it;
- the parameters each takes, where they come from, and what they convert to;
- the request body, as a shape or as raw bytes;
- the statuses the operation answers with;
- what a caller has to prove, where the service asks for anything.

What a reader does *not* get is a second site. The page is a page of the same
documentation: the same sidebar, the same search index, the same
cross-references. A reader who arrived at the module reference finds the API
reference from it, and a reader who arrived at the API reference finds the rest
from there.

(api:where)=
## Where the routes come from

The plugin reads them, with the compiler's own parser, from the project
directory the build was given. That is the same reading `rho-router` does and
the same reading the service's `openapi.json` is written from, so the page and
the document cannot disagree.

A module with no routes gets no page: most modules are libraries, and a page
saying so would be noise on every one of them.

(api:published)=
## A published package

A registry documents what it receives. It unpacks a package and runs the same
generator over the unpacked tree, so a module that serves HTTP arrives with its
API documented — the page describes the routes in the package that was
published.

Only the plugins the toolchain installed run for a published package. The tree
in that case came from a registry, so it came from someone else, and a
documentation build is not a reason to run a stranger's program.

(api:hooks)=
## The hook it answers

| Hook | Asked | For |
| --- | --- | --- |
| `index` | once, over the site | adding a page the build does not have |

The whole-site hook rather than the per-page one, because the API reference is a
page that did not exist. The per-page hook rewrites a page that is already
there.

## Installing

```sh
rho module publish modules/plugins/rho-doc-api
```

The plugin is then found by every documentation build the toolchain runs,
including the ones a registry runs over the packages it holds.
