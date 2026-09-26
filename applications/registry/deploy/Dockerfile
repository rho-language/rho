# Build and run the Rho module registry.
#
# The image carries the Rho toolchain and the Rho sources, and starts the
# service through `rho run`, so the container runs the service the same way a
# developer does locally: rho generates the Go project, then Go builds it.
#
# That means the final image needs a Go toolchain as well as rho. A distroless
# image would be smaller but could not run the build, so this uses a slim
# Debian base.

# --- Stage 1: build the Rho toolchain -----------------------------------------
FROM golang:1.22 AS rho-build
WORKDIR /src
# The Rho CLI links a C toolchain for its native target.
RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc \
 && rm -rf /var/lib/apt/lists/*
COPY go.mod ./
COPY cmd ./cmd
COPY src ./src
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/rho ./cmd/rho

# --- Stage 2: the service ------------------------------------------------------
# The Go toolchain builds the project rho generates at start-up.
FROM golang:1.22-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --create-home --home-dir /home/rho --shell /usr/sbin/nologin rho

COPY --from=rho-build /out/rho /usr/local/bin/rho

# The sources rho compiles at run time: the standard library it resolves
# imports against, and the service itself.
WORKDIR /app
COPY --chown=rho:rho std ./std
COPY --chown=rho:rho applications/registry ./applications/registry
COPY --chown=rho:rho examples ./examples

# Generated projects and the module store live under the service directory.
RUN mkdir -p /app/applications/registry/data \
 && chown -R rho:rho /app/applications/registry

USER rho
WORKDIR /app

ENV RHO_HTTP_ADDR=:8080 \
    RHO_REGISTRY_DATA=/app/applications/registry/data \
    GOFLAGS=-buildvcs=false \
    GOCACHE=/tmp/go-build

VOLUME ["/app/applications/registry/data"]
EXPOSE 8080

# The service is started by rho: it generates the Go project from the Rho
# sources and then runs it.
ENTRYPOINT ["rho", "run", "applications/registry/src/main.rho"]
