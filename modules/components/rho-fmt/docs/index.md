# The source formatter

`rho fmt` parses Rho source with the compiler's own parser and prints it back in
canonical form: one statement per line, one declaration per block, includes and
imports kept with the module they belong to.

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
