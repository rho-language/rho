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

## Which backends a module is for

Most modules are plain Rho and work on every backend. A module that does not
says so in its header, with a line the build reads:

```rho
// @backends go-server
// @port sql
```

`@backends` lists where the module works: `c`, `go` (any Go target) or
`go-server` (a Go target of kind `server`). `@port` names the port that
supplies its foreign functions on a Go target. A program that imports such a
module for a target it is not for is refused before anything is generated,
with the module, the backend and the file that imported it named. A built-in
function only some backends supply (`get_args`, `escape_html`, `store_list` …)
is refused the same way, at the line that calls it; the
{ref}`built-in functions <language/builtins>` page lists which backends supply
each.

| Module | Backends | Tested by |
|---|---|---|
| `std.core.*`, `std.strings.*`, `std.collections.*`, `std.assert.assert` | every backend | the parity suite: C (with ASan and UBSan) and Go must print the same |
| `std.math.*` | every backend | the parity suite |
| `std.os.*` | `c`, `go` | the parity suite |
| `std.embedded.*` | `c` | a C build on the host with the platform's `*_hw_*` functions replaced by stubs |
| `std.sql` | `go`, with the `sql` port | a Go program against SQLite through the port |
| `std.web.http`, `std.web.template` | `go-server` | a generated service, started and asked over HTTP |

The functions a program calls without importing anything (text, numbers,
files, processes, time) are not in `std/`. They are the
{ref}`built-in functions <language/builtins>`.

## core

The types every other module is built out of.

| Module | What it carries |
|---|---|
| `std.core.option` | `Option[T]`: `Some`, `None`, `is_some`, `is_none`, `value`, `unwrap` |
| `std.core.result` | `Result[T, E]`: `Ok`, `Err`, `is_ok`, `is_err`, `value`, `error`, `unwrap` |
| `std.core.ptr` | The functions over the language's pointer `Ptr[T]`: `from_u64`, `to_u64`, `index` |
| `std.core.slice` | `Slice[T]`: a pointer and a length, and `slice_empty` |
| `std.core.range` | `Range`: a half-open range of `u32`, with `range` and `next` (`Some(i)` while `start <= i < end`) |
| `std.core.maybe_uninit` | `MaybeUninit[T]`: a value not yet written, with `uninit` and `assume_init` |
| `std.core.builtin` | Nothing to call: a written account of the primitive types and the built-ins, which the compiler supplies. Importing it changes nothing |

`Option` and `Result` are structs, not sum types. `None[T]()` and `Err(e)` hold
a default value beside the flag, and `unwrap` panics on the empty case: it
writes `rho: unwrap None` (or `Err`) to standard error and exits with status 1.

A primitive type cannot be declared again: `type u32` or `struct u8` is refused.

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
| `std.collections.bitset` | `BitSet`: a pointer to 64-bit words the caller owns and how many, built with `bitset(&words[0], n)`, with `capacity`, `set`, `clear` and `get`. An index past the last word is outside the set |
| `std.collections.map` | `Map[K, V, N]`: a fixed-capacity hash table of `N` slots, with `insert` and `get` |

The map is plain Rho, with no port behind it. It uses open addressing with
linear probing over entries the caller supplies. A key is hashed as a `u32`, so
it must convert to one. `insert` returns `false` when every slot is taken.
`get` returns an `Option[V]`.

`Array` and `Map` take their count as a value parameter: `Array[u32, 4]` is
written as a type and as a literal (`Array[u32, 4]{ Ptr[u32](&storage[0]) }`),
and `N` inside the generic functions is that number. An array of a generic
struct is written `Entry[u32, u32][4]`. All three hold a pointer, so a write
through one is a write to the caller's memory.

An `Array[T, N]` answers a question an embedded program asks often: how big is
this, exactly. The size is a type parameter, so it is known at compile time.

## math

| Module | What it carries |
|---|---|
| `std.math.basic` | `min_u32`, `max_u32`, `clamp_u32`, `abs_i32`, `min_f64`, `max_f64`, `clamp_f64`, `map_f64`, `deadband_f64`, `smoothstep` |
| `std.math.float` | `sqrt`, `pow`, `sin`, `cos`, `atan2`, `abs_f64`, `deg_to_rad`, `rad_to_deg`, `wrap_pi`, `lerp` |
| `std.math.linear` | `Vec2`, `Vec3`, `Vec4`, `Mat2x2`, `Mat3x3` and `Quaternion`, with add, subtract, dot, cross, length, normalize, lerp, transpose, inverse, a rotation and slerp |
| `std.math.fixed` | Q16.16 and Q8.8 fixed-point values, for a part without a floating-point unit; a product is formed in 64 bits |
| `std.math.fast` | `fast_sin`, `fast_cos`, `fast_sqrt`, `fast_atan2`: direct calls to the C functions, with nothing in between |
| `std.math.control` | `PID`, `LowPass`, `MovingAverage4`, `Complementary` and `Kalman1D`, each a struct with an `_init` and an `_update` |
| `std.math.embedded` | `map_range`, `saturate`, `deadband`, `smoothstep01` and angle conversions, without a C library |

`std.math.float`, `std.math.fast` and `std.math.linear` call the runtime's
floating-point functions, which both backends supply: the C library's on C,
with the generated CMake linking the maths library (`-lm`), and the `math`
package on Go. A program may also call `sqrt`, `sin`, `cos`, `atan2`, `floor`,
`clamp_f64` and `lerp` without importing anything.
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

These modules are for the C backend (`@backends c`). A Go build that imports
one is refused by name, because no Go target has a platform that supplies the
`*_hw_*` functions. The standard library writes their prototypes into the
generated C, so a platform implements exactly the signature declared here.

The `*_hw_*` functions are `extern fn` that a *platform* implements. A program
written against `std.embedded.gpio` builds for any board whose platform provides
them. That is what "the same program builds for an STM32, a Pico and an ESP32"
means: the code does not change, because nothing in it names a part.
{ref}`building/platforms` describes what a platform supplies.

## os

`std.os.sys`, `std.os.fs` and `std.os.time` call the runtime, which on C uses
the C library's wrappers rather than system call numbers, so they mean the same
thing on every architecture:

| Module | What it carries | C | Go |
|---|---|---|---|
| `std.os.time` | `now()`, a `TimeSpec` of seconds and nanoseconds from the realtime clock | yes | yes |
| `std.os.sys` | `exit(code)`, `setenv(key, value)` (replaces a value already set) | yes | yes |
| `std.os.sys` | `write(fd, buf, len)`, `read(fd, buf, len)`: bytes moved, or `0xFFFFFFFF` on failure | yes | yes |
| `std.os.fs` | `open(path, flags)` with the C library's `O_*` flags (a created file is mode 0644), `close(fd)` | yes | yes |

The descriptor calls move bytes through a `Ptr[u8]`, which on both backends is
the program's own memory: `write(fd, &buf[0], n)` writes an array's bytes where
they are, and a path is a NUL-terminated `Ptr[u8]`. A hosted program that
handles text reads and writes whole files, runs programs and reads the clock
with the runtime functions in {ref}`built-in functions <language/builtins>`,
which both backends supply.

## sql

`std.sql` is the database interface a program writes against: `SqlDb`, `SqlStmt`,
`SqlValue` and `SqlError`, the `sql_open`, `sql_exec`, `sql_prepare`, `sql_run`,
`sql_read_column` family, and the value constructors (`int_value`,
`text_value` …).

The functions are `extern fn`. The generated Go project implements them with
`database/sql` and the driver the environment names (`RHO_SQL_DRIVER`), through
the `sql` port, which declaring `sql:` asks for and a target may also list in
its `ports`. The module is tagged `@backends go` and `@port sql`: the toolchain
has no C implementation, so a C build that imports it is refused, and so is a
Go target without the port. A statement's columns (`sql_column_count`,
`sql_column_name`, `sql_column_type`) are known once it has run once.
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

Both are for the Go service target (`@backends go-server`), and a C or Go
command-line build that imports either is refused by name. A board has no HTTP
server, and a library that assumed one would not build for one. A server has
the template renderer whether or not its target carries template files, so
`fill` and `escape` work on markup a handler built itself.

## assert

`std.assert.assert` is what a test asserts with: `assert`, `assert_true`,
`assert_false`, `assert_eq`, `assert_ne` and `fail`. A failure prints
`ASSERTION FAILED: ` and the message on standard output, then aborts: `rho:
aborted` on standard error and status 1, on both backends. It is a module rather than a
keyword, so a program that does not test does not carry it, and what a test
asserts is a call a person can read in the diff. {ref}`tools/testing` covers the
runner.

## Where to go next

- The **API reference** in the sidebar, for every declaration above.
- {ref}`built-in functions <language/builtins>` — what the runtime supplies
  without an import.
- {ref}`building/ports` — what a port is, and which of these modules need one.
- {ref}`types <language/types>` — how these types are written.
