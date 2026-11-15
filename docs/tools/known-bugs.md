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
| 25 | compiler: diagnostics | A mistake in an included file was reported against the file that included it: `let y: u32 = "no"` on line 4 of `helper.rho`, included from `main.rho`, printed `main.rho:…: cannot assign string to u32`. A node did not keep the file it came from once included files were merged into one module, and a report used the file being built. | A position carries its file (`ast.Position.File`, set by `parser.ParseFile`); every diagnostic prints the file, line and column of the expression that is wrong, through includes (nested), imports, the standard library and the refusals made after lowering. | `TestBrokenProgramsAreReportedWhereTheMistakeIs` (`included`, `nested`, `imported`, `stdmisuse`, `crefusal`, `arena`, `included_parse`) |
| 26 | lexer | Every column on a line after the first was one too far: the newline was counted as column 1 of the next line, so `fn` at the start of line 3 was at 3:2. | The newline is column 0, and the first character of every line is at 1. | `TestATokenSaysWhereItIs`, `TestParseErrorCarriesPosition` |
| 27 | rho-lsp | Diagnostics were published one line low (the compiler's lines count from one, the protocol's from zero), and a file was checked on its own, so a file that imported anything was not type-checked as the build checks it. | The server checks the file as part of its program (`rho.DiagnoseInProgram`) and converts to the protocol's coordinates. | `TestTheEditorPublishesWhatTheBuildReports`, `TestAnIncludedFilesErrorIsPublishedForThatFile` |
| 28 | parser | A token kind was printed as a number: `expected 34, got 30 ("{")`. | Kinds have names: ``expected `:`, got `{` ``. | `TestBrokenProgramsAreReportedWhereTheMistakeIs` (`parse`) |
| 29 | Go backend | An if expression whose condition is a call ending in one name, `if is_none(o) { "a" } else { "b" }`, lost the call in Go: the condition was written before ` {`, and the translator read `(o) {` as a compound literal, giving `isNoneo`. | The condition of the rewritten conditional is parenthesised. | `TestATypedConditionalKeepsACallAsItsCondition`; parity: "enums: Option and Result are enums…" |
| 30 | type checker | `.value` of an `Option[Ptr[T]]` was typed `Ptr[u8]` whatever it pointed at, so `use_thing(found.value)` was refused for a `Ptr[Thing]` parameter. | It is the pointer the Option holds. | `TestAnOptionalPointerIsStillThePointer` |
| 31 | build | The prelude scan loaded `std.core.option` only for an outermost `Option[...]`: `Box[Option[u32]]`, `Option.None`, and a `match` or `for` body were not looked into, so Option was the built-in placeholder and its functions undefined. | Every application in a written type, a variant named by its enum, and every statement are scanned. parity: "enums: Option named only inside an application, and enum constants" |
| 32 | rho-fmt | An `enum` was printed as `/* unprinted declaration */`, so formatting a schema dropped its enums. | Every enum form is printed, and `if let` and `Option[u32].None` with it. | `TestFormatKeepsEnumsAndPatterns`, `TestFormatterPreservesEveryRepoSource` |
| 33 | C backend | A local declared without a value, `var x: u32` or `var s: Shape`, was uninitialised in C and zero in Go, so reading it before it was assigned printed different things (a garbage number; for an enum, an arbitrary variant). | It is initialised to the zero of its type: `0`, `""` for a string, `NULL` for a pointer, `{0}` for an array, a struct or an enum. | parity: "a local declared without a value is zero on both backends" |
| 34 | type checker | An element of a fixed array of strings, `names[i]` of a `string[4]`, was typed `u8` — the byte of a string — so it could not be compared with a string or passed as one. | The length of an array is asked about before its kind. | parity: "a local declared without a value…", "slices: strings built in loops…" |
| 35 | runtime (C) | `rho_str_eq` answered false when either side was `NULL`, the zero of a string field in C, while Go's zero string `""` equals `""`. | A `NULL` string is the empty string there, as everywhere else in the runtime. | parity: "a local declared without a value is zero on both backends" |
| 36 | compiler: generics | A local of a type parameter in a generic body, `var out: T`, was declared as `T` in the C and the Go of every specialisation. | It is declared as the type the specialisation substitutes. | parity: "slices: a generic over Slice[T], a Slice field, and a module-level array" |
| 37 | type checker | A fixed array whose length is a module constant, `var buf: u8[MAX]`, was not a fixed array: it could not be cut, did not become a `Slice[T]`, and in a struct field it was emitted as a single element (`items uint32`). | The constant's value is the length wherever the type is written. | parity: "arrays: a length named by a constant, in a local, a parameter and a struct field" |
| 38 | C backend | A struct field of a fixed-array type, `items: u32[4]`, was declared `uint32_t[4] items;`, which no C compiler accepts. | The length follows the name: `uint32_t items[4];`. | parity: "arrays: a length named by a constant…" |
| 39 | type checker | A member of a struct named by a generic application with a nested one in it, `Pair[Slice[u8], u32]`, or a function type, `Slice[fn(u32) -> u32]`, read its type arguments by splitting at every comma and resolved none of them, so `s[i]` of a `Slice[fn(u32) -> u32]` was typed as a struct named `fn_u32_u32`. | The arguments are split at the top level and a function type among them is resolved. | parity: "closures: arrays, a Slice and an Option of functions…" |
| 40 | rho-lint | An import used only as the module of a type, `fn f(r: alert.Rule)`, was reported as unused: the rule recorded the type's name `alert.Rule` and not its module. | A dotted type name counts its module as used, and a function type counts every type it names. | `TestRhoRulesReadClosuresAndFunctionTypes` |
| 41 | type checker | A generic function named where a value goes, `return None` for `None()`, was typed as a placeholder every struct type accepted, and the build failed in the C and Go compilers on a name nothing declared. | It is refused where it is written, with the call to write instead. | `TestClosuresThatCannotBeHonouredAreRefused` ("a generic function named where…") |
| 42 | C backend | `<`, `<=`, `>` and `>=` between two strings compared their addresses on C and their bytes on Go, so `"b" < "a"` could be true on C. | Both compare the bytes as unsigned values, through `rho_str_cmp` on C. | parity: "generics: Ordered on numbers and strings…", "std.collections.slices: sort over Ordered…" |
| 43 | compiler: generics | A cast to a type parameter in a generic body, `x as T`, was emitted as a cast to the name `T` in every specialisation. | It is a cast to the type the specialisation substitutes. | parity: "generics: Integer remainder and bitwise operators, conversions…" |
| 44 | compiler: generics | A local of a type parameter, `let key: T = s[i]`, was the name `T` to what was lowered after it in a specialisation, so an operation on it was lowered for no type: a `<` or `==` on a string compared addresses on C, and a `+` on an i32 was C's `+`, which overflows. | The local has the type the specialisation substitutes. | parity: "std.collections.slices: sort over Ordered…", "generics: Number on every integer width…" |
| 45 | compiler: generics | A generic function returning a call of another, `return add(a, b)`, deduced the inner call's parameter from its own unsubstituted result type, so it called a specialisation `add_T` nothing could compile. | The result type a return is deduced from is the specialised one. | parity: "generics: a constrained generic calls another…" |
| 46 | type checker | A fixed array of a plain enum, `Mode[2]`, was not taken where a `Slice[Mode]` is, as every other fixed array is. | It converts to a Slice of its elements. | parity: "std.collections.slices: sum, contains and index_of…" |
| 47 | type checker | A function named like a C type word, `fn double(n: u32) u32`, was accepted and the C build failed on it, while a parameter or a local of that name was refused with a reason. | A function of that name is refused the same way, on every backend, at the declaration. | `TestWhatCCannotCompileIsRefusedByTheChecker` |
| 48 | type checker | `==` between two structs was accepted and emitted as C's `==`, which C refuses for a struct, while Go compared field by field. | It is refused on every backend, saying to compare the fields. | `TestWhatCCannotCompileIsRefusedByTheChecker` |

## Open

An open entry is a row of `#`, where, what is wrong and a reproduction.

| # | Where | What is wrong | Reproduction |
|---|---|---|---|
| 49 | rho-fmt | `rho fmt` prints a file from its syntax tree and drops every comment, so formatting a file loses its documentation and its `@file`, `@stability` and `@deprecated` tags. Blocks beta. | `rho fmt -w` on any `std/` file |

A case added to the cross-backend suite with `diverges:` is a new open entry
here.

Limitations, recorded so they are not mistaken for bugs: `std.collections.map`
hashes its key through `u32(key)`, so a key must convert to one; a Go platform
port could in principle supply the `*_hw_*` functions, but the embedded modules
are tagged for C and a Go build is refused; there are no `<<=`, `>>=` or `%=`
operators; a compound assignment evaluates its target twice, which matters only
for a target with a call in it; an index into a `[T]` or a `Ptr[T]` is not
checked.

Limitations of generics (Phase 3d): a type parameter is constrained only by
the language's constraints (`Integer`, `Float`, `Number`, `Ordered`, `Equal`),
not by functions a type must have — Rho has no overloading, so `to_string(x: T)`
could name one function only; pass a function value (`fn(T) -> string`)
instead; a constraint is written on a function's type parameter, not a
struct's or an enum's; arithmetic and ordering on an unconstrained parameter
are refused, but `==`, `!=` and a conversion such as `u32(x)` are accepted on
one as they were before constraints, and are checked only by the C and Go
compilers at each specialisation (`T: Equal` checks `==` at the call); a shift on
a type parameter is refused; a written number meeting a `T` is an integer from 0
to 127 (to 2^24 for a parameter that is only `Float`), anything else is written
`T(...)`; both sides of an operator are the same parameter; a call writes all of
its type arguments or none (those with defaults may be left out), and does not
deduce the ones it leaves out; a type argument is deduced from what a call
passes and from the type it is expected to have, not from how its result is
used later; an enum cannot hold itself, even through a type parameter.

Limitations of closures (Phase 3c): a closure captures by value and cannot
assign to what it captured or take its address; it cannot capture a pointer, a
`[T]`, a `Slice[T]` (or a struct, array or enum holding one), because what it
points at may be the frame that made the closure, nor a fixed array, which C
cannot copy into an environment — pass those as parameters, or wrap an array in
a struct; a closure cannot name itself, so recursion goes through a named
function; a function type cannot take or return a fixed array, and an array of
functions is written `(fn(...) -> R)[N]`; a generic function is a value only
where a function type says which specialisation is meant or when its type
arguments are written (`identity[u32]`), and a built-in such as
`print` never is; function values cannot be compared or printed; an environment
lives in the string arena under the string rule, so a loop that keeps the
closures it makes is not released (refused on a board, as for strings), and a
call through a function value counts as building strings because its body is
not known where it is called; on C every function value is one type, `RhoFn`,
whose signature the checker alone enforces.

Limitations of enums (Phase 3a): a value written inside a struct literal does not
take its type from the field (`Box[Option[u32]]{ Some(7) }` builds an
`Option[i32]`; write `Some[u32](7)`) — the same as any generic value in a struct
literal; `Option` and `Result` each declare a generic `value` and `unwrap`, and
when a program imports both the first declared wins for the unqualified name
(take the value apart with `match`, or use `.value` / `.ok`); a name inside a
pattern always binds, so `Circle(LIMIT)` binds `LIMIT` rather than comparing with
the constant; there are no or-patterns (`A | B`) and no guards; an arm's body is
one expression or a block; `print` does not take an enum, and there is no
variant-name function; `==` on an enum that carries data is refused rather than
compared field by field; a variant is always written after its enum's name
outside a pattern.

Limitations of slices (Phase 3b): `append` grows a slice only into the fixed
array it was declared from in the same function, starting at its first element,
and is written only as `s = append(s, v)`; a slice that is a parameter or a field
is not grown, and a function filling a caller's buffer writes `out[i]` and
returns a count. `s[i]` and a cut need the slice (and a cut's bounds) to be names
or arithmetic over them, since each is read twice. A returned Slice pointing at
the function's own array or at an array literal is refused, but a slice of a
local array stored into something that outlives the function is not detected.
There is no `for i, x in s`. `Slice[T].len` is a `u32` (`uint32_t` in C), not a
`size_t`: the language has no `usize`, and every existing use compares it with a
`u32`. `len`, `for` and `append` on a `[T]` are refused on every backend; a
Go program that builds a file of a size it learns at run time cuts it from a
fixed array large enough for the largest it accepts (robohub-pico's `elf2uf2`
does, for 16 MB of flash).

Diagnostics, recorded so they are not mistaken for bugs: a diagnostic carries
where the mistake begins and not where it ends, so the editor underlines the
token at that place; the refusals made after lowering (a C build, a target that
cannot supply a module, a board's string arena) are reported only for a program
the checker accepts; the language server publishes a file's diagnostics when
that file is opened or changed, so an included file's error is not shown until
it is open (unless the file does not parse, which the including file is told
at its `include` line).
