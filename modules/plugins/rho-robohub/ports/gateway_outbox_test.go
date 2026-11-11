package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A consumer's acknowledgement drops only what every known consumer has stored,
// and the outbox and its cursors are readable by their owner only.
func TestOutboxKeepsACursorPerConsumer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox", "gateway-outbox.jsonl")
	gatewayOutbox = gatewayOutboxState{}
	if !gatewayOutboxOpen(path) {
		t.Fatal("the outbox did not open")
	}
	for i := 0; i < 4; i++ {
		gatewayOutboxAppend(1, 7, "temperature_c", float32(20+i), 0, "")
	}
	gatewayOutboxFlush()
	if removed := gatewayOutboxAckFor("client:mesh", 2); removed != 2 {
		t.Fatalf("the only consumer acknowledged 2, removed %d", removed)
	}
	// A second consumer appears and acknowledges less: nothing it has not
	// stored is dropped, whatever the first one acknowledged.
	if removed := gatewayOutboxAckFor("client:backup", 1); removed != 0 {
		t.Fatalf("a consumer behind the others removed %d", removed)
	}
	if removed := gatewayOutboxAckFor("client:mesh", 4); removed != 0 {
		t.Fatalf("the first consumer removed %d while the second was behind", removed)
	}
	if got := gatewayOutboxFetch(0, 10); !strings.Contains(got, `"seq":3`) || !strings.Contains(got, `"seq":4`) {
		t.Fatalf("records the second consumer has not stored are gone: %s", got)
	}
	if removed := gatewayOutboxAckFor("client:backup", 4); removed != 2 {
		t.Fatalf("once every consumer has them, they go; removed %d", removed)
	}
	for _, name := range []string{path, path + ".consumers"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s is %o, want 600", filepath.Base(name), info.Mode().Perm())
		}
	}
}
