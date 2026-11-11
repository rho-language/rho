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
- {ref}`protocol <language/protocol>` — schemas, and the C and Go generated from
  one of them.
- {ref}`stdlib <language/stdlib>` — what the standard library carries.

## What Rho is

Rho is a systems language with the shape of C and the ergonomics of a modern
one:

- **No hidden allocation.** There is no heap and no garbage collector. A value
  lives where it is bound, and a pointer is a pointer.
- **No mutable global state to speak of.** Top-level bindings exist, but state
  belongs to a value, which is what makes a function testable without a fixture.
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

## What the language does not have yet

Stated plainly, because a reference that implies otherwise is worse than none:

- No heap allocation, and no `new`. A slice is a view over memory someone else
  owns.
- No mutable global state to speak of: a module may declare a `const`, `let` or
  `var` at the top level, but state belongs to a value that is passed or reached
  through a struct field.
- No time and no network, unless a port supplies it; a background task waits
  between ticks with `rho_sleep_ms(milliseconds)`. File I/O is `rho_read_text`
  and `rho_write_text`, which read and write a whole text file, and
  `rho_write_secret`, which writes one only its owner can read (mode 0600, and an
  existing file is narrowed to it) for a token, a key or a credential; anything
  beyond that is a port.
- A program runs another with `rho_exec(program, arguments, report)`, whose
  arguments are split on spaces and double quotes, or with
  `rho_exec_args(program, arguments, report)`, which takes one argument per line
  and interprets nothing in it. A value that came from a request, a file or a
  user goes through `rho_exec_args`: it reaches the program as exactly one
  argument, and cannot become a second one or a command.
- A Go program that is a service calls other services with an identity its
  supervisor gave it (`RHO_CLIENT_ID`, `RHO_CLIENT_SECRET_FILE`, `RHO_AUTH_URL`):
  `rho_service_token(audience, scopes)` is a short-lived token for one audience,
  fetched and cached; `rho_http_request(method, url, body, content_type,
  audience, scopes, report)` sends a request carrying one; and
  `rho_sse_follow(url, audience, scopes, file)` follows another service's event
  stream into a file that `sse_events` serves, reconnecting with a fresh token.
  No token is put on a command line or in a URL.
- No closures, and no higher-order functions.
- No statement-level reflection.

## Where to go next

- {ref}`handbook/modules` — how a module, component or plugin is declared.
- {ref}`handbook/ports` — how a program asks for what only a host can do.
- {ref}`handbook/documentation` — how this documentation is produced, including
  how a module documents itself.
- The **API reference** in the sidebar for the exact interface of each module.
