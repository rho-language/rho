# rho-sql-sqlite

The sqlite engine for [`rho-sql`](../../components/rho-sql/README.md).

A service names this engine; the component reaches it as a session over the
plugin protocol, so the connection is opened once and held.

```sh
RHO_SQLITE_PATH=data/app.sqlite   # where the database is; this default
```

```go
RHO_SQLITE_PATH=/var/lib/orders/orders.sqlite ./orders
```

## What it is

sqlite, compiled to Go rather than a C library behind cgo, so an engine plugin
builds wherever a service does and a deployment needs no C toolchain for it.

## The dialect, and what this engine does about it

- **Placeholders.** sqlite takes `?`, `$1` and `:name`. A statement written in
  any of them runs as written; one written in a marker sqlite does not take is
  rendered into `?` and the rewrite is reported, because a silent rewrite is a
  rewrite nobody can find when a value lands in the wrong place.
- **`RETURNING`.** sqlite has had it since 3.35, so a write that asks for the
  rows it changed is one statement rather than two.
- **Types.** sqlite is dynamically typed, so a column's declared type is an
  affinity rather than a type. It is reported as the declaration says, because
  that is what a service wrote and what a generated Go type should follow.
- **`INTEGER PRIMARY KEY`.** An alias for the rowid, so it is an identity that
  the database assigns. The engine says so rather than pretending a value can be
  supplied for it.

## What it refuses

A statement against a table that is not there, and a constraint the service
declared being broken, are reported by kind rather than as a string, because a
caller that cannot tell "no rows" from "the engine refused" reads a failure as a
fact about the data.
