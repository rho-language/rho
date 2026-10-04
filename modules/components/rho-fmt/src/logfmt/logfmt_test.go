//
// @file logfmt_test.go
// @brief Tests for the shared log formatting core.
//
// The output is a contract: a console is read by eye and a pipeline is parsed by
// machine, so both the shape and the bytes are pinned here. A logger built on
// this has to produce something a person and something else can both use.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package logfmt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sample() Record {
	return Record{
		Stamp:   1792000000000,
		Level:   LevelInfo,
		Subject: "registry",
		Message: "started",
		Fields:  []Field{{Key: "port", Value: "8080"}, {Key: "attempt", Value: "1"}},
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Shape                                                               */
/* -------------------------------------------------------------------------- */

// A record renders as bracketed stamp, level and subject, then the message and
// its fields.
func TestTextShape(t *testing.T) {
	// The expected stamp is derived rather than written out, so the test
	// states the rule instead of freezing a value that drifts.
	want := "[" + time.UnixMilli(sample().Stamp).UTC().Format(DefaultStampLayout) + "]"
	got := string(NewText(DefaultOptions()).Format(sample()))
	if !strings.HasPrefix(got, want) {
		t.Fatalf("\n got %q\nwant prefix %q", got, want)
	}
	// A stamp carries no timezone offset, so lines from a fleet are comparable.
	if strings.Contains(want, "+") {
		t.Errorf("the stamp should be UTC without an offset, got %q", want)
	}
	// The default writes the subject unpadded: a short name followed by a run
	// of spaces reads as noise, not as alignment.
	if rest, expected := strings.TrimPrefix(got, want),
		" [info] [registry] started attempt=1 port=8080"; rest != expected {
		t.Fatalf("unexpected remainder\n got %q\nwant %q", rest, expected)
	}
}

// The field order is fixed, so two runs render identically.
func TestFieldsAreSorted(t *testing.T) {
	r := sample()
	r.Fields = []Field{{Key: "zebra", Value: "1"}, {Key: "alpha", Value: "2"}}
	got := string(NewText(DefaultOptions()).Format(r))
	if !strings.Contains(got, "alpha=2 zebra=1") {
		t.Fatalf("expected sorted fields, got %q", got)
	}
	if strings.Index(got, "alpha") > strings.Index(got, "zebra") {
		t.Fatalf("fields are out of order: %q", got)
	}
}

// A subject can be padded into a column when that is what is wanted, so a
// table of records lines up.
func TestSubjectCanBePadded(t *testing.T) {
	opts := DefaultOptions()
	opts.SubjectWidth = 12
	got := string(NewText(opts).Format(sample()))
	if want := "[" + pad("registry", 12) + "]"; !strings.Contains(got, want) {
		t.Fatalf("expected %q in %q", want, got)
	}
}

// A record with no subject omits that bracket rather than leaving it empty.
func TestMissingSubjectIsOmitted(t *testing.T) {
	r := sample()
	r.Subject = ""
	got := string(NewText(DefaultOptions()).Format(r))
	if strings.Contains(got, "[]") {
		t.Fatalf("an absent subject should not leave empty brackets: %q", got)
	}
}

func TestMissingFieldsEndAfterTheMessage(t *testing.T) {
	r := sample()
	r.Fields = nil
	got := strings.TrimSpace(string(NewText(DefaultOptions()).Format(r)))
	if !strings.HasSuffix(got, "started") {
		t.Fatalf("expected the line to end at the message, got %q", got)
	}
}

func TestLevelCanBeHidden(t *testing.T) {
	opts := DefaultOptions()
	opts.ShowLevel = false
	got := string(NewText(opts).Format(sample()))
	if strings.Contains(got, "[info]") {
		t.Fatalf("the level should be absent when hidden, got %q", got)
	}
}

// A source that has its own readable stamp supplies it, which is how a board
// with no wall clock still prints something a person can read.
func TestPreRenderedStamp(t *testing.T) {
	r := sample()
	r.Stamp = 0
	r.StampText = "TICK-000042"
	got := string(NewText(DefaultOptions()).Format(r))
	if !strings.HasPrefix(got, "[TICK-000042]") {
		t.Fatalf("expected the supplied stamp, got %q", got)
	}
}

// A value with a space is quoted so the field boundary stays readable.
func TestValuesAreQuotedWhenNeeded(t *testing.T) {
	r := sample()
	r.Fields = []Field{{Key: "msg", Value: "two words"}}
	got := string(NewText(DefaultOptions()).Format(r))
	if !strings.Contains(got, `msg="two words"`) {
		t.Fatalf("expected a quoted value, got %q", got)
	}
}

func TestQuoteValue(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		"":           `""`,
		"two words":  `"two words"`,
		`say "hi"`:   `"say \"hi\""`,
		`back\slash`: `"back\\slash"`,
		"new\nline":  `"new\nline"`,
		"tab\there":  `"tab\there"`,
	}
	for in, want := range cases {
		if got := QuoteValue(in); got != want {
			t.Errorf("QuoteValue(%q) = %q, want %q", in, got, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Bounded Output                                                      */
/* -------------------------------------------------------------------------- */

// The point of FormatInto is that a caller with no allocator can still format,
// so a buffer that is too small must be filled exactly and never grown.
func TestFormatIntoFillsWithoutGrowing(t *testing.T) {
	dst := make([]byte, 0, 32)
	n := NewText(DefaultOptions()).FormatInto(dst, sample())
	if n > 32 {
		t.Fatalf("wrote %d bytes into a 32 byte buffer", n)
	}
	if n == 0 {
		t.Fatal("expected something to be written")
	}
	// Whatever fit is still valid output.
	if !strings.HasPrefix(string(dst[:n]), "[") {
		t.Fatalf("expected the record to start at the bracket, got %q", dst[:n])
	}
}

// A roomy buffer is filled completely, so the bounded path and the allocating
// path agree.
func TestFormatIntoAgreesWithFormat(t *testing.T) {
	roomy := make([]byte, 0, 4096)
	n := NewText(DefaultOptions()).FormatInto(roomy, sample())
	if string(roomy[:n]) != string(NewText(DefaultOptions()).Format(sample())) {
		t.Fatal("the bounded and allocating paths disagree")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Levels                                                              */
/* -------------------------------------------------------------------------- */

func TestLevelNames(t *testing.T) {
	cases := map[Level]string{
		LevelTrace: "trace", LevelDebug: "debug", LevelInfo: "info",
		LevelWarn: "warn", LevelError: "error", LevelFatal: "fatal",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("level %d: got %q, want %q", int(level), got, want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"info": LevelInfo, "INFO": LevelInfo, " warning ": LevelWarn,
		"error": LevelError, "err": LevelError,
	}
	for in, want := range cases {
		got, ok := ParseLevel(in)
		if !ok {
			t.Errorf("ParseLevel(%q) should succeed", in)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
	if _, ok := ParseLevel("shouty"); ok {
		t.Error("an unknown level should be reported rather than guessed")
	}
}

func TestIsLevel(t *testing.T) {
	if !IsLevel("warn") {
		t.Error("warn should be a level")
	}
	if IsLevel("nope") {
		t.Error("nope should not be a level")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: JSON                                                               */
/* -------------------------------------------------------------------------- */

// JSON output has to survive a real parser, since that is the point of it.
func TestJSONIsParseable(t *testing.T) {
	var out map[string]any
	raw := DefaultJSON().Format(sample())
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the output is not valid JSON: %v\n%s", err, raw)
	}
	if out["subject"] != "registry" {
		t.Errorf("subject: got %v", out["subject"])
	}
	if out["level"] != "info" {
		t.Errorf("level: got %v", out["level"])
	}
	if out["message"] != "started" {
		t.Errorf("message: got %v", out["message"])
	}
	fields, ok := out["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields should be an object, got %T", out["fields"])
	}
	if fields["port"] != "8080" {
		t.Errorf("fields.port: got %v", fields["port"])
	}
}

func TestJSONEscapesControlCharacters(t *testing.T) {
	r := sample()
	r.Message = "line\nbreak\there"
	raw := DefaultJSON().Format(r)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("a control character broke the JSON: %v\n%s", err, raw)
	}
	if out["message"] != "line\nbreak\there" {
		t.Fatalf("message did not survive: %q", out["message"])
	}
}

func TestQuoteString(t *testing.T) {
	cases := map[string]string{
		"plain":  `"plain"`,
		"a\"b":   `"a\"b"`,
		"a\\b":   `"a\\b"`,
		"a\nb":   `"a\nb"`,
		"bell\a": `"bell\u0007"`,
	}
	for in, want := range cases {
		if got := QuoteString(in); got != want {
			t.Errorf("QuoteString(%q) = %q, want %q", in, got, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Formatters share the record                                         */
/* -------------------------------------------------------------------------- */

// Every formatter has to satisfy the interface, which is what lets a logger
// choose one without knowing the details.
func TestFormattersImplementTheInterface(t *testing.T) {
	formatters := []Formatter{NewText(DefaultOptions()), DefaultJSON()}
	for _, f := range formatters {
		if f.Name() == "" {
			t.Error("a formatter should name itself")
		}
		if len(f.Format(sample())) == 0 {
			t.Errorf("%s produced nothing", f.Name())
		}
	}
}
