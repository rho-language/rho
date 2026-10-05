# Table of contents

A `rho-doc` plugin. It adds a table of contents to a documentation site and a
notice to every generated page, so a reader can tell what a person wrote from
what a build derived.

A plugin is not a command a person types: it is invoked by the tool it extends.
Here that tool is {ref}`rho-doc <rho-doc/index>`, which runs the plugin as a
process and reads its answer from a JSON document on its standard output.

## The attachment

The plugin names the component it plugs into, the contract it fills and the hook
it answers:

```yaml
extends: rho-doc

implements:
  - contents

hooks:
  - index
```

`rho-doc` offers the hook `index` once, with the finished set of pages. A plugin
that answers it gets every page at once, which is what a table of contents
needs: it cannot be built from one page, because what it lists is the others.

## What it answers with

Two files, in the shape `rho-doc` reads from a plugin: a table of contents page
and a snippet every generated page carries, so that a generated page says so.

```json
{"files": [{"path": "...", "content": "..."}]}
```

## Enabling it

```sh
rho plugin enable rho-doc-toc
rho doc . -out build/docs    # the plugin runs because it is enabled
```

A disabled plugin is not loaded, so a site built without it is the site
`rho-doc` produces on its own — which is what makes a plugin something a person
opts into rather than something that arrives with the tool.

## Related

- {ref}`rho-doc <rho-doc/index>` — the generator this extends, and the plugin
  contract in full.
- {ref}`rho-doc-api <rho-doc-api/index>` — the other plugin in this tree, which
  adds a service's HTTP API reference rather than a contents page.
