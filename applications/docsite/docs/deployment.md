# Deploying the docsite service

It is deployed the same way the registry is. The full walkthrough — Ubuntu,
systemd, nginx and ISPConfig, TLS and firewall — is in the registry's
`applications/registry/docs/deployment.md`; this page gives the differences.

## What is different from the registry

- **Nothing to back up.** The registry's only state is its data directory; this
  service has none. Its pages come from the documentation tree the image carries
  (or a volume mounts), so a redeploy is the whole of an upgrade.
- **The repository is the tree.** The service builds the site from `RHO_DOCS_ROOT`
  (the project root, default `.`) and lists the menu from `RHO_DOCS_TREE` (the
  documentation directory, default `docs`). It generates the site on first use,
  so run it where the tree is — the image copies the repository in.
- **The toolchain is still needed**: a page is rendered by calling `rho doc`, so
  the run host — or the image — needs `rho-doc` beside the toolchain, exactly as
  the registry does.

## Container

```sh
docker build -f applications/docsite/deploy/Dockerfile -t <dockerhub-user>/rho-docsite .
```

## systemd

`deploy/docsite-docker.service` is the unit; install and enable it as the
registry's is:

```sh
sudo install -m 0644 applications/docsite/deploy/docsite-docker.service /etc/systemd/system/docsite-docker.service
sudo sed -i 's#^Environment=IMAGE=.*#Environment=IMAGE=<dockerhub-user>/rho-docsite:latest#' /etc/systemd/system/docsite-docker.service
sudo systemctl daemon-reload
sudo systemctl enable --now docsite-docker
curl -fsS http://127.0.0.1:8081/healthz
```

## nginx and ISPConfig

Create the site for the documentation hostname, enable Let's Encrypt, and put the
proxy directives in *Sites → Website → Options → Nginx Directives*:

```nginx
location / {
    proxy_pass http://127.0.0.1:8081;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Unlike the registry there is no upload, so no `client_max_body_size` is needed. The port is 8081 rather than the registry's 8080, so both can run on one host.
The service binds loopback only, so nothing but nginx reaches it.
