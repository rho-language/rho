# Provisioning and identity

`rho bootstrap` reads a JSON provisioning contract, creates an instance UUID
and Ed25519 identity key locally, and keeps them in an owner-only directory.

```json
{
  "device": {"id": "garden-01", "kind": "node", "board": "esp32c3"},
  "placement": {"project": "default", "area": 1, "node": 7},
  "keeper": {"url": "https://keeper.local:19090", "trust": "ed25519:..."},
  "mesh": {"gateway": "https://mesh.local:8478"},
  "secrets": {"generate": ["instance_id", "identity_key"], "store": "~/.rho/identity"},
  "permissions": {"directory": "0700", "files": "0600"}
}
```

```sh
rho bootstrap init -config ~/.rho/provisioning.json -token <one-time-token>
rho bootstrap status
```

The contract is public configuration. Long-lived credentials are issued by
Keeper during enrollment and stored as `0600` files. A second run is
idempotent: it does not regenerate the identity unless `-force` is given.

## Firmware-level work

The host bootstrap and Keeper enrollment are implemented. For a microcontroller
node, the following still belongs to the firmware layer:

- generating or importing the identity key into NVS/secure storage;
- enabling flash encryption where the part supports it;
- enrollment over serial, BLE, WiFi, or the mesh gateway;
- persisting the issued credential and refreshing it after rotation;
- factory reset clearing the identity and credential;
- refusing to boot with a world-readable or uninitialised identity.

A host or node that cannot protect a local key should be documented as having no
strong identity; physical access to the storage is then equivalent to identity
access.
