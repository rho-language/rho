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
    "file": "Dockerfile", "image": "my-service", "tag": "v1",
    "run": ["--net", "host", "-p", "8080:8080", "{image}"]
  }
}
```

The image is built here, saved, carried, loaded and started on the target.

An image that is only to be built — for a registry, or to run locally — needs no
host. `-build-only` builds it and stops, with no artifact named, and `-push`
publishes it. The block can also say where the image is built from and with what:

```json
"docker": {
  "file": "deploy/Dockerfile", "context": "../..", "image": "me/site",
  "contexts": {"rho": "${RHO_WORKSPACE}/rho"}, "build_args": {"VERSION": "1"}
}
```

`context` is relative to the project, `contexts` are `--build-context` names whose
paths may use `${RHO_WORKSPACE}`, and `build_args` are `--build-arg`s.

```sh
rho deploy -target container -docker -build-only          # build the image
rho deploy -target container -docker -build-only -push    # and publish it
```

`-n` prints every step and runs nothing. The address, login, path, port and
method can be redirected from the environment — `RH_DEPLOY_HOST`,
`RH_DEPLOY_USER`, `RH_DEPLOY_PATH`, `RH_DEPLOY_PORT`, `RH_DEPLOY_METHOD` — so a
spec is not a fact about one bench.

## Signed artifacts

`rho flash` and `rho deploy` accept `-artifact` with `-pub` (or
`-allow-unsigned` for development). The artifact is verified and extracted
before the flasher or deployment command runs. Remote OTA capability and
firmware rollback are described in `rho-artifact`'s remaining-work notes.
