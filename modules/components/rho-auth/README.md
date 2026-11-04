# rho-auth

Registry credentials: signing in, inspecting and revoking tokens.

```sh
rho auth login   --registry http://localhost:8080 --user miraur --days 30
rho auth status
rho auth list
rho auth inspect --registry http://localhost:8080
rho auth revoke  --registry http://localhost:8080
```

## The model

A token is three base64url segments, `header.payload.signature`, where the
signature is an HMAC-SHA256 over the first two. The shape follows JWT, so the
tokens are recognisable to anyone who has worked with a web API.

The signing secret never leaves the registry. A client asks for a token and
receives a signed one, so a stolen credential file cannot be used to mint new
credentials. A token is valid for a bounded period, at most 90 days, and a
caller may ask for less but not for more.

**A token is signed, not encrypted.** Anyone holding one can read its claims;
the signature is what stops them changing them. A registry also refuses to
issue a token to an anonymous caller once issuance is closed, so the first
credential can be obtained and later ones cannot.

`rho auth status` reports anything expiring or expired, so a failure in an
unrelated command is not the first sign of a lapsed credential.

## Extending

The mechanism is pluggable. `Verifier` turns a credential into a `Principal`,
and `Chain` tries several in turn, so an API key, a client certificate or a
third-party scheme is added by implementing one interface rather than changing
the command. The store is keyed by mechanism and by registry, so several kinds
of credential coexist.

Credentials live in `~/.rho/auth/credentials.json`, written owner-only.

## SSO tickets

`rho-auth` also carries the short-lived ticket a resource server verifies
locally. The keeper holds the private key; every application holds only the
public key.

```sh
rho auth keygen --dir ~/.rho/auth
rho auth ticket --private ~/.rho/auth/private.pem \
  --issuer keeper --audience aquarium --subject miraur --ttl 30s
rho auth verify --public ~/.rho/auth/public.pem \
  --issuer keeper --audience aquarium --token "$TICKET"
```

A ticket is an ES256 JWT with `sub`, `iss`, `aud`, `jti`, `iat`, `nbf`, `exp`,
and optionally `redirect_uri` and `nonce`. `verify` checks the signature,
issuer, audience and lifetime; the application must also keep the `jti` it has
already used.
