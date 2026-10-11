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
| `namespace robohub.telemetry.v1` | names a protocol schema's wire namespace (schema files) |
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

`import` keeps the names apart, so two modules may declare the same name without
colliding:

```rho
module app
import std.core.option
```

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

fn first[T](items: [T]) T {
    return items[0]
}
```

`extern fn` declares a function the host supplies rather than one the program
writes. Its body is absent; a [port](#storage-and-ports) implements it:

```rho
extern fn read_all(path: string) string
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
function:

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
| `[T]` | a dynamic array of `T` |
| `T[N]` | a fixed array of `N` of `T` |
| `Name[T]` | a generic type, such as `Option[u32]` |
| `Ptr[T]` | see the note under [pointers](#pointers) |

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

`for` walks the elements of an array, binding each one:

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
fn every_other(values: u32[6]) u32 {
    var sum: u32 = 0
    for value in values step 2 {
        sum = sum + value
    }
    return sum
}
```

`match` runs the first arm whose pattern equals the value; `_` matches anything,
and an arm's body is an expression or a block:

```rho
fn describe(n: u32) void {
    match n {
        1 => print("one")
        2 => print("two")
        _ => print("many")
    }
}
```

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

A cast is written `as`, or as the type applied to the value:

```rho
let wide: u32 = 300
let byte: u8 = wide as u8
let again: u32 = u32(byte)
```

`sizeof(T)` is how many bytes a type occupies, decided when the program is
compiled so both backends agree on the number:

```rho
let word: u32 = sizeof(u64) // 8
```

Array literals are `[a, b, c]`; a fixed array may be filled by repeating a value
with `[value; count]`; an empty literal takes the element type from the
declaration:

```rho
let primes: [u32] = [2, 3, 5]
let zeros: u32[4] = [0; 4]
let empty: [u8] = []
```

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

`&value` takes an address and `*pointer` reads through one. These are the
foreign operators: they are used to pass addresses to `extern fn` calls and to
read what such a call leaves behind.

`Ptr[T]` is the handle a program holds: it is a struct with a `raw` address, so
it can be named, passed, stored and returned. `std.core.ptr` carries it:

```rho
import std.core.ptr

fn main() void {
    let p: Ptr[u32] = from_u64[u32](0)
    print(int_to_str(u32(to_u64(p))))
}
```

The same spelling `Ptr[T]` is a **C pointer** in an `extern fn` signature —
there it is the pointer the foreign declaration names, and an argument reaches
it as an address:

```rho
extern fn uart_hw_write(ptr: Ptr[u8], len: u32) void

fn send(buf: u8[4]) void {
    uart_hw_write(&buf[0], 4)
}
```

Which one `Ptr[T]` is depends on where it is written: a foreign signature is
the C pointer, and every other type position is the handle struct.

## Protocol declarations

A schema file, read by `rho protocol` rather than by a Rho program, declares the
messages on a wire. The typechecker does not use these; a Rho program does not
contain them.

```rho
module telemetry

namespace robohub.telemetry.v1

enum NodeStatus {
    OK = 0
    FAULT = 1
}

message Telemetry {
    node_id: u16
    temperature_c: f32
    status: NodeStatus
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

`rho protocol generate --target c|go|rho` turns these into the payload codecs,
and for Rho also into the transport that carries them.

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

- **The length of a dynamic array in C.** A `[T]` is a Go slice and a C
  pointer. Declaration, a literal, indexing and passing one work on both; what
  a bare pointer does not carry is its length, so `len` and `for` over a `[T]`
  are Go-only. A slice that carries its length is `std.core.slice`'s
  `Slice[T]`, which has a pointer and a length and works on both.
- **Bit fields store as integers.** A struct with bit fields is laid out as the
  storage integers it packs into — the layout is the language's, not the C
  compiler's — so a `struct`-shaped ABI a C header expects would have to be
  declared to match rather than assumed.

## Where to go next

- {ref}`types <language/types>` — the types and the shapes built out of them.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.
