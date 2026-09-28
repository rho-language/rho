# The sqlite engine

The sqlite engine for [`rho-sql`](../../components/rho-sql/README.md). A service
names it; the component reaches it as a session over the plugin protocol, so the
connection is opened once and held.

```sh
RHO_SQLITE_PATH=data/app.sqlite   # where the database is; this default
```

```sh
RHO_SQLITE_PATH=/var/lib/orders/orders.sqlite ./orders
```

The path is read from the environment rather than from a request, because a
database is a deployment decision: a caller that could change it by asking
differently would be a caller that could reach a database it was not given.

## What it is

sqlite, compiled to Go rather than a C library behind cgo
(`modernc.org/sqlite`), so an engine plugin builds wherever a service does and a
deployment needs no C toolchain for it. The version is pinned: a driver that
needs a newer Go than a component is built with cannot be loaded by a toolchain
that predates it, and the failure would arrive at the first statement rather than
at the build.

One writer, one process. A sqlite file is a file, and a second writer to the
same path is a second process holding the same database — so the connection is
opened once and nothing else opens it.

## The dialect, and what this engine does about it

**Placeholders.** sqlite takes `?`, `$1` and `:name`, so a statement written in
any of them runs as written. One written in a marker sqlite does not take is
rendered into `?` and the rewrite is reported — a silent rewrite is a rewrite
nobody can find when a value lands in the wrong place.

**`RETURNING`.** sqlite has had it since 3.35, so a write that asks for the rows
it changed is one statement rather than two, and a caller that inserted a row
learns the id the database gave it in the same call.

**Types.** sqlite is dynamically typed, so a column's declared type is an
*affinity* rather than a type: `INTEGER` is a number, `TEXT` is text, and anything
else is a number. That is what this engine reports, because it is what the
database has, and the component reduces it to what a value is.

**A scaled number is not a float.** `NUMERIC(10, 2)` has an affinity of a number
and a precision of two places. The component carries it as text for exactly that
reason, and this engine hands it back as the digits the database stored rather
than as a float64 that has already lost them.

**`INTEGER PRIMARY KEY`.** An alias for the rowid, so it is an identity the
database assigns. The engine says so rather than pretending a value can be
supplied for it.

## What it refuses

A statement against a table that is not there, and a constraint the service
declared being broken, are reported by kind rather than as a string:

| Kind | What it means |
|------|---------------|
| `not_found` | no such table or column |
| `invalid` | a constraint the service declared was broken |
| `engine` | the database refused it |
| `unsupported` | this engine does not do what was asked |

A caller that cannot tell "no rows" from "the engine refused" reads a failure as
a fact about the data, which is the failure mode a kind exists to prevent.

## Building

```sh
cd modules/plugins/rho-sql-sqlite && go build -o rho-sql-sqlite ./src
```
