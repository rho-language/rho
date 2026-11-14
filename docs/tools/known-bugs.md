# Known compiler bugs

Every bug found in the compiler, the runtime fragments and the standard library
is listed here with its status, so a fix can be checked and a bug that stays
open stays visible. A fixed entry names the test that keeps it fixed. An open
entry carries a reproduction.

The cross-backend suite (`TestBackendsAgree`, `src/parity_test.go` and the
`src/parity_std_*_test.go` files) builds each program for C, with
AddressSanitizer and UndefinedBehaviorSanitizer, and for Go, and requires the
same output. A case listed there with `diverges:` is a known divergence and
appears below as open.

## Fixed

| # | Where | What was wrong | Fix | Kept fixed by |
|---|---|---|---|---|
| 1 | compiler: binary operators | A literal with a fraction meeting an integer (`raw / 65536.0`, `n * 0.5`, `n > 6.5`) was left untyped. C converted the integer to `double`; Go converted the constant to the integer's type, so the division was an integer division, or the build failed. | The integer operand is converted to `double` explicitly. | parity: "a literal with a fraction meeting an integer…", "std.math.fixed: Q16.16 and Q8.8 arithmetic" |
| 2 | std.math.fixed | `q16_mul` and `q8_mul` multiplied two `i32` raws in 32 bits: undefined behaviour in C, a wrapped wrong value in Go. | The product is formed in `i64`. | parity: "std.math.fixed: a product wider than 32 bits" |
| 3 | parser | A value argument in an expression (`Array[u32, 4]{ ... }`, a call with `[T, 4]`) did not parse, though it did as a type. | A number after the first argument is a value argument in both places. | parity: "std.collections.array…" |
| 4 | compiler: generics | A value parameter (`N` of `Array[T, N]`) used in a generic body was emitted as the name `N`, and a user generic using one was refused as "undefined name". | `N` is the number it was specialised with, and a `u32` while the body is checked. | parity: "std.collections.array…", "generics: arrays of a specialisation…" |
| 5 | compiler: generics | An array of a generic struct, `Entry[u32, u32][4]`, was read as an application with a third argument, so the element type was `Entry_u32_u32_4` with a field of type `u32_4`. | The trailing length is split off first, in type resolution, declarations, scanning and the C spelling. | parity: "std.collections.map…", "generics: arrays of a specialisation…" |
| 6 | compiler: generics | A specialised struct's field `Ptr[Entry[K, V]]` was emitted with an unmangled element (`Entry_u32_ u32*`). | The element is resolved to its specialisation. | parity: "std.collections.map…" |
| 7 | compiler: generics | Outside a generic body, a member reached through a pointer field of a generic struct kept its parameters (`Pair_T_T`). | Substitution reaches into a written application. | parity: "generics: arrays of a specialisation…" |
| 8 | std.collections.bitset | `BitSet[N]` held its words by value, so `set` and `clear` changed a copy; and `BitSet[2]` reads as an array of two BitSets, so the type could not be written. | `BitSet` is a pointer to the caller's words and a count (`bitset(&words[0], n)`), with `capacity`; an index past the end is outside the set. **API change.** | parity: "std.collections.bitset…" |
| 9 | std.core.builtin, compiler | The module redeclared the primitive types (`type u32`) and declared foreign functions nothing implements; importing it made `int_to_str(u32(n))` a type error ("has type u32, want u32"). | The module declares nothing and documents the built-ins; declaring a primitive type again is refused. | parity: "std.core.builtin: importing it changes nothing"; `TestAPrimitiveCannotBeDeclaredAgain` |
| 10 | runtime (C) | A hosted C panic wrote its message to standard output without the `rho: ` prefix; Go writes `rho: <msg>` to standard error. The same program printed different output on the two backends. | A hosted C panic writes `rho: <msg>` to standard error. A board still prints on its console. | parity: "std.core.result: unwrap of an Err stops the program…" |
| 11 | C project generator | `app_init` called a bare `init()` when any module — `std.embedded.uart` — declared an `init`, which is a link error. | Only the program's own `init` is called. | `TestEmbeddedDriversOnAStubBoard` |
| 12 | C backend | The standard library's foreign functions (`gpio_hw_init` …) had no prototype, so each call was an implicit declaration: an error in C99 and GCC 14, and a wrong return type for anything not an `int`. | Their prototypes are emitted; a platform implements that signature. | `TestEmbeddedDriversOnAStubBoard` (built with `-Werror=implicit-function-declaration`) |
| 13 | Go server generator | A server target without template files had no `templateFill`, `escapeHtml` or catalog functions, so `std.web.template`'s `fill` and `escape` did not build. | A server always carries the renderer and the (empty) catalogs. | `TestStdWebOnTheGoServer` |
| 14 | build | A program importing a module, or calling a built-in, its target cannot supply compiled and then failed in a C link (`undefined reference to get_args`) or a Go build (`undefined: stdSqlSqlOpen`). | Modules say where they work (`@backends`, `@port`); built-ins are in a table; the build refuses by name first. | `TestAModuleOrBuiltinIsRefusedByNameOnABackendThatCannotSupplyIt` |
| 15 | std.sql (documentation) | It said a statement's columns could be read after `sql_prepare`; through `database/sql` they are known after the first `sql_run`. | The comment and the stdlib page say so. | `TestStdSqlThroughTheSqlPort` |
| 16 | std.core.range | `next` answered `Some(i)` for an `i` below the start, and the module named `Option` without importing it, written `Option<u32>`. | `Some(i)` only for `start <= i < end`; the import and the bracket spelling. | parity: "std.core.range…" |
| 17 | type checker | A constant conversion that does not fit, `u32(7 / -2)`, built on C (wrapping) and failed to build on Go; so did `let b: u8 = 300` and `b + 300` for a `u8` b. | A constant that does not fit the integer type it becomes is refused on both (`constant -3 overflows u32`), at a conversion, a `let`, an assignment, a return, an argument and an operand. | `TestConstantOperationsThatCannotBeRightAreRefused` |
| 18 | language semantics | Signed overflow, a shift by the type's width, the most negative value divided by -1, an index past a fixed array and division by zero were undefined behaviour in the generated C and defined (wrap, 0, wrap, panic, panic) in Go; `abs_i32` of the most negative `i32` was the same case. | Rho defines them ({ref}`language/types`): arithmetic wraps; an over-wide shift gives 0 or the sign; a division by zero and an index outside a fixed array or a `Slice[T]` stop the program with one message on both. C lowers each to an inline helper (`rho_add_i32`, `rho_shl_u8`, `rho_div_i64`, `rho_index`) that computes in an unsigned type and folds a constant away; Go uses its operator where it already agrees and a runtime function where it does not. A constant shift count of at least the width, a constant zero divisor and a constant index out of range are refused. | parity: the four former divergence cases and `src/parity_semantics_test.go`, built with UBSan; `TestConstantOperationsThatCannotBeRightAreRefused` |
| 19 | compiler | A shift's operands were widened to a common type (`u8 << u32` computed in 32 bits) while the checker typed the shift as its left operand, so `u8` 1 `<< 8` was 256. | A shift is not promoted: its type is the left operand's and the count is only a count. | parity: "a shift by the width of the type or more…" |
| 20 | compiler | `x += v` was emitted as C's `+=`, which overflows as `+` does. | A compound assignment on an integer is lowered as `x = x op v`. | parity: "signed overflow wraps…" |
| 21 | C backend | `-x` and `~x` on a `u8`, `u16`, `i8` or `i16` were computed in C's `int`, so `u32(-k)` for a `u8` k was not the `u8` negation Go gives. | Negation goes through the wrapping helper and a complement is converted back to the type. | parity: "signed overflow wraps…" |
| 22 | type checker | The standard library's function bodies were never type-checked (`checkFunc` skipped `std.*`). Checking them found `std.sql.ok`, which passed the message string as the caller's error type `E`. | Std bodies are checked when their module is loaded. `ok` is `ok[T](value, code, message) Result[T, string]`. **API change** (nothing in the tree called it). | `TestEveryStandardLibraryBodyTypeChecks`, `TestAStandardLibraryBodyIsTypeChecked` |
| 23 | C backend | A program's own `extern fn` got no prototype, so without a header each call was an implicit declaration, which GCC 14 refuses. | Its prototype is emitted unless a connected library's headers or the rho-cimport shim declare the program's externs; one the C library or the runtime declares is left to that declaration. | `TestAProgramsOwnExternIsDeclaredForTheCCompiler`, `TestAProgramsOwnForeignFunctionIsDeclaredWhenNoHeaderDoes` |
| 24 | runtime | `abort()` stopped C with SIGABRT (status 134) and Go with status 1. | `abort()` is `panic("aborted")` on both: `rho: aborted` on standard error and status 1 (a board's platform stop). | parity: "abort stops with the same status and message…", and every failing case, which now compares status and standard error |

## Open

None. A case added to the cross-backend suite with `diverges:` is a new open
entry here.

Limitations, recorded so they are not mistaken for bugs: a generic body cannot
do arithmetic on a type parameter (`T + T` is refused); `std.collections.map`
hashes its key through `u32(key)`, so a key must convert to one; a Go platform
port could in principle supply the `*_hw_*` functions, but the embedded modules
are tagged for C and a Go build is refused; there are no `<<=`, `>>=` or `%=`
operators; a compound assignment evaluates its target twice, which matters only
for a target with a call in it; an index into a `[T]` or a `Ptr[T]` is not
checked.
