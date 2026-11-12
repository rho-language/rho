package main

import (
	"net"
	"testing"
	"time"
)

// A relay dials its parent, sends it frames closed with a zero, and reads the
// frames the parent sends back; the connection has a generation while it is up.
func TestARelayDialsItsParentAndCarriesFrames(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	if gatewayParentConnected() != 0 {
		t.Fatal("connected before dialling")
	}
	if !gatewayParentStart(listener.Addr().String()) {
		t.Fatal("the parent was not dialled")
	}
	var parent net.Conn
	select {
	case parent = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("the relay never dialled")
	}
	defer parent.Close()
	deadline := time.Now().Add(2 * time.Second)
	for gatewayParentConnected() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if gatewayParentConnected() == 0 {
		t.Fatal("no generation while connected")
	}
	if !gatewayParentSend("up") {
		t.Fatal("a frame could not be sent up")
	}
	buffer := make([]byte, 3)
	parent.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := parent.Read(buffer); err != nil || string(buffer) != "up\x00" {
		t.Fatalf("the parent read %q (%v)", buffer, err)
	}
	parent.Write([]byte("down\x00"))
	var got string
	for i := 0; i < 200 && got == ""; i++ {
		got = gatewayParentNext()
		time.Sleep(10 * time.Millisecond)
	}
	if got != "down" {
		t.Fatalf("the relay read %q", got)
	}
}
