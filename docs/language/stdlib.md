# The standard library

The library is Rho sources in `std/`, and the reference beside this page is
generated from them — so what each module declares exactly is listed there rather
than copied here.

## What it is made of

The library has no allocation of its own to hide. A string is a view over bytes
somebody else owns; a slice is a pointer and a length; an array is a fixed-size
block the program declares. There is no heap, so nothing in the library can quietly
take one.

## core

The types every other module is built out of.

| Module | What it carries |
|---|---|
| `core.option` | `Option[T]`: `Some`, `None`, `is_some`, `is_none`, `value` |
| `core.result` | `Result[T, E]`: `Ok`, `Err`, `is_ok`, `is_err`, `value` |
| `core.ptr` | `Ptr[T]`: `from_u64`, `to_u64`, `index` — a handle to memory the program does not own |
| `core.slice` | `Slice[T]`: a pointer and a length, and nothing more |
| `core.range` | `Range`: what a `for` walks, with `range`, `next` |
| `core.maybe_uninit` | `MaybeUninit[T]`: a value not yet written, for memory a program fills itself |
| `core.builtin` | What the compiler makes available without an import |

## strings

`strings/string` is `String`, which is a `Slice[u8]` — a string *is* a slice of
bytes, so passing one costs a pointer and a length and nothing else:

```rho
len(s), eq(a, b), find(haystack, needle), starts_with(s, prefix), ends_with(s, suffix)
```

`strings/utf8` is the UTF-8 layer over those bytes.

## collections

| Module | What it carries |
|---|---|
| `collections.array` | `Array[T, N]`: a fixed-size array, sized by a type parameter, with `get`, `set`, `as_slice` |
| `collections.bitset` | A set of bits over a fixed-size block |
| `collections.map` | A map, supplied by a port |

An `Array[T, N]` is the answer to a question an embedded program asks often: how
big is this, exactly. The size is a type parameter, so it is known at compile
time and the block is as long as the program declared and no longer.

## math

| Module | What it carries |
|---|---|
| `math.basic` | `min`, `max`, `abs`, `clamp`, for each width and for `f64` |
| `math.float` | Floating-point operations |
| `math.fixed` | Fixed-point arithmetic, for a value without a floating-point unit |
| `math.fast` | The same operations arranged for speed rather than for generality |
| `math.control` | Controllers — a PID among them — for a program that steers something |
| `math.embedded` | The parts of the above that a microcontroller can do without a floating-point unit |

## embedded

What a program driving a board calls, against the board's own interfaces rather
than against a part:

`embedded/gpio`, `embedded/i2c`, `embedded/spi`, `embedded/uart`, `embedded/timer`,
`embedded/modbus`, `embedded/rs485`.

These are interfaces a *platform* implements. A program written against
`embedded/gpio` builds for any board whose platform provides one, which is what
"the same program builds for an STM32, a Pico and an ESP32" means: not that the
code changes, but that nothing in it names a part.

## os

`os/fs`, `os/sys` and `os/time` — files, the system interface, and the clock.
These are *ports*: a program that uses them declares that it needs a port which
supplies them, and a backend that does not supply one fails the build rather than
producing a program that cannot open a file.

## sql and web

`sql` is what a service declares, and the engine that runs it;
`web/http` and `web/template` are the HTTP surface a generated service answers
on. Both are for the Go server target rather than for firmware: a board has no
HTTP server, and a library that assumed one would not build for one.

## assert

`assert` is what a test says with. It is a module rather than a keyword so that a
program does not carry it — and so that what a test asserts is a call a person can
read in the diff.

## Where to go next

- The **API reference** in the sidebar, for every declaration above.
- {ref}`handbook/ports` — what a port is, and which of these modules need one.
- {ref}`types <language/types>` — how these types are written.
