# Rho Module Registry

A remote registry for Rho modules, written in Rho and compiled to a Go HTTP
service. It speaks the three endpoints the Rho module client already uses, so
`rho module publish`, `download` and `search` work against it.

## Endpoints

| Method | Path                                | Purpose                     |
|--------|-------------------------------------|-----------------------------|
| GET    | `/healthz`                          | Liveness probe              |
| POST   | `/auth/token`                       | Mint a token (issuance policy applies) |
| POST   | `/modules/{name}/{version}/rho-pkg` | Publish a module package    |
| GET    | `/modules/{name}/{version}/rho-pkg` | Download a module package   |
| DELETE | `/modules/{name}/{version}`         | Withdraw a version (owner)  |
| PUT    | `/modules/{name}/visibility`        | Set public/private (owner)  |
| GET    | `/modules/{name}/{version}/docs/{path...}` | A version's documentation |
| GET    | `/search?q=<query>`                 | Search published modules    |
| GET    | `/modules/{name}`                   | List a module's versions    |
| GET    | `/openapi.json`                     | This service's API description |

## Layout

```
applications/registry/
├── module.yml                     build configuration, server target only
├── project.yml                    descriptor: the generated port and targets
├── src/
│   ├── main.rho                   routes and handlers
│   └── registry/
│       ├── storage.rho            the storage port, as extern declarations
│       ├── validate.rho           request validation and settings
│       ├── auth.rho               token issuance, reading policy
│       ├── web.rho                the HTML pages
│       ├── docs.rho               documentation pages for a version
│       └── respond.rho            response and JSON shapes
└── tests/
    └── validate_test.rho          Rho tests
```

Routing, validation, the protocol and the responses are Rho. Persistence sits
behind a port: Rho has no heap, no globals and no file I/O yet, so the compiler
generates the store (`fsstore`) into `ports.go` rather than the project
shipping host code. Nothing outside `src/` and `tests/` is hand-written Go.

## Tests

Two layers, because Rho tests can only reach what a plain program can:

**Rho tests** cover the logic a command line build can run: version
validation, bearer-token parsing, and the response shapes. They live in
`tests/validate_test.rho`. They are declared in `tests/tests.yml` as skipped:
the descriptor generates `go`/`go-server` only, and the `go` target's generated
SQL port does not build for a program that omits the generated db models, so
there is no target to build them on yet. The Go integration tests cover the same
logic meanwhile.

```sh
rho test applications/registry --target go
```

**Integration tests** cover what only the generated service has: the storage
port and the HTTP protocol. They build the service and drive it over real
HTTP, including a byte-exact publish/download round trip, authentication,
duplicate rejection and path traversal.

```sh
go test ./src/ -run TestRegistry
```

## Build and run

The Rho toolchain generates the Go project and then builds it, so a separate
`go build` is not needed:

Both commands take the project, and the manifest supplies the entry point, so
the source file never has to be named:

```sh
rho build applications/registry     # generate + build
rho run  applications/registry     # generate + build + run
```

From inside the project the path can be left off entirely:

```sh
cd applications/registry
rho run                             # uses module.yml's main and build_dir
```

The executable is written to the descriptor's `build_dir`, so a build produces
one obvious artifact:

```
applications/registry/build/registry                 the executable
applications/registry/build/generated/go-server/    the generated Go project
```

That binary is a normal program and can be run on its own, which is what the
systemd unit does:

```sh
./build/registry
```

The generated Go project is also an ordinary project, so it can be rebuilt by
hand after editing generated sources:

```sh
cd build/generated/go-server && go build -o ../../registry .
```

## Pages

Public: the landing page, search, a version list, a module's documentation and
its download need no credential, whatever `access` says:

| Path                          | What it shows                                        |
|-------------------------------|------------------------------------------------------|
| `/`                           | every module, with its newest version badged         |
| `/search?q=`                  | matching modules, one row per version                |
| `/modules/{name}`             | one module's versions, newest first, each with its release statement |
| `/modules/{name}/{version}`   | that version's metadata — publisher, publish time, release note — and its download path |

Every page carries a footer with the registry version from its manifest, the
year, and a link to rho-lang.org.

### Release history

A module's description says what it is and does not change between releases, so
two versions of one module would read alike. The other half is a `CHANGELOG.md`
beside the module's manifest: a section per release, in the module's own words.
`rho module publish` reads the section for the version being published and sends
it with the package, so the registry records it against that version without
unpacking anything:

```markdown
## [0.1.1] - 2026-10-12

Added a remote registry section.
```

A version's page shows the statement, and a version list shows it beside each
version, so 0.1.0 and 0.1.1 are told apart by what changed rather than by two
identical descriptions. The note is plain text a publisher wrote, so it is
escaped before it is shown and never trusted as markup. A version published
before a module kept a changelog simply has no note.

The machine surface is kept apart under `/api`, so a client and a person get
what each expects from the same service:

| Path                          | What it returns                        |
|-------------------------------|----------------------------------------|
| `/api/search?q=`              | JSON array of module metadata          |
| `/api/modules/{name}`         | JSON with the published version list, and each version's `sha256` |
| `/modules/{name}/{v}/rho-pkg` | the uploaded package, verbatim        |

The `sha256` is the digest of the package as it was uploaded, computed at
publish and stored with the version. A client checks what it downloaded against
it, so a package altered where it is kept no longer agrees with what the
registry says it published.

## Configuration

Settings are read at start-up from `RHO_REGISTRY_CONFIG`, or
`~/.rho/registry/registry.conf`, as `key = value` lines. See
[registry.conf.example](registry.conf.example); every setting has a default, so
the service runs with no settings file at all.

| Setting           | Default                   | Purpose                                    |
|-------------------|---------------------------|--------------------------------------------|
| `data_dir`        | `data`                    | where modules are stored                   |
| `access`          | `public`                  | `public`, or `secured` to restrict publishing to the allowlist |
| `title`           | `Rho module registry`    | the name on the pages                      |
| `site_url`        | `https://rho-lang.org`   | the footer link                            |
| `registry_url`    | `/`                       | the address this service is published under; set it behind a proxy |
| `token_days`      | `30`                      | how long an issued token lasts             |
| `token_max_days`  | `90`                      | the ceiling, never exceeded               |
| `open_tokens`     | `false`                   | whether a token can be issued without one  |
| `max_upload_mb`   | `16`                      | the largest package accepted; the HTTP body ceiling follows it |

**Keep `data_dir` outside the application directory.** That is the point of it:
installing a new build over the old one cannot take the published modules with
it. `/var/lib/rho-registry` is a good place on a server, and
`RHO_REGISTRY_DATA` overrides the setting, which is what a container uses.

Reading is always public: the index, search, a version list, a download and a
module's documentation need no credential. Publishing always needs a token,
because the registry records who owns a module from the token and an unowned
module is one nobody can add to or withdraw; `access = secured` additionally
requires the publisher to be named in `RHO_REGISTRY_ALLOWED_USERS`, so a
deployment decides who may publish rather than every holder of a token.

### Who may publish

Issuing a token decides who may publish, so issuance is **closed by default**.
A token is minted only when one of these is true:

- the caller presents the **administrator secret**
  (`RHO_REGISTRY_BOOTSTRAP_SECRET`), which mints a token for any subject — the
  root credential, held out of band and compared in constant time;
- the caller holds a valid token and is renewing it for the subject it already
  is;
- the subject is in the **allowlist** (`RHO_REGISTRY_ALLOWED_USERS`, a
  comma-separated list of subjects), which lets those users obtain their own
  token;
- `open_tokens = true`, which a laptop or a test sets and a deployment does not.

So an administrator mints the first token with the secret and either mints the
rest or names the users in the allowlist. Publishing also checks the allowlist
when one is set, so removing a user takes publishing away from a token they
already hold. With no allowlist, the set of publishers is exactly the set the
administrator minted tokens for.

Environment overrides: `RHO_HTTP_ADDR` (default `:8080`), `RHO_REGISTRY_DATA`,
`RHO_REGISTRY_CONFIG`, `RHO_REGISTRY_BOOTSTRAP_SECRET`,
`RHO_REGISTRY_ALLOWED_USERS`, `RHO_REGISTRY_OPEN_TOKENS`.

## Logging

The application logs through `rho-log`, so it depends on that component and asks
the backend for its port:

```yaml
# module.yml
deps:
  rho-log: ">=0.1.0"

# project.yml
targets:
  - id: go-server
    ports:
      - fsstore
      - logemit
```

What it records is the application's own decisions rather than the traffic a
service manager already sees: a publish accepted or refused and why, a download
served or missed, a store that refused a write, and what this instance is on
startup.

```
[2026-09-27T09:25:39.464Z] [info] [registry] starting, data_dir=/tmp/rl3/data access=secured
[2026-09-27T09:25:41.514Z] [info] [registry] published math@1.0.0
[2026-09-27T09:25:41.523Z] [warn] [registry] download missed nope@1.0.0
```

The generated HTTP middleware writes its own request line separately, so the
two do not compete:

```
2026/09/27 12:25:41 POST /modules/math/1.0.0/rho-pkg 182.895µs
```

Records go to standard error, which systemd collects. Setting `RHO_LOG_FILE`
also appends them to a file, so the application's own events survive
independently of whoever captured the process output.

Startup belongs in an `init` function rather than `main`: a server target runs
its own entry point and calls `init`, so `main` would never run.

Data is stored as:

```
<data>/modules/<name>/<version>/package.rho-pkg   uploaded package
<data>/modules/<name>/<version>/meta.json        module metadata
<data>/index/<name>.json                         version list per module
```

Publishing is idempotent per version: a repeat publish is a `409`. Module names
and versions are validated by the store, which refuses any value that could
escape the data directory.

## Trying it locally

```sh
rho run applications/registry
```

In another shell (a publish always needs a token; see *With authentication*):

```sh
printf 'package' > /tmp/m.rho-pkg

curl 'http://localhost:8080/search?q=math'
curl -o /tmp/out.rho-pkg http://localhost:8080/modules/math/0.1.0/rho-pkg
```

With authentication, set the administrator secret and mint a publish token, then
present it. Publishing always needs a token, even on a public registry:

```sh
RHO_REGISTRY_BOOTSTRAP_SECRET=secret rho run applications/registry

TOKEN=$(curl -s -H 'Authorization: Bearer secret' \
  'http://localhost:8080/auth/token?user=miraur&host=localhost&days=1&scopes=push' \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
curl -X POST -H "Authorization: Bearer $TOKEN" --data-binary @/tmp/m.rho-pkg \
     "http://localhost:8080/modules/math/0.1.0/rho-pkg?author=miraur"
```

With `access = secured` in the settings file, the same token is required to
publish, and the publisher must also be named in `RHO_REGISTRY_ALLOWED_USERS`.
Reads stay public.

## Pointing the Rho client at it

```sh
rho config set remote.url   http://localhost:8080
# The value is a token minted from POST /auth/token, not the administrator
# secret; see *With authentication*.
rho config set remote.token "$TOKEN"
rho module search math
```

## Deployment

The generated project is a static Go binary, so deployment is whatever suits the
host. See [docs/deployment.md](docs/deployment.md) for the options and the
recommended default.
