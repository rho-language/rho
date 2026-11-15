# The source formatter

`rho fmt` parses Rho source with the compiler's own parser and prints it back in
canonical form: one statement per line, one declaration per block, includes and
imports kept with the module they belong to, and a struct literal written as the
language reference writes it, `Point { 0, 0 }`.

```sh
rho fmt src/main.rho              # print the canonical form
rho fmt -w src/*.rho              # rewrite the files in place
rho fmt -l src/*.rho              # list the files that are not canonical
rho fmt -check src/*.rho          # the same, and exit 1 if there are any
```

## Flags

| Flag | What it does |
|---|---|
| `-w` | Write the result back to each file that differs |
| `-l` | Print the name of each file that differs |
| `-check` | Print the name of each file that differs, and exit 1 when any does |

Without `-w`, `-l` or `-check`, the canonical form of every file is printed to
standard output, whether or not it differs. `-check` prints no source: a check
in CI gets the names of the files to fix and a failing exit code.

A file that cannot be read or parsed stops the run with exit 1 and the parser's
message; no file names at all is a usage error, exit 2.

## Comments

Every comment is kept, `//` and `/* */` alike, where it was written:

- the header above `module`, with its `@file`, `@stability`, `@deprecated`,
  `@backends` and `@port` tags, exactly as it was;
- a doc comment directly above its declaration, with nothing put between them;
- a comment above or at the end of the line of a field, an enum variant, a
  statement or a match arm, at the end of a block, and inside a closure;
- a blank line next to a comment, kept as one, so a comment that stood apart
  from the declaration below it still does.

The printer puts a call, an array or a struct literal on one line, so a comment
written between its elements goes on a line of its own above the statement, in
the order it was written; the comment after its last element stays at the end of
the line. A file whose comments would not come back exactly is refused with
exit 1 and left as it was, rather than formatted without one.

## Test files

A test file's `test fn` declarations are formatted like any function and keep
their marker, so `rho fmt -w tests/*_test.rho` is safe.

## Round-trip safety

Formatting a file and parsing the result yields the same tree the file started
from. That is checked over every Rho source in the repository, because a
formatter that loses code is worse than no formatter.

Anything the printer does not understand is emitted as a visible marker rather
than dropped, so a file that compiles and is not what was written never happens
quietly. Among the constructs it keeps are `const` declarations, fixed array
lengths and dimensions, `[value; count]`, `Ptr[T](&x)`, local `var`, `continue`,
`for … in` with `reverse` and `step`, and `match`.

## The log record format

The formatter also owns the shape of a log record, in its `logfmt` package:

```
[2026-09-27T05:44:31.412Z] [info] [registry] started port=8080 attempt=1
```

a bracketed stamp, level and subject, the message, then the fields as
`key=value`. {ref}`rho-log <rho-log/index>` depends on it, so a record written by
a board and one written by a service are formatted by the same code and cannot
drift apart.
