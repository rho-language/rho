# Security

## Reporting a vulnerability

Please do not open a public issue for a security problem. Report it privately
through GitHub's "Report a vulnerability" on this repository's Security tab (the
maintainer needs private vulnerability reporting enabled in the repository settings
for that button to exist). Say what you found, how to reproduce it, and which
version (`rho --version`) it was in.

Rho is at an early stage with one maintainer, and there is no response-time
commitment yet.

## What the toolchain trusts, and what it does not

- **A registry is not trusted to keep its packages honest.** A downloaded package is
  checked against the SHA-256 the registry recorded, and one with no recorded
  checksum is refused unless `RHO_ALLOW_UNVERIFIED_PACKAGES=1` says otherwise.
- **A checksum is not a publisher.** `rho module keygen` makes an ed25519 key and
  `rho module publish` signs with it. An install pins a module to the first signer it
  shows (trust on first use), and refuses a later package signed by anyone else or
  not signed at all. `rho module trust <name|*> <key>` pins a key by hand, and
  `RHO_REQUIRE_SIGNATURES=1` refuses anything unsigned or unpinned.
- **An archive is not trusted to stay where it is told.** Packages and fetched
  toolchains are unpacked with path confinement, no links or devices, no setuid
  bits, and caps on entries, file size and total size.
- **A toolchain download must name its digest.** A `fetch` block in a module's
  `system_deps` carries the `sha256` the vendor publishes, and a recipe without one
  is refused before anything is downloaded.
- **A component is a program that runs with your authority.** Installing one is
  installing code. Pin its publisher.

## Known limits

- Signing is trust-on-first-use. The first install of a module cannot tell the real
  publisher from an attacker who got there first; pin keys you know in advance.
- The registry stores and serves signatures but does not verify them.
- `rho keeper` listens on loopback and answers only to its own host names; it is a
  local tool, not a service to expose.
- Generated C has no bounds checks, and signed overflow, division by zero and
  over-wide shifts are undefined behaviour in C where Go defines or traps them. See
  `src/parity_cases_test.go` for the list of known differences.
