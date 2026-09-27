# rho-fmt

Source formatter for Rho, and the home of the shared record formatter.

## Source formatting

```sh
rho fmt file.rho          # print the canonical form
rho fmt -w file.rho        # rewrite the file
rho fmt -l *.rho           # list files that differ
rho fmt -check .           # exit non-zero if anything differs
```

Formatting is round-trip safe: formatting a file and parsing the result yields
the same program. The test suite checks this over every Rho source in the
repository, so a printer change that silently drops a declaration, statement or
expression fails rather than eating code.

## Record formatting

`src/logfmt` turns a log record into bytes, identically wherever the record
came from, so an embedded target writing to a UART and a server writing to
stdout agree on the shape without being kept in step by hand:

```
[2026-09-27T05:44:31.412Z] [info] [registry       ] started attempt=1 port=8080
```

A record is a stamp, a level, a subject, a message and fields. Every part is
optional, fields are quoted only when they need to be, and they are sorted so
two runs render identical bytes. `FormatInto` writes into a caller-owned
buffer, which is what lets a device with no allocator format at all.

JSON is available for a pipeline that would rather parse than read.
