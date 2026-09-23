package handlers

import (
	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
	"testing"
)

func TestHandleDidCloseRemovesDocument(t *testing.T) {
	ws := state.NewWorkspace()
	ws.OpenDocument("file:///close.rho", "module close")
	HandleDidClose(protocol.Request{JsonRPC: "2.0", Method: "textDocument/didClose", Params: map[string]any{"textDocument": map[string]any{"uri": "file:///close.rho"}}}, ws)
	if ws.GetDocument("file:///close.rho") != nil {
		t.Fatal("document still present")
	}
}
