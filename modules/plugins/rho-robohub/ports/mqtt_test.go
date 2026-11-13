package main

import (
	"bufio"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testBroker is the little of an MQTT broker the port needs: it signs a client
// in when its name and password match, subscribes it to exact topics and to
// filters ending in '#', and hands every PUBLISH to the subscribers.
type testBroker struct {
	listener net.Listener
	mu       sync.Mutex
	subs     map[net.Conn][]string
	user     string
	password string
	refused  int
}

func newTestBroker(t *testing.T, user, password string) *testBroker {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &testBroker{listener: l, subs: map[net.Conn][]string{}, user: user, password: password}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	t.Cleanup(func() { l.Close() })
	return b
}

func (b *testBroker) addr() string { return b.listener.Addr().String() }

func readField(body []byte) (string, []byte) {
	n := int(binary.BigEndian.Uint16(body))
	return string(body[2 : 2+n]), body[2+n:]
}

func (b *testBroker) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		kind, body, err := mqttRead(r)
		if err != nil {
			b.mu.Lock()
			delete(b.subs, conn)
			b.mu.Unlock()
			return
		}
		switch kind >> 4 {
		case 1: // CONNECT
			_, rest := readField(body) // "MQTT"
			flags := rest[1]
			rest = rest[4:]
			_, rest = readField(rest) // client id
			user, password := "", ""
			if flags&0x80 != 0 {
				user, rest = readField(rest)
			}
			if flags&0x40 != 0 {
				password, _ = readField(rest)
			}
			if user != b.user || password != b.password {
				b.mu.Lock()
				b.refused++
				b.mu.Unlock()
				conn.Write([]byte{0x20, 2, 0, 5})
				return
			}
			conn.Write([]byte{0x20, 2, 0, 0})
		case 8: // SUBSCRIBE
			id := body[:2]
			filter, _ := readField(body[2:])
			b.mu.Lock()
			b.subs[conn] = append(b.subs[conn], filter)
			b.mu.Unlock()
			conn.Write([]byte{0x90, 3, id[0], id[1], 0})
		case 3: // PUBLISH
			topic, rest := readField(body)
			qos := (kind >> 1) & 3
			if qos > 0 {
				conn.Write([]byte{0x40, 2, rest[0], rest[1]})
				rest = rest[2:]
			}
			b.mu.Lock()
			for sub, filters := range b.subs {
				for _, f := range filters {
					if f == topic || (strings.HasSuffix(f, "#") && strings.HasPrefix(topic, strings.TrimSuffix(f, "#"))) {
						sub.Write(mqttPacket(0x30, append(mqttString(topic), rest...)))
					}
				}
			}
			b.mu.Unlock()
		case 12: // PINGREQ
			conn.Write([]byte{0xd0, 0})
		case 14: // DISCONNECT
			return
		}
	}
}

func TestAFollowedTopicKeepsItsNewestMessage(t *testing.T) {
	broker := newTestBroker(t, "mesh", "s3cret")
	dir := t.TempDir()
	secret := filepath.Join(dir, "password")
	os.WriteFile(secret, []byte("s3cret\n"), 0o600)
	events := filepath.Join(dir, "events.jsonl")
	if !mqttFollow(broker.addr(), "mesh", secret, "shelly/#", events) {
		t.Fatal("the filter was not followed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mqttPublish(broker.addr(), "mesh", secret, "shelly/plug/status", `{"on":true,"power":37.1}`, 1, false)
		if mqttLatest("shelly/plug/status") != "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := mqttLatest("shelly/plug/status"); got != `{"on":true,"power":37.1}` {
		t.Fatalf("the newest message is %q", got)
	}
	mqttPublish(broker.addr(), "mesh", secret, "shelly/plug/status", `{"on":false}`, 0, false)
	time.Sleep(200 * time.Millisecond)
	if got := mqttLatest("shelly/plug/status"); got != `{"on":false}` {
		t.Fatalf("a later message did not replace it: %q", got)
	}
	if topics := mqttTopics("shelly/"); !strings.HasPrefix(topics, "shelly/plug/status\t") {
		t.Fatalf("topics: %q", topics)
	}
	if raw, _ := os.ReadFile(events); !strings.Contains(string(raw), `"topic":"shelly/plug/status"`) {
		t.Fatalf("no event line was written: %q", raw)
	}
	if mqttPublish(broker.addr(), "mesh", secret, "shelly/#", "x", 0, false) {
		t.Fatal("a publish to a filter was sent")
	}
}

func TestAWrongPasswordIsRefused(t *testing.T) {
	broker := newTestBroker(t, "energy", "right")
	dir := t.TempDir()
	secret := filepath.Join(dir, "password")
	os.WriteFile(secret, []byte("wrong"), 0o600)
	if mqttPublish(broker.addr(), "energy", secret, "a/b", "x", 1, false) {
		t.Fatal("a publish with the wrong password was accepted")
	}
	if broker.refused == 0 {
		t.Fatal("the broker was not asked")
	}
}

func TestTheRemainingLengthIsEncodedAsMQTTSays(t *testing.T) {
	for n, want := range map[int][]byte{0: {0}, 127: {0x7f}, 128: {0x80, 0x01}, 16383: {0xff, 0x7f}, 16384: {0x80, 0x80, 0x01}} {
		if got := mqttLength(n); string(got) != string(want) {
			t.Errorf("%d: % x, want % x", n, got, want)
		}
	}
}

func TestARelayedFileArrivesLineForLine(t *testing.T) {
	broker := newTestBroker(t, "mesh", "s3cret")
	dir := t.TempDir()
	secret := filepath.Join(dir, "password")
	os.WriteFile(secret, []byte("s3cret"), 0o600)
	source := filepath.Join(dir, "telemetry-events.jsonl")
	copy := filepath.Join(dir, "mesh-live.jsonl")
	os.WriteFile(source, []byte("{\"old\":true}\n"), 0o600)
	if !mqttFollowLines(broker.addr(), "mesh", secret, "mesh/default/telemetry", copy) {
		t.Fatal("the topic was not followed")
	}
	if !mqttRelay(broker.addr(), "mesh", secret, source, "mesh/default/telemetry") {
		t.Fatal("the file was not relayed")
	}
	if mqttRelay(broker.addr(), "mesh", secret, source, "mesh/+/telemetry") {
		t.Fatal("a relay to a filter was started")
	}
	time.Sleep(300 * time.Millisecond) // the follower subscribes
	lines := `{"data":"{\"source\":\"ecf65959c911e247\",\"field\":\"power\",\"value\":\"37.1\"}","event":"telemetry"}` + "\n" +
		`{"data":"{\"source\":\"ecf65959c911e247\",\"field\":\"state\",\"value\":\"1\"}","event":"telemetry"}` + "\n"
	f, _ := os.OpenFile(source, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(lines[:40]) // half a line first: it waits for the rest
	f.Sync()
	time.Sleep(400 * time.Millisecond)
	f.WriteString(lines[40:])
	f.Close()
	deadline := time.Now().Add(3 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		got, _ = os.ReadFile(copy)
		if string(got) == lines {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if string(got) != lines {
		t.Fatalf("the copy is %q, want %q", got, lines)
	}
	os.Truncate(source, 0) // the writer starts the file again
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(source, []byte("{\"after\":\"truncate\"}\n"), 0o600)
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ = os.ReadFile(copy)
		if strings.HasSuffix(string(got), "{\"after\":\"truncate\"}\n") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("a line written after a truncation did not arrive: %q", got)
}
