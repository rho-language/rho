# Types

What a Rho value can be, and how a shape is built out of them.

## The types

| Kind | What it is |
|------|------------|
| `u8` `u16` `u32` `u64` | Unsigned integers, fixed width |
| `i8` `i16` `i32` `i64` | Signed integers, fixed width |
| `f32` `f64` | Floating point |
| `bool` | `true` or `false` |
| `Unit` | The type of a function that returns nothing; written `void` at a signature |
| `Ptr[T]` | A pointer to `T` |
| `[T]` | A slice of `T`: a view over memory someone else owns |
| `Option[T]` | A value that may be absent |
| `Result[T, E]` | A value, or an error |
| `map[K, V]` | A map, supplied by a port |
| `struct` | A product type, declared in the module |
| `enum` | A sum type, declared in the module |

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is always visible in the source.

## Option and Result

A value that may be absent is an `Option[T]`, and it is asked about rather than
guessed at:

```rho
let led: Option[Ptr[RoboHubGPIO]] = robohub_get_gpio("user_led")
if is_some(led) {
    write(value(led), 0, 1)
}
```

This is why an absent handle is something a program handles rather than something
it trips over. A registry answering "no such name" is a real answer, and the type
is what makes the program see it.

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

## Slices

A slice is a pointer and a length. It is a view, not an owner: making one costs
nothing, and what it points at is somebody else's memory.

```rho
fn first[T](items: [T]) Option[T] {
    if length(items) == 0 {
        return None()
    }
    return Some(items[0])
}
```

There is no resizing. A slice that must grow is a slice over a buffer the program
owns, sized when it was declared.

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

Because there are no globals, a value is passed or reached through a field. That
is what makes a function testable: a counter is an argument, not something a test
has to set up around it.

## Enums

An enum is a sum type. Each variant carries what it needs to be told:

```rho
enum Shape {
    Circle(f64)
    Rect(f64, f64)
}

fn area(s: Shape) f64 {
    match s {
        Circle(r) => return 3.14159 * r * r
        Rect(w, h) => return w * h
    }
}
```

A match is exhaustive: a function that matches an enum returns in every arm or
does not compile, so adding a variant is a change every reader of the type sees.

## Foreign types

A type a C header declares is a Rho type by name, and the pointer to it is a
`Ptr`:

```rho
extern fn get_gpio(name: string) Option[Ptr[RoboHubGPIO]]
```

Nothing about the header is written out by hand: {ref}`rho-cimport
<rho-cimport/index>` generates these declarations from the headers the project
named, so a header that changes changes them with it. An opaque C struct — one
the program may hold but not read — becomes a name the program can pass around and
nothing else, which is the same thing the header's own author intended.

## Where to go next

- {ref}`syntax <language/syntax>` — how these are written.
- {ref}`stdlib <language/stdlib>` — what the standard library is built from.
