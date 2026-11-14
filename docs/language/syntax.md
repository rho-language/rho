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

fn first[T](items: [T]) T {
    return items[0]
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

`match` runs the first arm whose pattern equals the value; `_` matches anything,
and an arm's body is an expression or a block. `return` is a statement, so an
arm that returns is a block: `1 => { return "one" }`. A pattern may be a
literal or a constant, and a `match` is not checked for exhaustiveness:

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

`Ptr[T]` is the handle a program holds: a struct with a `raw` address, so it can
be named, stored and returned. `Ptr[T](&value)` makes one from a variable's
address, and `std.core.ptr` declares `from_u64`, `to_u64` and `index` over it.
The handle is not finished on either backend. See the
[known limitations](#known-limitations).

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
the C pointer, and every other type position is meant to be the handle struct.

## Protocol declarations

A schema file, read by `rho protocol` rather than by a Rho program, declares the
messages on a wire. The type checker refuses `enum`, `message` and `rpc` in a
program, with the reason.

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
- **The length of a dynamic array in C.** A `[T]` is a Go slice and a C
  pointer. Declaration, a literal, indexing and passing one work on both; what
  a bare pointer does not carry is its length, so `len`, `for` and `append`
  over a `[T]` are Go-only. On C, `len` of a string fails at the link too; use
  `strlen`. `std.core.slice`'s `Slice[T]` carries a pointer and a length.
- **The pointer handle.** `std.core.ptr`'s generic functions do not type-check
  at a call: `from_u64[u32](0)` is reported as a `Ptr_T` that cannot be assigned
  to a `Ptr[u32]`. The C backend spells a `Ptr[T]` parameter of a Rho function as
  a C pointer, so reading `.raw` through it does not compile. The Go backend
  does not import `unsafe` for `Ptr[T](&value)`. A `Ptr[T]` in an `extern fn`
  signature, reached with `&`, is the form that works.
- **No casts as an index.** `values[u32(i)]` does not parse; `values[i as u32]`
  does.
- **`test fn` is not syntax.** A `_test.rho` file marks its cases `test fn`,
  and {ref}`rho-test <tools/testing>` removes the marker before compiling. The
  compiler itself does not accept `test fn`.
- **A generic literal is written with brackets.** A type may be spelled
  `Box<u32>`, but a struct literal may not: `Box<u32> { 3 }` does not parse.
  Write `Box[u32] { 3 }`.
- **Bit fields store as integers.** A struct with bit fields is laid out as the
  storage integers it packs into — the layout is the language's, not the C
  compiler's — so a `struct`-shaped ABI a C header expects would have to be
  declared to match rather than assumed.

## Where to go next

- {ref}`types <language/types>` — the types and the shapes built out of them.
- {ref}`built-in functions <language/builtins>` — what a program calls without
  writing it.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.
