# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.2.2] - 2026-11-12

Ships the `gateway` and `telemetry` ports (`ports/*.go.txt`), which the toolchain
used to carry. A project of this platform that asks for them by name gets them
from here, merged into its generated port file; the toolchain keeps only the
ports every project may ask for. The package now includes `bindings/` and
`ports/`.

## [0.2.1] - 2026-11-12

Ships `bindings/c-bus.yml`, the types and functions of robohub-core's event bus
that generated C calls, and `bindings/rho-transport.rho`, the COBS, Fletcher-16
and header framing robohub-transport writes. `rho protocol generate --platform
robohub` reads them, so the toolchain no longer carries RoboHub's names or wire
framing to generate `protocol_bus.h/.c` or the transport in a schema's Rho.

## [0.2.0] - 2026-11-11

The plugin now answers `rho-module`'s `contract` hook with the module contract
RoboHub publishes in `robohub-common/contracts/`. The toolchain validates a
RoboHub module against the ecosystem's own types and schemas instead of carrying
them, which is what keeps Rho a language for more than one ecosystem. A device
is the first type this makes possible: it is a `device`, not a `driver`.

## [0.1.0] - 2026-10-12

Initial release.
