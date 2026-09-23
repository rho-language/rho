//
// @file server.go
// @brief Rho LSP Server - Enterprise v1.
//
// Implements the core JSON-RPC loop for the Rho Language Server.
// Responsible for:
//   - reading LSP messages
//   - decoding JSON-RPC
//   - dispatching to handlers
//   - writing responses
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"rho/cmd/rho-lsp/handlers"
	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

type Server struct {
	workspace *state.Workspace
	cancelMu  sync.Mutex
	cancelled map[string]bool
}

func New() *Server {
	return &Server{
		workspace: state.NewWorkspace(), cancelled: map[string]bool{},
	}
}

/* -------------------------------------------------------------------------- */
/*  Start                                                                      */
/* -------------------------------------------------------------------------- */

func (s *Server) Start(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	for {
		// Read header: Content-Length
		length := -1
		for {
			header, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			trimmed := strings.TrimSpace(header)
			if trimmed == "" {
				break
			}
			if strings.Contains(strings.ToLower(trimmed), "content-length:") {
				value := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
				parsed, parseErr := strconv.Atoi(value)
				if parseErr != nil || parsed < 0 {
					return fmt.Errorf("invalid Content-Length: %q", value)
				}
				length = parsed
			}
		}
		if length < 0 {
			return fmt.Errorf("missing Content-Length")
		}

		// Read JSON body
		body := make([]byte, length)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			return err
		}

		var req protocol.Request
		if err := json.Unmarshal(body, &req); err != nil {
			return fmt.Errorf("decode LSP request: %w", err)
		}

		resp := s.dispatch(req)

		// JSON-RPC notifications (no "id") must never receive a response.
		if resp != nil && req.ID != nil {
			data, _ := json.Marshal(resp)
			fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
		}
		if req.Method == "textDocument/didOpen" || req.Method == "textDocument/didChange" {
			var params struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			raw, _ := json.Marshal(req.Params)
			_ = json.Unmarshal(raw, &params)
			if doc := s.workspace.GetDocument(params.TextDocument.URI); doc != nil {
				notification := protocol.Notification{JsonRPC: "2.0", Method: "textDocument/publishDiagnostics", Params: map[string]any{"uri": doc.URI, "version": doc.Version, "diagnostics": doc.Diags}}
				data, _ := json.Marshal(notification)
				fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
			}
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Dispatch                                                                   */
/* -------------------------------------------------------------------------- */

func (s *Server) dispatch(req protocol.Request) *protocol.Response {
	if req.Method == "$/cancelRequest" {
		var params protocol.CancelParams
		raw, _ := json.Marshal(req.Params)
		_ = json.Unmarshal(raw, &params)
		s.cancelMu.Lock()
		s.cancelled[fmt.Sprint(params.ID)] = true
		s.cancelMu.Unlock()
		return nil
	}
	switch req.Method {

	case "initialize":
		return handlers.HandleInitialize(req, s.workspace)

	case "textDocument/didOpen":
		return handlers.HandleDidOpen(req, s.workspace)

	case "textDocument/didChange":
		return handlers.HandleDidChange(req, s.workspace)
	case "textDocument/didClose":
		return handlers.HandleDidClose(req, s.workspace)
	case "shutdown":
		return &protocol.Response{JsonRPC: "2.0", ID: req.ID, Result: nil}
	case "exit":
		return nil

	case "textDocument/completion":
		return handlers.HandleCompletion(req, s.workspace)

	case "textDocument/hover":
		return handlers.HandleHover(req, s.workspace)

	case "textDocument/definition":
		return handlers.HandleDefinition(req, s.workspace)

	case "textDocument/semanticTokens/full":
		return handlers.HandleSemanticTokens(req, s.workspace)
	case "textDocument/semanticTokens/full/delta":
		return handlers.HandleSemanticTokensDelta(req, s.workspace)

	case "textDocument/references":
		return handlers.HandleReferences(req, s.workspace)

	default:
		return &protocol.Response{
			JsonRPC: "2.0",
			ID:      req.ID,
			Error:   &protocol.Error{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) HandleMessage(data []byte) []byte {
	var req protocol.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil
	}
	response := s.dispatch(req)
	// JSON-RPC notifications (no "id") must never receive a response.
	if response == nil || req.ID == nil {
		return nil
	}
	out, _ := json.Marshal(response)
	return out
}
