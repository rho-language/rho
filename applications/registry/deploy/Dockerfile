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

# Build the tools, then install them.
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho      ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-fmt   ./cmd/rho-fmt \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-test  ./cmd/rho-test \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps  ./cmd/rho-deps \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-doc   ./cmd/rho-doc \
 && HOME=/home/rho ./bin/rho install --force

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
ENV HOME=/home/rho \
    PATH=/home/rho/.rho/bin:$PATH \
    RHO_HTTP_ADDR=:8080 \
    RHO_REGISTRY_DATA=/app/applications/registry/data \
    GOFLAGS=-buildvcs=false \
    GOCACHE=/tmp/go-build

VOLUME ["/app/applications/registry/data"]
EXPOSE 8080

# The service is started by rho: it generates the Go project from the Rho
# sources and then runs it.
ENTRYPOINT ["rho", "run", "applications/registry"]
