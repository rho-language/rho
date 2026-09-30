package handlers

import (
	"encoding/json"

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

func HandleDidClose(req protocol.Request, ws *state.Workspace) *protocol.Response {
	var params protocol.DidCloseTextDocumentParams
	raw, _ := json.Marshal(req.Params)
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)
	ws.CloseDocument(normalizedPath)

	return nil
}
