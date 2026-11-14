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
| `T[N]` | A fixed array of `N` elements, a value of its own |
| `[T]` | A dynamic array: a Go slice, and on C a pointer to someone else's elements |
| `Slice[T]` | A pointer and a length, from `std.core.slice` |
| `Ptr[T]` | In an `extern fn` signature, a C pointer; elsewhere a handle carrying an address |
| `Option[T]` | A value that may be absent |
| `Result[T, E]` | A value, or an error |
| `struct` | A product type, declared in the module |

There is no enum, sum type or map among a program's types. `enum` belongs to a
{ref}`protocol schema <language/protocol>`, and a program that names numbers uses
`const`.

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is always visible in the source.

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
    let at: Option[u32] = find(readings, 6)
    if is_some(at) {
        print(int_to_str(value(at)) + "\n")
    }
}
```

An absent value is something a program handles rather than something it trips
over. "Not found" is a real answer, and the type is what makes the program see
it. Both `Option` and `Result` are structs: a flag and a value, with a default
value beside the flag when there is none.

A function that can fail says so in what it returns:

```rho
fn divide(numerator: i32, denominator: i32) Result[i32, string] {
    if denominator == 0 {
        return Err("division by zero")
    }
    return Ok(numerator / denominator)
}
```

## Generics

A declaration may take type parameters, and a parameter may have a default:

```rho
fn None[T = Unit]() Option[T] {
    return Option[T] { false, default(T) }
}
```

`None()` on its own means an `Option[Unit]`: no annotation, and no ambiguity
about what "no value" holds.

Generic declarations are templates, and they are instantiated rather than looked
up through a table: every application of a declaration is a concrete function, so
a call is a direct, inlinable call and there is no vtable to indirect through.

## Arrays and slices

A fixed array `T[N]` is a value: it is as long as the program declared, and the
length is part of its type.

A dynamic array `[T]` is a view of elements somebody else owns. Making one costs
nothing, and there is no resizing on C:

```rho
fn first[T](items: [T]) T {
    return items[0]
}

fn main() {
    let primes: [u32] = [2, 3, 5]
    print(int_to_str(first(primes)) + "\n")
}
```

Declaring one, a literal, indexing and passing work on both backends. On Go a
`[T]` is a slice, so `len`, `for` and `append` work on it. On C it is a bare
pointer, which carries no length, so those three are Go-only; `len` on one stops a
C build with the call named. `len` of a fixed-size array `T[N]` is `N` on both.

`Slice[T]` from `std.core.slice` is a struct of a `Ptr[T]` and a `len`. A
function that has to know how many elements it was given on both backends takes
one.

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

## Named numbers

A program has no `enum`: the type checker refuses one, and says that it belongs
to a protocol schema. A set of named values is a set of constants, and `match`
compares against them:

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

A `match` is not checked for exhaustiveness, so the function returns after it.
An arm's body is an expression or a block, and `return` is a statement, so an arm
that returns is written as a block.

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

- {ref}`syntax <language/syntax>` — how these are written.
- {ref}`stdlib <language/stdlib>` — what the standard library is built from.
