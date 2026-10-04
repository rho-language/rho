# rho-monitor

The target's output, without taking the board from a debugger.

```sh
rho monitor                       # read it, whichever transport has one
rho monitor --backend=uart        # read it over a particular transport
rho monitor --out=raw.log         # and keep a copy
```

## The constraint it is built around

A board has exactly one debug probe, and a program that opens it takes it from
whatever is already holding it. Everything else about a board is free — and that is
what makes a monitor possible at all:

| Transport | How it is reached | Costs a debugger the board? |
|---|---|---|
| **RTT** | A socket the debug server already offers | No |
| **UART** | A USB interface beside the debug one | No |
| **SWO / ITM** | Drained by whoever holds the probe | **Yes** |

RTT's buffer is a block of the target's own memory; the server holding the probe
reads it out and publishes it on a socket. Opening a TCP connection costs nothing.
A USB console is a CDC interface with its own device node. SWO comes out of the
core's trace port, and draining it is the probe's job — so a SWO stream and a
debug session cannot both have it.

## The order is the design

`auto` asks for RTT, then a serial console, then SWO — and SWO is never chosen
automatically over a working alternative. Reaching it is a decision, not a default.

```yaml
monitor:
  default_backend: rtt      # auto, rtt, uart or swo
```

## Zero configuration

A board plugged in with a USB console and a debug server offering RTT is read
without being told anything, because both backends that matter find themselves.
`rho monitor` with no arguments is the whole interface for most boards.

What a project declares is for when that is wrong:

```yaml
monitor:
  rtt:
    host: localhost
    port: 9090
    channel: 0
    block_if_full: false
  uart:
    auto_detect: true
    baud_rate: 115200
  formatting:
    timestamps: true
    color_levels: true
    log_file: logs/target_output.log
```

Naming a `uart.port` turns the scan off — a project that says which console it has
must not be overridden by whichever node appears first.

## What it adds to what the target printed

Two things, and the target's output belongs to the target, so both are read from
the text rather than imposed on it:

- **When a line arrived.** From this machine's clock, not the target's — the
  target has no way to know when its bytes reached a terminal.
- **How bad it sounds.** From the one word that says so. Uppercase tags anywhere
  (`ERROR`), a tag in brackets (`[warn]`), and a tag with a colon at the start of
  the line (`warning:`). Prose is not matched: a line reading "the error rate was
  measured" is a sentence, and colouring it as an error report would paint a healthy
  run as a broken one.

Colour is written only when the output is going to a terminal. The log file gets the
plain line, because escape sequences in a file are a file `grep` cannot read.

## The SWO decoder's scope

The framing is the ITM one — bit 0 of each byte says whether another follows. What
is decoded is the two console ports, recognised by the header bits a console packet
sets and nothing else.

What is **not** here is a full ITM demultiplexer: four-byte data packets and the
extended header packets are counted and dropped. They carry binary telemetry, which
a monitor has no business printing, and a general port decode here would be a
second demultiplexer to keep in step with a core rather than a reader. OpenOCD's
`itm` demux already does that. Point this component at a capture with

```sh
rho monitor --swo-file=capture.swo
```

and it will print the console out of it and ignore the rest.

## Building it

```sh
go build -o rho-monitor ./src
go test ./...
```
