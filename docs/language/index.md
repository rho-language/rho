# The Rho language

This is the language reference. It describes what Rho is today, which is a
subset: the front end, the type checker and the two backends are in the
repository, and what they accept is what is written here.

The reference beside this section in the sidebar is generated from the sources
in `std/`, so the exact set of functions each module declares is listed there
rather than copied here.

## What Rho is

Rho is a systems language with the shape of C and the ergonomics of a modern
one:

- **No hidden allocation.** There is no heap and no garbage collector. A value
  lives where it is bound, and a pointer is a pointer.
- **No globals.** State belongs to a value, which is what makes a function
  testable without a fixture.
- **The host is declared, not assumed.** A program that needs storage, logging
  or documentation declares `extern fn` and asks for a port.

```rho
module counter

// Counter is a value that holds a number. It is passed, not found.
struct Counter {
    value: u32
}

fn increment(c: Counter) Counter {
    return Counter { c.value + 1 }
}
```

## Modules and imports

A file declares the module it belongs to. The name is dotted, and it is the
name an import uses:

```rho
module std.core.option

fn is_some[T](o: Option[T]) bool {
    return o.is_some
}
```

A project imports a module by name:

```rho
module registry.pages

import std.core.option
import std.strings.string
```

Two forms of inclusion exist. `import` brings in another module and keeps its
declarations separate, so two modules may declare the same name without
colliding. `include` splices a file's declarations into the including module,
which is how a set of declarations is grouped.

:::{note}
An import of a component — `import rho.log`, say — reaches the component's Rho
sources, which travel with the component. A project that depends on a component
gets its library without carrying it, the same way a project that depends on a
library gets the library.
:::

## Types

| Kind | What it is |
|------|------------|
| `u8` `u16` `u32` `u64` `i8` `i16` `i32` `i64` | Fixed-width integers |
| `f32` `f64` | Floating point |
| `bool` | `true` or `false` |
| `Unit` | The type of a function that returns nothing; written `void` at a signature |
| `[T]` | A slice of `T` |
| `Option[T]` | A value that may be absent |
| `Result[T, E]` | A value or an error |
| `map[K, V]` | A map, supplied by a port |
| `struct` | A product type, declared in the module |
| `enum` | A sum type, declared in the module |

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is always visible in the source.

## Generics

A declaration may take type parameters, and a parameter may have a default:

```rho
fn None[T = Unit]() Option[T] {
    return Option[T] { false, default(T) }
}
```

`None()` on its own means an `Option[Unit]`: no annotation, and no ambiguity
about what "no value" holds.

## Functions

```rho
fn divide(numerator: i32, denominator: i32) Result[i32, string] {
    if denominator == 0 {
        return Err("division by zero")
    }
    return Ok(numerator / denominator)
}
```

Every function names what it returns. A function that returns nothing says
`void`, so a caller never has to infer it.

## Storage, and what a port is

Rho cannot open a file, take a lock or write to a socket. A program that needs
to declares the function it needs:

```rho
extern fn read_all(path: string) string
```

and the project asks for a port that supplies it:

```yaml
# project.yml
targets:
  - id: go
    ports:
      - fsstore
```

The backend generates the implementation. What the project declares and what
the backend supplies are separate, so a project stays pure Rho and still gets
storage. The ports available today are `fsstore`, `logemit` and `docemit`.

:::{warning}
A port is a promise the backend makes. A program written against `fsstore` and
generated for a backend that does not supply it fails at the call, not at the
manifest, so a project declares the ports it depends on and keeps the list
honest.
:::

## What the language does not have yet

Stated plainly, because a reference that implies otherwise is worse than none:

- No heap allocation, and no `new`. A slice is a view over memory someone else
  owns.
- No globals, so a value is passed or reached through a struct field.
- No file I/O, no time, no network, unless a port supplies it.
- No closures, and no higher-order functions.
- No statement-level reflection.

## Where to go next

- {ref}`handbook/modules` — how a module, component or plugin is declared.
- {ref}`handbook/documentation` — how this documentation is produced, including
  how a module documents itself.
- The **API reference** in the sidebar for the exact interface of each module.
