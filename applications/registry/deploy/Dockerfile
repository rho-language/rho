# Build and run the Rho module registry.
#
# The image carries the Rho toolchain and the Rho sources, and starts the
# service through `rho run`, so the container runs the service the same way a
# developer does locally: rho generates the Go project, then Go builds it.
#
# That means the final image needs a Go toolchain as well as rho. A distroless
# image would be smaller but could not run the build, so this uses a slim
# Debian base.

# Build and install the Rho toolchain, then run the registry.
#
# The image carries a properly installed rho: the tool home, its binaries, and
# the registry, cache and config layout it expects. That is what `rho install`
# produces, run here exactly as a developer would run it.
#
# The service starts through `rho run`, so the container generates the Go
# project from the Rho sources and runs it, and the image needs a Go toolchain
# for that build.

FROM golang:1.22-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --create-home --home-dir /home/rho --shell /usr/sbin/nologin rho

# The repository, which is what the toolchain is built from and what the
# service is compiled against.
WORKDIR /src
COPY . .

# Build the core toolchain, then install it. The formatter, the test runner and
# the documentation generator are components rather than core tools, so they are
# built and published below instead of being compiled into the CLI.
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho     ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps ./cmd/rho-deps \
 && HOME=/home/rho ./bin/rho install --force

# Build the components this service depends on and publish them into the tool
# home. Each of them is something the registry would be a lesser thing without:
#
#   rho-fmt, rho-log   the formatter the compiler uses and the logger the
#                      service writes its own records with;
#   rho-router         reads a Rho service's routes, so the registry can
#                      describe its own API and a held module's;
#   rho-openapi        writes that description as an OpenAPI 3.1 document, which
#                      the openapiemit port embeds at build time;
#   rho-doc            documents a published module, and is what renders the
#                      reference the API plugin below adds to;
#   rho-doc-api        a plugin for rho-doc that puts a served module's HTTP API
#                      into its documentation, so a published module arrives
#                      with its API described.
#
# A registry that could not reach a generator would hold modules it cannot
# document, and one that could not write a description would serve an API it
# does not describe.
# -buildvcs=false because the image carries sources without the history, which
# is what a copied tree is; the go.work file is what lets a component resolve
# the toolchain it is built against from the tree rather than from a network.
RUN cd /src/modules/components/rho-fmt   && go build -buildvcs=false -o rho-fmt   ./src \
 && cd /src/modules/components/rho-log   && go build -buildvcs=false -o rho-log   ./src \
 && cd /src/modules/components/rho-router && go build -buildvcs=false -o rho-router ./src \
 && cd /src/modules/components/rho-openapi && go build -buildvcs=false -o rho-openapi ./src \
 && cd /src/modules/components/rho-doc   && go build -buildvcs=false -o rho-doc   ./src \
 && cd /src/modules/plugins/rho-doc-api  && go build -buildvcs=false -o rho-doc-api ./src \
 && cd /src \
 && HOME=/home/rho ./bin/rho module publish modules/components/rho-fmt \
 && HOME=/home/rho ./bin/rho module publish modules/components/rho-log \
 && HOME=/home/rho ./bin/rho module publish modules/components/rho-router \
 && HOME=/home/rho ./bin/rho module publish modules/components/rho-openapi \
 && HOME=/home/rho ./bin/rho module publish modules/components/rho-doc \
 && HOME=/home/rho ./bin/rho module publish modules/plugins/rho-doc-api

# The runtime tree: an installed toolchain plus the sources rho compiles at
# start-up, being the standard library and the service.
RUN mkdir -p /app/applications/registry/data \
 && cp -a /src/std /app/std \
 && cp -a /src/applications/registry/. /app/applications/registry/ \
 && rm -rf /app/applications/registry/build \
 && chown -R rho:rho /app /home/rho

USER rho
WORKDIR /app

# rho install puts the binaries in the tool home and adds that to PATH
# through the shell profile, which a container never sources, so PATH is set
# here as well.
# The address the service is published under, which is what the documentation it
# generates leads back to. Override it when the service is behind a name.
ENV HOME=/home/rho \
    PATH=/home/rho/.rho/bin:$PATH \
    RHO_HTTP_ADDR=:8080 \
    RHO_REGISTRY_DATA=/app/applications/registry/data \
    RHO_REGISTRY_URL=http://localhost:8080 \
    GOFLAGS=-buildvcs=false \
    GOCACHE=/tmp/go-build

VOLUME ["/app/applications/registry/data"]
EXPOSE 8080

# The service is started by rho: it generates the Go project from the Rho
# sources and then runs it.
ENTRYPOINT ["rho", "run", "applications/registry"]
