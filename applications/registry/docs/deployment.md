# Deploying the Rho module registry

The registry generates to a single static Go binary with no runtime
dependencies beyond libc, so every usual deployment method works. This note
compares them and records the recommended default.

## What you are deploying

```
applications/registry/src/*.rho      the service, in Rho
        | rho build
        v
applications/registry/build/generated/go-server/
        | go build
        v
registry                              one static binary
        + a data directory            uploaded module packages
```

`rho build` performs both steps, so a build host needs only the Rho toolchain
and a Go toolchain. `rho run` does the same and then executes the result, which
is what the container image uses.

State is only the data directory, so the service is stateless to deploy: any
instance started against the same directory sees the same modules. That also
means exactly one instance may write to a given directory, unless you put a
real store behind the port later.

**A prebuilt binary is usually the better runtime artifact.** Building on the
deploy host means the host needs both toolchains, and a build failure becomes a
startup failure. The systemd option below builds on the host because that is
simplest for a single box; if you would rather not, build elsewhere with
`rho build` and ship the binary (see option 4). The container builds in the
image, where the toolchains are part of the image.

## Options

### 1. systemd on the host (recommended default)

A single binary and a single directory is exactly what a systemd unit is for.
No container runtime, no reverse proxy required unless you want one.

`deploy/registry.service`:

```ini
[Unit]
Description=Rho module registry
After=network.target

[Service]
Type=simple
User=rho
Group=rho
Environment=RHO_HTTP_ADDR=127.0.0.1:8080
Environment=RHO_REGISTRY_DATA=/var/lib/rho-registry
EnvironmentFile=-/etc/rho/registry.env
ExecStart=/usr/local/bin/registry
Restart=on-failure
RestartSec=2
# The service only needs its own state directory.
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/rho-registry

[Install]
WantedBy=multi-user.target
```

Build the binary with the Rho toolchain, then install it:

```sh
rho build applications/registry
# or, if the sources are on the host, install rho and let the unit build:
#   rho install --force

sudo useradd --system --home /var/lib/rho-registry --shell /usr/sbin/nologin rho
sudo install -d -o rho -g rho /var/lib/rho-registry
sudo install -m 0755 applications/registry/build/registry /usr/local/bin/registry
sudo install -m 0644 applications/registry/deploy/registry.service /etc/systemd/system/registry.service
sudo systemctl enable --now registry
```

**Pros** smallest moving parts, one `journalctl` for logs, trivial upgrades.
**Cons** the Go toolchain must be on the host to build.

### 2. Docker (recommended when the deploy host should not build)

The image carries the Rho toolchain, the Rho sources and a Go toolchain, and
starts the service through `rho run`, so the container runs the service exactly
as a developer does locally: rho generates the Go project, Go builds it.

`deploy/Dockerfile` does that in two stages: rho is built in the first, and the
runtime stage keeps the sources plus a Go toolchain because the generated
project is compiled at start-up.

```sh
docker build -f applications/registry/deploy/Dockerfile -t rho-registry .
docker run -d --name registry -p 8080:8080 \
  -v registry-data:/app/applications/registry/data \
  -e RHO_REGISTRY_TOKEN=secret rho-registry
```

**Pros** reproducible from the repository alone, no host toolchains, and the
deployed process is `rho run`, matching development.
**Cons** a larger image, because a Go toolchain has to be present to build the
generated project at start-up.

If you would rather ship only the binary, build it in a first stage and copy
just the result into a small runtime image; that is the "prebuilt" variant of
this option.

### 3. nginx in front

nginx is orthogonal: it belongs in front of any of the options above. It is
worth adding when you want TLS, a public hostname, or upload size limits.

`deploy/nginx.conf`:

```nginx
server {
    listen 443 ssl http2;
    server_name registry.example.com;

    ssl_certificate     /etc/letsencrypt/live/registry.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/registry.example.com/privkey.pem;

    # A published package can be large; the service enforces its own limit too.
    client_max_body_size 256m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }
}
```

Note the service binds `127.0.0.1` when nginx fronts it, so the port is not
public.

### 4. rsync / copy a release

The simplest option, and fine for a single internal host: build elsewhere,
copy the binary, restart.

```sh
tar -czf registry.tar.gz -C build/generated/go-server registry
rsync -av registry.tar.gz deploy@host:/tmp/
ssh deploy@host 'sudo systemctl restart registry'
```

**Pros** nothing to install on the host.
**Cons** no build reproducibility on the host, so keep the artefact.

## Recommendation

Start with **systemd**, optionally with **nginx** in front for TLS: one binary,
one directory, no runtime. Move to **Docker** when the deploy host should not
build anything, because the image reproduces the build from the repository
alone and starts the service with `rho run`.

Either way, the data directory is the thing to back up and to keep on one
writer.

Whichever you pick, the data directory is the thing to back up and to keep on
one writer.

## Authentication

`RHO_REGISTRY_TOKEN` turns publishing into an authenticated operation; with it
unset the registry is open, which suits local development only. Set it in
`/etc/rho/registry.env` with `0600` permissions:

```
RHO_REGISTRY_TOKEN=<long random string>
```

The client reads the same value from `~/.rho/config/registry.yaml` under
`remote.token`.
