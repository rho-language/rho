# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-10-12

Initial release.

## [0.1.1] - 2026-11-09

Fixed the formatter losing declarations it did not print: a top-level `const`
is written back as a `const`, a fixed array type keeps its length and further
dimensions (`u8[8]`, `u32[2][2]`), `[value; count]` keeps its repeat count, and
`Ptr[T](&x)` keeps its type argument in brackets instead of being rewritten as
a call with the type as the first argument. Local `var` bindings are preserved
rather than rewritten to `let`.

## [0.1.2] - 2026-11-09

The formatter now writes `continue`, `for ... in ...` (with `reverse` and
`step`) and `match`, instead of replacing them with an unprinted-statement
comment. A program that used any of them no longer changed meaning when it was
formatted.

## [0.1.3] - 2026-11-14

`-check` names the files that need formatting and exits 1, and prints no source; it used to print the formatted file. A plain run prints every file it is given, formatted or already formatted, where a file that needed no change printed nothing.

### Breaking

- `rho fmt -check FILE` no longer prints the formatted source: a script that captured it runs `rho fmt FILE` for the source, and reads `-check` only for the names of the files that need formatting and its exit status (1 when any does).
- A plain `rho fmt FILE` prints a file that is already formatted, where it printed nothing: a script that took empty output to mean "already formatted" uses `-check` instead.

## [0.1.4] - 2026-11-14

Enums are printed (they were dropped as an unprinted declaration).

## [0.1.5] - 2026-11-14

Slice cuts (`s[a..b]`, `s[..b]`, `s[a..]`, `s[..]`) are printed.

## [0.1.6] - 2026-11-15

Function types (`fn(A) -> R`, `(fn(u32) -> u32)[4]`) and function literals are printed.

## [0.1.7] - 2026-11-15

Constraints on type parameters (`[T: Ordered + Equal]`) and explicit type arguments are printed.

## [0.2.0] - 2026-11-15

Every comment is kept, in place: the file header, doc comments and their tags, comments above and after statements, fields, variants and match arms, and at the end of blocks; a file whose comments would change is refused and left alone. `test fn`, `namespace`, `message`, `rpc` and struct bit widths are printed (they were dropped). A cast's, call's or member's operand is parenthesised when it needs to be (`((n*37)%600) as i32` was printed as `n * 37 % 600 as i32`, which means something else). Struct literals print as `Point { 0, 0 }`.

### Breaking

None: output changes only where the old output was wrong or dropped code.
