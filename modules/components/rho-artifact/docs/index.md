# Signed artifacts

`rho artifact` packs a build into a signed artifact and verifies one before it is
used. An artifact is a `.tar.gz` holding two files: `artifact.json`, the manifest,
and `payload.tar.gz`, the files themselves. The manifest records the payload's
SHA-256 and size and, when a key was given, an Ed25519 signature over the
manifest — so a payload that was changed, a manifest that was edited, and an
artifact built for another target are all refused.

```sh
rho artifact pack -name sensor-api -version 1.2.0 -kind service \
  -target linux-arm64 -entrypoint sensor-api -service sensor-api \
  -health /healthz -health-timeout 20 -key release.key \
  -out sensor-api-1.2.0.tar.gz build/output/

rho artifact verify -pub release.pub sensor-api-1.2.0.tar.gz
rho artifact verify -pub release.pub -target linux-arm64 \
  -extract /tmp/sensor-api sensor-api-1.2.0.tar.gz
rho artifact inspect sensor-api-1.2.0.tar.gz
```

## pack

`rho artifact pack [flags] <input-dir>` packs a directory.

| Flag | Default | What it is |
|---|---|---|
| `-name`, `-version` | required | What the artifact is |
| `-target` | required | The target id it was built for; `verify -target` checks it |
| `-entrypoint` | required | The file inside the payload to run or flash |
| `-kind` | `service` | `service` (a program a host runs) or `firmware` (an image a board is flashed with) |
| `-service` | none | For a service: the name of its system service; required for `service` |
| `-health` | `/healthz` | For a service: the local path its health is checked on |
| `-health-timeout` | `20` | Seconds the health check may take |
| `-keep` | `2` | How many previous versions a host keeps, to roll back to |
| `-automatic` | `true` | Roll back automatically when the health check fails |
| `-key` | none | The private key to sign with; without one the artifact is unsigned |
| `-out` | `<name>-<version>.tar.gz` | Where to write it |

The manifest is validated before anything is written: a service needs a service
name and an entry point, a health path must be a local path, and the kind must be
`service` or `firmware`.

## verify

`rho artifact verify [flags] <artifact>` checks, in order, that the manifest is
valid, that the signature matches the public key, that the target is the one asked
for, and that the payload's checksum and size are the ones the manifest records.
It prints `ok <name>@<version> <target>` and exits 0, or exits 1 with the reason.

| Flag | What it does |
|---|---|
| `-pub FILE` (or `-key FILE`) | The Ed25519 public key; required for a signed artifact |
| `-target ID` | Refuse an artifact built for another target |
| `-extract DIR` | Unpack the payload here once it has verified |
| `-allow-unsigned` | Accept an artifact with no signature (its checksum is still checked) |

An unsigned artifact is refused unless `-allow-unsigned` is given, which is for
development. Extraction refuses absolute paths, parent references and anything
else that would write outside the directory.

## inspect

`rho artifact inspect <artifact>` prints the manifest as sorted `key=value` lines
— `name`, `version`, `kind`, `target`, `entrypoint`, `service`, `health`,
`health_timeout`, `keep`, `automatic`, `sha256`, `size`, `signed` — which a shell
script or another program reads without a JSON parser. It does not verify.

## Keys

Keys are files holding base64: for signing, a 32-byte Ed25519 seed or a 64-byte
private key; for verifying, the 32-byte public key. The private key stays where
artifacts are built; only the public key goes to the machines that verify.

## Using an artifact

`rho flash` and `rho deploy` take `-artifact FILE` with `-pub FILE` (or
`-allow-unsigned`): the artifact is verified for the target being written to and
extracted, and the entry point is what is flashed or deployed. See
{ref}`rho-flash <rho-flash/index>` and {ref}`rho-deploy <rho-deploy/index>`.

## Not implemented yet

The artifact format and its verification are complete; what acts on an artifact
remotely is not. Not yet implemented:

- a remote host rollout through `rho deploy` with staging on the target and
  rollback;
- target metadata saying what over-the-air update a part supports (`ab`,
  `bootloader`, `unsupported`);
- A/B partition writes and boot confirmation on microcontrollers that have them;
- delivering firmware to a device through an intermediate gateway, in chunks;
- automatic firmware rollback on targets whose bootloader supports it;
- staged (canary) rollout across a fleet.

`rho flash` flashes a verified artifact locally, and does not claim a remote update
on a target without a rollback-capable bootloader.
