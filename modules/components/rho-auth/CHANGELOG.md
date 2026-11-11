# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

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
