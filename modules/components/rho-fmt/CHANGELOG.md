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
