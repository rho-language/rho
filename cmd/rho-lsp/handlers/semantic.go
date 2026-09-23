//
// @file semantic.go
// @brief Rho LSP Semantic Tokens Handler - Enterprise v1.
//
// Implements the `textDocument/semanticTokens/full` request.
// Generates semantic tokens for syntax highlighting.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package handlers

import (
	"encoding/json"
	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

/* -------------------------------------------------------------------------- */
/*  Token Types (LSP Standard)                                                 */
/* -------------------------------------------------------------------------- */

const (
	TokenKeyword  = 0
	TokenType     = 1
	TokenFunction = 2
	TokenVariable = 3
	TokenField    = 4
	TokenLiteral  = 5
	TokenOperator = 6
)

/* -------------------------------------------------------------------------- */
/*  HandleSemanticTokens                                                       */
/* -------------------------------------------------------------------------- */

func HandleSemanticTokens(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.SemanticTokensParams
	_ = json.Unmarshal(raw, &params)

	uri := params.TextDocument.URI

	doc := ws.GetDocument(uri)
	if doc == nil {
		return emptySemantic(req)
	}

	tokens := []uint32{}

	// Iterate over lexical tokens
	previousLine, previousColumn := 0, 0
	for index, tok := range doc.Tokens {
		kind, modifiers := classifyToken(doc, index)

		// LSP semantic token format:
		// [deltaLine, deltaStart, length, tokenType, tokenModifiers]
		tokens = append(tokens,
			uint32(tok.Line-previousLine),
			uint32(func() int {
				if tok.Line == previousLine {
					return tok.Column - previousColumn
				}
				return tok.Column
			}()),
			uint32(len(tok.Text)),
			uint32(kind),
			uint32(modifiers),
		)
		previousLine, previousColumn = tok.Line, tok.Column
	}
	ws.SemanticTokens[uri] = append([]uint32{}, tokens...)

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result: protocol.SemanticTokens{
			ResultID: "1",
			Data:     tokens,
		},
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func emptySemantic(req protocol.Request) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result: protocol.SemanticTokens{
			Data: []uint32{},
		},
	}
}

func HandleSemanticTokensDelta(req protocol.Request, ws *state.Workspace) *protocol.Response {
	var params protocol.SemanticTokensDeltaParams
	raw, _ := json.Marshal(req.Params)
	_ = json.Unmarshal(raw, &params)
	current := ws.SemanticTokens[params.TextDocument.URI]
	if current == nil {
		full := HandleSemanticTokens(req, ws)
		return full
	}
	return &protocol.Response{JsonRPC: "2.0", ID: req.ID, Result: protocol.SemanticTokensDelta{ResultID: "1", Edits: []protocol.SemanticTokensEdit{{Start: 0, DeleteCount: len(current), Data: current}}}}
}

func classifyToken(doc *state.Document, index int) (int, int) {
	tok := doc.Tokens[index]
	if doc.AST != nil {
		for _, node := range doc.AST.Nodes {
			if node.Line() == tok.Line && node.Column() == tok.Column {
				switch node.Role() {
				case "module":
					return TokenType, 0
				case "function":
					return TokenFunction, 2
				case "field":
					return TokenField, 0
				}
			}
		}
	}
	if index > 0 && doc.Tokens[index-1].Text == "." {
		return TokenField, 0
	}
	if index > 0 && (doc.Tokens[index-1].Text == "import" || doc.Tokens[index-1].Text == "module") {
		return TokenType, 0
	}
	if index > 0 && doc.Tokens[index-1].Text == "fn" {
		return TokenFunction, 2
	}
	if tok.Kind == "keyword" {
		return TokenKeyword, 0
	}
	if tok.Kind == "type" {
		return TokenType, 0
	}
	if tok.Kind == "literal" {
		return TokenLiteral, 0
	}
	if tok.Kind == "operator" {
		return TokenOperator, 0
	}
	return TokenVariable, 0
}

func classifyLegacyToken(kind string, text string) int {
	switch kind {
	case "keyword":
		return TokenKeyword
	case "type":
		return TokenType
	case "fn":
		return TokenFunction
	case "identifier":
		return TokenVariable
	case "literal":
		return TokenLiteral
	case "operator":
		return TokenOperator
	default:
		return TokenVariable
	}
}
