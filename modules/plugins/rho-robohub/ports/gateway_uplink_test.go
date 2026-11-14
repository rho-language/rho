package main

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The uplink binds where it is told, then where the service binds, and only
// with neither on every interface.
func TestTheUplinkBindsWhereTheGatewayDoes(t *testing.T) {
	t.Setenv("UPLINK_BIND", "")
	t.Setenv("RHO_HTTP_ADDR", "")
	if got := gatewayUplinkBindAddress(); got != "" {
		t.Fatalf("with nothing said the uplink binds %q", got)
	}
	t.Setenv("RHO_HTTP_ADDR", ":8080")
	if got := gatewayUplinkBindAddress(); got != "" {
		t.Fatalf("a service on every interface binds the uplink to %q", got)
	}
	t.Setenv("RHO_HTTP_ADDR", "127.0.0.1:8080")
	if got := gatewayUplinkBindAddress(); got != "127.0.0.1" {
		t.Fatalf("a service on loopback binds the uplink to %q", got)
	}
	t.Setenv("UPLINK_BIND", "127.0.0.2")
	if got := gatewayUplinkBindAddress(); got != "127.0.0.2" {
		t.Fatalf("UPLINK_BIND is not what the uplink binds: %q", got)
	}
}

// closedSoon reports whether the peer closes a connection within a while.
func closedSoon(connection net.Conn, within time.Duration) bool {
	connection.SetReadDeadline(time.Now().Add(within))
	buffer := make([]byte, 64)
	for {
		_, err := connection.Read(buffer)
		if err == io.EOF {
			return true
		}
		if err != nil {
			if strings.Contains(err.Error(), "reset") {
				return true
			}
			return false
		}
	}
}

func takeFrame(within time.Duration) string {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if taken := gatewayUplinkTake(); taken != "" {
			return taken
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ""
}

// The listener faces a network: it holds a bounded number of connections,
// closes one that is idle or sends too much without a delimiter, and queues the
// frames the others close.
func TestTheUplinkBoundsWhatAPeerCanHold(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	t.Setenv("UPLINK_BIND", "127.0.0.1")
	t.Setenv("UPLINK_MAX_CONNECTIONS", "2")
	t.Setenv("UPLINK_IDLE_TIMEOUT_S", "1")
	if !gatewayUplinkStart(uint32(port)) {
		t.Fatal("the uplink did not start")
	}
	if host, _, _ := net.SplitHostPort(gatewayUplinkListener.Addr().String()); host != "127.0.0.1" {
		t.Fatalf("the uplink listens on %s", gatewayUplinkListener.Addr())
	}
	address := "127.0.0.1:" + strconv.Itoa(port)
	dial := func() net.Conn {
		connection, dialErr := net.Dial("tcp", address)
		if dialErr != nil {
			t.Fatal(dialErr)
		}
		return connection
	}
	open := func() int {
		gatewayUplinkMu.Lock()
		defer gatewayUplinkMu.Unlock()
		return len(gatewayUplinkByID)
	}
	waitOpen := func(want int) {
		deadline := time.Now().Add(3 * time.Second)
		for open() != want && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if open() != want {
			t.Fatalf("%d connections are open, want %d", open(), want)
		}
	}

	// A frame closed with a zero is queued with its connection.
	first := dial()
	defer first.Close()
	first.Write([]byte("abc\x00"))
	if taken := takeFrame(2 * time.Second); !strings.HasSuffix(taken, ":abc") {
		t.Fatalf("the frame was queued as %q", taken)
	}

	// Two are allowed; a third is closed at once.
	second := dial()
	defer second.Close()
	waitOpen(2)
	third := dial()
	defer third.Close()
	if !closedSoon(third, 2*time.Second) {
		t.Fatal("a connection past UPLINK_MAX_CONNECTIONS was kept")
	}

	// More than the pending cap without a delimiter closes the connection.
	second.Write(bytes.Repeat([]byte{0x41}, gatewayUplinkMaxPending+600))
	if !closedSoon(second, 2*time.Second) {
		t.Fatal("a connection that never closed a frame was kept")
	}

	// A connection that sends nothing is closed after the idle timeout.
	if !closedSoon(first, 3*time.Second) {
		t.Fatal("an idle connection was kept past UPLINK_IDLE_TIMEOUT_S")
	}
	waitOpen(0)

	// With the others gone there is room again.
	fourth := dial()
	defer fourth.Close()
	fourth.Write([]byte("xyz\x00"))
	if taken := takeFrame(2 * time.Second); !strings.HasSuffix(taken, ":xyz") {
		t.Fatalf("a connection after the others closed was not heard: %q", taken)
	}
}
