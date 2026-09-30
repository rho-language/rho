//
// @file server.go
// @brief Rho LSP Server - Enterprise v1.
//
// Implements the core JSON-RPC loop for the Rho Language Server.
//
// SPDX-License-Identifier: MIT
//

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rho/rho-lsp/src/handlers"
	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
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

// DocumentURItoPath converts VS Code document URIs (including vscode-remote://) to standard local paths.
func DocumentURItoPath(uri string) string {
	decoded, err := url.PathUnescape(uri)
	if err != nil {
		decoded = uri
	}

	// Handle vscode-remote:// URIs (e.g. vscode-remote://wsl+ubuntu/home/...)
	if strings.HasPrefix(decoded, "vscode-remote://") {
		parts := strings.SplitN(decoded, "/", 4)
		if len(parts) == 4 {
			return filepath.Clean("/" + parts[3])
		}
	}

	// Handle standard file:// URIs
	if strings.HasPrefix(decoded, "file://") {
		return filepath.Clean(strings.TrimPrefix(decoded, "file://"))
	}

	return filepath.Clean(decoded)
}

/* -------------------------------------------------------------------------- */
/*  Start                                                                     */
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
			// The stream is not where a message starts, so there is nothing to
			// resynchronise to and no way to know where the next one begins.
			// Dying is the only honest thing left, and the client is told why.
			return fmt.Errorf("no Content-Length: the stream is not a sequence of messages")
		}

		// Read JSON body
		body := make([]byte, length)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			return err
		}

		var req protocol.Request
		if err := json.Unmarshal(body, &req); err != nil {
			// The frame was the right length and the wrong contents. The stream
			// itself is still good — the next message starts where this one was
			// meant to — so a parse error is answered and the loop carries on.
			// Returning here killed the server over one malformed message.
			data, _ := json.Marshal(&protocol.Response{
				JsonRPC: "2.0",
				Error:   &protocol.Error{Code: -32700, Message: "the message is not JSON: " + err.Error()},
			})
			fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
			continue
		}

		// An exit is a request to stop, and it stops the loop after it is read.
		if req.Method == "exit" {
			return nil
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

			// Normalize the URI before querying s.workspace
			normalizedPath := state.NormalizeURI(params.TextDocument.URI)

			if doc := s.workspace.GetDocument(normalizedPath); doc != nil {
				// Keep original params.TextDocument.URI in the published notification
				// so VS Code UI matches the diagnostic target
				notification := protocol.Notification{
					JsonRPC: "2.0",
					Method:  "textDocument/publishDiagnostics",
					Params: map[string]any{
						"uri":         params.TextDocument.URI,
						"version":     doc.Version,
						"diagnostics": doc.Diags,
					},
				}
				data, _ := json.Marshal(notification)
				fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
			}
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Dispatch                                                                  */
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
		// Shutdown answers, and exit stops. Doing the first without the second
		// leaves a process reading a stream nobody is writing to, which is a
		// server that only goes away when the editor does.
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
