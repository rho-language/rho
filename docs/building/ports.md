# Ports

Rho has no allocation a program asks for (no `new`) and no mutable global
state to speak of, and the runtime
supplies only a handful of host functions, each declared with `extern fn` where
it is used: whole text files (`rho_read_text`, `rho_write_text`), the
environment, the clock, a subprocess, a pause (`rho_sleep_ms`). That is a deliberate limit rather than a missing feature: it keeps a
program predictable, and it makes the parts that do need a host — a module
store, a log with a rotating file, a database, an HTTP router — visible instead
of implicit.

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

A target asks the backend for an implementation by name, in `project.yml`:

```yaml
# project.yml
version: 1
targets:
  - id: go
    ports:
      - fsstore
```

The backend generates the implementation into the project, as `ports.go`, and
the generated file is ordinary code in the project: readable, and removable if a
person wants to see what it does. (A target is changed in `project.yml` because
that file is read on top of the built-in target; see {ref}`building/targets`.)

A port is usually not asked for by the application at all. A module whose
foreign functions a port supplies says so, and every Go program the module is
part of gets the port:

- a built-in port names the module it implements — `logemit` implements
  `rho.log`, so a program that imports `rho.log` has its logger;
- a module, an application included, lists the ports its own foreign functions
  need under `ports:` in its `module.yml`:

```yaml
# module.yml of an application that declares docs_render
ports:
  - docstore
```

Two ports arrive without being named. A server target that names a `router` or
lists any `middleware` gets `httproute`, and a project that declares `sql:` gets
`sql`, because the code generated from its statements calls into it.

Ports are Go. A C program's foreign functions come from its platform's library
and the C sources its manifest names, not from a port.

## The built-in ports

| Port | What it supplies |
|------|------------------|
| `fsstore` | A module store on the filesystem: put, get, search, list |
| `logemit` | Level-filtered logging in text or JSON, with a rotating file and runtime configuration; implements `rho.log` |
| `docemit` | The documentation generator, as a page or a site, reached as the `rho-doc` component |
| `docstore` | A built documentation tree: walking it, rendering a page through `rho-doc`, and release notes |
| `httproute` | A server's router, typed parameter binding and middleware pipeline — see {ref}`building/services` |
| `openapiemit` | The service's own OpenAPI description, written at generation time by the `rho-openapi` component |
| `sql` | A database for the statements a project generated from SQL |

A port that is neither built in nor provided by a plugin (below) stops the
generation and lists what is available:

```text
rho build: target "go": unknown port "sensor" (available: docemit, docstore, fsstore, httproute, logemit, openapiemit, sql)
```

## A worked example

A component declares a library; the backend supplies the implementation.
Nothing in the project is generated from the component's sources, and nothing
in the component is generated for the project.

```rho
// modules/components/rho-log/src/log.rho
module rho.log

extern fn emit(level: u32, subject: string, message: string)

fn info(subject: string, message: string) void {
    emit(2, subject, message)
}
```

A project that imports `rho.log` and calls `info(...)` gets the `logemit` port
without naming it. The port supplies `emit`, writing the record in the shape the
component's own formatter produces. A service calls
`configure(level, format, file, max_bytes, keep, stderr)` after reading its own
settings; the port filters levels, writes text or JSON and rotates the file
itself. Environment values (`RHO_LOG_LEVEL`, `RHO_LOG_FORMAT`, `RHO_LOG_FILE`,
`RHO_LOG_MAX_BYTES`, `RHO_LOG_KEEP`, `RHO_LOG_STDERR`) are the operator and test
defaults.

## Ports a plugin provides

A port that is not the toolchain's can come from a plugin. An **integration
plugin** — the one plugin through which an ecosystem meets the toolchain, see
{ref}`modules/integrations` — names a directory of Go files under
`integration.provides.ports`. Each file is one port, named after the file:
`ports/sensor.go` is the port `sensor`. A file is a complete Go file in
`package main`, and its `_test.go` files are its own tests rather than ports; the
directory is a Go module of its own, with a `go.mod`, so the ports are built and
tested where they live.

A plugin's ports are offered only to a project built for a platform the plugin
serves. A project's platform is the first `platform:` its module graph declares —
its own manifest, then its dependencies by path — and a plugin serves the
platforms it lists under `serves:`. A project with no platform gets the built-in
ports only, which is the standard build. The `rho-robohub` plugin is one example:
it provides the ports its ecosystem's applications are built with.

### Ports in the project's own tree

A port is Go compiled into the project, as the project's own sources are, so an
application can keep one beside its code and version it with it. A plugin
folder in the project's tree is read when the project is generated; it needs
`serves:` and `integration.provides.ports`, and no `extends:`.

```text
acme-station/
  module.yml
  src/main.rho
  station-ports/
    module.yml
    ports/
      go.mod
      sensor.go
```

```yaml
# acme-station/module.yml
module:
  name: acme-station
  type: rho
  version: 0.1.0
description: Reads a temperature sensor
main: src/main.rho
platform: acme
ports:
  - sensor
build:
  language: go
  targets:
    - go
```

```yaml
# acme-station/station-ports/module.yml
module:
  name: acme-station-ports
  type: plugin
  version: 0.1.0
description: The host side of the station's sensor
main: ports
serves:
  - acme
integration:
  ecosystem: acme
  provides:
    ports: ports/
```

```rho
// src/main.rho
module acme_station

extern fn sensor_celsius() i32

fn main() void {
    print("temperature: " + int_to_str(sensor_celsius()) + "\n")
}
```

```go
// station-ports/ports/sensor.go
package main

// sensorCelsius is the station's temperature, read from the host.
func sensorCelsius() int32 {
	return 21
}
```

The port defines the function under the name the Go backend calls the extern by,
which is the Rho name in camel case: `sensor_celsius` is `sensorCelsius`. The
tree is searched for every directory with a `module.yml`, skipping `.git`,
`build`, `vendor` and `node_modules`, and an installed plugin of the same name
is preferred over the tree's.

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
A port is a promise the backend makes, checked when the project is generated: a
port nobody supplies stops the build at generation, before anything is compiled.
The ports a project's targets and modules declare are therefore the honest
record of what it needs from its host.
:::

## A service that also does something else

An HTTP service answers requests, but some services also do something else
while they answer — poll a sensor, watch a directory — and a program has no
threads of its own. A server-kind module declares `fn background() void`, and
the generated service runs it on its own goroutine, after `init()` and beside
the listener:

```rho
extern fn sensor_celsius() i32
extern fn rho_write_text(path: string, text: string) i32
extern fn rho_sleep_ms(milliseconds: u32) void

fn init() void {
    print("station: starting\n")
}

fn background() void {
    while true {
        let reading: string = int_to_str(sensor_celsius())
        rho_write_text("latest.txt", reading)
        rho_sleep_ms(1000)
    }
}
```

A value `background()` writes and a handler reads cannot be a global, so it lives
with the host: in a file, as here, or behind a port whose implementation guards
it with its own lock.
