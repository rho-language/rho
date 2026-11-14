# Types

What a Rho value can be, and how a shape is built out of them.

## The types

| Kind | What it is |
|------|------------|
| `u8` `u16` `u32` `u64` | Unsigned integers, fixed width |
| `i8` `i16` `i32` `i64` | Signed integers, fixed width |
| `f32` `f64` | Floating point |
| `bool` | `true` or `false` |
| `string` | A byte string: a literal, and what the built-in text functions take and return |
| `Unit` | The type of nothing, a generic's default; written `void` at a signature |
| `T[N]` | A fixed array of `N` elements, a value whose length is part of its type |
| `Slice[T]` | A pointer and a length: the sequence type, what a function takes and std uses |
| `[T]` | A bare pointer to elements someone else owns, for C interfaces; it has no length |
| `Ptr[T]` | In an `extern fn` signature, a C pointer; elsewhere a handle carrying an address |
| `Option[T]` | A value that may be absent: the enum `None` or `Some(T)` |
| `Result[T, E]` | A value, or an error: the enum `Err(E)` or `Ok(T)` |
| `struct` | A product type, declared in the module |
| `enum` | One of a fixed set of variants, each with or without values ([enums](#enums)) |
| `fn(A, B) -> R` | A function value: a named function or a closure ([function types](#function-types)) |

There is no map among a program's types; `std.collections.map` is a table
written in Rho.

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is always visible in the source.

## What an integer operation means

An integer operation means the same on every backend, including where C would
leave it undefined:

| Operation | What it gives |
|---|---|
| `+`, `-`, `*`, unary `-` | The result wraps, two's complement, at the type's width: `i32` 2147483647 `+ 1` is -2147483648, `u8` 200 `+ 100` is 44, and the negation of the most negative `i32` is itself (so `abs_i32` of it is too) |
| `<<`, `>>` by a count the type is not wide enough for | 0, and for a signed `>>` the sign (-1 for a negative value). A right shift of a signed value is arithmetic. A count is not converted to the shifted type and a shift's type is its left operand's; a negative count is treated as its unsigned value, which is too large |
| `/`, `%` by zero | The program stops: `rho: division by zero` on standard error and status 1 on a host, the platform's stop on a board |
| The most negative value `/ -1` | Itself, with `% -1` giving 0 |
| `a[i]` on a fixed array `T[N]`, or `s[i]` (and `s.ptr[i]`) on a `Slice[T]` | The program stops when `i` is outside `[0, N)` or `[0, s.len)`: `rho: index 7 out of range [0,4)` |
| `s[a..b]` on a `Slice[T]` or a fixed array | The program stops unless `0 <= a <= b <= len`: `rho: slice [1..5] out of range [0,3]` |

A constant that makes one of these certain is refused at compile time: a shift
by a constant count of at least the width (`shift count 32 overflows type u32`),
a division by a constant zero, a constant index outside a fixed array (or a
constant cut bound past one), and a constant that does not fit the integer type it becomes (`u32(7 / -2)`,
`let b: u8 = 300`, `b + 300` for a `u8` b: `constant -3 overflows u32`).

The checks are cheap. On C each is an inline function beside the call, so with a
constant operand in range it folds to the plain operator, and arithmetic is done
in an unsigned type and converted back, so nothing relies on `-fwrapv` or on a
compiler flag a target might not pass. A bare pointer — `[T]`, or a `Ptr[T]`
indexed directly — carries no length and is not checked, and neither is a
`Slice[T]` read through a `ptr` taken out of it first.

A local declared without a value, `var x: u32` or `var s: Shape`, is the zero
of its type on both backends: 0, `false`, `""`, a struct of zeros, an enum's
first variant, a `Slice[T]` of length 0, a null `Ptr[T]`.

## Option and Result

A value that may be absent is an `Option[T]`, and it is asked about rather than
guessed at:

```rho
fn find(values: u32[4], wanted: u32) Option[u32] {
    var i: u32 = 0
    while i < 4 {
        if values[i] == wanted {
            return Some(i)
        }
        i = i + 1
    }
    return None[u32]()
}

fn main() {
    let readings: u32[4] = [4, 5, 6, 7]
    match find(readings, 6) {
        Some(at) => print(int_to_str(at) + "\n")
        None => print("not there\n")
    }
}
```

An absent value is something a program handles rather than something it trips
over. "Not found" is a real answer, and the type is what makes the program see
it.

A function that can fail says so in what it returns:

```rho
fn divide(numerator: i32, denominator: i32) Result[i32, string] {
    if denominator == 0 {
        return Err("division by zero")
    }
    return Ok(numerator / denominator)
}
```

Both are [enums](#enums) of the standard library, declared in
`std.core.option` and `std.core.result`:

```rho
enum Option[T] { None, Some(T) }
enum Result[T, E = Unit] { Err(E), Ok(T) }
```

`None` and `Err` come first, so a zeroed value — `default(...)`, a field never
set — is `None` or an `Err`, as it was when the two were structs. `Some(x)`,
`None()`, `Ok(x)` and `Err(e)` are still functions, and `Option.Some(x)` or
`Result[u32, string].Err(e)` name the variants directly.

Code written when they were structs keeps working unchanged. Their fields are
kept as read-only accessors of the enum:

| Written | Means |
|---|---|
| `o.is_some` | `o` is `Some` |
| `o.value` | the value of a `Some`; the zero value of `T` for `None` |
| `r.is_ok` | `r` is `Ok` |
| `r.ok` | the value of an `Ok`; the zero value of `T` for an `Err` |
| `r.err` | the error of an `Err`; the zero value of `E` for an `Ok` |
| `Option[T]{ flag, v }` | `Some(v)` when `flag` is true, `None` otherwise |
| `Result[T, E]{ flag, v, e }` | `Ok(v)` when `flag` is true, `Err(e)` otherwise |

They cannot be assigned to — a value of an enum is replaced whole — and new code
takes the value apart with `match` or `if let`.

An `Option` of a pointer, `Option[Ptr[T]]`, is the pointer on C: `None` is
`NULL` and `Some(p)` is `p`, so a C function that answers a pointer or `NULL`
answers an `Option[Ptr[T]]` with no translation.

## Generics

A declaration may take type parameters, and a parameter may have a default:

```rho
fn None[T = Unit]() Option[T] {
    return Option[T].None
}
```

`None()` on its own means an `Option[Unit]`: no annotation, and no ambiguity
about what "no value" holds.

Generic declarations are templates, and they are instantiated rather than looked
up through a table: every application of a declaration is a concrete function, so
a call is a direct, inlinable call and there is no vtable to indirect through.

## Arrays, slices and pointers

There are three sequence types, each with one job.

A **fixed array** `T[N]` is a value: as long as the program declared, with the
length part of its type, so `len(a)` is the constant `N` and a constant index
past the end is refused when the program is compiled. It converts to the other
two where one is wanted: to a `Slice[T]` of all `N` elements, pointing at the
array, and to a `[T]`, the pointer to its first element.

A **`Slice[T]`** is a pointer and a length — `{ T* ptr; uint32_t len; }` in C and
the same two fields in Go — and it is the idiomatic sequence: what a function
takes when it is given elements, what the standard library takes, and what
RoboHub's contracts are written in. The language provides it; no import is
needed. On both backends alike:

| Written | What it does |
|---|---|
| `s.len`, `len(s)` | the number of elements, a `u32` |
| `s[i]` | element `i`, read or written; `i` outside `[0, s.len)` stops the program |
| `s[a..b]`, `s[..b]`, `s[a..]`, `s[..]` | a `Slice[T]` of elements `a` to `b - 1`, over the same storage; unless `0 <= a <= b <= s.len` the program stops |
| `for x in s` | each element in turn (`reverse` and `step` as for an array); `s` is read once, when the loop starts |
| `s = append(s, v)`, `s = append(s, v, w)` | writes each value at `s[s.len]` and lengthens `s`; see below |
| `Slice.from_raw(p, n)`, `Slice[T].from_raw(p, n)` | the `n` elements starting at `p`, a `Ptr[T]` or a `[T]`; nothing checks that they are there |
| `let s: Slice[T] = buf`, `f(buf)` | a fixed array as a Slice of all of it |
| `let s: Slice[T] = [1, 2, 3]` | an array literal as a Slice over storage of the block it is written in |

```rho
fn sum(values: Slice[u32]) u32 {
    var total: u32 = 0
    for v in values {
        total = total + v
    }
    return total
}

fn main() void {
    var raw: u32[6] = [1, 2, 3, 4, 5, 6]
    print(int_to_str(sum(raw)) + " " + int_to_str(sum(raw[2..4])) + "\n")  // 21 7
}
```

A `s[i]` and a cut read the slice twice — its pointer and its length — and a cut
reads its bounds twice, so they are written over names: `f()[i]` and
`s[next()..]` are refused with a request to bind the value first.

**append** has to put the element somewhere, and the C backend has no heap: the
only memory it manages is the string arena, released in bulk where the program's
text makes that safe, and a board's arena is a fixed size. So a slice grows into
the fixed array it was cut from, and nowhere else:

```rho
var buf: u32[16]
var readings = buf[..0]          // empty, capacity 16
readings = append(readings, 7)
readings = append(readings, 8, 9)
readings = readings[..0]         // empty again, the same 16 slots
```

The capacity is the `N` of the array, which the compiler knows because it knows
the array: `append` is accepted on a slice declared in the same function from a
fixed array, starting at its first element (`buf`, `buf[..n]` or `buf[0..n]`),
and afterwards assigned only `append(s, ...)`, a cut of itself from its start,
or a fresh cut of the same array from its start. Appending to a full slice stops
the program with `rho: append past capacity 16` on both backends. Anything else —
a slice that is a parameter, a field, cut from the middle of an array, or
reassigned from elsewhere — would need a capacity the slice does not carry, and
is refused with what to write instead. A function that fills a caller's buffer
takes it as a `Slice[T]`, writes `out[i]` and returns how many it wrote.

A slice points at storage, so it must not outlive it. Returning a `Slice[T]` that
points at the function's own fixed array, at a slice that grew into one, or at
an array literal is refused: on C that memory is gone once the function returns.
Storing such a slice where it outlives the function — in a struct the caller
keeps, through a pointer — is not detected, and is the program's to avoid.

Strings kept in a slice follow the string arena's rules (see {ref}`memory
<language/index>`): a loop whose iterations append or store a string they built
into a slice declared outside it is not released per iteration, and a `while`
loop that does so is reported, a warning on a host and an error on a board.

A **`[T]`** is a bare pointer — `T*` in C, 2 bytes on AVR, matching C's ABI — for
C interfaces, DMA and MMIO buffers, and an unsized pointer handed to a callback.
It has no length, so declaring one, a literal, indexing it (unchecked) and
passing it work on both backends, and nothing that needs a length is offered: a
cut `p[a..b]` is refused, and `len`, `for` and `append` on one stop the build
with the place named and the Slice to use — on every backend, so a program means
the same on each. `Slice.from_raw(p, n)` is how a `[T]` gets a length.

A fixed array's length may be a module constant: with `const MAX: u32 = 32`,
`u32[MAX]` is the type `u32[32]`, in a local, a parameter or a struct field.

## Structs

A struct is a product type, and a value of it is passed whole:

```rho
struct Counter {
    value: u32
}

fn increment(c: Counter) Counter {
    return Counter { c.value + 1 }
}
```

A module's top-level bindings are constants, so state is passed or reached
through a field. That is what makes a function testable: a counter is an
argument, not something a test has to set up around it.

## Enums

An enum is a type whose value is one of a fixed set of variants. A variant may
carry values, by position or by name, or none:

```rho
enum Reading {
    Temperature(i32)
    Switch(bool)
    Labelled { sensor: string, value: i32 }
    Missing
}
```

A variant is built by naming it: `Reading.Temperature(21)`,
`Reading.Labelled { "hall", 4 }` (the values in declared order, as a struct
literal takes them), `Reading.Missing`. It is taken apart with `match`, whose
arms name variants and bind what they carry:

```rho
fn describe(r: Reading) string {
    match r {
        Reading.Temperature(t) => { return "temperature " + i64_to_str(i64(t)) }
        Reading.Switch(true) => { return "on" }
        Reading.Switch(false) => { return "off" }
        Reading.Labelled { sensor } => { return sensor }
        Missing => { return "missing" }
    }
}
```

A pattern is a variant — qualified, `Reading.Missing`, or by its name alone,
since the value says which enum it is — with a pattern for each value it
carries: a name binds the value, `_` ignores it, a literal compares it, and a
variant of another enum tests it in turn (`Slot.Filled(Shape.Circle(r))`). A
struct-like variant may be taken apart by name, binding the fields it lists:
`Reading.Labelled { sensor }`. A name inside a pattern always binds, even one a
constant has; compare against a constant inside the arm. `_` as a whole arm
matches anything.

A match on an enum is checked:

- **Exhaustive.** Every value must be handled — every variant, and every value
  of what a variant carries when an arm tests it (`Switch(true)` and
  `Switch(false)` handle `Switch`). A match that misses one is refused, naming
  the variants: `this match does not handle every variant of Reading:
  Reading.Missing is missing; add an arm for each, or a _ => arm for the rest`.
- **Unreachable arms.** An arm that matches nothing the arms above it leave is a
  warning, and so is a `_` after every variant is handled.

A match that handles every value and whose arms all `return` returns, so nothing
is written after it.

`if let` tests one pattern, binding what it names, with an optional `else`:

```rho
if let Reading.Temperature(t) = sample {
    total = total + t
} else {
    misses = misses + 1
}
```

It is a match of one arm and a wildcard, and is not checked for exhaustiveness.

### Plain enums

An enum whose variants carry nothing is a set of numbers, and may say what they
are — what a C header or a wire protocol agrees on:

```rho
enum Mode { Off, On, Auto = 8, Manual }   // 0, 1, 8, 9
```

A variant without a number is the one before it plus one, from 0; two variants
may not have the same number. The enum is an integer of the narrowest width that
holds them (`u8` here; signed when one is negative), which is what C and Go see.

It is still a type of its own: a `Mode` is not a number and not another enum.
`==` and `!=` compare two of them; ordering and arithmetic are refused. Its number
is written out — `u32(m)`, `m as i64` — and a number becomes one with `Mode(n)`,
which stops the program (`rho: Mode(n): n is not a value of Mode`) when `n` is
none of its numbers; a constant that is none is refused when built. A plain enum
is how a database column or a wire field holds a choice: store `i64(mode)`, read
it back with `Mode(n)`.

Only a plain enum has numbers: writing `= 3` on a variant of an enum that carries
data is refused.

### Generic enums

An enum may take type parameters, as a struct does, and is specialised for each
use:

```rho
enum Pair[T] {
    Both(T, T)
    One(T)
    Neither
}

let p = Pair.Both(4, 5)          // Pair[i32], from the values
let q: Pair[u32] = Pair.One(1)   // Pair[u32], from the declared type
let r = Pair[string].Neither     // written, when nothing else says
```

A variant whose values do not say what `T` is takes it from where it is used —
a declared type, a parameter, a return — or from a default, or is written.

### What an enum is, and what it is not

An enum is a value: assigning, passing or returning one copies the variant and
everything it carries. On C an enum that carries data is a tagged union — a tag
of the narrowest width, and the variants sharing storage — and on Go a struct
with the tag and a field per variant; both are built by the same helper
functions, so the two backends agree. `Option` and `Result` keep the layout their
structs had: the flag (`is_some`, `is_ok`) is the tag, and each value is kept
under its old field name (`value`, `ok`, `err`) — side by side in Go, in an
anonymous union in C — so Go code written against those structs, such as a
port that calls `sql_open`, reads them unchanged. A string a variant carries follows the
arena's rule for any string: kept when the enum is stored outside the loop that
built it, released otherwise.

`==` on an enum that carries data is refused: such a value is compared by taking
it apart. Such an enum has no number, so it cannot be converted to one, stored in
a database column or put on a wire; a {ref}`protocol schema <language/protocol>`
refuses one, and its variants' values go in message fields instead.

## Named numbers

A number a program names without needing a type of its own is a constant, and
`match` compares against it:

```rho
const SHAPE_CIRCLE: u32 = 0
const SHAPE_RECT: u32 = 1

fn label(kind: u32) string {
    match kind {
        SHAPE_CIRCLE => { return "circle" }
        SHAPE_RECT => { return "rect" }
    }
    return "unknown"
}
```

A `match` on a number is not checked for exhaustiveness, so the function returns
after it. An arm's body is an expression or a block, and `return` is a
statement, so an arm that returns is written as a block.

## Function types

`fn(A, B) -> R` is the type of a function taking an `A` and a `B` and returning
an `R`; `fn(A)` returns nothing. Two function types are the same type only when
every parameter and the result are exactly the same: a `fn(i32)` is not a
`fn(u8)`, because the two are called differently, and nothing converts one into
the other.

A value of one is a named function (a generic one where the function type says
which specialisation is meant) or a closure ({ref}`syntax <language/syntax>`).
Generic functions take function values and deduce through them:

```rho
fn map_into[T, U](s: Slice[T], out: Slice[U], f: fn(T) -> U) void {
    var i: u32 = 0
    while i < s.len {
        out[i] = f(s[i])
        i = i + 1
    }
}
```

**How a value is represented.** Every function value is a function that takes
an environment first, and that environment. A closure's body becomes a function
of its own and what it captured is copied into a struct; a named function is
reached through a thunk with no environment. In C a value is an `RhoFn`,
`{ void (*fn)(void); void* env; }`; in Go it is a `func(A, B) R`. Both backends
call through a helper that stops the program when the value holds no function,
with the same words.

**Where an environment lives.** There is no general heap on a board, so a
closure's environment lives in the string arena and follows the rule strings
follow (see {ref}`memory <language/index>`): the compiler treats a function
value everywhere as a value that holds a string. An environment built in a loop
iteration lives until the end of that iteration unless the closure is stored in
something declared outside the loop; one kept by a `let` lives as long as the
string would; one returned from a function reaches the caller, as a returned
string does. A loop that keeps the closures it makes is not released, and on a
board that is refused as it is for strings. What makes this sound is what a
closure may capture: values only, never a reference into a frame (a pointer, a
`[T]`, a `Slice[T]`), and nothing it can change after it was made. A call
through a function value is counted as building strings, because its body is
not known where it is called.

A function value cannot be compared (`==` is refused) or printed, and cannot
take or return a fixed array — use a `Slice[T]`.

## Foreign types

A type a C header declares is a Rho type by name, and the pointer to it is a
`Ptr`:

```rho
extern fn get_gpio(name: string) Option[Ptr[BoardGPIO]]
```

Nothing about the header is written out by hand: {ref}`rho-cimport
<rho-cimport/index>` generates these declarations from the headers the project
named, so a header that changes changes them with it. An opaque C struct — one
the program may hold but not read — becomes a name the program can pass around and
nothing else, which is the same thing the header's own author intended.

## Where to go next

- {ref}`syntax <language/syntax>` — how these are written, closures included.
- {ref}`stdlib <language/stdlib>` — what the standard library is built from.
