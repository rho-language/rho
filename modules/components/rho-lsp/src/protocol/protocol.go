//
// @file protocol.go
// @brief Rho LSP Protocol Types - Enterprise v1.
//
// Defines JSON-RPC and LSP protocol structures used by the Rho Language Server.
// These types are used for request/response encoding and decoding.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package protocol

/* -------------------------------------------------------------------------- */
/*  JSON-RPC Base Types                                                       */
/* -------------------------------------------------------------------------- */

type Request struct {
	JsonRPC string         `json:"jsonrpc"`
	ID      interface{}    `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type Response struct {
	JsonRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	// Result is always sent, even when it is null. A response with no result key
	// is not a response: the specification says a result is null when there is
	// none, and clients that asked to shut down read that null as the answer they
	// were waiting for.
	Result any    `json:"result"`
	Error  *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Notification struct {
	JsonRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type CancelParams struct {
	ID interface{} `json:"id"`
}

/* -------------------------------------------------------------------------- */
/*  LSP Core Types                                                             */
/* -------------------------------------------------------------------------- */

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Message  string `json:"message"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

/* -------------------------------------------------------------------------- */
/*  Text Document Types                                                        */
/* -------------------------------------------------------------------------- */

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type InitializeParams struct {
	RootURI          string `json:"rootUri"`
	WorkspaceFolders []struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	} `json:"workspaceFolders"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Text  string `json:"text"`
	Range *Range `json:"range,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Completion Types                                                           */
/* -------------------------------------------------------------------------- */

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Hover Types                                                                */
/* -------------------------------------------------------------------------- */

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Hover struct {
	Contents any   `json:"contents"`
	Range    Range `json:"range,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Definition Types                                                           */
/* -------------------------------------------------------------------------- */

type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

/* -------------------------------------------------------------------------- */
/*  References Types                                                           */
/* -------------------------------------------------------------------------- */

type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

/* -------------------------------------------------------------------------- */
/*  Semantic Tokens Types                                                      */
/* -------------------------------------------------------------------------- */

type SemanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type SemanticTokensDeltaParams struct {
	TextDocument     TextDocumentIdentifier `json:"textDocument"`
	PreviousResultID string                 `json:"previousResultId"`
}

type SemanticTokens struct {
	ResultID string   `json:"resultId,omitempty"`
	Data     []uint32 `json:"data"`
}

type SemanticTokensEdit struct {
	Start       int      `json:"start"`
	DeleteCount int      `json:"deleteCount"`
	Data        []uint32 `json:"data,omitempty"`
}
type SemanticTokensDelta struct {
	ResultID string               `json:"resultId,omitempty"`
	Edits    []SemanticTokensEdit `json:"edits"`
}
