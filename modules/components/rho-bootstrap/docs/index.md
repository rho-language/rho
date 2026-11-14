# Provisioning and identity

`rho bootstrap` gives a machine an identity of its own. It reads a provisioning
contract, creates an instance UUID and an Ed25519 key pair locally, keeps them in
an owner-only directory, and — given a one-time token — enrolls the identity with
an enrollment server, which issues the credential the machine uses from then on.
The private key is generated where it is used and never leaves it.

```sh
rho bootstrap init -config ~/.rho/provisioning.json -token <one-time-token>
rho bootstrap status
```

## The provisioning contract

The contract is public configuration in JSON: who this machine is meant to be,
where it belongs, and where it enrolls.

```json
{
  "device": {"id": "sensor-01", "kind": "node", "board": "esp32c3"},
  "placement": {"project": "acme-lab", "area": 1, "node": 7},
  "keeper": {"url": "https://enroll.example.com", "trust": "ed25519:..."},
  "secrets": {"generate": ["instance_id", "identity_key"], "store": "~/.rho/identity"},
  "permissions": {"directory": "0700", "files": "0600"}
}
```

| Field | What it is |
|---|---|
| `device.id` | The device's name; required |
| `device.kind` | `node`, `host` or `service`; required |
| `placement.project` | The project the device belongs to; required |
| `placement.area`, `placement.node` | Where in it; a `node` needs a node number |
| `keeper.url` | The enrollment server; required only when a token is given |
| `secrets.store` | The identity directory (default `~/.rho/identity`) |
| `permissions` | The modes asked for; only `0700`, `0750`, `0600` and `0640` are accepted |

A contract that names no device, has an unknown kind, or asks for a looser
permission is refused before anything is written. Without `-config`, the contract
is read from `/etc/rho/provisioning.json`, then `~/.rho/provisioning.json`.

## The commands

| Command | Flags | What it does |
|---|---|---|
| `init` | `-config FILE`, `-dir DIR`, `-token TOKEN`, `-force`, `-offline` | Create the identity, then enroll if a token is given |
| `status` | `-dir DIR` | Print the identity as `key=value` lines |
| `token` | `-device ID` (required), `-project`, `-area`, `-node`, `-ttl` (default `15m`), `-file` | On the enrollment server's machine: create a one-time token for one device |

`init` writes `identity.json` (the instance id, the device and placement, the
public key) and `identity.key` (the private seed), both `0600`, and
`identity.pub`. Run again, it keeps the identity it finds unless `-force` is
given, so a second run is safe. With a token and the contract's `keeper.url`, it
posts the device id, the public key and the token to `<url>/api/enroll` and stores
the credential that comes back as `credential`, `0600`; `-offline` skips
enrollment.

`token` appends one line to the server's token file (default
`~/.rho/keeper/enrollment.tokens`) and prints the token and its expiry; the
server's enrollment endpoint reads that file.

## What belongs to firmware

The host side — the identity, its storage and the enrollment — is implemented. On
a microcontroller the following belongs to the firmware, not to this command:

- generating or importing the identity key into the part's secure storage;
- enabling flash encryption where the part supports it;
- enrolling over serial, BLE, Wi-Fi or a gateway;
- keeping the issued credential and refreshing it after it is rotated;
- a factory reset that clears the identity and the credential;
- refusing to boot with a world-readable or missing identity.

A machine that cannot protect a local key has no strong identity: physical access
to its storage is then access to its identity, and it should be documented as
such.
