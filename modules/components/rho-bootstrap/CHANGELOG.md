# Changelog

## [0.1.0] - 2026-11-08

First release. Adds local identity generation, permission enforcement,
idempotent first boot, and optional Keeper enrollment.

## [0.2.0] - 2026-11-14

A keeper is reached over https only (plain http only to this machine), with no redirects followed. The enrollment token is read from a file or standard input (`-token-file`), never from the command line, where every user can read it. `token` writes under the keeper's lock, requires a device and an expiry, and can mark a token `-reenroll`. `key` (and `init -board-key`) fetches the host's board key from Mesh with its enrollment credential and stores it 0600 with the environment file a host node reads.
