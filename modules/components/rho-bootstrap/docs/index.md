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
