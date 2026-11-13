# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.5.0] - 2026-11-13

People. `rho auth user add|verify|passwd|set|remove|list` keeps a store of the
people who sign in — a name, a display name, a role (admin, operator, viewer),
grants per application (`mesh:*`, `aquarium:read`) and a password hash
(PBKDF2-HMAC-SHA256, 600 000 iterations, a salt of its own). A password is handed
over in a file (`--password-file`), never on the command line; a wrong name costs
as long as a wrong password and is answered the same; the first person is an
admin and the last admin stays one.

A ticket can carry the issuer's session (`--sid`, the `sid` claim), so an
application can end its session when the issuer's ends.

## [0.4.1] - 2026-11-13

`rho auth revoke --jti` writes the revocation list under the Rho home
(`$RHO_HOME/auth/revoked.txt`, `~/.rho/auth/revoked.txt` by default), where every
service reads it, instead of under the user's own home.

## [0.4.0] - 2026-11-11

Client credentials for service-to-service calls. `rho auth client add|remove|list`
registers a service with the audiences and scopes it may be granted, keeping only a
hash of its secret, in a file only its owner can read. `rho auth service-token`
exchanges a client's secret, read from standard input or a file and never from the
command line, for an ES256 token for one audience, with the scopes it asked for
that it may hold and a lifetime of at most ten minutes.

## [0.1.0] - 2026-10-12

Initial release.

## [0.2.0] - 2026-11-04

Adds the SSO half of the component: `keygen` creates the ECDSA P-256
keypair, `ticket` mints a short-lived ES256 login ticket, and `verify` checks
one against the public key. The registry credential commands are unchanged.

## [0.3.0] - 2026-11-06

Scoped tokens and tickets (--scopes), token revocation (--jti), TLS support
in generated servers, UPLINK_BIND for gateway uplink.
