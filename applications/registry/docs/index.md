# The Rho module registry

The registry is the service a Rho installation asks where a module comes from:
`rho module search`, `rho module install` and `rho module publish` all speak
HTTP to a registry, and the registry this repository ships is one.

It is an application rather than a component, because it has a `main` and
generates a program. It generates the Go service target alone: a registry is
only useful behind a network, so there is no native build.

## What it does

| Operation | Route | What it does |
|-----------|-------|--------------|
| Publish | `POST /api/modules/{name}/{version}` | Accepts a package and records its metadata |
| Download | `GET /api/modules/{name}/{version}` | Serves the published package |
| Detail | `GET /api/modules/{name}` | Lists a module's versions, newest first |
| Search | `GET /api/search?q=…` | Searches by name and description |
| Health | `GET /api/health` | Reports the service and its version |

The pages around those routes are HTML: an index of published modules, a page
per module, a page per version, and a not-found page that is styled rather than
bare, because a reader who mistypes a name is still a reader.

## Access

A registry is open by default, which suits a trusted network, and a deployment
that needs it secured sets one setting:

```ini
# registry.conf
access = secured
```

With `access = secured`, publishing requires a token. Tokens are issued by the
credential manager, expire, and have a ceiling: a token can never outlive
`token_max_days`, whatever the settings say.

## Storage

Packages are stored on the filesystem, under a data directory that lives
outside the application:

```
<data>/modules/<name>/<version>/package.rho-pkg   the uploaded package
<data>/modules/<name>/<version>/meta.json         its metadata
<data>/index/<name>.json                          the version list
```

The data directory is separate so that installing a new version of the registry
over the old one cannot take the modules with it. `RHO_REGISTRY_DATA` overrides
it, which is what a container uses.

## Building it

```sh
rho build applications/registry
```

That generates the Go service under `build/generated/go-server/` and builds the
binary. The service carries its own templates, its own static files and the log
port, so the result is one binary with no runtime dependencies beyond the C
library.

## Where to go next

- {ref}`handbook/documentation` — how this page and the one beside it are
  produced, and how a module documents itself the same way.
- The deployment note beside this page, for running it.
