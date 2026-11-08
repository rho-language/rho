# Signed artifacts

An artifact is a signed `.tar.gz` carrying `artifact.json` and a
`payload.tar.gz`. The manifest is validated by the Rho artifact schema; the
payload is checked by SHA-256.

```sh
rho artifact pack -name aquarium -version 0.4.0 -kind service \
  -target host-linux-amd64 -entrypoint aquarium -service aquarium \
  -health /healthz -health-timeout 20 -key build.key -out aquarium-0.4.0.tar.gz build/

rho artifact verify -pub build.pub aquarium-0.4.0.tar.gz
rho artifact verify -pub build.pub -target host-linux-amd64 \
  -extract /tmp/aquarium aquarium-0.4.0.tar.gz
rho artifact inspect aquarium-0.4.0.tar.gz
```

Keys are base64 files: a 32-byte Ed25519 seed or a 64-byte private key for
packing, and a 32-byte public key for verification. `inspect` prints
`key=value` lines, which is what Keeper reads without a JSON parser.

## Remaining OTA work

The artifact and host rollout path is implemented. The following are not yet
implemented:

- remote host rollout through `rho deploy` with target-side staging and rollback;
- target OTA capability metadata (`ab`, `bootloader`, `unsupported`);
- ESP32 A/B partition writes and boot confirmation;
- Mesh gateway firmware delivery and node-side chunked OTA;
- automatic firmware rollback on targets whose bootloader supports it;
- canary/fleet rollout policy.

The flasher still flashes a verified artifact locally. It does not claim remote
OTA on a target that has no rollback-capable bootloader.
