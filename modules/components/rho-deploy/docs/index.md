# Deploying to a host

`rho deploy` puts a built program on a host that runs an operating system — a
Linux board, a gateway machine — as opposed to a part that is written by a probe.
What the host is, and how it is reached, is a property of the target: a spec
carries a `deploy` block, and this command reads it through `rho target show` and
carries it out.

```sh
rho deploy -target uno-q -run build/gateway
```

The `deploy` block says:

```json
"deploy": {
  "method": "rsync",            // rsync (default) or scp
  "host": "uno-q.local",
  "port": 22,
  "user": "arduino",
  "path": "/home/arduino/apps",
  "run": "cd {artifact} && ./gateway"
}
```

A `docker` block in the same place deploys a container image instead:

```json
"deploy": {
  "host": "uno-q.local", "user": "arduino", "path": "/home/arduino/apps",
  "docker": {
    "file": "Dockerfile", "image": "robohub-gateway", "tag": "v1",
    "run": ["--net", "host", "-p", "8080:8080", "{image}"]
  }
}
```

The image is built here, saved, carried, loaded and started on the target.

`-n` prints every step and runs nothing. The address, login, path, port and
method can be redirected from the environment — `RH_DEPLOY_HOST`,
`RH_DEPLOY_USER`, `RH_DEPLOY_PATH`, `RH_DEPLOY_PORT`, `RH_DEPLOY_METHOD` — so a
spec is not a fact about one bench.
