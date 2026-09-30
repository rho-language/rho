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
	"rho/src/frontend/lexer"

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

/* -------------------------------------------------------------------------- */
/*  Token Types (LSP Standard)                                                */
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
/*  HandleSemanticTokens                                                      */
/* -------------------------------------------------------------------------- */

func HandleSemanticTokens(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.SemanticTokensParams
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)

	doc := ws.GetDocument(normalizedPath)
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
			uint32(len(tok.Lex)),
			uint32(kind),
			uint32(modifiers),
		)
		previousLine, previousColumn = tok.Line, tok.Column
	}
	ws.SetSemanticTokens(normalizedPath, append([]uint32{}, tokens...))

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
/*  Helpers                                                                   */
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

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)
	current, _ := ws.SemanticTokensFor(normalizedPath)
	if current == nil {
		full := HandleSemanticTokens(req, ws)
		return full
	}
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result: protocol.SemanticTokensDelta{
			ResultID: "1",
			Edits: []protocol.SemanticTokensEdit{
				{Start: 0, DeleteCount: len(current), Data: current},
			},
		},
	}
}

func classifyToken(doc *state.Document, index int) (int, int) {
	tok := doc.Tokens[index]
	if doc.Index != nil {
		for _, node := range doc.Index.Names {
			if node.Line == tok.Line && node.Column == tok.Column {
				switch node.Role {
				case state.RoleModule, state.RoleStruct, state.RoleType:
					return TokenType, 0
				case state.RoleFunction:
					return TokenFunction, 2
				case state.RoleField:
					return TokenField, 0
				case state.RoleVariable, state.RoleParameter:
					return TokenVariable, 0
				}
			}
		}
	}
	if index > 0 && doc.Tokens[index-1].Lex == "." {
		return TokenField, 0
	}
	if index > 0 && (doc.Tokens[index-1].Lex == "import" || doc.Tokens[index-1].Lex == "module") {
		return TokenType, 0
	}
	if index > 0 && doc.Tokens[index-1].Lex == "fn" {
		return TokenFunction, 2
	}
	// The compiler's own token kinds, rather than this server's names for them:
	// a token highlighted as a keyword is a keyword the compiler lexed, and the
	// two cannot disagree about what the language contains.
	switch tok.Kind {
	case lexer.TokenModule, lexer.TokenImport, lexer.TokenInclude, lexer.TokenStruct,
		lexer.TokenEnum, lexer.TokenFn, lexer.TokenConfig, lexer.TokenExtern,
		lexer.TokenIf, lexer.TokenFor, lexer.TokenMatch, lexer.TokenReturn,
		lexer.TokenReverse, lexer.TokenStep, lexer.TokenLet, lexer.TokenWhile,
		lexer.TokenElse, lexer.TokenBreak, lexer.TokenType, lexer.TokenRoute:
		return TokenKeyword, 0
	case lexer.TokenTrue, lexer.TokenFalse:
		return TokenType, 0
	case lexer.TokenNumber, lexer.TokenString:
		return TokenLiteral, 0
	case lexer.TokenAssign, lexer.TokenArrow, lexer.TokenEqual, lexer.TokenNotEqual,
		lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater,
		lexer.TokenGreaterEqual, lexer.TokenAnd, lexer.TokenOr, lexer.TokenNot:
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
