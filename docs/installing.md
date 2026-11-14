# Installing Rho

Rho is built from source. Installing it means building the `rho` CLI from a
checkout and running `rho install`, which builds the rest of the core toolchain
and puts it under the Rho home, `~/.rho` (or `$RHO_HOME` when that is set).

## What you need

| | Why | Needed for |
|---|---|---|
| **Go 1.26.10 or newer** | The toolchain is written in Go, and Rho compiles to Go | everything |
| A checkout of the repository | `rho install` builds what is checked out | installing |
| A C compiler (`gcc`, and `cc` for tests) | `rho run` compiles a single file or a C project with `gcc`; `rho test` with `cc` | C targets, `rho run file.rho`, `rho test` |
| CMake | Building a generated C project | C targets |
| `git` | Fetching a dependency that declares a `git:` source | only those dependencies |

Go 1.26.10 is what the tree's `go.mod` asks for and what CI builds with. The
`go` line names a minimum, so an older Go refuses the tree rather than building it
against something it was not written for; the `toolchain` line names the release
the tree is tested with, and a newer `go` that finds itself older than that
downloads it. A generated Go project carries its own `go.mod` asking for the same
release.

## Installing

```sh
git clone https://github.com/rho-language/rho
cd rho
go build -o ./bin/rho ./cmd/rho
./bin/rho install
```

`rho install` must be run from the root of the checkout: it reads the standard
library, the documentation and the sources it builds from the working directory.
In order, it:

1. creates the Rho home and its directories (`bin`, `modules`, `registry`,
   `cache`, `config`, `auth`, `pkg`);
2. copies the standard library (`std/`) and these pages (`docs/`) into it;
3. builds `rho`, `rho-deps` and the language server `rho-lsp` into `./bin`;
4. copies them into `~/.rho/bin`;
5. registers `rho-lsp` as a component and enables it, so `rho lsp` works at once;
6. creates the local registry, the package cache and the configuration;
7. adds `~/.rho/bin` to the `PATH` in your shell's start-up file (`.bashrc`,
   `.zshrc`, fish's `config.fish`, or `.profile`).

Open a new shell, or put the directory on your `PATH` yourself:

```sh
export PATH="$HOME/.rho/bin:$PATH"
rho version
```

Every command except `help`, `version`, `init`, `new` and `install` needs this
installation, because it reads the local registry. Run before it, they say so
and stop.

## Components after a first install

A first install has exactly one component, `rho-lsp`. The others — `rho test`,
`rho doc`, `rho fmt`, `rho target` and the rest — are separate modules, and a
command does not exist until its component is installed **and** enabled. `rho
help` lists what this machine can reach now.

From a checkout, the shortest way to have all of them is the repository's own
script, which builds every component and plugin in the tree, publishes each into
the local registry at the version its manifest declares, enables it, and then
runs the tests:

```sh
./scripts/check.sh                          # everything, then the test suite
./scripts/check.sh publish rho-test rho-doc # only these, then stop
```

One at a time, by hand, a component's binary has to be built first, because the
manifest's `main:` names a binary in the module's own root:

```sh
cd modules/components/rho-test && go build -o rho-test ./src && cd -
rho module publish --local modules/components/rho-test
rho module enable rho-test
```

`rho module publish` also uploads to the remote registry when one is configured
and you are logged in to it; `--local` installs here and uploads nothing. The
script publishes without `--local`, so on a machine that is logged in it uploads
too.

Without a checkout, a component comes from the remote registry:

```sh
rho module install rho-test     # fetch and install it
rho module enable rho-test      # turn it on
```

Installing and enabling are separate on purpose: a module can be on a machine
because something depended on it, and running it is a decision somebody makes.

**A C build needs `rho-target`.** `rho build` hands a generated C project to the
`rho-target` component, which knows the compilers and boards; without it the
build stops with `this build needs the rho-target component`. A project that
declares a C connection also needs `rho-cimport`, and one with a `config:`
section needs `rho-kconfig`. A Go or Go-service target needs none of them.

## Updating and reinstalling

```sh
./bin/rho install --update     # rebuild the toolchain from the checkout, in place
./bin/rho install --force      # remove the toolchain, then install it again
```

`--update` builds first and replaces second, so a build that fails leaves the
installed toolchain as it was. It refreshes the binaries in `~/.rho/bin`, the
standard library and the documentation, registers `rho-lsp` if it is not
registered yet, and touches nothing else: not the registry, the installed
modules, the cache, the credentials or the configuration. On a machine with
nothing installed it performs a full install.

`--force` removes these directories under the Rho home, and only these:

```
bin  cache  docs  modules  pkg  plugins  registry  std  targets
```

and then installs again. Every installed component and plugin goes with
`modules` and `registry`, so they have to be published or installed again
afterwards. What is yours or your applications' stays: the credentials and
signing keys (`auth/`), the configuration (`config/`), any application state
kept under the Rho home, and the fetched cross toolchains (`toolchains/`).

## Using it from a container

`applications/registry/deploy/Dockerfile` is a working install from scratch, and
the shortest description of what one takes:

```dockerfile
FROM golang:1.26.10-bookworm
RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho     ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps ./cmd/rho-deps \
 && HOME=/home/rho ./bin/rho install --force
RUN HOME=/home/rho ./scripts/check.sh publish rho-fmt rho-log rho-doc
ENV PATH=/home/rho/.rho/bin:$PATH GOFLAGS=-buildvcs=false
```

Two things there matter outside a container too:

- **`HOME` is set for the install.** `rho install` writes under the home it is
  given, so a user the image runs as needs `HOME` pointing at that user's home.
- **`GOFLAGS=-buildvcs=false`.** An image carries the sources without the git
  history, and `go build` would otherwise stop trying to stamp version control
  information it cannot find.

## What needs a network

Very little. The toolchain is built from the checkout, and a component built
inside it resolves its Go dependencies from the same tree.

A network is needed for:

- `rho auth login` — the registry issues the token;
- `rho module publish` without `--local`, when you are logged in — the upload;
- `rho module install <name>` and `rho module download` — the fetch;
- a dependency that declares a `git:` source;
- a generated service that uses a third-party router rather than the standard
  library's `ServeMux`.

A project built for the host, or a web service on the standard library's router,
needs none of these: `go build` in its generated directory resolves nothing
outside it.
