# Rho Module Registry

A remote registry for Rho modules, written in Rho and compiled to a Go HTTP
service. It speaks the three endpoints the Rho module client already uses, so
`rho module publish`, `download` and `search` work against it.

## Endpoints

| Method | Path                                | Purpose                     |
|--------|-------------------------------------|-----------------------------|
| GET    | `/healthz`                          | Liveness probe              |
| POST   | `/modules/{name}/{version}/rho-pkg` | Publish a module package    |
| GET    | `/modules/{name}/{version}/rho-pkg` | Download a module package   |
| GET    | `/search?q=<query>`                 | Search published modules    |
| GET    | `/modules/{name}`                   | List a module's versions    |

## Layout

```
applications/registry/
├── module.yml                     build configuration, server target only
├── project.yml                    descriptor: the generated port and targets
├── src/
│   ├── main.rho                   routes and handlers
│   └── registry/
│       ├── storage.rho            the storage port, as extern declarations
│       ├── validate.rho           request validation and auth
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
validation, bearer-token parsing, and the response shapes. They run on the
`go` target, which the descriptor defines for exactly this purpose.

```sh
rho test applications/registry go
# or, from inside the project:
cd applications/registry && rho test go
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

All public, no credential needed:

| Path                          | What it shows                                        |
|-------------------------------|------------------------------------------------------|
| `/`                           | every module, with its newest version badged         |
| `/search?q=`                  | matching modules, one row per version                |
| `/modules/{name}`             | one module's versions, newest first                  |
| `/modules/{name}/{version}`   | that version's public metadata and its download path |

Every page carries a footer with the registry version from its manifest, the
year, and a link to rho-lang.org.

The machine surface is kept apart under `/api`, so a client and a person get
what each expects from the same service:

| Path                          | What it returns                        |
|-------------------------------|----------------------------------------|
| `/api/search?q=`              | JSON array of module metadata          |
| `/api/modules/{name}`         | JSON with the published version list   |
| `/modules/{name}/{v}/rho-pkg` | the uploaded package, verbatim        |

## Configuration

Settings are read at start-up from `RHO_REGISTRY_CONFIG`, or
`~/.rho/registry/registry.conf`, as `key = value` lines. See
[registry.conf.example](registry.conf.example); every setting has a default, so
the service runs with no settings file at all.

| Setting           | Default                   | Purpose                                    |
|-------------------|---------------------------|--------------------------------------------|
| `data_dir`        | `data`                    | where modules are stored                   |
| `access`          | `public`                  | `public`, or `secured` to require a token to publish |
| `title`           | `Rho module registry`    | the name on the pages                      |
| `site_url`        | `https://rho-lang.org`   | the footer link                            |
| `token_days`      | `30`                      | how long an issued token lasts             |
| `token_max_days`  | `90`                      | the ceiling, never exceeded               |
| `open_tokens`     | `true`                    | whether a token can be issued without one  |
| `max_upload_mb`   | `256`                     | the largest package accepted               |

**Keep `data_dir` outside the application directory.** That is the point of it:
installing a new build over the old one cannot take the published modules with
it. `/var/lib/rho-registry` is a good place on a server, and
`RHO_REGISTRY_DATA` overrides the setting, which is what a container uses.

Reading is always public. `access = secured` turns on rho-auth for publishing
only, so the index, search and downloads stay open.

Environment overrides: `RHO_HTTP_ADDR` (default `:8080`), `RHO_REGISTRY_DATA`,
`RHO_REGISTRY_CONFIG`.

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

In another shell:

```sh
printf 'package' > /tmp/m.rho-pkg
curl -X POST --data-binary @/tmp/m.rho-pkg \
     http://localhost:8080/modules/math/0.1.0/rho-pkg

curl 'http://localhost:8080/search?q=math'
curl -o /tmp/out.rho-pkg http://localhost:8080/modules/math/0.1.0/rho-pkg
```

With authentication:

```sh
RHO_REGISTRY_TOKEN=secret rho run applications/registry
curl -X POST -H 'Authorization: Bearer secret' --data-binary @/tmp/m.rho-pkg \
     http://localhost:8080/modules/math/0.1.0/rho-pkg
```

## Pointing the Rho client at it

```sh
rho config set remote.url      http://localhost:8080
rho config set remote.token    secret
rho module search math
```

## Deployment

The generated project is a static Go binary, so deployment is whatever suits the
host. See [docs/deployment.md](docs/deployment.md) for the options and the
recommended default.
