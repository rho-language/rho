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

## Configuration

| Variable              | Default   | Purpose                                   |
|-----------------------|-----------|-------------------------------------------|
| `RHO_HTTP_ADDR`       | `:8080`   | Listen address                            |
| `RHO_REGISTRY_DATA`   | `./data`  | Where packages are stored                 |
| `RHO_REGISTRY_TOKEN`  | unset     | Bearer token required to publish; open if unset |

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
