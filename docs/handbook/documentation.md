# Documentation as code

Every module documents itself, and the documentation lives in the module. There
is no separate documentation tree to keep in step: the page about a component is
in the component, travels with it, and is read from the same working tree as the
code it describes.

## A module documents itself with a `docs` section

```yaml
docs:
  title: Module registry
  section: applications
  order: "20"
  index: docs/index.md
  sources:
    - docs/index.md
    - docs/deployment.md
```

| Field | What it means |
|-------|---------------|
| `sources` | The doc-as-code files, relative to the module root. A glob names a tree. |
| `section` | Where the pages appear in the sidebar |
| `order` | How the module sorts against its peers in a section |
| `index` | The page the section leads with |
| `title` | The one-line summary a page shows, when the manifest does not already say it |

A `docs` directory beside the manifest is picked up without being declared, so a
module that drops a file into `docs/` gets it documented. The section says is
the one thing worth declaring, because it is what places the pages in a site.

## The format

Sources are MyST-style Markdown: Markdown for prose, with the constructs Sphinx
provides for a reference.

````markdown
# Publishing a version {#publishing}

A version is published by uploading a package to the API.

:::{note}
A version may be published once. Re-uploading the same version is refused.
:::

:::{warning}
The token is the only credential a publisher needs. Keep it out of a log.
:::

Fields: `package`, `version`, `description`.
````

What the dialect gives a reference, and what it is for:

| Construct | What it means |
|-----------|---------------|
| `# Title {#target}` | A heading with an explicit target, which a cross-reference can name |
| `:::{note}` … `:::` | An admonition: `note`, `warning`, `tip`, `important`, `caution`, `danger`, `hint` |
| `:::{name}` … `:::` | Any other directive, kept as a named block a plugin can interpret |
| ```` ```rho ```` | A code block, tagged with its language |

A cross-reference is written in either spelling, and a heading with an explicit
target is one a reference can name. Both spellings are shown here as a code
block rather than as prose, because a reference written for this page would
otherwise be one the generator tries to resolve:

:::{code-block} markdown
See {ref}`handbook/ports` for what a port is.
See :ref:`the same page <handbook/ports>` in the classic spelling.

## A section {#a-section}

A reference to a section lands on it: {ref}`a-section`.
:::

A construct that is not understood is kept as text rather than dropped. A page
that shows what the author wrote is easier to fix than a page that quietly lost
a paragraph.

## Cross-references

A reference names a target, and the site resolves it to the page that declares
it:

```markdown
Publishing is refused without a token. See {ref}`the API <registry-api>`.
```

A reference that names a target no page declares is reported by the build, with
the file and the line, because a broken cross-reference is a defect a reader
cannot fix and an author cannot see:

```
rho doc: docs/publishing.md:42: reference "the API" names no target
```

Two pages that share a target — which happens whenever two components each have
an `index.md` — are reported as ambiguous, and a page that needs its own address
is given one with `{#target}`.

## Building a site

```sh
rho doc . -out build/docs
```

The build:

1. finds every module in the tree and reads the sources it declares;
2. generates an API reference from the Rho sources it finds;
3. runs the plugins, so a plugin may add pages or rewrite one;
4. writes the pages, the index, the stylesheet and a search index.

The output is a directory of static files. It can be served by anything that
serves files, and it keeps working when nothing serves it.

| Flag | What it does |
|------|--------------|
| `-out DIR` | Where to write the site |
| `-api DIR` | Generate the reference from this tree rather than the one given |
| `-api-section NAME` | Where the reference appears in the sidebar |
| `-section NAME` | Put the pages of this path in a section |
| `-title TITLE` | The site title |
| `-no-plugins` | Build without extensions |
| `-list-plugins` | List the extensions that are available |
| `-strict` | Fail when a source reports a problem |

## The API reference

The reference is generated from the Rho sources with the compiler's own front
end, so every entry is the declaration the compiler sees. A reference copied
into prose is wrong the day the code changes, and a reader who finds one wrong
stops trusting the rest.

A generated page is marked as generated in the index, and a plugin may add a
notice to every generated page — which is what `rho-doc-toc` does.

## Extending the build

A plugin is attached to this generator: it names `rho-doc` in its manifest and
declares the hooks of this component it answers. A run of the generator is the
only thing that ever calls it, and it loads the plugins that name it. See
{ref}`handbook/modules` for the contract and what is checked.

A hook is run at most once per plugin, and only if the plugin declared it. The
answer arrives as a process: the generator writes a request to the plugin's
standard input and reads a response from its standard output, both JSON. The
envelope is the toolchain's rather than this component's — `rho/src/plugins` —
so a plugin is written the same way whichever component it extends, and only the
payload differs: pages for a documentation build, test results for a test run.

```json
{
  "version": 1,
  "hook": "index",
  "site": "Rho",
  "pages": [{ "target": "index", "title": "Rho", "href": "index.html" }]
}
```

```json
{
  "version": 1,
  "pages": [{
    "target": "contents",
    "title": "Contents",
    "section": "language",
    "html": "<nav>...</nav>"
  }],
  "formats": ["latex"]
}
```

| Hook | When it runs | What a plugin answers with |
|------|--------------|---------------------------|
| `index` | Once, over the finished set of pages | Pages to add — a table of contents is an answer about every page |
| `page` | Once per page, with that page in the request | Rewritten bodies, keyed by the page's target |

A plugin that fails is reported and skipped. One broken extension must not cost
a reader the documentation of everything else.

A hook this generator does not run is never asked for, whatever a plugin
declares, so a plugin written against another version of the manifest is inert
here rather than called with a request it cannot answer.

## Documentation as a dependency

A project that serves documentation declares the generator as a dependency and
asks for the `docemit` port:

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

The port supplies the same implementation the command uses, so a page rendered by
a service and a page written by a build are produced by one generator. See
{ref}`handbook/ports` for what a port is.
