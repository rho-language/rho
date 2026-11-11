# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.2.5] - 2026-11-12

The gateway outbox keeps a cursor per consumer, by the subject of the token it
presented (`gateway_outbox_ack_for`): a record is dropped once every known
consumer has acknowledged it, so a second reader no longer deletes what Mesh has
not stored, and OUTBOX_MAX_ROWS still bounds one that stopped. The outbox, its
cursors and its acknowledgement mark are written owner-only. The ports' own tests
run with the plugin's.

## [0.2.4] - 2026-11-12

The telemetry port keeps a live feed per project. `telemetry_poll` takes the
project's feed and appends the readings it stored, so readings from a gateway
reach a page as they arrive (before, only readings Mesh read itself did);
`telemetry_feed_record` appends one reading Mesh read itself; and
`telemetry_latest_events` gives a project's current values as feed lines, the
snapshot a stream opens with. The feed is written owner-only and starts again
past 256 KB.

## [0.2.3] - 2026-11-12

Ships the `gateway` and `telemetry` ports (`ports/*.go`, a Go module of its own), which the toolchain
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
