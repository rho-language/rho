package server

import (
	"github.com/rho/rho-lsp/src/protocol"
	"testing"
)

func TestCancelRequestIsRecorded(t *testing.T) {
	s := New()
	s.dispatch(protocol.Request{Method: "$/cancelRequest", Params: map[string]any{"id": 42}})
	s.cancelMu.Lock()
	cancelled := s.cancelled["42"]
	s.cancelMu.Unlock()
	if !cancelled {
		t.Fatal("cancel request was not recorded")
	}
}
