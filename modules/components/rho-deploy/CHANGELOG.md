# Changelog

## [0.1.0] - 2026-10-20

Initial release.

- `rho deploy` copies a built program to a host over SSH (rsync or scp) and can
  start it, driven by the target spec's `deploy` block.
- A `docker` block deploys a container image: built here, carried, loaded and
  started on the target.
- The environment can redirect the address, login, path and method
  (`RH_DEPLOY_HOST`, `RH_DEPLOY_USER`, `RH_DEPLOY_PATH`, `RH_DEPLOY_PORT`,
  `RH_DEPLOY_METHOD`).
- `-n` prints the steps and runs nothing.

## [0.2.0] - 2026-11-08

`-artifact` verifies a signed rollout artifact and deploys the entrypoint it
names. The artifact is checked against its payload checksum and Ed25519 manifest
before anything is copied to the target.
