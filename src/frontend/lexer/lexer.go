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
// SPDX-License-Identifier: Apache-2.0
//

package lexer

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"fmt"
	"strings"
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
	TokenRoute

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
	TokenFatArrow     // =>
)

/* -------------------------------------------------------------------------- */
/*  Token Type                                                                */
/* -------------------------------------------------------------------------- */

// Token represents a single lexical unit produced by the lexer.
type Token struct {
	Kind Kind
	Lex  string
	Pos  int // byte offset in source
	// Line and Column are where the token starts, counting from one. They are
	// kept as the lexer moves rather than counted afterwards, because counting
	// afterwards is something every consumer would have to do, and would do
	// differently.
	//
	// Column counts runes, not bytes: a column that counted bytes would put
	// everything after a character with an accent in it in the wrong place, and
	// an editor that highlights the wrong character is worse than one that
	// highlights nothing.
	Line   int
	Column int
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
	// line and column are where the character in ch begins, counting from one.
	// They describe the character already read rather than the next one to be,
	// which is what makes a token's recorded position the position of its first
	// character and not the one after it.
	line   int
	column int
	// stringErr records why a string literal was rejected, so the caller can
	// report an invalid escape instead of a misleading unterminated string.
	stringErr string
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
		line:   1,
		// The column is zero until the first character has been read, because
		// readChar is what puts the first character at one. Starting at one
		// would make every character in a file one further along than it is.
		column: 0,
	}
	l.readChar()
	return l
}

// Next returns the next token from the input stream.
func (l *Lexer) Next() Token {
	l.skipWhitespace()
	// Where this token begins, taken after the whitespace and before anything
	// reads a character for it. Taken any later and every token would carry the
	// position of the one after it.
	startPos, startLine, startColumn := l.pos, l.line, l.column

	if l.ch == 0 {
		return Token{Kind: TokenEOF, Lex: "", Pos: startPos, Line: startLine, Column: startColumn}
	}

	// Identifiers / keywords
	if isLetter(l.ch) {
		ident := l.readIdent()
		return Token{Kind: lookupKeyword(ident), Lex: ident, Pos: startPos, Line: startLine, Column: startColumn}
	}

	// Numbers
	if isDigit(l.ch) {
		num := l.readNumber()
		return Token{Kind: TokenNumber, Lex: num, Pos: startPos, Line: startLine, Column: startColumn}
	}

	// Strings
	if l.ch == '"' {
		l.stringErr = ""
		str := l.readString()
		if str == "\x00unterminated" {
			if l.stringErr != "" {
				return Token{Kind: TokenIllegal, Lex: l.stringErr, Pos: startPos, Line: startLine, Column: startColumn}
			}
			return Token{Kind: TokenIllegal, Lex: "unterminated string", Pos: startPos, Line: startLine, Column: startColumn}
		}
		return Token{Kind: TokenString, Lex: str, Pos: startPos, Line: startLine, Column: startColumn}
	}

	// Symbols & operators
	switch l.ch {
	case '{':
		l.readChar()
		return Token{Kind: TokenLBrace, Lex: "{", Pos: startPos, Line: startLine, Column: startColumn}
	case '}':
		l.readChar()
		return Token{Kind: TokenRBrace, Lex: "}", Pos: startPos, Line: startLine, Column: startColumn}
	case '(':
		l.readChar()
		return Token{Kind: TokenLParen, Lex: "(", Pos: startPos, Line: startLine, Column: startColumn}
	case ')':
		l.readChar()
		return Token{Kind: TokenRParen, Lex: ")", Pos: startPos, Line: startLine, Column: startColumn}
	case '[':
		l.readChar()
		return Token{Kind: TokenLBracket, Lex: "[", Pos: startPos, Line: startLine, Column: startColumn}
	case ']':
		l.readChar()
		return Token{Kind: TokenRBracket, Lex: "]", Pos: startPos, Line: startLine, Column: startColumn}
	case ':':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: ":=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenColon, Lex: ":", Pos: startPos, Line: startLine, Column: startColumn}
	case ';':
		l.readChar()
		return Token{Kind: TokenSemicolon, Lex: ";", Pos: startPos, Line: startLine, Column: startColumn}
	case ',':
		l.readChar()
		return Token{Kind: TokenComma, Lex: ",", Pos: startPos, Line: startLine, Column: startColumn}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenRange, Lex: "..", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenDot, Lex: ".", Pos: startPos, Line: startLine, Column: startColumn}
	case '-':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenArrow, Lex: "->", Pos: startPos, Line: startLine, Column: startColumn}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "-=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenMinus, Lex: "-", Pos: startPos, Line: startLine, Column: startColumn}
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "+=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenPlus, Lex: "+", Pos: startPos, Line: startLine, Column: startColumn}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "*=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenStar, Lex: "*", Pos: startPos, Line: startLine, Column: startColumn}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAssign, Lex: "/=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenSlash, Lex: "/", Pos: startPos, Line: startLine, Column: startColumn}
	case '%':
		l.readChar()
		return Token{Kind: TokenPercent, Lex: "%", Pos: startPos, Line: startLine, Column: startColumn}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenNotEqual, Lex: "!=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenNot, Lex: "!", Pos: startPos, Line: startLine, Column: startColumn}
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenEqual, Lex: "==", Pos: startPos, Line: startLine, Column: startColumn}
		}
		// "=>" binds a route to its handler, as in
		// `route GET "/health" => handle_health`.
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenFatArrow, Lex: "=>", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenAssign, Lex: "=", Pos: startPos, Line: startLine, Column: startColumn}
	case '<':
		if l.peekChar() == '<' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenLess, Lex: "<<", Pos: startPos, Line: startLine, Column: startColumn}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenLessEqual, Lex: "<=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenLess, Lex: "<", Pos: startPos, Line: startLine, Column: startColumn}
	case '>':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenGreater, Lex: ">>", Pos: startPos, Line: startLine, Column: startColumn}
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenGreaterEqual, Lex: ">=", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenGreater, Lex: ">", Pos: startPos, Line: startLine, Column: startColumn}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenAnd, Lex: "&&", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenBitAnd, Lex: "&", Pos: startPos, Line: startLine, Column: startColumn}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenOr, Lex: "||", Pos: startPos, Line: startLine, Column: startColumn}
		}
		l.readChar()
		return Token{Kind: TokenBitOr, Lex: "|", Pos: startPos, Line: startLine, Column: startColumn}
	case '^':
		l.readChar()
		return Token{Kind: TokenCaret, Lex: "^", Pos: startPos, Line: startLine, Column: startColumn}
	case '~':
		l.readChar()
		return Token{Kind: TokenBitNot, Lex: "~", Pos: startPos, Line: startLine, Column: startColumn}
	}

	// Unknown character
	illegal := string(l.ch)
	l.readChar()
	return Token{Kind: TokenIllegal, Lex: illegal, Pos: startPos, Line: startLine, Column: startColumn}
}

/* -------------------------------------------------------------------------- */
/*  Internal Helpers                                                          */
/* -------------------------------------------------------------------------- */

func (l *Lexer) readChar() {
	if l.pos >= l.length {
		l.ch = 0
		l.width = 0
		l.column++
		return
	}
	r, width := utf8.DecodeRuneInString(l.src[l.pos:])
	if r == utf8.RuneError && width == 1 {
		l.ch = 0
		l.width = 0
		l.column++
		return
	}
	// A newline ends the line and starts a column; every other character is one
	// further along. The position recorded is that of the character being read,
	// so the first character of a file is at one, one.
	if r == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
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
		if l.ch == '/' && l.peekChar() == '*' {
			l.readChar()
			l.readChar()
			for l.ch != 0 && !(l.ch == '*' && l.peekChar() == '/') {
				l.readChar()
			}
			if l.ch != 0 {
				l.readChar()
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
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.src[start : l.pos-l.width]
}

// readString scans a double-quoted string and decodes escape sequences.
//
// Decoding here keeps a single semantic value: the typechecker re-encodes with
// strconv.Quote when lowering, which produces escapes valid in both C and Go,
// so neither backend needs its own escape handling.
func (l *Lexer) readString() string {
	// skip opening quote
	l.readChar()
	var b strings.Builder
	for {
		switch {
		case l.ch == 0:
			return "\x00unterminated"

		case l.ch == '"':
			l.readChar()
			return b.String()

		case l.ch == '\\':
			l.readChar()
			r, ok := l.readEscape()
			if !ok {
				if l.stringErr == "" {
					l.stringErr = "unterminated string"
				}
				return "\x00unterminated"
			}
			b.WriteRune(r)

		default:
			b.WriteRune(l.ch)
			l.readChar()
		}
	}
}

// readEscape decodes the escape sequence whose backslash has been consumed.
// The second return value is false when the sequence is invalid, in which case
// the lexer has been left in a state that makes readString report a bad token.
func (l *Lexer) readEscape() (rune, bool) {
	if l.ch == 0 {
		return 0, false
	}
	escape := l.ch
	l.readChar()

	switch escape {
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	case '0':
		return 0, true
	case '\\':
		return '\\', true
	case '"':
		return '"', true
	case '\'':
		return '\'', true
	case 'x', 'u', 'U':
		if escape == 'x' {
			// C-style byte escape: one or two hex digits.
			value, ok := l.readHexRange(1, 2)
			if !ok {
				l.stringErr = "invalid x escape in string literal"
				return 0, false
			}
			return rune(byte(value)), true
		}
		digits := 4
		if escape == 'U' {
			digits = 8
		}
		value, ok := l.readHex(digits)
		if !ok {
			l.stringErr = fmt.Sprintf("invalid %s escape in string literal", string(escape))
			return 0, false
		}
		if !utf8.ValidRune(rune(value)) {
			l.stringErr = fmt.Sprintf("\\%s escape is not a valid unicode code point", string(escape))
			return 0, false
		}
		return rune(value), true
	default:
		// An unknown escape is almost always a typo such as "\d". Reporting
		// it is safer than silently dropping the backslash.
		l.stringErr = fmt.Sprintf("unknown escape sequence \\%c in string literal", escape)
		return 0, false
	}
}

// readHex reads exactly count hexadecimal digits.
func (l *Lexer) readHex(count int) (int, bool) {
	return l.readHexRange(count, count)
}

// readHexRange reads between min and max hexadecimal digits, stopping as soon
// as a non-hex character is reached. It fails when fewer than min are present.
func (l *Lexer) readHexRange(min, max int) (int, bool) {
	value := 0
	seen := 0
	for seen < max {
		digit, ok := hexValue(l.ch)
		if !ok {
			break
		}
		value = value*16 + digit
		seen++
		l.readChar()
	}
	if seen < min {
		return 0, false
	}
	return value, true
}

func hexValue(ch rune) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10, true
	default:
		return 0, false
	}
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
	case "route":
		return TokenRoute
	default:
		return TokenIdent
	}
}
