# rho-doc

The documentation generator. It renders what a Rho tree carries: the doc-as-code
sources each module ships, an API reference generated from the Rho sources, and
whatever the plugins add to the result.

```sh
rho doc . -out build/docs
```

The output is a directory of static files: one HTML page per source, an index,
a stylesheet and a search index. Nothing in it needs a server, so it can be
published to a file share, an object store or a plain web server, and it keeps
working when none of them change.

## The component in one page

| | |
|---|---|
| Command | `rho doc` |
| Library | `rho.doc`, declared in `src/doc.rho` |
| Port | `docemit`, for a project that serves documentation |
| Depends on | `rho-log`, for structured logging |
| Extensible by | Plugins, over a JSON process protocol |

It is a component like any other: published, installed and dispatched to by the
command its manifest publishes, rather than compiled into the CLI. See the
handbook in `docs/handbook/modules.md` for what that means.

## What it reads

Three kinds of source, in this order:

1. the `docs:` section of a module manifest, which names files or a glob;
2. a `docs/` directory beside that manifest, which is the convention;
3. a `docs/` directory at the root of a repository, which is how a repository
   documents the language rather than one module.

A module's pages are placed in a section of the sidebar, ordered by what its
manifest declares.

## The format

MyST-style Markdown: Markdown for prose, with the constructs a reference needs.

````markdown
# Publishing a version {#publishing}

Publishing is refused without a token. See {ref}`the API <registry-api>`.

:::{warning}
The token is the only credential a publisher needs. Keep it out of a log.
:::
````

| Construct | What it means |
|-----------|---------------|
| `# Title {#target}` | A heading with an explicit target a reference can name |
| `:::{note}` … `:::` | An admonition, or any other directive |
| `{ref}`label`` / `:ref:`label`` | A cross-reference |
| ```` ```rho ```` | A code block, tagged with its language |

A construct that is not understood is kept as text rather than dropped, and a
cross-reference that names nothing is reported with the file and the line.

## The API reference

The reference is generated from the Rho sources with the compiler's own front
end, so every entry is the declaration the compiler sees. Generating it is the
only way it stays true: a reference copied into prose is wrong the day the code
changes.

Externs are listed apart from functions, because an extern is what a port
supplies rather than what Rho declares.

## Plugins

A plugin adds to a build. It declares what it contributes in its manifest and
answers hooks as a process: the generator writes a request to its standard input
and reads JSON from its standard output.

| Hook | When it runs | What a plugin answers with |
|------|--------------|---------------------------|
| `index` | Once, over the finished set of pages | Pages to add |
| `page` | Once per page, with that page in the request | Rewritten bodies, keyed by the page's target |

A plugin that fails is reported and skipped. One broken extension must not cost
a reader the documentation of everything else.

`modules/plugins/rho-doc-toc` is the worked example: it adds a table of contents
and a notice to generated pages.

## As a dependency

A project that serves documentation declares the component and asks for the
port, so the page it renders and the page a build writes come from one
implementation:

```yaml
deps:
  rho-doc: ">=0.1.0"
```

```yaml
targets:
  - id: go
    ports:
      - docemit
```

## Building it

```sh
cd modules/components/rho-doc
go build -o rho-doc ./src      # the binary the manifest's main: names
go test ./...
```

Then publish it, which installs it under the Rho home, and `rho doc` reaches
it:

```sh
./bin/rho module publish modules/components/rho-doc
```

## Layout

| Path | What it holds |
|------|---------------|
| `src/main.go` | The command |
| `src/fragment.go` | Answering a port's request |
| `src/doc.rho` | The Rho library a project imports |
| `src/rhodoc/document.go` | The document model |
| `src/rhodoc/parse.go` | The MyST-style parser |
| `src/rhodoc/render.go` | The HTML renderer |
| `src/rhodoc/layout.go` | The page layout and the stylesheet |
| `src/rhodoc/source.go` | Finding the documentation a tree carries |
| `src/rhodoc/site.go` | Building a site |
| `src/rhodoc/api.go` | The reference, from the sources |
| `src/rhodoc/plugin.go` | The plugin protocol |
| `src/rhodoc/listing.go` | Listings a Rho handler can read |
