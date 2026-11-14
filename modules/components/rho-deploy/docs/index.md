# Deploying to a host

`rho deploy` puts a built program on a host that runs an operating system — a
Linux single-board computer, a server, a virtual machine — as opposed to a part
that is written by a probe, which is {ref}`rho flash <rho-flash/index>`'s job.
What the host is and how it is reached is a property of the target: a target spec
carries a `deploy` block, and this command reads it through `rho target show` and
carries it out over SSH.

```sh
rho deploy -target my-sbc -run build/output/sensor-api
```

The target spec's `deploy` block says where and how:

```json
"deploy": {
  "method": "rsync",
  "host": "192.0.2.20",
  "port": 22,
  "user": "deploy",
  "path": "/opt/apps",
  "run": "cd {artifact} && ./sensor-api"
}
```

| Key | What it is |
|---|---|
| `method` | `rsync` (the default) or `scp` |
| `host`, `port`, `user` | Where SSH reaches; with no `host`, the local machine — a toolchain running on the target deploys to itself |
| `path` | The directory on the host the program is copied into |
| `run` | What `-run` executes on the host after copying; `{artifact}` is the copied program's path there |

## Containers

A `docker` block in the same place deploys a container image instead: the image
is built here, saved, copied, loaded on the host and started.

```json
"deploy": {
  "host": "192.0.2.20", "user": "deploy", "path": "/opt/apps",
  "docker": {
    "file": "Dockerfile", "image": "sensor-api", "tag": "v1",
    "run": ["--net", "host", "-p", "8080:8080", "{image}"]
  }
}
```

An image that is only to be built — for a registry, or to run locally — needs no
host. `-build-only` builds it and stops, and `-push` with it publishes the image.
The block can also say what the image is built from:

```json
"docker": {
  "file": "deploy/Dockerfile", "context": "../..", "image": "acme/sensor-api",
  "contexts": {"shared": "${RHO_WORKSPACE}/shared"}, "build_args": {"VERSION": "1"}
}
```

| Key | What it is |
|---|---|
| `file` | The Dockerfile |
| `image`, `tag` | The image's name and tag |
| `run` | The `docker run` arguments on the host; `{image}` is the image reference |
| `context` | The build context, relative to the project (the project itself when left out) |
| `contexts` | Named `--build-context` directories; a path may use `${RHO_WORKSPACE}` |
| `build_args` | `--build-arg` values |

```sh
rho deploy -target my-sbc -docker -run .                 # build, copy, load, start
rho deploy -target my-sbc -docker -build-only            # only build the image
rho deploy -target my-sbc -docker -build-only -push      # build and publish it
```

## Flags

| Flag | What it does |
|---|---|
| `-target NAME` | The target whose spec says where to deploy; required |
| `-project DIR` | The project whose target is read (default: the working directory) |
| `-run` | Start the program on the host after copying |
| `-docker` | Deploy as a container image |
| `-build-only` | With `-docker`: build the image and stop |
| `-push` | With `-docker -build-only`: push the built image |
| `-n` | Print every step, and run nothing |
| `-artifact FILE` | Deploy a signed artifact instead of a loose file |
| `-pub FILE` | The artifact's Ed25519 public key |
| `-allow-unsigned` | Accept an unsigned artifact after checking its checksum |

Flags may come before or after the file. Exactly one file or directory is named,
unless `-artifact` is given or the run only builds an image.

## The environment

A spec describes a kind of host; which machine on which bench is a fact about the
bench. These override the spec without editing it:

| Variable | Overrides |
|---|---|
| `RH_DEPLOY_HOST`, `RH_DEPLOY_PORT`, `RH_DEPLOY_USER` | Where SSH reaches |
| `RH_DEPLOY_PATH` | The directory on the host |
| `RH_DEPLOY_METHOD` | `rsync` or `scp` |
| `RH_DEPLOY_DOCKER` | `1`, `true`, `yes` or `on`: the same as `-docker` |
| `RH_DEPLOY_DOCKER_FILE`, `RH_DEPLOY_DOCKER_IMAGE`, `RH_DEPLOY_DOCKER_TAG`, `RH_DEPLOY_DOCKER_RUN` | The container's Dockerfile, image, tag and run arguments |

## Signed artifacts

With `-artifact`, the artifact is verified against `-pub` (its signature, its
target and its payload checksum) and unpacked before anything is copied, and the
program deployed is the entry point its manifest names. `rho flash` takes the
same three flags. {ref}`rho-artifact <rho-artifact/index>` packs and verifies
artifacts.

## Exit codes

0 on success, 1 when a step failed, and 2 for a usage error: no `-target`, no file
named, or `-build-only`/`-push` without `-docker`.
