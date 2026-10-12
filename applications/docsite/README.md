# docsite

The Rho documentation, served as a site: an index that says what is here, a menu
beside every page, the pages themselves, and the project's release notes.

It is the [`applications/registry`](../registry) shape — a Rho application
compiled to a Go HTTP service — pointed at the documentation tree rather than at
a module store. The pages are read from the tree and rendered with the same
generator a build uses, through the `docstore` port, so a page served here and a
page a build writes are the same page.

## Routes

Every page is public: this is documentation.

| Path                          | What it shows                                  |
|-------------------------------|------------------------------------------------|
| `/`                           | what the site holds, where to start, recent releases |
| `/docs`                       | the documentation's index page, with the menu  |
| `/docs/{path...}`             | one documentation page, with the menu          |
| `/releases`                   | the project's release notes in full            |
| `/healthz`                    | liveness                                       |

A wrong address is a `404`, not the index: the menu is shown so a reader can
find their way, but the status tells a crawler the truth.

## Layout

```
src/main.rho                 routes and handlers
src/docsite/storage.rho     the store, as extern declarations
src/docsite/web.rho         the shell, the menu and the rows
templates/                   the markup
```

The store is `include`d rather than imported, so its extern names stay exactly as
declared and the generated `docstore` port can implement them — the same reason
the registry includes its storage.

## Configuration

Everything has a default, so the service runs with none of it set:

| Variable              | Default            | What it is                            |
|-----------------------|--------------------|---------------------------------------|
| `RHO_HTTP_ADDR`       | `:8080`            | where it listens                      |
| `RHO_DOCS_ROOT`       | `.`                | the project the site is built from    |
| `RHO_DOCS_TREE`       | `docs`             | the documentation directory the menu lists |
| `RHO_SITE_TITLE`      | `Rho documentation`| what the site calls itself            |
| `RHO_SITE_RELEASES`   | `CHANGELOG.md`     | the release notes the overview reads  |
| `RHO_SITE_REPOSITORY` | *(unset)*          | the project the pages belong to       |
| `RHO_SITE_VERSION`    | *(unset)*          | the version the footer and index show; falls back to `VERSION` beside the service |

The service shells out to `rho doc` to render a page, so the host that runs it
needs the Rho toolchain and the `rho-doc` component present — the same
requirement the registry has. With neither, the index and the menu still serve,
and a page that cannot be rendered is answered as missing.

## Build and run

```sh
rho build applications/docsite

RHO_DOCS_ROOT=. RHO_DOCS_TREE=docs RHO_SITE_RELEASES=CHANGELOG.md \
RHO_SITE_REPOSITORY=https://github.com/rho-language/rho \
  ./applications/docsite/build/docsite
# rho docsite listening on :8080
```

## Deployment

See [docs/deployment.md](docs/deployment.md). It is deployed exactly as the
registry is: the same Docker image pattern, the same systemd unit, the same nginx
and ISPConfig reverse proxy. The one difference is what it reads.
