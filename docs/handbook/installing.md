# Installing Rho

Rho is a compiler and a toolchain. Installing it means building the compiler
once and putting the result somewhere the shell will find it.

## What you need

| | Why | Needed for |
|---|---|---|
| **Go 1.18 or newer** | Rho compiles to Go, and the toolchain is written in it | everything |
| The repository | `rho install` builds what is checked out | installing |
| `git` | cloning a dependency that declares a `git:` source | only those dependencies |
| `gcc` | building a `c` target, and running `rho test` | C projects |

Go 1.22 is what the tree's own `go.mod` asks for and what CI builds with, but
the compiler builds and runs on 1.18. The one thing that needs 1.22 is a
generated project that routes through the standard library's `http.ServeMux`, which
gained method-and-wildcard patterns in that release — so a project that
declares `router: serveMux` needs 1.22, and one that does not does not.

`rho install` reports what is missing rather than failing on it later. A
missing `go` is an error at the step that needs it; a missing `gcc` or `git` is
reported as not needed for anything you declared.

## Installing

```sh
git clone <this repository>
cd rho
go build -o ./bin/rho ./cmd/rho
./bin/rho install
```

That:

1. creates `~/.rho` and its directories;
2. builds `rho` and `rho-deps` from the checkout;
3. copies them into `~/.rho/bin`;
4. creates the local registry and the cache;
5. adds `~/.rho/bin` to the PATH in your shell profile.

Put `~/.rho/bin` on your PATH yourself if you would rather:

```sh
export PATH="$HOME/.rho/bin:$PATH"
```

## Reinstalling

```sh
./bin/rho install --force
```

`--force` replaces the two binaries and leaves everything else alone: the
installed modules, the registry index, the credentials, the cache and the
configuration are yours, and reinstalling does not take them.

## After installing

`rho install` publishes no components. A fresh installation can build, run and
document, but `rho fmt`, `rho doc` and the rest do not exist until something
publishes them, because a component is what makes a command reachable:

```sh
rho help                    # what can be reached right now
rho module publish <dir>    # make a component reachable, and install it here
```

## Using it from a container

`applications/registry/deploy/Dockerfile` is a working end-to-end install and
the shortest description of what one takes:

```dockerfile
FROM golang:1.22-bookworm
RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY . /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps ./cmd/rho-deps \
 && HOME=/home/rho ./bin/rho install --force
ENV PATH=/home/rho/.rho/bin:$PATH
```

Two things there are worth knowing outside a container:

- **`HOME` is set for the install.** `rho install` writes to the home directory
  it is given, so a user the image creates needs `HOME` pointing at it.
- **The component builds use `-buildvcs=false`.** An image carries sources
  without the history, which is what a copied tree is, and the workspace file is
  what lets a component resolve the toolchain from the tree rather than the
  network.

## What needs a network

Almost nothing. The toolchain is built from the checkout, and a component built
inside it resolves its dependencies from the same tree.

A network is needed to:

- `rho auth login` — the registry issues the token;
- `rho module publish` — the upload;
- `rho module install <name>` and `rho module download` — the fetch;
- a dependency that declares a `git:` source;
- building a generated project that uses a router adapter other than the
  standard library, which is a third-party module.

A project built for the host, or for a web service using the standard library's
`ServeMux`, needs none of these: `go build` in the generated directory resolves
nothing outside it.
