//
// @file swo.go
// @brief Decoding an ARM ITM/SWO trace.
//
// This is the one backend that has a real protocol on this side, because the SWO
// stream is the raw output of the core's trace port rather than a socket a server
// decoded for us. The bytes are ITM packets, and a packet's low bit says whether
// another follows — which is what makes the stream self-delimiting with no framing
// of its own.
//
// What is decoded is the ITM software and hardware console ports, which are the two
// a firmware prints through: port 0 is `ITM_SendChar`-style output, port 1 is the
// hardware trace port. Everything else — timestamp packets, the reserved header
// range, extension packets for counters and clocks — is recognised and dropped
// rather than printed as rubbish.
//
// The cost of this backend is stated in its own module: draining the trace port is
// the debug probe's job, so a SWO stream and a debug session cannot both have it.
// That is why it is last in the detection order and never chosen over a working
// RTT or UART.
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
	"io"
	"os"
	"strings"
	"time"
)

// The ITM stimulus ports a firmware prints through.
const (
	// ITMPortSoftware is the software console: what `ITM_SendChar` and most
	// retargeted `printf` end up writing.
	ITMPortSoftware = 0
	// ITMPortHardware is the hardware console, which is port 1 unless the trace
	// was configured otherwise.
	ITMPortHardware = 1
)

// SWO packet header bytes.
//
// The core emits these when tracing is switched on, and they are what makes a
// trace readable at all: without them an ITM stream is a line of nothing and an
// occasional byte. Recognised by their upper nibble, which is why the reserved
// range is dropped rather than printed.
const (
	// SWOHeaderTime is an "I know my overhead" packet: the core says it needed
	// N cycles to get here.
	SWOHeaderTime = 0x00
	// SWOHeaderStart identifies a trace format the reader may not understand.
	SWOHeaderStart = 0x01
	// SWOHeaderCount is a counter extension packet.
	SWOHeaderCount = 0x02
	// SWOHeaderClock is a clock-frequency extension packet.
	SWOHeaderClock = 0x03
	// SWOHeaderReserved is the rest of the header range, none of which carries
	// console output.
	SWOHeaderReserved = 0x0F
)

// SWOOptions is where a trace is read from.
//
// Either a file the capturing program wrote, or a socket it is writing to. A file
// is what `openocd`'s `swo` capture produces and what a CI job keeps; a socket is
// what a live capture offers. Both, because both exist and a monitor that only
// read one would be a monitor somebody has to wrap.
type SWOOptions struct {
	// File is a capture on disk, read to the end and then followed if it grows.
	File string
	// Host and Port are a live capture.
	Host string
	Port int
	// Follow keeps reading after the end, which is what makes a capture file
	// useful while the board is still running.
	Follow bool
}

// swoSource is an ITM/SWO trace.
type swoSource struct {
	options SWOOptions
	timeout time.Duration
	// ports are the stimulus ports whose output is kept. Zero value means the
	// software and hardware consoles.
	ports []int
}

func (s *swoSource) Kind() Kind { return KindSWO }

func (s *swoSource) Describe() string {
	switch {
	case s.options.File != "":
		return fmt.Sprintf("%s%s", s.options.File, followNote(s.options.Follow))
	case s.options.Host != "":
		return fmt.Sprintf("ITM trace from %s:%d", s.options.Host, s.options.Port)
	default:
		return "the ITM trace"
	}
}

func followNote(follow bool) string {
	if follow {
		return ", followed"
	}
	return ""
}

// Available is whether there is a capture to read.
func (s *swoSource) Available() error {
	if s.options.File != "" {
		if _, err := os.Stat(s.options.File); err != nil {
			return fmt.Errorf("no trace capture at %s", s.options.File)
		}
		return nil
	}
	if s.options.Host != "" {
		if _, err := dialTCP(s.options.Host, s.options.Port, s.timeout); err != nil {
			return fmt.Errorf("no trace capture at %s:%d: %w", s.options.Host, s.options.Port, err)
		}
		return nil
	}
	return fmt.Errorf("no capture to read: set `monitor.swo.file`, or a host and port for a live one")
}

// Open opens the capture for decoding.
func (s *swoSource) Open() (Stream, error) {
	var reader io.ReadCloser
	var description string
	switch {
	case s.options.File != "":
		file, err := os.Open(s.options.File)
		if err != nil {
			return nil, fmt.Errorf("read the trace capture: %w", err)
		}
		reader, description = file, s.Describe()
	case s.options.Host != "":
		connection, err := dialTCP(s.options.Host, s.options.Port, s.timeout)
		if err != nil {
			return nil, err
		}
		reader, description = connection, s.Describe()
	default:
		return nil, fmt.Errorf("no capture to read: set `monitor.swo.file`, or a host and port for a live one")
	}
	return &swoStream{
		reader:  reader,
		decoder: NewSWODecoder(s.ports),
		what:    description,
	}, nil
}

// swoStream is an open trace, decoded as it is read.
type swoStream struct {
	reader  io.ReadCloser
	decoder *SWODecoder
	what    string
	closed  bool
}

func (s *swoStream) Describe() string { return s.what }

// Read decodes the trace into text.
//
// The decoder keeps whatever a packet split across two reads left over, so a read
// boundary in the middle of a packet is a thing that happens and not a thing that
// corrupts the output.
func (s *swoStream) Read(p []byte) (int, error) {
	buffer := make([]byte, len(p))
	count, err := s.reader.Read(buffer)
	if count > 0 {
		text := s.decoder.Feed(buffer[:count])
		written := copy(p, []byte(text))
		return written, nilIfShort(written, len(text), err)
	}
	return 0, err
}

func (s *swoStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.reader.Close()
}

// SWODecoder turns an ITM packet stream into the text a firmware printed.
//
// Stateful in exactly one way: a packet split across two reads has to be joined
// before it can be read, because its length is only known once its last byte
// arrives. Everything else is a pure function of the bytes, which is what makes it
// testable against captured traces.
type SWODecoder struct {
	// ports kept. Empty means the software and hardware consoles.
	ports map[int]bool
	// pending is a packet not yet complete.
	pending []byte
	// decoded is what a Feed produced but the last Read did not take. A console
	// writes a line at a time and the reader takes a buffer at a time, and those
	// are not the same size.
	decoded string
	// dropped counts packets that were recognised and not console output, kept so
	// a reader can say how much of the trace was not text.
	dropped int
	// emitted counts the console packets that were kept.
	emitted int
}

// NewSWODecoder is a decoder over the given stimulus ports, or the software and
// hardware consoles when none are named.
func NewSWODecoder(ports []int) *SWODecoder {
	kept := map[int]bool{}
	for _, port := range ports {
		kept[port] = true
	}
	if len(kept) == 0 {
		kept[ITMPortSoftware] = true
		kept[ITMPortHardware] = true
	}
	return &SWODecoder{ports: kept}
}

// Feed is bytes from the trace, and the text they carried.
//
// Everything decoded is returned, including text a previous Feed produced and the
// reader did not take: a decoded line and a read buffer are different sizes, so
// one read can carry several lines and several reads can carry part of one.
func (d *SWODecoder) Feed(raw []byte) string {
	if d.decoded != "" {
		carry := d.decoded
		d.decoded = ""
		return carry + d.consume(raw)
	}
	return d.consume(raw)
}

// Take is the text decoded so far and leaves the decoder empty.
func (d *SWODecoder) Take() string {
	out := d.decoded
	d.decoded = ""
	return out
}

// consume decodes as many whole packets as `raw` holds, and returns the text.
//
// A packet left over from the last read goes in front of what arrived this time:
// it was held precisely because it was not yet whole, and a decoder that held it
// without ever completing it would lose the packet and everything after it.
func (d *SWODecoder) consume(raw []byte) string {
	if len(d.pending) > 0 {
		raw = append(append([]byte{}, d.pending...), raw...)
		d.pending = nil
	}
	out := &strings.Builder{}
	for len(raw) > 0 {
		first := raw[0]
		// The low bit says whether another byte of this packet follows, so the
		// packet is 1, 2 or 4 bytes. This is the whole of the framing.
		var size int
		// A console packet is always two bytes: a header and one character.
		// Recognised before the continuation walk, because the walk cannot be
		// trusted here — the header carries the character's own bit 0 in its bit 1,
		// so a character with bit 0 set would otherwise look like the start of a
		// four-byte packet and the framing would lose its place.
		if _, console := consolePort(first); console {
			size = 2
		} else {
			switch {
			case first&0x01 == 0:
				size = 1
			case len(raw) < 2:
				size = 2
			case raw[1]&0x01 == 0:
				size = 2
			case len(raw) < 4:
				size = 4
			case raw[3]&0x01 == 0:
				size = 4
			default:
				// A packet that says more of itself follows but never ends. One
				// more byte is not enough to make sense of and dropping the whole
				// trace is worse than skipping this packet.
				d.dropped++
				raw = raw[1:]
				continue
			}
		}
		if len(raw) < size {
			// Not yet. Kept for the next read, which is why this decoder has state.
			d.pending = append([]byte{}, raw...)
			break
		}
		out.WriteString(d.packet(raw[:size]))
		raw = raw[size:]
	}
	return out.String()
}

// consolePort is which console a packet header introduces, and whether it
// introduces one at all.
//
// A console packet is two bytes: a header and one character. Three bits of the
// header say which:
//
//   - bit 0 is the continuation flag, and a console packet is two bytes, so it is
//     always set. A header without it is a one-byte packet — a header or a bare
//     data value — and never a character.
//   - bit 7 is the console: clear for the software port, set for the hardware one.
//   - bit 1 says the character that follows has its own bit 0 set.
//
// Every other bit is part of the wider port field, which a monitor does not read:
// a port above the two consoles carries binary telemetry. A header with anything
// else set is therefore not a console packet and is dropped.
// It is not part of the port, and it is the reason a console packet cannot be
// framed by walking continuation bits: `0x01 'h'` is a packet and `0x03 'i'` is a
// packet, but walking the bits of the second would read the character as a
// continuation and go looking for a four-byte packet that is never coming.
//
// That is narrower than a full ITM header decode, and deliberately so. The other
// stimulus ports carry binary telemetry, which a monitor has no business printing,
// and a general port decode here would be a second demultiplexer to keep in step
// with a core rather than a reader. OpenOCD's `itm` demux already does that, and
// this component reads a console.
func consolePort(first byte) (int, bool) {
	if first&0x01 == 0 {
		// A one-byte packet: a header in the reserved range, or a bare data value
		// from a port. Neither is a character.
		return 0, false
	}
	// Bits 7 and 1 are the ones a console header uses; anything else set means a
	// wider port this component does not read.
	const consoleMask = 0x80 | 0x02
	if first&^byte(consoleMask) != 0x01 {
		// Not a console header: the only bits a console packet may set are bit 0,
		// bit 7 and bit 1.
		return 0, false
	}
	if first&0x80 != 0 {
		return ITMPortHardware, true
	}
	return ITMPortSoftware, true
}

// packet is one ITM packet, and whatever text it carried.
//
// Three shapes:
//
//   - A **one-byte** packet is either a header or a bare data value. The header
//     range is the first sixteen — the core's own instrumentation: overhead, trace
//     format, counters, clocks — and none of it is console output. A one-byte
//     packet outside that range is a data value from a port, with no payload, and
//     printing its bits as a character would print a firmware's own telemetry as
//     rubbish.
//   - A **console packet** is two bytes and its payload is one character.
//   - Anything else is a data packet, counted and dropped.
func (d *SWODecoder) packet(raw []byte) string {
	first := raw[0]

	if len(raw) == 1 {
		if first&0xF0 <= SWOHeaderReserved {
			d.dropped++
			return ""
		}
		d.dropped++
		return ""
	}

	port, console := consolePort(first)
	if !console {
		d.dropped++
		return ""
	}
	if !d.ports[port] {
		d.dropped++
		return ""
	}
	if len(raw) < 2 {
		d.dropped++
		return ""
	}
	d.emitted++
	return string(rune(raw[1]))
}

// Counts is how much of the trace was console output and how much was not.
func (d *SWODecoder) Counts() (emitted, dropped int) { return d.emitted, d.dropped }

// Pending is how many bytes of an unfinished packet are held.
func (d *SWODecoder) Pending() int { return len(d.pending) }

// nilIfShort is the error a short read reports.
//
// A decoder that produced more text than the buffer held has not failed: the rest
// is in the decoder and comes out of the next read. Reporting io.EOF there would
// end a run in the middle of the output.
func nilIfShort(written, produced int, err error) error {
	if written >= produced && err == io.EOF {
		return nil
	}
	if written < produced {
		return nil
	}
	return err
}
