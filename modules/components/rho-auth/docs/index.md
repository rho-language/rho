# Credentials and tokens

`rho auth` holds the credentials of this toolchain and of the services built with
it. It does three jobs, each with its own commands:

- **Registry tokens** — signing in to a remote module registry, so `rho module
  publish` and `install` are authenticated.
- **Login tickets and service tokens** — the short-lived, signed tokens a
  generated service's `auth` middleware verifies locally, and the key pair that
  signs them.
- **User and client stores** — the files of people and services an issuer
  authenticates before it signs anything.

```sh
rho auth login --registry https://registry.example.com --user ada
rho auth status
```

## Registry tokens

| Command | What it does |
|---|---|
| `login` | Ask a registry for a token and store it |
| `status` | The stored credentials, one line each, valid or expired, and any about to lapse |
| `list` | The same as `status` |
| `inspect` | Decode a token's claims: subject, issuer, token id, scopes, lifetime |
| `revoke` | Forget the stored credential for a registry |

| Flag | Default | What it is |
|---|---|---|
| `--registry URL`, `-r` | `https://registry.rho-lang.org` | The registry the command is about |
| `--user NAME`, `-u` | `$RHO_USER`, `$USER` or `$LOGNAME` | Who the token is issued to |
| `--host NAME` | this machine's hostname | The host recorded in the token |
| `--days N` | 30 | How long the token lasts; at most 90 |
| `--scopes a,b` | `pull,push` | The scopes asked for |
| `--token TOKEN` | the stored one | For `inspect`: decode this token instead |

`login` asks the registry's `/auth/token` endpoint for a token; the registry signs
it, and its secret never leaves the registry. A token is three base64url segments,
`header.payload.signature`, signed with HMAC-SHA256 in the shape of a JWT: anyone
holding one can read its claims, and the signature is what stops them changing
them. An existing credential is presented as proof of identity when asking again,
so a registry that has closed issuance to newcomers still renews a client it
knows.

Credentials are kept in `~/.rho/auth/credentials.json`, readable by you alone.
The module client presents the stored token whenever it talks to that registry,
in preference to a `remote.token` set with `rho config`, so there is one way to
configure a registry rather than two.

## Login tickets

A service built with the `auth` middleware does not call back to whoever signed
a user in. It is given a short-lived **ticket** — an ES256 JWT — and checks it
with the issuer's public key: the signature, the issuer, the audience and the
lifetime. Remembering that a ticket's `jti` was already used is the service's own
job.

```sh
rho auth keygen --dir ~/.rho/auth

TICKET=$(rho auth ticket --private ~/.rho/auth/private.pem \
  --issuer login.example.com --audience my-app --subject ada --ttl 30s)

rho auth verify --public ~/.rho/auth/public.pem \
  --issuer login.example.com --audience my-app --token "$TICKET"
```

| Command | Flags | What it does |
|---|---|---|
| `keygen` | `--dir DIR` (default `~/.rho/auth`), `--force` | Write a P-256 key pair, `private.pem` and `public.pem`, and print its key id; an existing key is kept unless `--force` |
| `ticket` | `--private FILE` (required), `--issuer`, `--audience`, `--subject` (default the current user), `--ttl` (default `30s`), `--scopes a,b`, `--sid`, `--nonce`, `--redirect-uri` | Sign a ticket and print it |
| `verify` | `--public FILE`, `--token` (both required), `--issuer`, `--audience`, `--revoked FILE` | Check a ticket and print its claims as JSON; exits 1 when it does not hold or its id is in the revocation list |
| `revoke --jti ID` | `--file FILE` (default `~/.rho/auth/revoked.txt`) | Add a ticket id to the revocation list the middleware reads, so a ticket stops working before it expires |

`--sid` records the issuer's session a ticket was minted from, so signing out of
that session can be traced to every ticket it produced. `--scopes` puts the
scopes in the ticket, and the middleware refuses a request to a route whose
required scopes the ticket does not carry.

## Clients and service tokens

A service that calls another service authenticates as a registered **client**
and exchanges its secret for a **service token**: a ticket whose subject is
`client:<id>`, for one audience, with only the scopes its registration allows.

```sh
# Register a client. The secret is printed once, or written to a file
# readable by its owner; the store keeps only its hash.
rho auth client add --id reporter --audiences inventory \
  --scopes inventory:read --secret-out /run/secrets/reporter

rho auth client list
rho auth client remove --id reporter

# Exchange the secret for a token, valid ten minutes at most.
rho auth service-token --id reporter --secret-file /run/secrets/reporter \
  --audience inventory --scopes inventory:read \
  --private ~/.rho/auth/private.pem --issuer login.example.com --ttl 5m
```

`service-token` prints `{"token": "...", "expires_in": <seconds>}`. The secret is
read with `--secret-file FILE` or `--secret-stdin`, never from the command line,
where every user of the machine could read it. Asking for a scope the client was
not registered for, or an audience it may not reach, is refused. The client store
is `~/.rho/auth/clients` unless `--store FILE` names another.

## Users

An issuer that signs people in keeps them in a users file: a name, a display
name, a role, grants, and a PBKDF2-SHA256 hash of the password. A password is
handed over in a file, never on the command line.

```sh
rho auth user add    --store users.yml --name ada --role operator \
                     --display "Ada Example" --grants "inventory:*" --password-file pw.txt
rho auth user verify --store users.yml --name ada --password-file pw.txt
rho auth user passwd --store users.yml --name ada --password-file new.txt
rho auth user set    --store users.yml --name ada --role admin
rho auth user list   --store users.yml
rho auth user remove --store users.yml --name ada
```

`verify` prints the name, display name, role and grants when the password is
right and fails otherwise; a name that is not there costs as long and says as
little as a wrong password, so neither reveals which names exist. `set` changes
only what it is given, and `--grants -` clears the grants.

## Exit codes

0 on success, 1 when the operation failed (a registry refused, a ticket did not
verify, a file could not be read), and 2 for a missing or malformed argument.

## Related

- {ref}`modules/registries` — the local and remote registries, and what each is
  for.
- {ref}`registry/index` — the registry application these tokens are for.
- {ref}`building/services` — the generated `auth` middleware that verifies
  tickets.
