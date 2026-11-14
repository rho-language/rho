# rho-bootstrap

Reads a local provisioning contract, generates an instance UUID and Ed25519
identity key, stores them with owner-only permissions, and optionally enrolls
the instance with Keeper.

```sh
rho bootstrap init -config /etc/rho/provisioning.json
rho bootstrap status
```

The contract contains no long-lived secret. A one-time enrollment token, read
from a file or standard input (`-token-file FILE|-`, never the command line), is
exchanged over https for a credential issued by Keeper. A Linux host node then
fetches its board key from Mesh with that credential (`rho bootstrap key`).
