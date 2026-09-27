# Ports

Rho has no heap, no globals and no file I/O. That is a deliberate constraint
rather than a missing feature: it is what makes a program predictable, and it is
what makes the parts that do need a host visible instead of implicit.

A **port** is how a program asks for one of those parts.

## Declaring what a program needs

A program declares the functions the host must supply:

```rho
extern fn read_all(path: string) string
```

and calls them as it calls any other function. Nothing about the call site says
"this is I/O": the declaration is the whole of the dependency, and it is in the
module, where a reader of the module sees it.

## Asking for a port

A project asks the backend for the implementation by name:

```yaml
# project.yml
targets:
  - id: go
    ports:
      - fsstore
```

The backend generates the implementation into the project, and the generated
file is ordinary code in the project: readable, and removable if a person wants
to see what it does.

## The ports today

| Port | What it supplies | Used by |
|------|------------------|---------|
| `fsstore` | A module store on the filesystem: put, get, search, list | the registry |
| `logemit` | Writing a log record, with the formatter's own shape | any project that logs |
| `docemit` | The documentation generator, as a page or a site | a project that serves documentation |

## A worked example

A component declares a library; a project asks for a port; the backend supplies
the implementation. Nothing in the project is generated from the component's
sources, and nothing in the component is generated for the project.

```rho
// modules/components/rho-log/src/log.rho
module rho.log

extern fn emit(level: u32, subject: string, message: string) void

fn info(subject: string, message: string) void {
    emit(1, subject, message);
}
```

```yaml
# the project's project.yml
deps:
  rho-log: ">=0.1.0"

build:
  targets:
    - id: go
      ports:
        - logemit
```

The project calls `info(...)`. The `logemit` port supplies `emit`, writing the
record in the shape the formatter produces, so a device writing to a serial
console and a server writing to stdout cannot drift apart.

## Why a port and not a library

A library would have to be compiled into every project that uses it, and a
library that writes files and takes locks is a library every project then
carries. A port supplies the implementation at generation time, from the
backend, so:

- the project stays pure Rho, and every dependency it has is declared in its
  manifest;
- the implementation is one per backend, so two projects do not drift apart;
- the boundary is visible: what came from the host is what a port named, and
  nothing else did.

:::{note}
A port is a promise the backend makes, checked when the port is generated rather
than at the call. A project that declares a port no backend supplies fails at
the call site rather than at the manifest, so the list of ports in a
`project.yml` is the honest record of what a project needs.
:::
