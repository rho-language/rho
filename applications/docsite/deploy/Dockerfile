# Build and run the Rho docsite service.
#
# The image carries a properly installed rho, and starts the service through
# `rho run`, exactly as the registry image does: the container generates the Go
# project from the Rho sources and runs it, so it needs a Go toolchain for that
# build. What it serves comes from the repository's own docs/ tree and
# CHANGELOG.md, copied in below.

FROM golang:1.23-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --create-home --home-dir /home/rho --shell /usr/sbin/nologin rho

WORKDIR /src
COPY . .

# The core toolchain.
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho     ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps ./cmd/rho-deps \
 && HOME=/home/rho ./bin/rho install --force

# The components the service needs at build and run time:
#
#   rho-log   the logger the service writes its records with;
#   rho-fmt   the formatter the compiler uses;
#   rho-doc   the generator a page is rendered by — the service calls it per
#             page, so it must be present in the running image.
RUN HOME=/home/rho ./scripts/check.sh publish rho-fmt rho-log rho-doc

# The tree the site is built from. The whole repository is copied, because a
# documentation build reads a repository — the docs and the modules they
# document — so a tree with only docs/ would generate a site with no pages in it.
RUN mkdir -p /app \
 && cp -a /src/. /app/ \
 && rm -rf /app/applications/docsite/build \
 && chown -R rho:rho /app /home/rho

USER rho
WORKDIR /app

# A Go home the service can write: `rho run` compiles the generated project at
# start-up, and a module cache it cannot write makes that fail.
#
# RHO_DOCS_ROOT is the project the site is built from and RHO_DOCS_TREE the
# documentation directory the menu lists; RHO_SITE_HOME is where the built pages
# link back to, and RHO_SITE_RELEASES the changelog the overview reads.
ENV HOME=/home/rho \
    PATH=/home/rho/.rho/bin:$PATH \
    RHO_HTTP_ADDR=:8080 \
    RHO_DOCS_ROOT=/app \
    RHO_DOCS_TREE=/app/docs \
    RHO_SITE_RELEASES=/app/CHANGELOG.md \
    RHO_SITE_TITLE="Rho language and toolchain" \
    RHO_SITE_HOME=/ \
    RHO_SITE_REPOSITORY=https://github.com/rho-language/rho \
    GOFLAGS=-buildvcs=false \
    GOPATH=/home/rho/go \
    GOMODCACHE=/home/rho/go/pkg/mod \
    GOCACHE=/tmp/go-build

# Build once as the service user so the module cache is populated and owned by
# the user that rebuilds at start-up; the output is discarded.
RUN rho build applications/docsite \
 && rm -rf /app/applications/docsite/build

EXPOSE 8080

# Started by rho: it generates the Go project from the Rho sources and runs it.
ENTRYPOINT ["rho", "run", "applications/docsite"]
