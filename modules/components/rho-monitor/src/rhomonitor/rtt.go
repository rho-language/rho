//
// @file rtt.go
// @brief Reading a SEGGER RTT stream through a socket.
//
// RTT is not a wire protocol on this side of the probe. The RTT buffer is a block
// of the target's own memory; the debug server that holds the probe reads it out
// and offers it on a TCP port, and a program that connects to that port is reading
// what the target wrote. Nothing here opens a probe, which is the whole reason
// this backend can run while a debugger is holding the same board.
//
// The consequence is that this file opens a socket and reads bytes. The RTT
// framing — the control block, the up/down buffers, the channel numbers — is
// between the target and the server, and a client that reimplements it would be
// reimplementing the server's job against a protocol version it cannot check.
//
// What is read here is a text stream, and it is treated as one: a console writes
// lines, and a line that arrives in pieces is joined rather than printed twice.
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
	"net"
	"strings"
	"time"
)

// rttSource is an RTT stream reached over TCP.
type rttSource struct {
	options RTTOptions
	timeout time.Duration
}

func (s *rttSource) Kind() Kind { return KindRTT }

func (s *rttSource) Describe() string {
	return fmt.Sprintf("%s channel %d", s.options.Endpoint(), s.options.Channel)
}

// Available is whether the server is listening.
//
// A connection attempt rather than a port probe, because a listening socket is
// not a working one: a port that something else holds answers just as readily, and
// the difference shows up as a stream of nothing. Connecting and closing costs a
// round trip and is the only test that means anything.
func (s *rttSource) Available() error {
	connection, err := s.dial()
	if err != nil {
		return fmt.Errorf("no RTT server at %s: %w", s.options.Endpoint(), err)
	}
	connection.Close()
	return nil
}

// dial opens the connection detection and reading both use.
func (s *rttSource) dial() (net.Conn, error) {
	address := s.options.Endpoint()
	timeout := s.timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	if _, ok := connection.(*net.TCPConn); ok {
		// No deadline: a console that has nothing to say for a while is a console
		// that is working. The run is ended by the person or by --duration.
		connection.SetDeadline(time.Time{})
	}
	return connection, nil
}

// Open connects and reads what the target writes.
func (s *rttSource) Open() (Stream, error) {
	connection, err := s.dial()
	if err != nil {
		return nil, fmt.Errorf("no RTT server at %s: %w", s.options.Endpoint(), err)
	}
	return &rttStream{
		connection: connection,
		channel:    s.options.Channel,
	}, nil
}

// rttStream is an open RTT stream.
type rttStream struct {
	connection net.Conn
	channel    int
	closed     bool
}

func (s *rttStream) Describe() string {
	return fmt.Sprintf("RTT channel %d", s.channel)
}

func (s *rttStream) Read(p []byte) (int, error) { return s.connection.Read(p) }

func (s *rttStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.connection.Close()
}

// ReadAll drains a stream until it ends, returning what it said.
//
// Exported because it is the whole of what reading means, and because it is what
// a test — and a pipeline that wants the output rather than a terminal — needs.
func ReadAll(stream Stream, limit int64) (string, error) {
	if stream == nil {
		return "", fmt.Errorf("no stream")
	}
	defer stream.Close()
	var builder strings.Builder
	buffer := make([]byte, 4096)
	var read int64
	for limit <= 0 || read < limit {
		want := len(buffer)
		if limit > 0 && read+int64(want) > limit {
			want = int(limit - read)
		}
		count, err := stream.Read(buffer[:want])
		if count > 0 {
			builder.Write(buffer[:count])
			read += int64(count)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			// A timeout on a console is not the end of it: a target that has
			// nothing to print right now is still running. Reported, not treated
			// as EOF, so a run does not appear to have ended when the board simply
			// went quiet.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return builder.String(), err
		}
	}
	return builder.String(), nil
}
