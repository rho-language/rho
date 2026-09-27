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

## Extending

A later plugin, such as forwarding to a collector, implements `Sink` and is
registered in the component's descriptor. Nothing in the logger changes.
