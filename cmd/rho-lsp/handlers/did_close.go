package handlers

import (
	"encoding/json"
	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

func HandleDidClose(req protocol.Request, ws *state.Workspace) *protocol.Response {
	var params protocol.DidCloseTextDocumentParams
	raw, _ := json.Marshal(req.Params)
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)
	ws.CloseDocument(normalizedPath)

	return nil
}