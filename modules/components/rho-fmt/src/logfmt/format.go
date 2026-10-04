//
// @file format.go
// @brief Time and JSON rendering for log records.
//
// The time handling is kept behind one small function so a target without a
// wall clock can swap it, and the JSON formatter is here because a structured
// log is most useful when something else can parse it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package logfmt

import (
	"strings"
	"time"
)

// formatTime renders Unix milliseconds with a Go time layout.
//
// The value is always rendered in UTC. A log line is read on a machine that may
// be in another timezone, and an offset-free stamp is the only one that sorts
// and compares correctly across a fleet.
func formatTime(millis int64, layout string) string {
	return time.UnixMilli(millis).UTC().Format(layout)
}

// Now returns a stamp for the current time, in the units Record expects.
//
// A logger that only has a monotonic counter supplies its own value; this is the
// host convenience.
func Now() int64 { return time.Now().UnixMilli() }

// NowText returns a pre-rendered stamp, which is what a source with no numeric
// clock of its own provides.
func NowText() string { return time.Now().UTC().Format(DefaultStampLayout) }

/* -------------------------------------------------------------------------- */
/*  JSON                                                                      */
/* -------------------------------------------------------------------------- */

// JSON renders a record as one JSON object per line, the shape a collector such
// as Loki or a log pipeline can parse.
//
// Fields become an object, so a query can address them by key rather than
// scraping text.
type JSON struct {
	// StampLayout is a Go time layout applied to a numeric stamp.
	StampLayout string
	// LevelKey, SubjectKey and MessageKey name the fixed fields, so a pipeline
	// can be pointed at whatever the source calls them.
	LevelKey     string
	SubjectKey   string
	MessageKey   string
	FieldsKey    string
	TimestampKey string
}

// DefaultJSON returns a JSON formatter with the conventional names.
func DefaultJSON() *JSON {
	return &JSON{
		StampLayout:  DefaultStampLayout,
		LevelKey:     "level",
		SubjectKey:   "subject",
		MessageKey:   "message",
		FieldsKey:    "fields",
		TimestampKey: "time",
	}
}

func (j *JSON) Name() string { return "json" }

func (j *JSON) Format(r Record) []byte {
	return fit(r, j.FormatInto)
}

// FormatInto renders into dst without growing it.
func (j *JSON) FormatInto(dst []byte, r Record) int {
	w := &writer{buf: dst}

	w.byte('{')
	first := true
	layout := j.StampLayout
	if layout == "" {
		layout = DefaultStampLayout
	}
	if r.Stamp > 0 {
		w.key(first, j.TimestampKey)
		w.str(QuoteString(formatTime(r.Stamp, layout)))
		first = false
	} else if r.StampText != "" {
		w.key(first, j.TimestampKey)
		w.str(QuoteString(r.StampText))
		first = false
	}
	w.key(first, j.LevelKey)
	w.str(QuoteString(r.Level.String()))
	first = false
	if r.Subject != "" {
		w.key(first, j.SubjectKey)
		w.str(QuoteString(r.Subject))
		first = false
	}
	if r.Message != "" {
		w.key(first, j.MessageKey)
		w.str(QuoteString(r.Message))
		first = false
	}
	if len(r.Fields) > 0 {
		w.key(false, j.FieldsKey)
		w.byte('{')
		for i, f := range r.Sorted() {
			if i > 0 {
				w.byte(',')
			}
			w.str(QuoteString(f.Key))
			w.byte(':')
			w.str(QuoteString(f.Value))
		}
		w.byte('}')
	}
	w.byte('}')

	return w.flush()
}

// key writes a JSON member name, including the separating comma when needed.
func (w *writer) key(first bool, name string) {
	if !first {
		w.byte(',')
	}
	w.str(jsonString(name))
	w.byte(':')
}

// QuoteString renders a Go string as a JSON string literal.
func QuoteString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xf])
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// jsonString is an alias that keeps the writer's call site readable.
func jsonString(value string) string { return QuoteString(value) }
