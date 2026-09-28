# rho-doc-api

Adds a service's HTTP API reference to its documentation site.

This is a plugin for `rho-doc`. Install it and every module that serves HTTP
gets an API page in the same sidebar as its module reference, under the same
section.

```sh
rho module publish modules/plugins/rho-doc-api
```

## What it adds

One page per documentation build, in the site's `api` section beside the module
reference:

| Target | Title | Section |
| --- | --- | --- |
| `http-api` | HTTP API | `api` |

The page lists the endpoints, the parameters each takes — where they come from
and what they convert to — the request body, the statuses, and what a caller has
to prove. It is rendered by `rho-openapi` from the reading `rho-router` does, so
the page and the service's `openapi.json` are two renderings of one description
rather than two descriptions.

A module with no routes gets no page. Most modules are libraries, and a page
saying "this module serves no HTTP routes" would be noise on every one of them.

## One site

The page joins the site rather than sitting beside it: the same sidebar, the
same search index, the same cross-references. A reader who finds the module
reference finds the API reference from it, which is the whole reason this is a
plugin for the documentation generator rather than a service of its own.

## Where the routes come from

The plugin answers the whole-site hook, which carries the project directory the
build read. It reads the routes from there with the compiler's own parser — the
same reading a build does — and renders the page.

This works in both directions a build happens:

- `rho doc <project>` documents a working tree, and the page describes the
  routes in it.
- A registry documenting a published package runs the same generator over the
  unpacked tree, so a module that serves HTTP arrives with its API documented.

Only the plugins the toolchain installed are run for a published package, never
the ones the package carries: the tree in that case came from a registry, so it
came from someone else, and a documentation build is not a reason to run a
stranger's program.

## Building

```sh
cd modules/plugins/rho-doc-api && go build -o rho-doc-api ./src
```
