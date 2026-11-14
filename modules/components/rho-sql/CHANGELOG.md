# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.2.0] - 2026-11-13

A module that keeps records declares its own `sql:` (a schema and statements, no
driver, no `out`), and every application that has it locked gets its tables and
statements generated in, before the application's own. The module's definition
is the one used: a table or statement an application still repeats is passed
over, with one line naming the copies so they can be removed. Two modules
declaring one statement is an error.

`INSERT OR IGNORE INTO` and its kin (`OR REPLACE`, `OR ABORT`, ...) are read with
their table, as SQLite writes them.

A generated statement's wrapper is an `Err` exactly when the statement failed,
carrying the engine's message (read with the standard library's `sql_failed`),
and an `Ok` with the count otherwise: a read that finds no rows is `Ok(0)`, no
longer "the statement failed", and a write that fails is no longer `Ok(0)`.

A column or parameter named like a keyword of the language (`fn`, `type`, `route`,
`match` ...) is written with a `Value` suffix in the generated Rho (`fnValue`), so
the file parses.

## [0.2.1] - 2026-11-13

Each positional `?` is a value of its own: `UPDATE items SET qty = ? WHERE id = ?`
takes two, each typed by the column beside it, where before every `?` was one
parameter, the generated function took a single argument, and every marker was
given the first one's type. A `?` inside a `--` or `/* */` comment is text.

## [0.2.2] - 2026-11-14

Two modules declaring one table is refused, naming both, where the first silently won. A project statement that shares a module statement's name but differs from it is still passed over for the module's, and the warning says it differs so the project can rename it if it meant its own.
