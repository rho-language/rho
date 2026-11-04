# Build and serve the Rho documentation site.
#
# The generator and the docsite service run in a builder stage with the full
# Rho toolchain. The service is crawled once into static files, so the runtime
# carries only the same pages, menu and layout the live service serves, without
# a compiler, source tree or cache.

FROM golang:1.26.10-bookworm AS build

RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ca-certificates wget \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY . .

# The core toolchain and the components the service needs.
RUN CGO_ENABLED=0 go build -trimpath -o ./bin/rho     ./cmd/rho \
 && CGO_ENABLED=0 go build -trimpath -o ./bin/rho-deps ./cmd/rho-deps \
 && HOME=/root ./bin/rho install --force \
 && HOME=/root ./scripts/check.sh publish rho-fmt rho-log rho-doc

# Run the real docsite service and mirror its pages, so the static site keeps
# the landing page, menu, release notes and generated documentation exactly as
# the service renders them.
RUN rm -rf applications/docsite/build /out /site \
 && mkdir -p /out \
 && export HOME=/root PATH=/root/.rho/bin:$PATH RHO_HTTP_ADDR=:8080 \
      RHO_DOCS_ROOT=/src RHO_DOCS_TREE=/src/docs \
      RHO_SITE_RELEASES=/src/CHANGELOG.md \
      RHO_SITE_TITLE="Rho language and toolchain" \
      RHO_SITE_HOME=/ \
      RHO_SITE_REPOSITORY=https://github.com/rho-language/rho \
      GOPATH=/root/go GOMODCACHE=/root/go/pkg/mod GOCACHE=/tmp/go-build \
 && (./bin/rho run applications/docsite >/tmp/docsite.log 2>&1 & echo $! >/tmp/docsite.pid) \
 && for i in $(seq 1 90); do \
        if wget -q -O /dev/null http://127.0.0.1:8080/; then break; fi; \
        sleep 1; \
    done \
 && wget --recursive --level=inf --no-parent --domains=127.0.0.1 \
      --convert-links --adjust-extension --page-requisites \
      --no-host-directories --directory-prefix=/out \
      http://127.0.0.1:8080/ \
 && kill "$(cat /tmp/docsite.pid)" 2>/dev/null || true \
 && cp -a /out /site

FROM python:3.12-slim

WORKDIR /app
COPY --from=build /site /app/site

EXPOSE 8080
CMD ["python3", "-m", "http.server", "8080", "--directory", "/app/site"]
