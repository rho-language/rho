# Changelog

## [0.1.0] - 2026-11-08

First release. Adds `pack`, `verify` and `inspect` for signed host and firmware
artifacts, with SHA-256 payload checks and Ed25519 manifests.

## [0.1.1] - 2026-11-10

Security fix for archive extraction: the payload is written under its constant
name, and the verifier refuses archive members that name an absolute path, a
volume path or a parent path. Added a test for an escaping member.
