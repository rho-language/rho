# Syntax

Every keyword and construct in the language, each with an example. Every word
the lexer reserves has syntax behind it, so a reader is never sent to a form
that does not parse.

Examples here are complete programs unless a section says otherwise.

## Source files, and the top of a file

A file begins with the module it belongs to. The name is dotted, and it is the
name an import uses:

```rho
module std.core.option
```

After the module, and before any declaration, a file may carry `namespace`,
`feature`, `include` and `import`. They are read only in that order; a
declaration ends the preamble.

| Form | What it does |
|---|---|
| `module a.b.c` | names the module; required, first |
| `namespace acme.sensor.v1` | names a protocol schema's wire namespace (schema files) |
| `feature "math"` | turns on a manifest feature; may repeat |
| `include "path.rho"` | splices another file's declarations into this module |
| `import std.core.ptr` | brings in a module, keeping its names separate |

`include` splices, so the included file declares a module of its own and its
names join the including module without a prefix:

```rho
module app
include "extra.rho"

fn main() void {
    print(int_to_str(greet(1)))
}
```

```rho
module app.extra

fn greet(n: u32) u32 {
    return n + 1
}
```

An include that is a bare name, with no `/` and no `.rho`, names a module rather
than a file: `include "forms"` is the entry of the module called `forms`. That is
a dependency the manifest declares, matched by its module name or its folder,
or else a module in the same repository under `modules/`, `plugins/`,
`providers/` or `contracts/`. The entry is the `main:` the module's manifest
declares, so a module never names a path into another one.

An `extern` declares a foreign function. Two included files may each declare the
same one, because a module that is to build on its own says what it calls; the
program has one function. Two declarations of one name with different types are
refused.

`import` brings in another module and keeps its names its own. Each module's
code resolves a name in its own declarations first, so two imported modules may
both declare `init` and each calls its own. A function of an imported module is
called by the last segment of the module's name:

```rho
module app
import rho.log
import app.storage

fn main() void {
    log.info("app", "starting")
    storage.open("data")
}
```

The bare name of an imported function still resolves when only one module
declares it, which is how programs written before qualified calls keep
building. A qualified call to a module that was not imported, or to a function
the module does not declare, is refused with what is wrong.

A module's structs, enums, type aliases and constants are its own in the same
way. Two imported modules may each declare a `Config` and a `LIMIT`, and an
application may declare its own `Config` beside them; inside each module the
name is that module's. Another module's type or constant is written with the
module's name in front of it, in a type, a literal or an expression:

```rho
module app
import app.storage

fn main() void {
    let options: storage.Options = storage.Options{ storage.DEFAULT_SIZE }
    storage.open("data", options)
}
```

The bare name works when the importing code does not declare it and only one
imported module does; when two do, the compiler says so and names both
qualified spellings. The standard library's types — `Request`, `Option` — are
the language's own and are written bare.

## Functions

A function names its parameters and what it returns. A function that returns
nothing says `void`.

```rho
fn divide(numerator: i32, denominator: i32) i32 {
    if denominator == 0 {
        return 0
    }
    return numerator / denominator
}
```

A generic function names its type parameters in brackets, and they may have
defaults:

```rho
fn identity[T](value: T) T {
    return value
}

fn first[T](items: Slice[T]) T {
    return items[0]
}
```

A parameter may carry a constraint after a colon, several joined by `+`, before
any default; the constraints and what each allows are in
{ref}`types <language/types>`:

```rho
fn clamp[T: Ordered](x: T, lo: T, hi: T) T {
    if x < lo {
        return lo
    }
    if x > hi {
        return hi
    }
    return x
}

fn count_equal[T: Ordered + Equal](items: Slice[T], x: T) u32 {
    var n: u32 = 0
    for v in items {
        if v == x {
            n = n + 1
        }
    }
    return n
}
```

`extern fn` declares a function the host supplies rather than one the program
writes. Its body is absent. The runtime implements the `rho_*` functions
({ref}`built-in functions <language/builtins>`); a port
({ref}`building/ports`) or the platform implements the rest:

```rho
extern fn rho_read_text(path: string) string
```

`main` is the entry point. A `main` declared `void` is emitted as one that hands
back success, because the host expects a value:

```rho
fn main() void {
    print("Hello\n")
}
```

Recursion is ordinary:

```rho
fn factorial(n: u32) u32 {
    if n <= 1 {
        return 1
    }
    return n * factorial(n - 1)
}
```

### Functions as values, and closures

A function type is written `fn(A, B) -> R`, or `fn(A)` for one that returns
nothing. A named function is a value of its type wherever one is wanted, and a
value of a function type is called like a function:

```rho
fn twice(x: i32) i32 {
    return x * 2
}

fn apply(f: fn(i32) -> i32, v: i32) i32 {
    return f(v)
}

fn main() void {
    print(int_to_str(apply(twice, 21) as u32))   // 42
}
```

A function literal — a closure — is written like a declaration without a name,
its result after `->` or not at all:

```rho
fn make_adder(k: i32) -> fn(i32) -> i32 {
    return fn(x: i32) -> i32 { return x + k }
}

let add3 = make_adder(3)
let say = fn(m: string) { print(m) }
```

A closure **captures by value**: each local of the enclosing function it names
is copied when the literal is evaluated, so a later change to the variable is
not seen, and the closure cannot assign to a captured name or take its address
(declare a local copy inside it instead). A literal may not capture a pointer, a
`[T]`, a `Slice[T]` or a struct holding one — what it points at may belong to
the function that made the closure, which a returned closure outlives — nor a
fixed array; pass those as parameters. Each is refused where it is written, with
what to write instead.

A function type may be a parameter, a result, a local, a struct field, the
element of a `Slice[T]` and the payload of an `Option`. An array of functions
puts the type in parentheses, `(fn(u32) -> u32)[4]`, because
`fn() -> u32[4]` would be a function returning an array, which is refused. A
generic function is a value where a function type says which specialisation is
meant (`let f: fn(u32) -> u32 = identity`), or when it is named with its type
arguments (`let f = identity[u32]`); a built-in such as `print` is not a
value, and is wrapped in a literal. Calling a value that holds no function — the
zero of a function type, as in a struct field never set — stops the program with
`call of a function value that holds no function` on both backends. Function
values cannot be compared or printed.

## Bindings

`let` binds a name that does not change, `var` one that does. The type may be
written or left to the value.

```rho
let seconds: u32 = 0
var total: u32 = 0
total = total + seconds
```

A binding may be declared without a value and assigned later:

```rho
var answer: u32
answer = 42
```

A top-level `const`, `let` or `var` is a module binding, visible in every
function. It is a constant whichever word introduced it, inlined where it is
named, since a module has no run-time initialisation order; assigning to one is
a compile error. A local of the same name is a variable of its own:

```rho
const LIMIT: u32 = 10

fn remaining(used: u32) u32 {
    return LIMIT - used
}
```

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is visible in the source:

```rho
let small: u8 = 200
let wide: u32 = u32(small)
```

## Types

| Type | Meaning |
|---|---|
| `u8 u16 u32 u64` | unsigned integers, 8 to 64 bits |
| `i8 i16 i32 i64` | signed integers, 8 to 64 bits |
| `f32 f64` | floating point, 32 and 64 bits |
| `bool` | true or false |
| `string` | a byte string |
| `void` | nothing |
| `Unit` | the type of nothing, a generic's default and a handle's element |
| `T[N]` | a fixed array of `N` of `T`, its length part of its type |
| `Slice[T]` | a pointer and a length: the sequence a function takes; see [slices](#slices) |
| `[T]` | a bare pointer to `T`, for C interfaces; it has no length |
| `Name[T]` | a generic type, such as `Option[u32]` |
| `Name` of an `enum` | one of the enum's variants; see [enums](#enums) |
| `Ptr[T]` | a pointer to a `T`; see [pointers](#pointers) |
| `fn(A, B) -> R` | a function value: a named function or a closure; see [functions as values](#functions-as-values-and-closures) |

`type` gives a name to another type:

```rho
type Count = u32

fn zero() Count {
    return 0
}
```

## Structs

A struct is a list of named fields. A literal is positional and written by
name, so a field added later does not silently change what an existing literal
means:

```rho
struct Point {
    x: i32
    y: i32
}

fn origin() Point {
    return Point { 0, 0 }
}
```

A generic struct takes type parameters:

```rho
struct Box[T] {
    value: T
}

fn wrap(n: u32) Box[u32] {
    return Box[u32] { n }
}
```

A field is reached with `.`:

```rho
fn sum(p: Point) i32 {
    return p.x + p.y
}
```

A field may be a **bit field**: its type says what it is stored in, and the
number after the colon says how many bits it uses. Consecutive bit fields of the
same type pack into one integer, each at the offset where the previous one
ended, and a value is masked to its width on the way in:

```rho
struct Flags {
    ready: u8 : 1
    error: u8 : 1
    mode:  u8 : 4
    count: u32
}

fn flags() Flags {
    return Flags { 1, 0, 5, 100 }
}
```

`ready`, `error` and `mode` share one `u8`, so the struct above stores two
integers, not four. A width must be 1 to the width of its type, and the type
must be an integer. The bits are packed the same on every backend: Go has no bit
field, so both read and write one as a shift and a mask.

## Enums

An enum is declared with its variants, separated by a comma, a semicolon or a
line break:

```text
enum Name [ "[" TypeParam { "," TypeParam } "]" ] "{" { Variant } "}"

Variant = Name                                   // carries nothing
        | Name "=" [ "-" ] Integer               // a plain enum's number
        | Name "(" Type { "," Type } ")"         // values by position
        | Name "{" Field { "," Field } "}"       // values by name, `name: Type`
```

```rho
enum Mode { Off, On, Auto = 8 }

enum Shape[T = u32] {
    Circle(T)
    Rect { w: T, h: T }
    Empty
}
```

A number is written only in an enum none of whose variants carries values; one
not written is the one before it plus one, from 0. A type parameter may have a
default, as a struct's may.

A variant is a value written after its enum's name:

| Written | Builds |
|---|---|
| `Mode.On` | a variant that carries nothing |
| `Shape.Circle(3)` | a variant from its values, by position |
| `Shape.Rect { 2, 3 }` | a struct-like variant from its values, in declared order |
| `Shape[u8].Empty` | a variant of a generic enum, its arguments written |
| `geo.Shape.Empty` | a variant of an enum module `geo` declares |
| `Mode(n)` | a plain enum from a number, checked to be one of its values |

## Control flow

`if` runs a block when a condition is true, with `else` and `else if`:

```rho
fn classify(n: i32) u32 {
    if n < 0 {
        return 0
    } else if n == 0 {
        return 1
    } else {
        return 2
    }
}
```

`while` repeats while a condition holds, and `break` leaves the innermost loop:

```rho
fn count_to(limit: u32) u32 {
    var i: u32 = 0
    while true {
        if i >= limit {
            break
        }
        i = i + 1
    }
    return i
}
```

`continue` goes on with the next iteration of the innermost loop. Outside a loop,
`break` and `continue` are refused:

```rho
fn count_odd(limit: u32) u32 {
    var i: u32 = 0
    var odd: u32 = 0
    while i < limit {
        i = i + 1
        if i % 2 == 0 {
            continue
        }
        odd = odd + 1
    }
    return odd
}
```

`for` walks the elements of a fixed array or a `Slice[T]`, binding each one:

```rho
fn total(values: u32[4]) u32 {
    var sum: u32 = 0
    for value in values {
        sum = sum + value
    }
    return sum
}
```

`reverse` walks it from the end, and `step` visits every Nth element:

```rho
fn last_first(values: u32[4]) void {
    for value in reverse values {
        print(int_to_str(value) + " ")
    }
}

fn every_other(values: u32[6]) u32 {
    var sum: u32 = 0
    for value in values step 2 {
        sum = sum + value
    }
    return sum
}
```

`match` runs the first arm whose pattern matches the value; `_` matches
anything, and an arm's body is an expression or a block. `return` and an
assignment are statements, so an arm that returns or assigns is a block:
`1 => { return "one" }`. Against a number or a string, a pattern is a literal or
a constant it is compared with, and the `match` is not checked for
exhaustiveness:

```rho
fn describe(n: u32) void {
    match n {
        1 => print("one")
        2 => print("two")
        _ => print("many")
    }
}
```

Against an enum, a pattern is a variant, with a pattern for each value it
carries, and the match must handle every value:

```text
Pattern = "_"                                    // anything
        | Name                                   // a variant, or (inside one) a binding
        | Enum "." Name                          // a variant, qualified
        | [ Enum "." ] Name "(" Pattern { "," Pattern } ")"
        | [ Enum "." ] Name "{" Name { "," Name } "}"   // bind fields by name
        | Literal                                // compared: 3, -1, "text", true
```

```rho
fn area(s: Shape) u32 {
    match s {
        Shape.Circle(0) => { return 0 }
        Shape.Circle(r) => { return 3 * r * r }
        Rect { w, h } => { return w * h }
        _ => { return 0 }
    }
}
```

`if let` tests one pattern, with an optional `else` (or `else if`):

```rho
if let Some(v) = lookup(key) {
    print(v)
} else {
    print("missing")
}
```

{ref}`types <language/types>` says what is checked: every variant handled, and
an arm that can never run warned about.

A condition is a `bool`. There is no implicit truth: a number is not a
condition, and neither is a value that may be absent.

## Expressions and operators

Literals, and the operators, from loosest to tightest:

- `||`, `&&`
- `==`, `!=`
- `<`, `<=`, `>`, `>=`, and `<<`, `>>`
- `+`, `-`, `|`, `^`
- `&`
- `*`, `/`, `%`

Unary `-`, `!`, `~`, `*` (indirection) and `&` (address) apply to the
expression after them.

Integer arithmetic wraps, a shift by a count too large for the type gives 0 (or
the sign), and a division by zero or an index out of range stops the program,
on both backends alike; a constant that makes one of these certain is refused.
{ref}`What an integer operation means <language/types>` has the whole of it.
`x += v`, `x -= v`, `x *= v` and `x /= v` are `x = x + v` and the rest, with the
same meaning.

```rho
let mask: u32 = (a | b) & ~c
let shifted: u32 = value << 3
let negated: i32 = -delta
let ok: bool = a > 0 && b != 0
```

An `if` is also an expression, and needs an `else`:

```rho
let label: string = if n > 2 { "big" } else { "small" }
```

A call passes arguments in parentheses; a generic call names its type arguments
in brackets:

```rho
fn identity[T](value: T) T {
    return value
}

fn main() void {
    let n: u32 = identity[u32](5)
    print(int_to_str(n))
}
```

The type arguments are left out when the call says what they are, and are
deduced ({ref}`types <language/types>`): `identity(n)`. A generic function named
with its type arguments and not called is a value, `let f = identity[u32]`.

A cast is written `as`, or as the type applied to the value:

```rho
let wide: u32 = 300
let byte: u8 = wide as u8
let again: u32 = u32(byte)
```

Inside an index, write the `as` form. `values[u32(i)]` does not parse, because
a type name after `[` begins a generic application. `values[i as u32]` does.

`sizeof(T)` is how many bytes a type occupies, decided when the program is
compiled so both backends agree on the number:

```rho
let word: u32 = sizeof(u64) // 8
```

Array literals are `[a, b, c]`; a fixed array may be filled by repeating a value
with `[value; count]`; an empty literal takes the element type from the
declaration:

```rho
let primes: u32[3] = [2, 3, 5]
let zeros: u32[4] = [0; 4]
let some: Slice[u32] = [2, 3, 5]
let empty: [u8] = []
```

A local declared without a value is the zero of its type on every backend: `0`,
`false`, `""`, a struct of zeros, an enum's first variant, a `Slice[T]` of
length 0.

## Slices

`s[a..b]` cuts a `Slice[T]` or a fixed array into a `Slice[T]` of the elements
from `a` up to, not including, `b`, over the same storage. Either bound may be
left out: `s[..b]` starts at 0, `s[a..]` runs to the length, and `s[..]` is all
of it. The bounds are checked, `0 <= a <= b <= len`, and a cut outside them
stops the program (`rho: slice [1..5] out of range [0,3]`):

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
    let all: Slice[u32] = raw         // a fixed array converts to a Slice
    let mid = all[1..4]               // 2, 3, 4
    print(int_to_str(sum(mid[1..])))  // 7, a cut of a cut
    print(int_to_str(mid.len + len(all)))
    mid[0] = 9                        // raw[1] is 9 now
}
```

`s[i]` reads or writes element `i`, checked against `s.len`. `append` grows a
slice into the fixed array it was cut from, and is written as the statement
`s = append(s, v)`, with one value or several:

```rho
var buf: string[8]
var names = buf[..0]          // empty, room for 8
names = append(names, "a")
names = append(names, "b", "c")
```

`Slice.from_raw(p, n)` gives a pointer its length: `p` is a `Ptr[T]` or a `[T]`
pointing at the first of `n` elements. {ref}`Arrays, slices and pointers
<language/types>` says what each of the three sequence types is for, and what
`append` can and cannot grow.

`+` joins two strings as well as adding two numbers:

```rho
let name: string = "sensor " + "one"
```

## Routes

A server target serves requests through routes. A handler takes a `Request` and
returns a `Response`:

```rho
route GET "/api/v1/status" => status

fn status(request: Request) Response {
    return json(200, "{\"ok\":true}")
}
```

The router port answers `GET /healthz` for every service, so a module does not
declare one.

## Pointers

`Ptr[T]` is the language's pointer: one type wherever it is written, in a
field, a parameter, a local or an `extern fn` signature. C spells it `T*`
(`void*` for `Ptr[Unit]`, how an opaque foreign handle is written) and Go spells
every pointer `unsafe.Pointer`, so the same value crosses a foreign boundary and
lives in a Rho struct without a conversion.

| Written | Means |
|---|---|
| `&value` | the address of a variable, field or element, a `Ptr[T]` |
| `*p`, `p[i]` | the element `p` points at, and the one `i` places after it; `p[i]` can be assigned |
| `p + n`, `p - n` | the address `n` elements on, or back |
| `Ptr[T](x)` | the pointer at an address held as a number, `Ptr[T](0)` being null; or another pointer, retyped |
| `u64(p)` | the address as a number |

`std.core.ptr` names the same operations as functions: `from_u64`, `to_u64`
and `index`.

```rho
extern fn uart_hw_write(ptr: Ptr[u8], len: u32) void

fn send(buf: u8[4]) void {
    uart_hw_write(&buf[0], 4)
}

fn fill(out: Ptr[u8], count: u32) void {
    var i: u32 = 0
    while i < count {
        out[i] = u8(i)
        i = i + 1
    }
}
```

A string passed where an `extern fn` takes a `Ptr[u8]` is its bytes, ended by a
zero byte as a C string is.

On Go a pointer is a reference the collector follows: the memory it points at
stays alive, and moves with it, for as long as a pointer to it does. An address
held as a number is not one. `Ptr[T](n)` of an address `u64(p)` gave is valid
while `p`, or another pointer to the same memory, is still in use.

## Protocol declarations

A schema file, read by `rho protocol` rather than by a Rho program, declares the
messages on a wire. The type checker refuses `message` and `rpc` in a program,
with the reason. An `enum` is both a program's and a schema's; a schema takes
only a plain one, whose variants are numbers, and refuses an enum that carries
data or is generic, because a field of an enum type is its number on the wire.

```rho
module sensor

namespace acme.sensor.v1

enum Health {
    OK = 0
    FAULT = 1
}

message Reading {
    sensor_id: u16
    temperature_c: f32
    health: Health
}

rpc SetSamplingRate {
    request {
        interval_ms: u16
    }
    response {
        actual_interval_ms: u16
    }
}
```

`rho protocol generate --target c|go|rho` turns these into the payload codecs.
{ref}`protocol <language/protocol>` describes what each target gets.

## Comments and layout

A comment runs to the end of the line:

```rho
// a line comment
let x: u32 = 1 // and one after code
```

A semicolon is optional at the end of a statement, and a comma separates fields
and arguments. A type name is written with a leading capital; a value with a
lower-case letter. That convention is what the parser uses to tell a struct
literal from an index.

## Known limitations

- **Names the backends cannot write.** A local or parameter named after a C type
  word (`int`, `char`, `float`, `double`, `long`, `signed`, `size_t`,
  `uint32_t` …) or a Go keyword that Rho does not have (`func`, `go`, `chan`,
  `defer`, `range`, `select`, `switch`, `case`, `default`, `interface`,
  `package`, `goto`, `fallthrough`) is refused, with the reason. Choose
  another name.
- **A `[T]` has no length.** A `[T]` is a bare pointer in C (and a Go slice in
  Go). Declaration, a literal, indexing and passing one work on both; `len`,
  `for`, `append` and a cut `[a..b]` need a length, so a C build refuses the
  first three with the place named and the cut is refused everywhere. Use a
  `Slice[T]`: a fixed array converts to one, and `Slice.from_raw(p, n)` makes
  one of a pointer.
- **There is no `for i, x in s`.** A loop that needs the index counts with
  `while`, or walks `0..len` by hand.
- **append grows into a fixed array, nowhere else.** `s = append(s, v)` needs
  `s` declared in the same function from a fixed array, starting at its first
  element (`var s = buf[..0]`), and changed only by `append` or by a cut back
  from its start; anything else is refused with what to write.
- **No casts as an index.** `values[u32(i)]` does not parse; `values[i as u32]`
  does.
- **`test fn` is not syntax.** A `_test.rho` file marks its cases `test fn`,
  and {ref}`rho-test <tools/testing>` removes the marker before compiling. The
  compiler itself does not accept `test fn`.
- **A generic literal is written with brackets.** A type may be spelled
  `Box<u32>`, but a struct literal may not: `Box<u32> { 3 }` does not parse.
  Write `Box[u32] { 3 }`.
- **An arm's body is one expression or a block.** `Some(v) => total = v` does
  not parse; write `Some(v) => { total = v }`.
- **A variant is named after its enum outside a pattern.** `Circle(3)` is a call
  to a function `Circle`; build the variant with `Shape.Circle(3)`. Inside a
  pattern the enum is known from the value, and `Circle(r)` is enough.
  (`Some`, `None`, `Ok` and `Err` are functions as well as variants.)
- **A closure captures by value and holds no references.** It cannot assign to
  what it captured, and it cannot capture a pointer, a `[T]`, a `Slice[T]` (or a
  struct holding one) or a fixed array; pass those as parameters. A closure
  cannot call itself by name: recursion goes through a named function.
- **Bit fields store as integers.** A struct with bit fields is laid out as the
  storage integers it packs into — the layout is the language's, not the C
  compiler's — so a `struct`-shaped ABI a C header expects would have to be
  declared to match rather than assumed.

## Where to go next

- {ref}`types <language/types>` — the types and the shapes built out of them.
- {ref}`built-in functions <language/builtins>` — what a program calls without
  writing it.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.
