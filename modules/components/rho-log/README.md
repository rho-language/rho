# rho-log

Structured logging for Rho components.

```sh
rho log -level info -subject registry -message started -field port=8080
```

```
[2026-09-27T06:27:11.826Z] [info] [registry        ] started attempt=1 port=8080
```

## Using it from a component

A logger is a subject, a level and a sink. The subject is stamped into every
record, so a component that depends on this one gets its own name in the log
without repeating it at each call site:

```go
var log = rholog.Default("rho-lsp")

func main() {
    log.Info("listening", "port", 2080)
    log.Warn("retrying", "attempt", 2)
}
```

Adding it to a component is a `deps` entry and a Go require:

```yaml
deps:
  rho-log: ">=0.1.0"
```

## Sinks

A sink receives an already-rendered line, so it never has to know the format.
Adding one is adding a type, not changing the logger.

- `WriterSink` covers standard output, a serial console and a test buffer.
- `FileSink` appends to a file and cycles it at a size limit, keeping the
  previous runs as `path.1`, `path.2` and so on. Cycling rather than deleting
  is deliberate: a device that has just rebooted has usually not written
  anything yet, and the previous run is the interesting one.
- `Discard` throws records away, so a component that is not configured still
  runs.

A sink that fails never takes the component down with it.

## Using it from Rho

The component ships a Rho library alongside the command, in the same structure:
`src/log.rho` is the interface a caller sees, the way a header is, and
`src/main.go` is the command. A project depends on the component, imports the
library, and asks the backend for the port behind it:

```yaml
deps:
  rho-log: ">=0.1.0"
```

```rho
import rho.log

fn main() void {
    info("registry", "listening", "port=8080")
}
```

```yaml
# project.yml
targets:
  - id: go
    ports:
      - logemit
```

Nothing from the component's internals is copied into the project: the library
is the only surface it needs, and the backend supplies `emit`.

## Published forms

The same component publishes in two forms, the way a C project either has a
`main` and becomes an executable or has none and becomes a library.

`binary: true` — the command form — carries the manifest, the documentation,
the header and the prebuilt command:

```
module.yml
README.md
rho-log          the command, prebuilt
src/log.rho      the interface a caller compiles against
```

`binary: false` — the library form — carries the header alone, with no command.

Neither form carries host source. The command is shipped built, and a caller
compiles against the header, so the `.go` files are build input rather than
something an installation needs. Leaving them out means there is nothing in an
installed component for a user to edit, and nothing to keep in step with the
component's own copy. The implementation comes from the `logemit` port, which
the backend supplies.

The library is declared with `interface:`, so a package that ships a header
cannot accidentally ship an implementation with it:

```yaml
package:
  binary: false
  interface:
    - src/log.rho
```

## Extending

A later plugin, such as forwarding to a collector, implements `Sink` and is
registered in the component's descriptor. Nothing in the logger changes.

A later *port*, for a capability the library exposes to Rho, is a new entry in
`portGenerators` in `src/codegen`. The library declares the function; the port
implements it; a project names the port.
