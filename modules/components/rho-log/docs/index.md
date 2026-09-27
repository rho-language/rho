# rho-log

Structured logging for Rho components.

```sh
rho log -level info -subject registry -message started -field port=8080
```

```text
[2026-09-27T06:27:11.826Z] [info] [registry] started port=8080
```

A record is a timestamp, a level, a subject and a message with fields. The
subject is what says who wrote it, and it is aligned so a column of records
reads as a column.

## Levels

`trace`, `debug`, `info`, `warn`, `error`, `fatal`. The names are the library's
own: a project that logs through this component and a service that logs through
the port write the same shape of record, so one reader can read both.

## The command

```sh
rho log -format text|json -level info -subject registry -message started -field port=8080
```

| Flag | What it does |
|------|--------------|
| `-format` | `text` or `json` |
| `-level` | The level, as named above |
| `-subject` | Who is writing the record |
| `-message` | The message |
| `-field` | A `key=value` field, repeatable, comma-separated |
| `-no-level` | Omit the level, for a compact line |
| `-stamp` | A pre-rendered timestamp, for a source with no clock |

## Using it from Rho

A project declares the component and asks for the `logemit` port:

```yaml
deps:
  rho-log: ">=0.1.0"
```

```yaml
build:
  targets:
    - id: go
      ports:
        - logemit
```

and calls the library:

```rho
import rho.log

fn main() void {
    info("registry", "started");
}
```

The port supplies the implementation and writes the record in the shape the
formatter produces, which is why a device writing to a serial console and a
server writing to standard output cannot drift apart.

## Where the records go

Standard error carries the process's own records, which a service manager
already collects. An application log is for the program's own domain events —
what it decided and why — and `RHO_LOG_FILE` sends them to a file as well, so
they survive independently of whoever captured the process output.

A log that cannot be opened never stops the program: the record still goes to
standard error, and the reason is reported there.
