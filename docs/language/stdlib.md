# The standard library

The library is Rho sources in `std/`, and the reference beside this page is
generated from them. What each module declares is listed there rather than
copied here. A module is imported by its full name: `import std.core.option`.

## What it is made of

Most of the library is plain Rho over values: structs passed whole, fixed
arrays, and views over memory the caller owns. Where it needs something only a
host can do, it says so with an `extern fn` and leaves the implementation to
the runtime, a port or the platform. Each section below says which of those it
relies on.

The functions a program calls without importing anything (text, numbers,
files, processes, time) are not in `std/`. They are the
{ref}`built-in functions <language/builtins>`.

## core

The types every other module is built out of.

| Module | What it carries |
|---|---|
| `std.core.option` | `Option[T]`: `Some`, `None`, `is_some`, `is_none`, `value`, `unwrap` |
| `std.core.result` | `Result[T, E]`: `Ok`, `Err`, `is_ok`, `is_err`, `value`, `error`, `unwrap` |
| `std.core.ptr` | `Ptr[T]`, a handle that carries an address as a number: `from_u64`, `to_u64`, `index` |
| `std.core.slice` | `Slice[T]`: a pointer and a length, and `slice_empty` |
| `std.core.range` | `Range`: a start and an end, with `range` and `next` |
| `std.core.maybe_uninit` | `MaybeUninit[T]`: a value not yet written, with `uninit` and `assume_init` |
| `std.core.builtin` | The primitive types, and `print`, `abort` and `assert` as the compiler knows them |

`Option` and `Result` are structs, not sum types. `None[T]()` and `Err(e)` hold
a default value beside the flag, and `unwrap` panics on the empty case.

## strings

`std.strings.string` is `String`, which is a `Slice[u8]`. A string in this module
is a pointer and a length, so passing one costs nothing else:

```rho
len(s), eq(a, b), find(haystack, needle), starts_with(s, prefix), ends_with(s, suffix), split(s, sep, out)
```

`split` writes the pieces into a `Slice[String]` the caller provides and returns
how many there were.

`std.strings.utf8` is the UTF-8 layer over those bytes: `is_ascii` and
`first_codepoint`.

A `String` is not the language's `string`. A `string` is what a literal is and
what the built-in text functions take; a `String` is a view a program builds
over bytes it holds.

## collections

| Module | What it carries |
|---|---|
| `std.collections.array` | `Array[T, N]`: a pointer to `N` elements, the count a type parameter, with `get`, `set` and `as_slice` |
| `std.collections.bitset` | `BitSet[N]`: `N` 64-bit words, with `set`, `clear` and `get` |
| `std.collections.map` | `Map[K, V, N]`: a fixed-capacity hash table of `N` slots, with `insert` and `get` |

The map is plain Rho, with no port behind it. It uses open addressing with
linear probing over entries the caller supplies. A key is hashed as a `u32`, so
it must convert to one. `insert` returns `false` when every slot is taken.
`get` returns an `Option[V]`.

An `Array[T, N]` answers a question an embedded program asks often: how big is
this, exactly. The size is a type parameter, so it is known at compile time.

## math

| Module | What it carries |
|---|---|
| `std.math.basic` | `min_u32`, `max_u32`, `clamp_u32`, `abs_i32`, `min_f64`, `max_f64`, `clamp_f64`, `map_f64`, `deadband_f64`, `smoothstep` |
| `std.math.float` | `sqrt`, `pow`, `sin`, `cos`, `atan2`, `abs_f64`, `deg_to_rad`, `rad_to_deg`, `wrap_pi`, `lerp` |
| `std.math.linear` | `Vec2`, `Vec3`, `Vec4`, `Mat2x2`, `Mat3x3` and `Quaternion`, with add, subtract, dot, cross, length, normalize, lerp, transpose, inverse, a rotation and slerp |
| `std.math.fixed` | Q16.16 and Q8.8 fixed-point values, for a part without a floating-point unit |
| `std.math.fast` | `fast_sin`, `fast_cos`, `fast_sqrt`, `fast_atan2`: direct calls to the C functions, with nothing in between |
| `std.math.control` | `PID`, `LowPass`, `MovingAverage4`, `Complementary` and `Kalman1D`, each a struct with an `_init` and an `_update` |
| `std.math.embedded` | `map_range`, `saturate`, `deadband`, `smoothstep01` and angle conversions, without a C library |

`std.math.float` and `std.math.fast` reach the C library through
`extern fn c_sqrt`, `c_sin`, `c_pow` and the like. The toolchain defines none of
these, so a program that uses them links its own: a C file the project
carries, or the platform. They are not available on the Go backend.
`std.math.linear` imports `std.math.float` and needs the same.
`basic`, `fixed`, `control` and `embedded` are arithmetic in Rho and need
nothing.

A module's `features:` list (`math.control`, `math.linear`) includes the
matching module and defines `RHO_FEATURE_*` in the generated C.

## embedded

What a program driving a board calls. Each module calls the board's interfaces
rather than a particular chip:

| Module | What it needs |
|---|---|
| `std.embedded.gpio` | `gpio_hw_init`, `gpio_hw_write`, `gpio_hw_read` |
| `std.embedded.uart`, `std.embedded.i2c`, `std.embedded.spi`, `std.embedded.timer` | the `*_hw_*` functions each declares |
| `std.embedded.rs485` | `uart` and `gpio`: a UART with driver-enable and receiver-enable pins |
| `std.embedded.modbus` | `rs485`: Modbus RTU framing and its CRC-16, in Rho |

The `*_hw_*` functions are `extern fn` that a *platform* implements. A program
written against `std.embedded.gpio` builds for any board whose platform provides
them. That is what "the same program builds for an STM32, a Pico and an ESP32"
means: the code does not change, because nothing in it names a part.
{ref}`building/platforms` describes what a platform supplies.

## os

`std.os.sys`, `std.os.fs` and `std.os.time` are a thin layer over raw Linux
system calls on x86-64. Each declares `extern fn c_syscall(num, a, b, c, d, e, f)`
and calls it with that architecture's numbers: `read` (0), `write` (1), `open`
(2), `close` (3), `exit` (60) and `clock_gettime` (228). `std.os.sys` also
declares `extern fn setenv` from the C library.

The toolchain does not define `c_syscall`, so a program that imports these
links its own. They are not a portable interface, and they do not exist on the
Go backend. A hosted program reads and writes files, runs programs and reads
the clock with the runtime functions in
{ref}`built-in functions <language/builtins>`, which both backends supply.

## sql

`std.sql` is the database interface a program writes against: `SqlDb`, `SqlStmt`,
`SqlValue` and `SqlError`, the `sql_open`, `sql_exec`, `sql_prepare`, `sql_run`,
`sql_read_column` family, and the value constructors (`int_value`,
`text_value` …).

The functions are `extern fn`. The generated Go service implements them with
`database/sql` and the driver the project declares, through the `sql` port that
declaring `sql:` asks for. The toolchain has no C implementation.
{ref}`building/services` describes how a service declares its database.

## web

`std.web.http` builds a `Response`: `text`, `json`, `html`, `redirect`,
`with_headers` and the `status_*` codes. It also declares two streaming
responses the Go service implements:

- `sse_file(path, interval_ms)` follows a file as Server-Sent Events. Each line
  becomes one `data:` event. A line beginning `__DONE__:<status>` closes the
  response with `done`, and the stream keeps following a file that is still being
  written. A long action reports progress this way: it writes a file, and the
  page listens to the same file.
- `sse_events(path, snapshot, interval_ms)` streams named fragments, one
  `{"event","data"}` JSON line each, the shape a page using HTMX swaps in. A
  `snapshot` file is sent first, so a page that connects sees the current state.

`std.web.template` is the template engine's interface: `vars` encodes a name
and value, `fill` renders, and `escape` makes a value safe for a page. Its
header documents the template syntax.

Both are for the Go service target. A board has no HTTP server, and a library
that assumed one would not build for one.

## assert

`std.assert.assert` is what a test asserts with: `assert`, `assert_true`,
`assert_false`, `assert_eq`, `assert_ne` and `fail`. A failure prints
`ASSERTION FAILED: ` and the message, then aborts. It is a module rather than a
keyword, so a program that does not test does not carry it, and what a test
asserts is a call a person can read in the diff. {ref}`tools/testing` covers the
runner.

## Where to go next

- The **API reference** in the sidebar, for every declaration above.
- {ref}`built-in functions <language/builtins>` — what the runtime supplies
  without an import.
- {ref}`building/ports` — what a port is, and which of these modules need one.
- {ref}`types <language/types>` — how these types are written.
