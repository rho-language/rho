# Target output

The target's output, without taking the board from a debugger.

## The problem

A board has one debug probe. A program that opens it takes it from whatever is
already holding it, so "watch the output while I debug" sounds like it needs a
choice between the two. It does not, and the reason is that almost everything about
a board is not the probe.

| Transport | How it is reached | Costs a debugger the board? |
|---|---|---|
| **RTT** | A socket the debug server already offers | No |
| **UART** | A USB interface beside the debug one | No |
| **SWO / ITM** | Drained by whoever holds the probe | **Yes** |

RTT's buffer is a block of the target's own memory. The server holding the probe
reads it out and publishes it on a socket, so a client that connects to that socket
is reading what the target wrote and taking nothing from anybody. A USB console is
a CDC interface with its own device node, beside the debug one.

SWO is the exception, and the exception is the whole design. The trace comes out of
the core's own trace port, and draining it is the probe's job — so a SWO stream and
a debug session cannot both have it.

## The order is the design

`auto` asks for RTT, then a serial console, then SWO.

Not a preference. The first two coexist with a debug session; the third does not, so
it is never chosen automatically over a working alternative. Reaching a SWO trace is
a decision somebody makes, not a default they inherit — which is why a board whose
firmware traces through SWO still gets its console read over UART by default, and
why `--backend=swo` exists for the ones that do not.

## Zero configuration

A board plugged in with a USB console and a debug server offering RTT is read
without being told anything. `rho monitor` with no arguments is the whole
interface for most boards.

What a project declares is for when that is wrong:

```yaml
# module.yml
monitor:
  default_backend: rtt          # auto, rtt, uart or swo
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

Naming a `uart.port` turns the scan off: a project that says which console it has
must not be overridden by whichever device node appears first on the machine.

## What it adds to what the target printed

A target's output is not written for a terminal, so two things are added — and both
are read from the text rather than asked of the target, because a target's log
format belongs to the target.

**When a line arrived.** From this machine's clock. The target has no way to know
when its bytes reached a terminal, and its own clock is the board's rather than this
machine's.

**How bad it sounds.** From the one word in the line that says so, matched on its
punctuation rather than case-insensitively:

- an uppercase tag anywhere — `ERROR`, `WARN`, `INFO`, `DEBUG`;
- a tag in brackets, in any case — `[error]`, `[WARN]`;
- a tag with a colon at the start of the line — `warning:`, `info:`.

That is what keeps prose out of the result. A line reading "the error rate was
measured" is a sentence about a measurement, and a case-insensitive scan calls it an
error report — which paints a healthy run as a broken one, and is the fastest way
to make a log unreadable.

Four levels rather than seven, because a serial console is 115200 baud and anything
a firmware prints is already expensive to produce. These are the four that change
what a person does next.

Colour is written only when the output is going to a terminal, and the log file gets
the plain line: escape sequences in a file are a file `grep` cannot read. The log
file is appended to and never truncated, because a run that truncated it would
destroy the output of the run before it — which is the run somebody wanted to
compare against.

## The SWO decoder's scope

The framing is the ITM one: bit 0 of each byte says whether another byte of the
same packet follows, which is what makes the stream self-delimiting with no framing
of its own.

What is decoded is the two console ports — the software console, which is what
`printf` ends up writing, and the hardware console. What is **not** here is a full
ITM demultiplexer: four-byte data packets and the extended header packets are
counted and dropped. They carry binary telemetry, which a monitor has no business
printing, and a general port decode here would be a second demultiplexer to keep in
step with a core rather than a reader. OpenOCD's `itm` demux already does that.

A capture on disk works the same way, and is what a CI job keeps:

```sh
rho monitor --swo-file=capture.swo
```

## The commands

| Command | What it does |
|---|---|
| `rho monitor [project]` | Read the output over whichever transport has one |
| `rho monitor --backend=NAME` | Read it over a particular transport |
| `rho monitor --console=DEVICE` | The serial console, instead of finding one |
| `rho monitor --baud=N` | The line speed |
| `rho monitor --channel=N` | The RTT channel |
| `rho monitor --swo-file=PATH` | Read an ITM/SWO capture |
| `rho monitor --out=PATH` | Write a copy of everything here |
| `rho monitor --duration=SECONDS` | Stop after this long — what a pipeline wants |
| `rho monitor --no-color` | Do not colour levels |
| `rho monitor backends` | What this component can read |
| `rho monitor ports` | The serial consoles on this machine |

`ports` is a command of its own because from inside a run, "no console found" and
"the wrong console found" look the same until you know what was there.

## A note on the line discipline

A console is opened raw, and the character size is set to eight bits explicitly.
Both matter more than they sound. With canonical mode on, the driver waits for a
newline the target may never send, and a firmware that prints a heartbeat without
one produces nothing at all. And a termios built field by field without `CS8` opens
the port successfully and receives nothing, because on Linux a zero in the
character-size bits means *five* bits — every byte is read at the wrong width and
discarded without a word.

Both failures are silent in the same way, which is the worst kind: a working board
that looks dead.
