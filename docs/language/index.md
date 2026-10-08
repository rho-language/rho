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
- {ref}`handbook/ports` — how a program asks for what only a host can do.
- {ref}`handbook/documentation` — how this documentation is produced, including
  how a module documents itself.
- The **API reference** in the sidebar for the exact interface of each module.
