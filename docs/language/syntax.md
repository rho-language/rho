# Syntax

Functions, control flow, modules and expressions.

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

A `main` that is declared `void` is emitted as an entry point that hands back a
value, because the entry the host runs expects one; a `main` that falls off the
end has run to completion, which is success.

## Control flow

The four Rho has, and no more:

```rho
if condition {
    // ...
} else {
    // ...
}

while condition {
    // ...
}

for x in items {
    // ...
}

break
```

There is no `else if`: an `else` whose block is one `if` is an `if`. There is no
`continue` and no `goto`, and there is no loop expression — a value produced by a
loop is produced by an assignment inside it, so what a loop computes is visible
in the body where it happens.

## Binding

```rho
let seconds: u32 = 0      // a binding that does not change
var total: u32 = 0        // a binding that does
```

Integers do not widen implicitly. A `u8` is not a `u32`; the conversion is
written, so a narrowing is always visible in the source.

## Structs and enums

```rho
struct Point {
    x: f64
    y: f64
}

enum Shape {
    Circle(f64)
    Rect(f64, f64)
}
```

A struct literal is written by name, so a field added later does not silently
change what an existing literal means:

```rho
let origin = Point { 0.0, 0.0 }
```

## Expressions

Arithmetic, comparison and logic are written as they are in C, with one
difference: there is no implicit truth. A condition is a `bool`, not a number
and not a pointer, and a value that is an `Option` is not a condition until it
has been asked whether it is present:

```rho
let led: Option[Ptr[RoboHubGPIO]] = robohub_get_gpio("user_led")
if is_some(led) {
    write(value(led), 0, 1)
}
```

That is what `is_some` and `value` are for, and why an absent handle is
something a program has to handle rather than something it can trip over.

## Storage, and what a port is

Rho cannot open a file, take a lock or write to a socket. A program that needs
to declares the function it needs:

```rho
extern fn read_all(path: string) string
```

and the project asks for a port that supplies it:

```yaml
# module.yml
build:
  targets:
    - id: go
      ports:
        - fsstore
```

The backend generates the implementation. What the project declares and what
the backend supplies are separate, so a project stays Rho and still gets
storage. The ports available today are `fsstore`, `logemit` and `docemit`.

:::{warning}
A port is a promise the backend makes. A program written against `fsstore` and
generated for a backend that does not supply it fails at the call, not at the
manifest, so a project declares the ports it depends on and keeps the list
honest.
:::

## Where to go next

- {ref}`types <language/types>` — the types and the shapes built out of them.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.
