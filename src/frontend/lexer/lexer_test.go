//
// @file lexer_test.go
// @brief Rho Compiler - Lexer subsystem tests.
//
// This file contains deterministic unit tests for the Rho lexer.
// All tests validate correct tokenization of identifiers, keywords,
// literals, operators, and symbols according to the Rho specification.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package lexer

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func tok(k Kind, lex string) Token {
	return Token{Kind: k, Lex: lex}
}

/* -------------------------------------------------------------------------- */
/*  Tests                                                                      */
/* -------------------------------------------------------------------------- */

func TestLexerIdentifiers(t *testing.T) {
	src := "alpha beta gamma123 _delta"
	l := New(src)

	expected := []Token{
		tok(TokenIdent, "alpha"),
		tok(TokenIdent, "beta"),
		tok(TokenIdent, "gamma123"),
		tok(TokenIdent, "_delta"),
		tok(TokenEOF, ""),
	}

	for i, exp := range expected {
		tok := l.Next()
		if tok.Kind != exp.Kind || tok.Lex != exp.Lex {
			t.Fatalf("token %d mismatch: got (%v, %q), expected (%v, %q)",
				i, tok.Kind, tok.Lex, exp.Kind, exp.Lex)
		}
	}
}

func TestLexerKeywords(t *testing.T) {
	src := "module import struct enum fn config extern if for match return reverse step"
	l := New(src)

	expectedKinds := []Kind{
		TokenModule, TokenImport, TokenStruct, TokenEnum, TokenFn,
		TokenConfig, TokenExtern, TokenIf, TokenFor, TokenMatch,
		TokenReturn, TokenReverse, TokenStep,
		TokenEOF,
	}

	for i, k := range expectedKinds {
		tok := l.Next()
		if tok.Kind != k {
			t.Fatalf("keyword %d mismatch: got %v, expected %v", i, tok.Kind, k)
		}
	}
}

func TestLexerBlockComments(t *testing.T) {
	l := New("module demo /* section\n details */ fn main")
	if token := l.Next(); token.Kind != TokenModule {
		t.Fatalf("expected module, got %v", token)
	}
	if token := l.Next(); token.Kind != TokenIdent || token.Lex != "demo" {
		t.Fatalf("expected demo, got %v", token)
	}
	if token := l.Next(); token.Kind != TokenFn {
		t.Fatalf("expected fn after block comment, got %v", token)
	}
	if token := l.Next(); token.Kind != TokenIdent || token.Lex != "main" {
		t.Fatalf("expected main, got %v", token)
	}
}

func TestLexerNumbers(t *testing.T) {
	src := "0 42 9999"
	l := New(src)

	expected := []Token{
		tok(TokenNumber, "0"),
		tok(TokenNumber, "42"),
		tok(TokenNumber, "9999"),
		tok(TokenEOF, ""),
	}

	for i, exp := range expected {
		tok := l.Next()
		if tok.Kind != exp.Kind || tok.Lex != exp.Lex {
			t.Fatalf("number %d mismatch: got (%v, %q), expected (%v, %q)",
				i, tok.Kind, tok.Lex, exp.Kind, exp.Lex)
		}
	}
}

func TestLexerStrings(t *testing.T) {
	src := `"hello" "rho-lang" ""`
	l := New(src)

	expected := []Token{
		tok(TokenString, "hello"),
		tok(TokenString, "rho-lang"),
		tok(TokenString, ""),
		tok(TokenEOF, ""),
	}

	for i, exp := range expected {
		tok := l.Next()
		if tok.Kind != exp.Kind || tok.Lex != exp.Lex {
			t.Fatalf("string %d mismatch: got (%v, %q), expected (%v, %q)",
				i, tok.Kind, tok.Lex, exp.Kind, exp.Lex)
		}
	}
}

func TestLexerRejectsUnterminatedString(t *testing.T) {
	if _, err := Lex(`"unterminated`); err == nil {
		t.Fatalf("expected unterminated string error")
	}
}

func TestLexerPreservesUTF8Identifiers(t *testing.T) {
	l := New("naive café")
	if got := l.Next(); got.Lex != "naive" {
		t.Fatalf("unexpected first identifier: %q", got.Lex)
	}
	if got := l.Next(); got.Lex != "café" {
		t.Fatalf("unexpected UTF-8 identifier: %q", got.Lex)
	}
}

func TestLexerSymbols(t *testing.T) {
	src := "{ } ( ) : ; , . := .. ->"
	l := New(src)

	expected := []Token{
		tok(TokenLBrace, "{"),
		tok(TokenRBrace, "}"),
		tok(TokenLParen, "("),
		tok(TokenRParen, ")"),
		tok(TokenColon, ":"),
		tok(TokenSemicolon, ";"),
		tok(TokenComma, ","),
		tok(TokenDot, "."),
		tok(TokenAssign, ":="),
		tok(TokenRange, ".."),
		tok(TokenArrow, "->"),
		tok(TokenEOF, ""),
	}

	for i, exp := range expected {
		tok := l.Next()
		if tok.Kind != exp.Kind || tok.Lex != exp.Lex {
			t.Fatalf("symbol %d mismatch: got (%v, %q), expected (%v, %q)",
				i, tok.Kind, tok.Lex, exp.Kind, exp.Lex)
		}
	}
}
