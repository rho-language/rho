# rho-doc-toc

A documentation plugin. It adds a table of contents to a documentation site and
a notice to every generated page, so a reader can tell what a person wrote from
what a build derived.

A plugin is not a command a person types: it is invoked by the tool it extends.
Here that tool is the `rho-doc` component, which runs the plugin as a process
and reads its answer from a JSON document on its standard output.

The attachment is declared rather than assumed. The plugin names the component it
plugs into and the hooks of that component it answers:

```yaml
extends: rho-doc

hooks:
  - index
  - page

commands:
  - name: table-of-contents
    run: rho-doc-toc
    hook: index
  - name: generation-notice
    run: rho-doc-toc
    hook: page
```

`rho-doc` declares the hooks it runs in its own manifest, and
`rho module validate` checks this plugin against that list: the component has to
exist, the hooks have to be ones it offers, and each command has to answer a
hook declared here. A plugin that fails those checks is refused rather than
installed.

## What it answers

| Hook | When the generator runs it | What the plugin does |
|------|---------------------------|----------------------|
| `index` | Once, over the finished set of pages | Adds a contents page listing every page, grouped by section |
| `page` | Once per page, with that page in the request | Adds a notice that the page was generated, keeping the page's own body |

The table of contents is why the whole-site hook exists: a contents page is an
answer about *every* page, so no single page can produce it.

## The protocol

The generator writes a request to the plugin's standard input:

```json
{
  "version": 1,
  "hook": "index",
  "site": "Rho",
  "pages": [
    {"target": "index", "title": "Rho", "href": "index.html", "section": "documentation"},
    {"target": "handbook/ports", "title": "Ports", "href": "handbook/ports.html", "section": "handbook"}
  ]
}
```

and reads the answer from its standard output:

```json
{
  "version": 1,
  "pages": [
    {"target": "contents", "title": "Contents", "section": "handbook", "html": "<nav>...</nav>", "plain": "contents"}
  ],
  "formats": ["latex"]
}
```

A `page` hook answers with `replacements` instead, keyed by the target of the
page it rewrote:

```json
{
  "version": 1,
  "replacements": {"api-std-core-option": "<div class=\"notice\">…</div><h1>std.core.option</h1>"}
}
```

## What this plugin does not do

It is not loaded by any other tool. A component loads the plugins that name it,
so this one runs in a `rho doc` build and nowhere else, and `rho doc
-list-plugins` says so for a plugin that is installed but belongs elsewhere.

## Failure

A plugin that fails is reported and skipped, because one broken extension must
not cost a reader the documentation of everything else. A plugin that answers a
protocol version it does not speak is reported too, rather than answered with
something the generator would misread.

## Running it

```sh
go build -o rho-doc-toc ./src
go test ./...
```

To have the generator run it, publish it and put the command on `PATH`:

```sh
rho module publish modules/plugins/rho-doc-toc
```
