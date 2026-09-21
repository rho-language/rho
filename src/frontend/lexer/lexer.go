//
// @file lexer.go
// @brief Rho Compiler - Lexer subsystem.
//
// This file implements the lexical analyzer (lexer) for the Rho language.
// It converts raw source text into a deterministic stream of tokens used
// by the parser. The lexer is designed to be simple, predictable, and
// fully compatible with the Rho → C compilation pipeline.
//
// @project Rho Programming Language
// @author Aurel Miron
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
	"fmt"
	"unicode"
	"unicode/utf8"
)

/* -------------------------------------------------------------------------- */
/*  Token Definitions                                                         */
/* -------------------------------------------------------------------------- */

// Kind represents the type of a token in the Rho language.
type Kind int

const (
	// Special
	TokenEOF Kind = iota
	TokenIllegal

	// Identifiers & literals
	TokenIdent
	TokenNumber
	TokenString

	// Keywords
	TokenModule
	TokenImport
	TokenInclude
	TokenStruct
	TokenEnum
	TokenFn
	TokenConfig
	TokenExtern
	TokenIf
	TokenFor
	TokenMatch
	TokenReturn
	TokenReverse
	TokenStep
	TokenLet
	TokenWhile
	TokenElse
	TokenBreak
	TokenTrue
	TokenFalse
	TokenType

	// Symbols
	TokenLBrace       // {
	TokenRBrace       // }
	TokenLParen       // (
	TokenRParen       // )
	TokenColon        // :
	TokenSemicolon    // ;
	TokenComma        // ,
	TokenDot          // .
	TokenAssign       // :=
	TokenArrow        // ->
	TokenRange        // ..
	TokenLBracket     // [
	TokenRBracket     // ]
	TokenPlus         // +
	TokenMinus        // -
	TokenStar         // *
	TokenSlash        // /
	TokenPercent      // %
	TokenEqual        // ==
	TokenNotEqual     // !=
	TokenLess         // <
	TokenLessEqual    // <=
	TokenGreater      // >
	TokenGreaterEqual // >=
	TokenAnd          // &&
	TokenOr           // ||
	TokenNot          // !
	TokenBitNot       // ~
	TokenCaret        // ^
	TokenBitAnd       // &
	TokenBitOr        // |
)

/* -------------------------------------------------------------------------- */
/*  Token Type                                                                */
/* -------------------------------------------------------------------------- */

// Token represents a single lexical unit produced by the lexer.
type Token struct {
	Kind Kind
	Lex  string
	Pos  int // byte offset in source
}

// Lex tokenizes a complete source string and returns the first lexical error.
func Lex(source string) ([]Token, error) {
	l := New(source)
	tokens := []Token{}
	for {
		token := l.Next()
		if token.Kind == TokenIllegal {
			return nil, fmt.Errorf("invalid token at byte %d: %s", token.Pos, token.Lex)
		}
		tokens = append(tokens, token)
		if token.Kind == TokenEOF {
			return tokens, nil
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Lexer Type                                                                */
/* -------------------------------------------------------------------------- */

// Lexer is the main lexical analyzer for Rho source code.
type Lexer struct {
	src    string
	pos    int
	length int
	ch     rune
	width  int
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new lexer instance for the given source string.
func New(source string) *Lexer {
	l := &Lexer{
		src:    source,
		length: len(source),
		pos:    0,
	}
	l.readChar()
	return l
}

// Next returns the next token from the input stream.
func (l *Lexer) Next() Token {
	l.skipWhitespace()
	startPos := l.pos

	if l.ch == 0 {
		return Token{Kind: TokenEOF, Lex: "", Pos: l.pos}
	}

	// Identifiers / keywords
	if isLetter(l.ch) {
		ident := l.readIdent()
		return Token{Kind: lookupKeyword(ident), Lex: ident, Pos: startPos}
	}

	// Numbers
	if isDigit(l.ch) {
		num := l.readNumber()
		return Token{Kind: TokenNumber, Lex: num, Pos: startPos}
	}

	// Strings
	if l.ch == '"' {
		str := l.readString()
		if str == "\x00unterminated" {
			return Token{Kind: TokenIllegal, Lex: "unterminated string", Pos: startPos}
		}
		return Token{Kind: TokenString, Lex: str, Pos: startPos}
	}

	// Symbols & operators
	switch l.ch {
	case '{':
		l.readChar()
		return Token{Kind: TokenLBrace, Lex: "{", Pos: startPos}
	case '}':
		l.readChar()
		return Token{Kind: TokenRBrace, Lex: "}", Pos: startPos}
	case '(':
		l.readChar()
		return Token{Kind: TokenLParen, Lex: "(", Pos: startPos}
	case ')':
		l.readChar()
		return Token{Kind: TokenRParen, Lex: ")", Pos: startPos}
	case '[':
		l.readChar()
		return Token{Kind: TokenLBracket, Lex: "[", Pos: startPos}
	case ']':
		l.readChar()
		return Token{Kind: TokenRBracket, Lex: "]", Pos: startPos}
	case ':':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: ":=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenColon, Lex: ":", Pos: startPos}
	case ';':
		l.readChar()
		return Token{Kind: TokenSemicolon, Lex: ";", Pos: startPos}
	case ',':
		l.readChar()
		return Token{Kind: TokenComma, Lex: ",", Pos: startPos}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenRange, Lex: "..", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenDot, Lex: ".", Pos: startPos}
	case '-':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenArrow, Lex: "->", Pos: startPos}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "-=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenMinus, Lex: "-", Pos: startPos}
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "+=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenPlus, Lex: "+", Pos: startPos}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "*=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenStar, Lex: "*", Pos: startPos}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "/=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenSlash, Lex: "/", Pos: startPos}
	case '%':
		l.readChar()
		return Token{Kind: TokenPercent, Lex: "%", Pos: startPos}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenNotEqual, Lex: "!=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenNot, Lex: "!", Pos: startPos}
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenEqual, Lex: "==", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenAssign, Lex: "=", Pos: startPos}
	case '<':
		if l.peekChar() == '<' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenLess, Lex: "<<", Pos: startPos}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenLessEqual, Lex: "<=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenLess, Lex: "<", Pos: startPos}
	case '>':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenGreater, Lex: ">>", Pos: startPos}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenGreaterEqual, Lex: ">=", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenGreater, Lex: ">", Pos: startPos}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAnd, Lex: "&&", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenBitAnd, Lex: "&", Pos: startPos}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenOr, Lex: "||", Pos: startPos}
		}
		l.readChar()
		return Token{Kind: TokenBitOr, Lex: "|", Pos: startPos}
	case '^':
		l.readChar()
		return Token{Kind: TokenCaret, Lex: "^", Pos: startPos}
	case '~':
		l.readChar()
		return Token{Kind: TokenBitNot, Lex: "~", Pos: startPos}
	}

	// Unknown character
	illegal := string(l.ch)
	l.readChar()
	return Token{Kind: TokenIllegal, Lex: illegal, Pos: startPos}
}

/* -------------------------------------------------------------------------- */
/*  Internal Helpers                                                          */
/* -------------------------------------------------------------------------- */

func (l *Lexer) readChar() {
	if l.pos >= l.length {
		l.ch = 0
		l.width = 0
		return
	}
	r, width := utf8.DecodeRuneInString(l.src[l.pos:])
	if r == utf8.RuneError && width == 1 {
		l.ch = 0
		l.width = 0
		return
	}
	l.ch = r
	l.width = width
	l.pos += width
}

func (l *Lexer) peekChar() rune {
	if l.pos >= l.length {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

func (l *Lexer) skipWhitespace() {
	for {
		for unicode.IsSpace(l.ch) {
			l.readChar()
		}
		if l.ch == '/' && l.peekChar() == '/' {
			for l.ch != 0 && l.ch != '\n' {
				l.readChar()
			}
			continue
		}
		break
	}
}

func (l *Lexer) readIdent() string {
	start := l.pos - l.width
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.src[start : l.pos-l.width]
}

func (l *Lexer) readNumber() string {
	start := l.pos - l.width
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar()
		l.readChar()
		for isDigit(l.ch) || (l.ch >= 'a' && l.ch <= 'f') || (l.ch >= 'A' && l.ch <= 'F') {
			l.readChar()
		}
		for isLetter(l.ch) || isDigit(l.ch) {
			l.readChar()
		}
		return l.src[start : l.pos-l.width]
	}
	for isDigit(l.ch) {
		l.readChar()
	}
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.src[start : l.pos-l.width]
}

func (l *Lexer) readString() string {
	// skip opening quote
	l.readChar()
	start := l.pos - l.width
	for l.ch != '"' && l.ch != 0 {
		l.readChar()
	}
	if l.ch == 0 {
		return "\x00unterminated"
	}
	str := l.src[start : l.pos-l.width]
	// skip closing quote
	if l.ch == '"' {
		l.readChar()
	}
	return str
}

func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isDigit(ch rune) bool {
	return unicode.IsDigit(ch)
}

func lookupKeyword(ident string) Kind {
	switch ident {
	case "module":
		return TokenModule
	case "import":
		return TokenImport
	case "include":
		return TokenInclude
	case "struct":
		return TokenStruct
	case "enum":
		return TokenEnum
	case "fn":
		return TokenFn
	case "config":
		return TokenConfig
	case "extern":
		return TokenExtern
	case "if":
		return TokenIf
	case "for":
		return TokenFor
	case "match":
		return TokenMatch
	case "return":
		return TokenReturn
	case "reverse":
		return TokenReverse
	case "step":
		return TokenStep
	case "let":
		return TokenLet
	case "while":
		return TokenWhile
	case "else":
		return TokenElse
	case "break":
		return TokenBreak
	case "true":
		return TokenTrue
	case "false":
		return TokenFalse
	case "type":
		return TokenType
	default:
		return TokenIdent
	}
}
