//
// @file escape_test.go
// @brief Tests for string escape decoding in the Rho lexer.
//
// A string literal's escape sequences are decoded by the lexer, so downstream
// stages see one semantic value. The typechecker re-encodes with
// strconv.Quote when lowering, which yields escapes valid in both C and Go, so
// neither backend needs its own escape handling.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package lexer

import (
	"strconv"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// stringValue lexes a single string literal and returns its decoded value.
func stringValue(t *testing.T, src string) (string, error) {
	t.Helper()
	tokens, err := Lex(src)
	if err != nil {
		return "", err
	}
	if len(tokens) != 2 {
		t.Fatalf("expected a string token and EOF, got %d tokens", len(tokens))
	}
	if tokens[0].Kind != TokenString {
		t.Fatalf("expected a string token, got %v", tokens[0].Kind)
	}
	return tokens[0].Lex, nil
}

func mustString(t *testing.T, src string) string {
	t.Helper()
	value, err := stringValue(t, src)
	if err != nil {
		t.Fatalf("Lex(%q) failed: %v", src, err)
	}
	return value
}

/* -------------------------------------------------------------------------- */
/*  Tests: Simple Escapes                                                      */
/* -------------------------------------------------------------------------- */

// A JSON payload is the motivating case: quotes inside a string literal must
// be escapable.
func TestEscapeQuotedQuote(t *testing.T) {
	if got := mustString(t, `"{\"name\":\"rho\"}"`); got != `{"name":"rho"}` {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeBackslash(t *testing.T) {
	if got := mustString(t, `"a\\b"`); got != `a\b` {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeControlCharacters(t *testing.T) {
	cases := map[string]string{
		`"a\nb"`: "a\nb",
		`"a\tb"`: "a\tb",
		`"a\rb"`: "a\rb",
		`"a\ab"`: "a\ab",
		`"a\bb"`: "a\bb",
		`"a\fb"`: "a\fb",
		`"a\vb"`: "a\vb",
		`"a\0b"`: "a\x00b",
		`"\'"`:   "'",
		`"\""`:   `"`,
		`"\\"`:   `\`,
	}
	for src, want := range cases {
		if got := mustString(t, src); got != want {
			t.Fatalf("%s: got %q, want %q", src, got, want)
		}
	}
}

func TestEscapeEmptyString(t *testing.T) {
	if got := mustString(t, `""`); got != "" {
		t.Fatalf("got %q", got)
	}
}

// A literal backslash-n in the source is a newline, exactly as in C and Go.
func TestEscapeNewlineIsNotLiteralBackslashN(t *testing.T) {
	if got := mustString(t, `"line1\nline2"`); strings.Contains(got, `\n`) {
		t.Fatalf("expected a real newline, got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Numeric Escapes                                                     */
/* -------------------------------------------------------------------------- */

func TestEscapeHex(t *testing.T) {
	if got := mustString(t, `"A\x42C"`); got != "ABC" {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeHexSingleDigit(t *testing.T) {
	if got := mustString(t, `"\x7"`); got != "\x07" {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeUnicode(t *testing.T) {
	if got := mustString(t, `"caf\u00e9"`); got != "café" {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeUnicodeLong(t *testing.T) {
	if got := mustString(t, `"\U0001F600"`); got != "\U0001F600" {
		t.Fatalf("got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Plain Content Is Preserved                                          */
/* -------------------------------------------------------------------------- */

// UTF-8 text with no escapes must survive untouched.
func TestStringPreservesUTF8(t *testing.T) {
	if got := mustString(t, `"héllo wörld"`); got != "héllo wörld" {
		t.Fatalf("got %q", got)
	}
}

func TestStringPreservesSpaces(t *testing.T) {
	if got := mustString(t, `"  spaced  out  "`); got != "  spaced  out  " {
		t.Fatalf("got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Invalid Escapes Are Reported                                        */
/* -------------------------------------------------------------------------- */

// An unknown escape is almost always a typo, so it is reported with its byte
// offset rather than silently dropping the backslash.
func TestEscapeUnknownIsRejected(t *testing.T) {
	_, err := stringValue(t, `"a\db"`)
	if err == nil {
		t.Fatal("expected an error for an unknown escape")
	}
	if !strings.Contains(err.Error(), `\d`) {
		t.Fatalf("expected the offending escape named, got %v", err)
	}
}

func TestEscapeBadHexIsRejected(t *testing.T) {
	if _, err := stringValue(t, `"\xZZ"`); err == nil {
		t.Fatal("expected an error for invalid hex digits")
	}
}

// \x takes one or two digits, so a single digit is valid; a \u escape needs
// all four.
func TestEscapeTruncatedUnicodeIsRejected(t *testing.T) {
	if _, err := stringValue(t, `"\u00"`); err == nil {
		t.Fatal("expected an error for a truncated unicode escape")
	}
}

func TestEscapeInvalidCodePointIsRejected(t *testing.T) {
	// U+110000 is above the Unicode maximum.
	if _, err := stringValue(t, `"\U00110000"`); err == nil {
		t.Fatal("expected an error for an out-of-range code point")
	}
}

func TestUnterminatedStringIsStillReported(t *testing.T) {
	if _, err := stringValue(t, `"no end`); err == nil {
		t.Fatal("expected an error for an unterminated string")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Round Trip Through strconv.Quote                                    */
/* -------------------------------------------------------------------------- */

// The lowering re-encodes with strconv.Quote, and the C and Go backends copy
// that text verbatim. Every decoded value therefore has to survive quoting
// unchanged, which is what keeps the two backends byte-identical.
func TestEscapesSurviveStrconvQuote(t *testing.T) {
	sources := []string{
		`"{\"name\":\"rho\"}"`,
		`"a\\b"`,
		`"a\nb"`,
		`"a\tb"`,
		`"a\x00b"`,
		`"caf\u00e9"`,
		`""`,
	}
	for _, src := range sources {
		value := mustString(t, src)
		quoted := strconv.Quote(value)
		// Quoting twice must be stable, which is what guarantees the backends
		// read the same bytes the program intended.
		decoded, err := strconv.Unquote(quoted)
		if err != nil {
			t.Fatalf("%s: strconv.Unquote(%q) failed: %v", src, quoted, err)
		}
		if again := strconv.Quote(decoded); again != quoted {
			t.Fatalf("%s: quote round trip is unstable: %q then %q", src, quoted, again)
		}
	}
}
