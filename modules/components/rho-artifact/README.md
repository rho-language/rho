# rho-artifact

Pack and verify signed host and firmware artifacts for RoboHub rollouts.

```sh
rho artifact pack -name aquarium -version 0.4.0 -kind service \
  -target host-linux-amd64 -entrypoint aquarium -service aquarium \
  -health /healthz -key build.key -out aquarium-0.4.0.tar.gz build/

rho artifact verify -pub build.pub -extract /tmp/aquarium aquarium-0.4.0.tar.gz
rho artifact inspect aquarium-0.4.0.tar.gz
```

An artifact is a `.tar.gz` with `artifact.json` and `payload.tar.gz`. The payload
is SHA-256 checked; the manifest carries an Ed25519 signature. Keeper and Mesh
verify before staging an update.
