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

What a registry knows is kept in tables, declared in `db/schema.yml` and created
when the service starts:

| Table | What it holds |
|-------|---------------|
| `users` | the identities, named by whatever credential presented them |
| `roles` | the named permissions an identity may hold |
| `user_roles` | which roles an identity holds |
| `tokens` | the credentials the service has issued, and which have been withdrawn |
| `modules` | the names it holds, who owns each, and whether each is public |
| `module_maintainers` | which identities may publish to a module, and as what |
| `module_versions` | the published versions, each with its package and its manifest |

A publish is therefore one commit: the upload and the record of who made it are
the same statement. A registry that wrote a package to a file and then wrote the
record beside it could be interrupted between the two, and what is left is a
module nobody owns.

What stays on the filesystem is what is *derived* rather than recorded — the
source tree a package is unpacked into, and the documentation generated from it:

```
<data>/registry.db                                 the records
<data>/docs/<name>/<version>/                      the generated pages
<data>/modules/<name>/<version>/src/               the unpacked package
```

The database sits beside the data directory rather than inside the served tree,
and is named by `RHO_SQL_DATABASE` — defaulting to `<data>/registry.db` — while
`RHO_SQL_DRIVER` names the driver, defaulting to `sqlite`. Both are the
deployment's to choose, because a binary that compiled either in would be a
binary this build decided them for.

The data directory is separate from the application so that installing a new
version over the old one cannot take the modules with it. `RHO_REGISTRY_DATA`
overrides it, which is what a container uses.

The tables are declared rather than written as SQL: adding a column is a change
to one file, and the schema the service creates and the code generated from it
come from the same reading of it. The generated Rho is an output of the build
and is not checked in.

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
