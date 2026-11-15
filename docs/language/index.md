# The Rho language

This is the language reference. It describes what Rho is today, which is a
subset: the front end, the type checker and the two backends are in the
repository, and what they accept is what is written here.

The reference beside this section in the sidebar is generated from the sources
in `std/`, so the exact set of functions each module declares is listed there
rather than copied here.

## Chapters

- {ref}`syntax <language/syntax>` — functions, control flow, modules and
  expressions.
- {ref}`types <language/types>` — the types, generics, and the shapes a program
  builds values out of.
- {ref}`built-in functions <language/builtins>` — what a program calls without
  writing it: text, numbers, files, processes, time, and the service functions,
  with which backend has each.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.
- {ref}`protocol <language/protocol>` — schemas, and the C, Go and Rho generated
  from one of them.

## What Rho is

Rho is a systems language with the shape of C and the ergonomics of a modern
one. One program compiles to C, for a board or a host, and to Go, for a command
or a web service.

- **No allocator in the language.** There is no `new` and no pointer a program
  allocates or frees. A value lives where it is bound, and a struct is passed
  whole.
- **State belongs to a value.** A module's top-level bindings are constants, so
  a function's state is its arguments, which is what makes it testable without a
  fixture.
- **The host is declared, not assumed.** What only a host can do is a function
  the runtime supplies, declared with `extern fn`, or a port the target asks for.
- **One meaning on every backend.** Integer overflow wraps, an over-wide shift
  gives 0, and a division by zero or an index out of range stops the program
  with the same message on C and on Go. None of these is left to undefined
  behaviour in the C a program compiles to ({ref}`language/types`); a bare
  pointer carries no length and its index is not checked.

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

## Memory

A program allocates nothing it can see. Scalars, structs and fixed arrays are
values. On C, a dynamic array literal is storage in the function that declares
it, and a `[T]` passed on is a view of it.

Text is the exception. A string the program builds — a `+`, `int_to_str`,
`str_replace`, a file read with `rho_read_text` — is new memory. The Go backend
uses Go strings, which its garbage collector frees. On C every such string comes
from one string arena, and the compiler gives strings back in bulk:

- **At the end of each loop iteration.** A `while` or `for` loop takes a mark
  before it starts, and every iteration, and every `continue`, releases back to
  it. A `break` releases after the loop. A `return` releases nothing, so the
  string it returns reaches the caller.
- **After a statement.** What an expression statement or an `if` condition
  builds is released once it has run. A `let` or an assignment of one string
  keeps that string and releases everything else the statement built.

The rule a program follows is the iteration lifetime rule: **a string built in a
loop iteration lives until the end of that iteration, unless it is stored in
something declared outside the loop.** Storing a literal, or a string that
existed before the loop, keeps nothing alive and does not count.

A loop that **replaces** what a variable declared outside it holds — the last
reading, the status text, the last error — keeps exactly what that variable
holds at the end of each iteration and releases the rest. The release that ends
an iteration, the one before a `continue` and the one after the loop (left by
its condition or a `break`) move the kept strings down to the loop's mark and
point the variables at their new place, so the loop runs in the size of what it
keeps plus one iteration's strings, for ever. Two variables holding one string,
or one holding the tail of another, share it. The variable may be:

- a `string`;
- a struct, assigned whole or a field at a time (`status.text = line`), whose
  strings are its own fields or fields of structs and enums inside it;
- an enum, an `Option` or a `Result`, whose strings are kept for whichever
  variant it holds;
- a fixed array of any of these, element by element (`ring[slot] = line`), up
  to 32 strings in all.

A value read out of a kept variable and stored in another outer variable
(`previous = last`) is kept too. An accumulator, `acc = acc + piece`, keeps only
its newest value each iteration, so it holds its own length and nothing more.

```rho
module sensor

extern fn read_temp() u32

fn main() void {
    var last: string = ""
    while true {
        let line = "T=" + int_to_str(read_temp())   // lives until the iteration ends
        print(line + "\n")
        last = line                                   // kept: the loop releases the rest
    }
}
```

This loop runs for ever in a few dozen bytes, and `last` holds the latest line.

A value the loop binds once from what was there before it, `let key = s[i]`
never assigned again, is as old as what it was read from: sorting strings in
place stores nothing new.

What is still not released is a loop that stores a string it built where the
loop cannot keep it by replacing a variable: an element of a `Slice[T]`
(`out[i] = field`, the caller's storage), a field reached through a pointer, a
variable given by address (`&outer`) or an outer array, pointer or slice passed
to a Rho function, a function value (a closure's environment is not a string the
runtime can measure), or more than 32 strings. Such a loop is not released at
all and keeps every string its iterations build.

It is reported when that garbage could live as long as the program: a `while`
loop that builds strings and never ends, or one in a function reached from
`main` other than from a loop that is released or a statement that keeps its
result. On a host the build warns: for a loop that never ends, that its memory
grows for as long as it runs; for one that ends, that it grows with every
iteration until the call that ran it is released. For a board the build is refused, because a board's
arena has a fixed size. Whether a target is a board is read from its resolved
spec, not its name. A target whose spec sizes the arena, or declares its RAM and
is not deployed to, is a board. A target with a `deploy` block runs an operating
system and is a host, even on a board that also has an MCU. A target whose spec
cannot be resolved is built as a host. Keep the latest value in a variable or a
fixed array of your own rather than in a Slice's element, build the string in
the iteration that uses it, or return it from a function. A `for` loop is bounded by its array and is not
reported.

**C functions borrow strings.** A string passed to an `extern fn` is valid for
the duration of the call. C code that keeps it, in a queue or a static, copies
it first. A string a C function returns belongs to C and is never moved or
released by the arena.

**The board arena size** comes from the target spec, `linker.string_arena_bytes`
({ref}`rho-target/index`). By default it is 4 kB, or an eighth of the RAM on a
part with less than 32 kB. It bounds the strings one iteration holds at once,
not the strings a program builds over its life, and what a loop keeps across its
iterations counts towards it. Running out stops the program with
`string arena exhausted`. A board's strings need no `malloc`. On a host the arena
grows in 64 kB chunks and reuses them; a string longer than a chunk gets one four
times its size, so an accumulator's chunks double rather than grow with every
iteration.

`print(int_to_str(n))`, `print(i64_to_str(n))` and `print(string.from_i32(n))`
write the number without building a string.

**Closures follow the same rule.** A closure's environment — the values it
captured — is copied into the arena when the literal is evaluated, and the
compiler treats a function value as a value holding a string: it lives as long
as a string built in the same place would, and is released with it
({ref}`function types <language/types>`). A loop that replaces a function value
declared outside it is not one the compiler keeps from: it is not released.

## What a program can reach

A program reaches the machine through the runtime functions in
{ref}`built-in functions <language/builtins>`. Both backends have these:

- **Files:** `rho_read_text`, `rho_write_text`, `rho_append_text` and
  `rho_write_secret` read, write or append a whole text file. A secret is written
  only its owner can read. `yaml.value`, `yaml.kind` and `yaml.keys` read a
  YAML document by a dotted path. `std.os.fs` and `std.os.sys` open, read,
  write and close a file descriptor, moving bytes through a `Ptr[u8]` into the
  program's own memory.
- **Processes:** `rho_exec(program, arguments, report)` splits its arguments on
  spaces and double quotes. `rho_exec_args` takes one argument per line and
  interprets nothing. `rho_exec_capture` returns the output instead of writing
  it to a file. A value that came from a request, a file or a user goes through
  the last two, which pass it as exactly one argument. No program is run through
  a shell.
- **Time:** `rho_now_s`, `rho_local_date`, `rho_local_clock`,
  `rho_utc_offset_at`, and `rho_sleep_ms` for a task that waits between ticks.
- **The environment:** `getenv`, `env_or`, `app_workspace`.
- **The command line:** `get_args`, `arg_count` and `rho_arg`; on a board there
  are no arguments.
- **Output and text:** `print` and `eprint` (standard error), `f64_to_str` and
  `f32_to_str` for a float, `str_to_bytes` and `str_from_bytes` between a string
  and a `Slice[u8]`, and `rho_path_exists`, which tells a missing file from an
  empty one.
- **Cryptography:** `rho_hmac_sha256_hex` and `rho_random_hex`.

Only the Go backend has these:

- **Other services:** `rho_service_token(audience, scopes)` is a short-lived
  token fetched with the identity the supervisor gave the program
  (`RHO_CLIENT_ID`, `RHO_CLIENT_SECRET_FILE`, `RHO_AUTH_URL`) and cached.
  `rho_http_request` and `rho_http_body` send a request carrying one.
  `rho_sse_follow` follows another service's event stream into a file. No token
  is put on a command line or in a URL.
- **The file system beyond one file:** `rho_list_dir`, and `read_file` and
  `write_file` for bytes (a `Slice[u8]`).

Anything else, such as a network socket, a serial line, a database or logging,
is a port ({ref}`building/ports`) or an `extern fn` the platform implements.

## What the language does not have yet

Stated plainly, because a reference that implies otherwise is worse than none:

- No `new`, and no way to free a string by hand. The compiler releases strings
  (see Memory).
- No mutable module state. A top-level `const`, `let` or `var` is a constant,
  and assigning to one is refused.
- No `message` or `rpc` in a program; they belong to protocol schemas. An
  `enum` is the program's own: a set of numbers, or variants that carry values,
  taken apart with an exhaustive `match` ({ref}`language/types`).
- Closures that capture by value only: a closure cannot change what it
  captured or capture a reference (a pointer, a `[T]`, a `Slice[T]`), and its
  environment lives in the string arena under the string rule
  ({ref}`language/types`).
- Generics constrained by the language's own constraints only — `Integer`,
  `Float`, `Number`, `Ordered`, `Equal` — and not by a set of functions a type
  must have: pass a function value (`fn(T) -> string`) instead
  ({ref}`language/types`).
- No maps in the language. `std.collections.map` is a fixed-capacity table
  written in Rho.
- No reflection.
- No length for a `[T]`, which is a bare pointer: `len`, `for` and `append`
  over one are Go-only and a cut of one is refused. A `Slice[T]` carries its
  length, is checked on every index and cut, and grows with `append` into the
  fixed array it was cut from — not onto a heap, which a board does not have
  ({ref}`language/types`).

## Where to go next

- {ref}`modules/modules` — how a module, component or plugin is declared.
- {ref}`building/ports` — how a program asks for what only a host can do.
- {ref}`tools/documentation` — how this documentation is produced, including
  how a module documents itself.
- The **API reference** in the sidebar for the exact interface of each module.
