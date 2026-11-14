# Provisioning and identity

`rho bootstrap` gives a machine an identity of its own. It reads a provisioning
contract, creates an instance UUID and an Ed25519 key pair locally, keeps them in
an owner-only directory, and — given a one-time token — enrolls the identity with
an enrollment server, which issues the credential the machine uses from then on.
The private key is generated where it is used and never leaves it.

```sh
rho bootstrap init -config ~/.rho/provisioning.json -token-file /run/enroll.token
printf '%s\n' "$TOKEN" | rho bootstrap init -token-file -     # or on standard input
rho bootstrap status
```

The token is read from a file or from standard input, never from the command
line: a command line is visible to every user of the machine (`ps`, `/proc`) and
stays in shell history, so `-token` is refused with a message saying so.

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
| `keeper.url` | The enrollment server; required only when a token is given. It must be `https://`; plain `http://` is accepted only for `127.0.0.1`, `::1` or `localhost` |
| `secrets.store` | The identity directory (default `~/.rho/identity`) |
| `permissions` | The modes asked for; only `0700`, `0750`, `0600` and `0640` are accepted |

A contract that names no device, has an unknown kind, or asks for a looser
permission is refused before anything is written. Without `-config`, the contract
is read from `/etc/rho/provisioning.json`, then `~/.rho/provisioning.json`.

## The commands

| Command | Flags | What it does |
|---|---|---|
| `init` | `-config FILE`, `-dir DIR`, `-token-file FILE\|-`, `-force`, `-offline`, `-board-key`, `-mesh-url URL` | Create the identity, then enroll if a token is given; with `-board-key`, then fetch the board key |
| `status` | `-dir DIR` | Print the identity as `key=value` lines |
| `token` | `-device ID` (required), `-project`, `-area`, `-node`, `-ttl` (default `15m`, must be positive), `-reenroll`, `-file` | On the enrollment server's machine: create a one-time token for one device and one placement |
| `key` | `-config FILE`, `-dir DIR`, `-mesh-url URL` | Fetch this node's board id and key from Mesh with the enrollment credential |

`init` writes `identity.json` (the instance id, the device and placement, the
public key) and `identity.key` (the private seed), both `0600`, and
`identity.pub`. Run again, it keeps the identity it finds unless `-force` is
given, so a second run is safe. With a token and the contract's `keeper.url`, it
posts the device id, the public key and the token to `<url>/api/enroll` and stores
the credential that comes back as `credential`, `0600`; `-offline` skips
enrollment. The placement the node is recorded at is the token's: the contract's
placement is sent only to be compared, and a token for another placement is
refused. Redirects are not followed.

`token` adds one line to the server's token file (default
`~/.rho/keeper/enrollment.tokens`) and prints the token, its expiry and its
placement. The keeper marks lines used in the same file, so the line is added
under the keeper's lock (the directory `enrollment.tokens.lock` beside the file)
and the file is replaced whole through a temporary file and a rename. A device
that is already enrolled is replaced only by a token made with `-reenroll`.

`key` is for a Linux host node that speaks the board protocol (a host gateway
node, for example). It posts to Mesh's node-key endpoint — by default
`<keeper.url>/app/mesh/mesh/api/nodes/key`, Mesh behind the keeper's origin —
with `Authorization: Node <device>:<credential>`. Mesh checks the credential with
the keeper, finds the board it planned at the node's placement, and answers its
id, key and key epoch. They are kept as `board.key` (`<epoch>:<32 hex digits>`;
the hex alone from an older Mesh that names no epoch) and `board.env`
(`RH_SECURE_UID` and `RH_SECURE_KEY_FILE`, to be read as an environment file),
both `0600`, and are never printed. A key the node's keystore kept beside the file
(`board.key.keystore`) is removed: the key Mesh answers is the board's current
one. When the gateway rotates the key, the host node rewrites `board.key` itself.

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
