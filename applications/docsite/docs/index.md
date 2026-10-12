# The docsite service

`rho build applications/docsite` produces one Go binary that serves the
documentation tree. Like the registry it is not self-contained: it renders each
page by calling `rho doc`, so the host that runs it needs the Rho toolchain and
the `rho-doc` component.

## What it serves

- `/` — an overview: how many pages and sections the tree has, where to start,
  and the project's most recent release notes.
- `/docs` and `/docs/{path...}` — the documentation, with a menu built from the
  tree beside every page.
- `/releases` — the project's `CHANGELOG.md`, one entry per release.
- `/healthz` — liveness.

Every page is public. A page is rendered from its Markdown with the same
generator a build uses, so cross-references resolve the same way in both.

## Configuration

| Variable              | Default             | What it is                           |
|-----------------------|---------------------|--------------------------------------|
| `RHO_HTTP_ADDR`       | `:8080`             | where it listens                     |
| `RHO_DOCS_ROOT`       | `.`                 | the project the site is built from   |
| `RHO_DOCS_TREE`       | `docs`              | the documentation directory the menu lists |
| `RHO_SITE_TITLE`      | `Rho documentation` | what the site calls itself           |
| `RHO_SITE_RELEASES`   | `CHANGELOG.md`      | the release notes the overview reads |
| `RHO_SITE_REPOSITORY` | *(unset)*           | the project the pages belong to      |
| `RHO_SITE_VERSION`    | *(unset)*           | the version the footer and index show; falls back to `VERSION` beside the service |

## Deployment

It is deployed exactly as the registry is — build a container image from the
repository, run it behind nginx — with the documentation tree mounted or copied
in and the release notes beside it. `deploy/Dockerfile`, `deploy/docsite-docker.service`
and `deploy/nginx.conf` are the equivalents of the registry's, and the same
systemd-unit and ISPConfig notes in the registry's `docs/deployment.md` apply.
