//
// @file source.go
// @brief Where a target's output can be read from, and which of them to read.
//
// Three transports, and the choice between them is not a preference — it decides
// whether a debugger can keep working while the output is being read.
//
// That is the constraint this component is built around. A board has exactly one
// debug probe, and a program that opens it takes it from whatever is already
// holding it. So:
//
//   - **RTT** is read through a socket the debug server already offers. The RTT
//     buffer is inside the target's memory; the server that holds the probe reads
//     it out and publishes it. Opening a TCP connection costs the debugger
//     nothing, so this is the one backend that is genuinely free.
//   - **UART** is a separate interface. A USB console is a CDC interface beside
//     the debug one, on its own device node, and reading it does not touch the
//     probe at all.
//   - **SWO** is the exception. The trace comes out of the core's own trace port
//     and is drained by whichever program holds the probe — so reading it is
//     exactly the contention the other two avoid, and it is offered last and
//     never chosen automatically over a working alternative.
//
// Which is why the default is "ask RTT, then the UART, then SWO": the first two
// coexist with `rho dbg`, and the third is a deliberate choice rather than a
// default.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Kind is a transport a target's output arrives on.
type Kind string

// The transports. Their names are what a project writes and what a person types.
const (
	// KindRTT is a SEGGER RTT stream over a debug server's socket.
	KindRTT Kind = "rtt"
	// KindUART is a serial console.
	KindUART Kind = "uart"
	// KindSWO is an ARM ITM/SWO trace.
	KindSWO Kind = "swo"
)

// The order `auto` resolves in.
//
// RTT first because it is the only one that is both free and lossless: the buffer
// is in the target's memory and is read out by a server that already has the
// probe. The UART second because it is free but lossy — a console that nobody is
// reading drops what it cannot keep — and because its speed is whatever the
// firmware was built to print at. SWO last because reaching it contends with a
// debugger for the probe, which is the thing this component exists to avoid.
//
// Exported because the order is a promise: a project that names `auto` is
// choosing this order, and a component that changed it between releases would
// change what a debugger could do while a monitor was running.
var AutoOrder = []Kind{KindRTT, KindUART, KindSWO}

// String is the transport's name, which is what a project writes and what a log
// line says.
func (k Kind) String() string { return string(k) }

// Source is one place a target's output can be read from.
type Source interface {
	// Kind is the transport, which is what a person asked for.
	Kind() Kind
	// Describe is what a person is told when this one is chosen, and when it is
	// not: an error naming `/dev/ttyACM1 refused to open` is the difference
	// between a monitor that says what it tried and one that says nothing found.
	Describe() string
	// Open returns the stream. Closing it releases whatever the transport holds.
	Open() (Stream, error)
	// Available reports whether this source could be opened right now, with the
	// reason when it could not. Used by detection, which must try each one and
	// say what it found.
	Available() error
}

// Stream is an open target output.
type Stream interface {
	// Read is the byte stream. A transport that decodes (SWO) returns text; the
	// ones that do not (RTT, UART) return whatever the target wrote.
	io_Reader
	// Close releases the transport.
	Close() error
	// Describe is what this open stream is, for the log line a run prints when it
	// starts.
	Describe() string
}

// io_Reader is the read half of io.Reader, named so that the intent is legible in
// the interface above without importing io for one method.
type io_Reader interface {
	Read(p []byte) (int, error)
}

// Config is what a project said about its output, and what a run was told.
type Config struct {
	// Backend is the transport to use: `auto`, or one of the kinds.
	Backend Kind
	// RTT is where an RTT stream is read from.
	RTT RTTOptions
	// UART is how a serial console is reached.
	UART UARTOptions
	// SWO is where an ITM trace is read from.
	SWO SWOOptions
	// Formatting is how the output is presented.
	Formatting Formatting
	// Timeout is how long detection waits for a transport to answer. Short,
	// because detection runs on every invocation and a person should not wait a
	// second to be told nothing was found.
	Timeout time.Duration
	// RunFor is how long a run reads before it stops. Zero means until the
	// stream ends or somebody interrupts it, which is what a person at a terminal
	// wants; a number is what a pipeline wants, because a pipeline has no way to
	// press Ctrl-C.
	RunFor time.Duration
}

// RTTOptions is where an RTT stream is read from.
type RTTOptions struct {
	Host        string
	Port        int
	Channel     int
	BlockIfFull bool
}

// UARTOptions is how a serial console is reached.
type UARTOptions struct {
	// Port is the device node. Empty with AutoDetect set means scan.
	Port string
	// AutoDetect scans the conventional device nodes.
	AutoDetect bool
	// BaudRate is the line speed.
	BaudRate int
}

// Formatting is how the output is presented.
type Formatting struct {
	Timestamps  bool
	ColorLevels bool
	// Color is whether ANSI colour is written at all, which is a property of
	// where the output is going rather than of what the project declared.
	Color bool
	// LogFile is where a copy of everything is written.
	LogFile string
}

// DefaultConfig is what a project that says nothing gets.
func DefaultConfig() Config {
	return Config{
		Backend: Kind("auto"),
		RTT:     RTTOptions{Host: "localhost", Port: 9090},
		UART:    UARTOptions{AutoDetect: true, BaudRate: 115200},
		SWO:     SWOOptions{Host: "localhost", Port: 9121},
		Formatting: Formatting{
			Timestamps: true, ColorLevels: true, Color: true,
		},
		Timeout: 750 * time.Millisecond,
	}
}

// Endpoint is where an RTT stream is read from.
func (o RTTOptions) Endpoint() string { return fmt.Sprintf("%s:%d", o.Host, o.Port) }

// Normalize fills in what the caller left out, so that every consumer of a Config
// sees a complete one rather than checking each field itself.
func (c Config) Normalize() Config {
	defaults := DefaultConfig()
	if c.Backend == "" {
		c.Backend = defaults.Backend
	}
	if c.RTT.Host == "" {
		c.RTT.Host = defaults.RTT.Host
	}
	if c.RTT.Port == 0 {
		c.RTT.Port = defaults.RTT.Port
	}
	if c.UART.BaudRate == 0 {
		c.UART.BaudRate = defaults.UART.BaudRate
	}
	if c.Timeout == 0 {
		c.Timeout = defaults.Timeout
	}
	// Naming a port is a decision not to guess.
	if c.UART.Port != "" {
		c.UART.AutoDetect = false
	}
	return c
}

// Detect finds a source to read, and says what it tried.
//
// Returns every failure rather than the first, because "nothing found" is the one
// answer a person cannot act on: which of the three transports was close, and what
// each one said, is what turns it into something they can fix.
func Detect(config Config, log func(string)) (Source, error) {
	config = config.Normalize()

	var wanted []Kind
	if config.Backend == Kind("auto") || config.Backend == "" {
		wanted = AutoOrder
	} else {
		wanted = []Kind{config.Backend}
	}

	candidates := map[Kind]Source{
		KindRTT:  &rttSource{options: config.RTT, timeout: config.Timeout},
		KindUART: &uartSource{options: config.UART},
		KindSWO:  &swoSource{options: config.SWO, timeout: config.Timeout},
	}

	failures := []string{}
	for _, kind := range wanted {
		source := candidates[kind]
		if source == nil {
			failures = append(failures, fmt.Sprintf("%s: not a transport this component reads", kind))
			continue
		}
		if err := source.Available(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", kind, err))
			continue
		}
		if log != nil {
			log(fmt.Sprintf("reading target output over %s (%s)", kind, source.Describe()))
		}
		return source, nil
	}

	if len(failures) == 1 {
		return nil, fmt.Errorf("nothing to read: %s", failures[0])
	}
	// Sorted so the message is the same on every run of the same machine.
	sort.Strings(failures)
	return nil, fmt.Errorf("nothing to read. Tried, in order:\n  %s", strings.Join(failures, "\n  "))
}
