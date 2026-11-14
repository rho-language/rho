# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.5.1] - 2026-11-13

The gateway port's serial link is opened again when it goes away — a port that
was not there at start, a board on USB that re-enumerated when it reset — every
two seconds at most, instead of once at start for good. A failed read or write
closes it; a failure to open is said once, not on every try. A read and a write
each keep the file they started with, under a lock.

## [0.5.0] - 2026-11-13

The mqtt port: `mqtt_publish` (QoS 0 or 1, the connection kept for the next
message), `mqtt_follow` (a filter followed in the background, reconnecting, the
newest message of every topic kept and optionally appended to a file as JSON
lines), `mqtt_latest` and `mqtt_topics`. MQTT 3.1.1 with the standard library; a
broker user's password is read from a file.
`mqtt_relay` publishes every line appended to a file to one topic and
`mqtt_follow_lines` appends every payload heard on a filter to a file as it came,
so a file of lines crosses the broker unchanged (Mesh's live feed).

## [0.4.2] - 2026-11-13

The gateway port dials a parent: `gateway_parent_start`, `gateway_parent_send`,
`gateway_parent_next` and `gateway_parent_connected` let a gateway in relay mode
reach its parent's uplink, redial when it drops, and carry COBS frames both ways,
with a generation per connection so a session is opened again on a new one. The
telemetry port moves a source's history between projects
(`telemetry_move_source`), so a board that moves to another mesh keeps it.

## [0.4.1] - 2026-11-12

`gateway_uplink_reply` writes a frame back down the uplink connection a frame
arrived on, so a gateway can answer a session to whoever dialled in — a board on
Wi-Fi or a relay — before it has announced itself. `gateway_uplink_take` hands back a queued frame
and its connection in one call, so concurrent drains cannot cross them.

## [0.4.0] - 2026-11-12

A series is one field of one source, and a source is a device by the identity it
keeps for its life, not by its area and node. The telemetry port keys readings,
latest values and rollups by source, records each source's placement in
`telemetry_source`, and migrates a store keyed by area and node in place, keeping
every series under `legacy:<area>.<node>` until `telemetry_source_bind` places a
source there and adopts it. A reading without an identity is placed by area and
node; a board reporting from another source's place is a fault, never a merge.
The gateway outbox carries the board a reading came from (`source`), through
`gateway_outbox_append_from`. The telemetry functions take a source where they
took an area and a node.

## [0.3.0] - 2026-11-12

An integration plugin by the toolchain's contract. Its `integration:` section
declares the ecosystem it bridges (`robohub`), the workspace variable its manifests
use (`ROBOHUB_WORKSPACE`), and what it provides by kind: the `protocol-bus-c` and
`protocol-transport-rho` bindings and its Go ports. The toolchain reads them through
its integration resolver rather than by looking in this plugin's folders, and
`rho module validate` checks the plugin against the contract.

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

## [0.5.2] - 2026-11-14

The telemetry port is gone from here: Mesh's telemetry store is Mesh's, and lives in its repository (`plugins/mesh-telemetry`), versioned with Mesh. Needs rho 0.8.2, which reads a project's own plugins for ports.

## [0.5.3] - 2026-11-14

The gateway port's uplink has limits: a number of connections (`UPLINK_MAX_CONNECTIONS`, 32), an idle timeout (`UPLINK_IDLE_TIMEOUT_S`, 300 s), a cap on what one connection may send without a frame end, a queue cap and write deadlines; `UPLINK_BIND` falls back to the host of `RHO_HTTP_ADDR`. Refusals are logged at most once a minute. A serial read times out as it says, so a gateway's background work runs on a quiet line.
