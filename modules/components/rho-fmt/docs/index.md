# rho-fmt

The source formatter. It re-parses a file and prints it in canonical form: one
statement per line, one declaration per block, includes and imports kept with
the module they belong to.

```sh
rho fmt src/main.rho
rho fmt -check src/main.rho
```

## Round-trip safety

Formatting a file and parsing the result yields the same tree the file started
from. That is checked over every source in the repository, because a formatter
that loses code is worse than no formatter.

Anything the printer does not understand is emitted as a visible marker rather
than dropped. A comment the formatter cannot place, a construct it does not
know: it is left visible, so the alternative — a file that compiles and is not
what was written — never happens quietly.

## What canonical form means

- One statement per line.
- One declaration per block, with its body indented.
- Imports and includes grouped where the module declares them.
- No trailing whitespace, and a final newline.

## Checking rather than writing

`-check` formats to nothing and reports whether the file was already canonical,
which is what a build asks for: a repository that is not formatted fails, and
nothing is rewritten behind the reader's back.

## The record format

The formatter also owns the shape of a log record, which is why the logger
depends on it: a record written by a device and a record written by a service
are formatted by the same code, so the two cannot drift apart. See
{ref}`components/rho-log` for the record format itself.
