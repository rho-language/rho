# Registry credentials

`rho auth` signs in to a remote registry, inspects what is signed in, and
revokes a token.

```sh
rho auth login   --registry http://localhost:8080 --user miraur --days 30
rho auth status
rho auth list
rho auth inspect --registry http://localhost:8080
rho auth revoke  --registry http://localhost:8080
```

## The model

A token is three base64url segments, `header.payload.signature`, where the
signature is an HMAC-SHA256 over the first two. The shape follows JWT, so a
token is recognisable to anyone who has worked with a web API, and the
signature is checked before anything in the payload is believed.

## Commands

| Command | What it does |
|---|---|
| `login` | Ask a registry for a token and store it |
| `status` | Whether a token is stored, and for which registry |
| `list` | Every token this machine holds |
| `inspect` | What the registry says about a token |
| `revoke` | Ask the registry to stop honouring one |

A stored token is presented by the module client when it talks to a registry,
so `rho module search` and `rho module install` are authenticated by the same
thing `rho auth` manages — there is one way to configure a registry, not two.

## Related

- {ref}`handbook/registries` — the local and remote registries, and what each
  one is for.
- {ref}`registry/index` — the registry application these credentials are for.
