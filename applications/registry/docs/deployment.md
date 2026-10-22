# Deploying the Rho module registry

The registry generates to a single Go binary, but it is not a fully standalone
artifact: at start-up it generates and builds the Go project from its Rho
sources, and at every publish it calls `rho doc` to document the module it just
received. **The host that runs it therefore needs the Rho toolchain and the
`rho-doc` component, even when the binary was built elsewhere.** That is the one
fact that decides most of what follows.

## What you are deploying

```
applications/registry/src/*.rho      the service, in Rho
        | rho build
        v
applications/registry/build/generated/go-server/
        | go build
        v
applications/registry/build/registry   one static Go binary
        + the Rho toolchain            rho + rho-doc at run time
        + a data directory             uploaded module packages and records
```

`rho build` performs both steps, so a build host needs the Rho toolchain and a
Go toolchain. At run time the service shells out to `rho doc`, so the run host
needs the toolchain too. State is only the data directory, so the service is
stateless to deploy: any instance started against the same directory sees the
same modules, and exactly one instance may write to a given directory (unless a
real store is put behind the port later).

## Prerequisites

On Ubuntu 22.04 or 24.04:

```sh
sudo apt-get update
sudo apt-get install -y git ca-certificates build-essential nginx
# Go 1.26.10, which is what the modules declare
sudo snap install go --classic      # or install the tarball from go.dev/dl
```

The Rho toolchain has no distribution package yet, so it is built from source.
It is installed into the service user's home under `/opt` — not `/home`, because
the systemd unit hides `/home` and the toolchain has to stay reachable. `scripts/check.sh`
builds every component, publishes it into the local registry, installs the
toolchain and runs the tests:

```sh
sudo useradd --system --create-home --home-dir /opt/rho --shell /usr/sbin/nologin rho
git clone <rho-repository-url> /opt/rho-src
sudo -u rho env HOME=/opt/rho RHO_HOME=/opt/rho/.rho \
  PATH=/snap/bin:/usr/local/go/bin:/usr/bin:/bin \
  bash -lc 'cd /opt/rho-src && ./scripts/check.sh'
```

After that the toolchain is `/opt/rho/.rho/bin/rho` with the components installed
beside it. The registry needs `rho-fmt`, `rho-log`, `rho-sql`, `rho-sql-sqlite`,
`rho-router`, `rho-openapi`, `rho-doc` and `rho-doc-api`; `./scripts/check.sh` publishes
all of them. `rho-fmt`, `rho-log` and `rho-doc` are needed at *run* time (the
service calls `rho doc` on every publish), the rest at build time.

## Build

```sh
sudo -u rho env HOME=/opt/rho RHO_HOME=/opt/rho/.rho \
  PATH=/opt/rho/.rho/bin:/snap/bin:/usr/bin:/bin \
  bash -lc 'cd /opt/rho-src && rho build applications/registry'
# /opt/rho-src/applications/registry/build/registry
```

## Option 1 — systemd on the host (recommended default)

A single binary, a single data directory and the toolchain already installed is
exactly what a systemd unit is for. No container runtime, and nginx is optional
until TLS or a public name is wanted.

```sh
# The data directory and the settings directory
sudo install -d -o rho -g rho /var/lib/rho-registry
sudo install -d -o root -g rho -m 0750 /etc/rho

# The built binary
sudo install -m 0755 /opt/rho-src/applications/registry/build/registry /usr/local/bin/registry

# Settings and the unit
sudo install -m 0640 /opt/rho-src/applications/registry/registry.conf.example /etc/rho/registry.conf
sudo install -m 0644 /opt/rho-src/applications/registry/deploy/registry.service /etc/systemd/system/registry.service
```

The unit points `RHO_HOME` at `/opt/rho/.rho`, so the service reaches the
toolchain the same way the `rho` user does. If you installed the toolchain
somewhere else, change those three `Environment=` lines to match.

Create the secret file with mode 0600. `RHO_REGISTRY_BOOTSTRAP_SECRET` is the
root credential that mints the first token; `RHO_REGISTRY_ALLOWED_USERS` names
the users who may obtain their own:

```sh
sudo tee /etc/rho/registry.env >/dev/null <<'EOF'
RHO_REGISTRY_BOOTSTRAP_SECRET=<openssl rand -hex 32>
RHO_REGISTRY_ALLOWED_USERS=alice,bob
EOF
sudo chmod 0600 /etc/rho/registry.env
```

Bring it up and check it:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now registry
systemctl status registry
curl -fsS http://127.0.0.1:8080/healthz
journalctl -u registry -f
```

The unit is already written for this: it binds loopback, points the service at
the toolchain in `/opt/rho`, keeps the data and settings outside the binary, and
locks the process down with `NoNewPrivileges`, `ProtectSystem=strict`,
`ProtectHome=yes` and the two paths it is allowed to write.

**Pros** smallest moving parts, one `journalctl`, trivial upgrades.
**Cons** the Rho toolchain must be on the host (Go only to build).

## Option 2 — Docker

The image carries the Rho toolchain, the Rho sources and a Go toolchain, and
starts the service through `rho run`, so the container runs the service exactly
as a developer does: rho generates the Go project, Go builds it.

```sh
docker build -f applications/registry/deploy/Dockerfile -t rho-registry .
docker run -d --name registry -p 127.0.0.1:8080:8080 \
  -v registry-data:/app/applications/registry/data \
  -e RHO_REGISTRY_BOOTSTRAP_SECRET=<long random string> \
  -e RHO_REGISTRY_ALLOWED_USERS=alice,bob \
  rho-registry
```

**Pros** reproducible from the repository alone, no host toolchains.
**Cons** a larger image, because a Go toolchain builds the generated project at
start-up.

## Option 3 — nginx in front

nginx is orthogonal and belongs in front of any of the options above. It is worth
adding for TLS, a public hostname, or an upload-size limit. `deploy/nginx.conf`
is a ready server block:

```nginx
server {
    listen 443 ssl http2;
    server_name registry.example.com;

    ssl_certificate     /etc/letsencrypt/live/registry.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/registry.example.com/privkey.pem;

    # At least the registry's max_upload_mb, or nginx refuses an upload the
    # service would have accepted.
    client_max_body_size 16m;

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

`X-Real-IP` is what the service counts rate limits against, and the service only
trusts it because a proxy sets it; run the service loopback-only (`RHO_HTTP_ADDR`
in the unit) so nothing can reach it to forge the header.

TLS with certbot:

```sh
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d registry.example.com
```

## Option 4 — Ubuntu with ISPConfig and nginx

ISPConfig owns the vhost, its TLS certificate and its DNS record, so the job is
to add a reverse-proxy location to an ISPConfig site. The service itself is
deployed exactly as in Option 1 (systemd, loopback-only).

1. **Deploy the service first** (Option 1) and confirm
   `curl http://127.0.0.1:8080/healthz` answers on the server.

2. **DNS.** In the DNS zone for `registry.example.com`, point an `A`/`AAAA`
   record at the server, or add the domain to the ISPConfig DNS module if
   ISPConfig is the nameserver. Wait for it to resolve.

3. **Create the site.** In ISPConfig: *Sites → Websites → Add new website*.
   - *Domain*: `registry.example.com`
   - *Redirect*: none (a reverse proxy is expressed in the directives, not with
     the redirect field)
   - *SSL*: enable Let's Encrypt, so ISPConfig issues the certificate
   - Leave the document root as ISPConfig chooses; the proxy does not serve files
     from it.

4. **Add the proxy directives.** Open the site → *Options* → *Nginx Directives*
   and paste:

   ```nginx
   client_max_body_size 16m;

   location / {
       proxy_pass http://127.0.0.1:8080;
       proxy_http_version 1.1;
       proxy_set_header Host              $host;
       proxy_set_header X-Real-IP         $remote_addr;
       proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
       proxy_set_header X-Forwarded-Proto $scheme;
       proxy_read_timeout 60s;
   }
   ```

   ISPConfig includes a site's directives into the generated vhost. If the active
   vhost template already declares its own `location /` (check
   `/etc/nginx/sites-available/registry.example.com.vhost` after saving), remove
   that static-file `location /` from the template under *System → Server Config →
   Web → nginx directives*, or merge the `proxy_pass` lines into the template's
   block, so nginx does not see two `location /` blocks.

5. **Apply and verify.**

   ```sh
   sudo nginx -t && sudo systemctl reload nginx
   curl -fsS https://registry.example.com/healthz
   ```

6. **Firewall.** Allow only 22, 80 and 443; the registry's 8080 stays loopback.

   ```sh
   sudo ufw allow 22,80,443/tcp
   sudo ufw enable
   ```

ISPConfig's panel itself should not be exposed on 8080 or any port the registry
uses; keep the panel on its own port and restrict it.

## Configuration and secrets

Settings are read at start-up from `RHO_REGISTRY_CONFIG` (the unit points it at
`/etc/rho/registry.conf`); every setting has a default, so the service runs with
none. The ones that matter for a public deployment:

| Setting           | Value for a server        | Note |
|-------------------|---------------------------|------|
| `data_dir`        | `/var/lib/rho-registry`   | keep it outside the binary |
| `access`          | `public` or `secured`     | `secured` restricts publishing to the allowlist; reads are public either way |
| `registry_url`    | `https://registry.example.com` | what generated documentation links back to |
| `max_upload_mb`   | `16` (raise as needed)    | the HTTP body ceiling follows it |
| `open_tokens`     | `false`                   | never open on a public host |

Secrets belong in the environment file, not the settings file:

```sh
RHO_REGISTRY_BOOTSTRAP_SECRET=<openssl rand -hex 32>
RHO_REGISTRY_ALLOWED_USERS=alice,bob
```

Mint the first token with the secret, then configure the client with the token
(not the secret):

```sh
curl -fsS -H "Authorization: Bearer $RHO_REGISTRY_BOOTSTRAP_SECRET" \
  'https://registry.example.com/auth/token?user=alice&host=registry&days=30&scopes=pull%20push'

rho config set remote.url   https://registry.example.com
rho config set remote.token <the minted token>
rho module search math
```

Reading is always public — the index, search, a version list, a download and a
module's documentation need no credential. Publishing always needs a token;
`access = secured` additionally requires the publisher to be in
`RHO_REGISTRY_ALLOWED_USERS`.

## Backups, upgrades and verification

- **Back up `/var/lib/rho-registry`** — the packages, their metadata and
  `registry.db`. It is the only state.
- **Upgrade** by rebuilding and reinstalling the binary, then
  `sudo systemctl restart registry`. The data directory is untouched.
- **Verify** after a deploy: `GET /healthz` is 200, the index page renders, a
  publish with a token is accepted, a download returns the bytes, and
  `GET /openapi.json` matches the routes the service serves.

## Rejected alternatives

- **A prebuilt binary with no toolchain on the host** is not enough: publishing
  calls `rho doc`, so a run host without the toolchain accepts packages it
  cannot document.
- **Binding the service publicly and skipping nginx** loses TLS and lets any
  client forge `X-Real-IP`, which is what the rate limiter counts.
- **More than one writer on one data directory** is unsupported while the store
  is the filesystem port.
