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
`str_replace`, a file read with `rho_read_text` — is new memory. The C runtime
takes it from `malloc` and never frees it. The Go backend uses Go strings,
which its garbage collector frees. A firmware loop that builds strings for ever
therefore grows on C. A firmware that only compares and slices what it was given
does not.

## What a program can reach

A program reaches the machine through the runtime functions in
{ref}`built-in functions <language/builtins>`. Both backends have these:

- **Files:** `rho_read_text`, `rho_write_text`, `rho_append_text` and
  `rho_write_secret` read, write or append a whole text file. A secret is written
  only its owner can read. `yaml.value`, `yaml.kind` and `yaml.keys` read a
  YAML document by a dotted path.
- **Processes:** `rho_exec(program, arguments, report)` splits its arguments on
  spaces and double quotes. `rho_exec_args` takes one argument per line and
  interprets nothing. `rho_exec_capture` returns the output instead of writing
  it to a file. A value that came from a request, a file or a user goes through
  the last two, which pass it as exactly one argument. No program is run through
  a shell.
- **Time:** `rho_now_s`, `rho_local_date`, `rho_local_clock`,
  `rho_utc_offset_at`, and `rho_sleep_ms` for a task that waits between ticks.
- **The environment:** `getenv`, `env_or`, `app_workspace`.
- **Cryptography:** `rho_hmac_sha256_hex` and `rho_random_hex`.

Only the Go backend has these:

- **Other services:** `rho_service_token(audience, scopes)` is a short-lived
  token fetched with the identity the supervisor gave the program
  (`RHO_CLIENT_ID`, `RHO_CLIENT_SECRET_FILE`, `RHO_AUTH_URL`) and cached.
  `rho_http_request` and `rho_http_body` send a request carrying one.
  `rho_sse_follow` follows another service's event stream into a file. No token
  is put on a command line or in a URL.
- **The file system beyond one file:** `rho_path_exists`, `rho_list_dir`,
  `read_file` and `write_file` for bytes, and `get_args` and `rho_arg` for the
  command line.

Anything else, such as a network socket, a serial line, a database or logging,
is a port ({ref}`building/ports`) or an `extern fn` the platform implements.

## What the language does not have yet

Stated plainly, because a reference that implies otherwise is worse than none:

- No `new`, and no way to free what the runtime allocated for a string.
- No mutable module state. A top-level `const`, `let` or `var` is a constant,
  and assigning to one is refused.
- No enums in a program. `enum`, `message` and `rpc` belong to protocol schemas.
  A program names its numbers with `const`.
- No sum types. A value that may be absent is an `Option[T]`, and one that may
  fail is a `Result[T, E]`, both structs.
- No closures, no function values, and no higher-order functions.
- No maps in the language. `std.collections.map` is a fixed-capacity table
  written in Rho.
- No reflection.
- On C, no length for a `[T]`. `len`, `for` and `append` over one are Go-only.

## Where to go next

- {ref}`modules/modules` — how a module, component or plugin is declared.
- {ref}`building/ports` — how a program asks for what only a host can do.
- {ref}`tools/documentation` — how this documentation is produced, including
  how a module documents itself.
- The **API reference** in the sidebar for the exact interface of each module.
