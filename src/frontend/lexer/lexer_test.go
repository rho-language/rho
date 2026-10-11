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
// SPDX-License-Identifier: Apache-2.0
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
	src := "module import struct enum fn extern if for match return reverse step let const var while else break true false"
	l := New(src)

	expectedKinds := []Kind{
		TokenModule, TokenImport, TokenStruct, TokenEnum, TokenFn,
		TokenExtern, TokenIf, TokenFor, TokenMatch,
		TokenReturn, TokenReverse, TokenStep,
		TokenLet, TokenConst, TokenVar, TokenWhile, TokenElse, TokenBreak,
		TokenTrue, TokenFalse,
		TokenEOF,
	}

	for i, k := range expectedKinds {
		tok := l.Next()
		if tok.Kind != k {
			t.Fatalf("keyword %d mismatch: got %v, expected %v", i, tok.Kind, k)
		}
	}
}

// `config` is a word, not a keyword: no syntax claimed it, so a program may use
// it as a name.
func TestLexerConfigIsAnIdentifier(t *testing.T) {
	l := New("config")
	if token := l.Next(); token.Kind != TokenIdent || token.Lex != "config" {
		t.Fatalf("expected the identifier config, got %v %q", token.Kind, token.Lex)
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

// A token says where it is. Everything downstream — a parser, a type checker, a
// language server — needs a line and a column to point at, and a byte offset
// alone makes every one of them count newlines for itself.
func TestATokenSaysWhereItIs(t *testing.T) {
	source := "module shop\n\nfn run() void {\n    let x: i32 = 1;\n}\n"
	tokens, err := Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		lex       string
		line, col int
	}{
		{"module", 1, 1},
		{"shop", 1, 8},
		{"fn", 3, 2},
		{"run", 3, 5},
		{"let", 4, 6},
		{"x", 4, 10},
		{";", 4, 20},
	}
	seen := 0
	for _, token := range tokens {
		if token.Kind == TokenEOF {
			break
		}
		// Only the tokens this test names, so a change elsewhere in the language
		// does not fail a test about positions.
		found := false
		for _, w := range want {
			if token.Lex == w.lex {
				found = true
				if token.Line != w.line || token.Column != w.col {
					t.Errorf("%q is at %d:%d, want %d:%d", token.Lex, token.Line, token.Column, w.line, w.col)
				}
			}
		}
		if found {
			seen++
		}
	}
	if seen < len(want) {
		t.Errorf("only %d of the %d tokens this test names were produced", seen, len(want))
	}
}

// A column counts characters, not bytes. One that counted bytes would put
// everything after a character with an accent in it in the wrong place, and an
// editor that highlights the wrong character is worse than one that highlights
// nothing.
func TestAColumnCountsCharactersNotBytes(t *testing.T) {
	source := "module café\n"
	tokens, err := Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if token.Lex == "café" {
			if token.Line != 1 || token.Column != 8 {
				t.Errorf("%q is at %d:%d, want 1:8", token.Lex, token.Line, token.Column)
			}
			return
		}
	}
	t.Fatal("the accented name was not lexed as one token")
}
