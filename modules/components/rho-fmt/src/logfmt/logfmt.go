//
// @file logfmt.go
// @brief Shared formatting core for Rho log records.
//
// This is the piece a logger needs before it needs a logger: taking a record
// and turning it into bytes, identically wherever the record came from. An
// embedded target writing to a UART and a server writing to stdout agree on the
// output because they share this definition, not because they were kept in
// step by hand.
//
// The rendered shape is
//
//	[2026-09-27T05:44:31.412Z] [info] [registry] started port=8080 attempt=1
//
// that is, a bracketed stamp, a bracketed level, a bracketed subject, the
// message, and then the fields as key=value pairs. Every part is optional: a
// record with no subject omits the third bracket, and a record with no fields
// ends after the message. Nothing is quoted or escaped beyond what the field
// writer states, because a console that has to be read by eye should stay
// readable.
//
// The record deliberately holds an integer timestamp rather than a time type.
// Rho has no wall clock on an embedded target, so the value is whatever the
// clock provided, and the same formatter renders a host's Unix milliseconds and
// a board's tick count.
//
// A caller that cannot allocate formats into a buffer it owns, so the same code
// serves a logger with a fixed static buffer and one using the heap.
//
package logfmt

import (
	"sort"
	"strconv"
	"strings"
)

// Level is a record's severity.
type Level int

// Levels, in increasing severity. The zero value is Info, so a record that
// forgot to set one still reads sensibly.
const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = [...]string{"trace", "debug", "info", "warn", "error", "fatal"}

// String renders the level as it appears in a record.
func (l Level) String() string {
	if l < LevelTrace || int(l) >= len(levelNames) {
		return "info"
	}
	return levelNames[l]
}

// ParseLevel reads a level name, case insensitively. It reports false for an
// unknown name rather than silently picking a level, so a typo in a
// configuration file is visible.
func ParseLevel(name string) (Level, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for i, candidate := range levelNames {
		if candidate == want {
			return Level(i), true
		}
	}
	// A few spellings people reach for.
	switch want {
	case "warning":
		return LevelWarn, true
	case "err":
		return LevelError, true
	case "none", "off":
		return LevelFatal, true
	}
	return LevelInfo, false
}

// IsLevel reports whether a name is a level at all.
func IsLevel(name string) bool {
	_, ok := ParseLevel(name)
	return ok
}

// Field is one structured key and value attached to a record.
type Field struct {
	Key   string
	Value string
}

// Record is one log entry, independent of where it came from.
type Record struct {
	// Stamp is the time the record was made, as the clock reported it: Unix
	// milliseconds on a host, ticks on a board.
	Stamp int64
	// Level is the severity.
	Level Level
	// Subject names what the record is about, usually a module or subsystem.
	Subject string
	// Message is the human-readable part.
	Message string
	// Fields are the structured extras, rendered in key order so output is
	// stable and therefore testable.
	Fields []Field
	// StampText overrides Stamp when a source has a pre-rendered timestamp,
	// which is how a board with no wall clock can still print a readable one.
	StampText string
}

// Formatter turns a record into bytes.
//
// The interface exists so a deployment can choose its own output shape without
// the logger caring: a plain text console, JSON for a collector, or a compact
// line for a serial link where every byte costs.
type Formatter interface {
	// Format renders a record, allocating.
	Format(r Record) []byte
	// FormatInto renders a record into dst, returning the number of bytes
	// written. It must not grow dst, which is what lets a fixed-size embedded
	// buffer be used with no allocator at all.
	FormatInto(dst []byte, r Record) int
	// Name identifies the format, for diagnostics.
	Name() string
}

// Options tune the text format.
type Options struct {
	// StampLayout is a Go time layout applied to a Unix millisecond stamp.
	// Empty means RFC3339 with milliseconds, or the stamp as supplied when the
	// record carries StampText.
	StampLayout string
	// SubjectWidth pads the subject so records line up in a column. Zero, the
	// default, writes the subject as it is: padding a short name leaves a run of
	// spaces that reads as noise rather than as alignment.
	SubjectWidth int
	// ShowLevel includes the level bracket. Off gives a compact line for a
	// serial link.
	ShowLevel bool
	// Separator goes between the message and the fields.
	Separator string
}

// DefaultOptions is what a caller gets with no configuration.
func DefaultOptions() Options {
	return Options{
		StampLayout: DefaultStampLayout,
		ShowLevel:   true,
		Separator:   " ",
	}
}

// Text is the human-facing formatter: bracketed stamp, level and subject,
// then the message and its fields.
type Text struct {
	opts Options
}

// NewText returns a text formatter.
func NewText(opts Options) *Text {
	if opts.SubjectWidth < 0 {
		opts.SubjectWidth = 0
	}
	if opts.Separator == "" {
		opts.Separator = " "
	}
	return &Text{opts: opts}
}

func (t *Text) Name() string { return "text" }

// Format renders a record, growing until it fits. Unlike FormatInto it never
// truncates, because a caller here has an allocator and a short line would be a
// silent loss of fields.
func (t *Text) Format(r Record) []byte {
	return fit(r, t.FormatInto)
}

// FormatInto renders into dst without growing it. A record that does not fit is
// truncated, which is the right behaviour for a fixed buffer on a device with
// no way to grow it.
func (t *Text) FormatInto(dst []byte, r Record) int {
	w := &writer{buf: dst}
	w.stamp(r, t.opts.StampLayout)
	if t.opts.ShowLevel {
		w.bracket(r.Level.String())
	}
	if r.Subject != "" {
		if t.opts.SubjectWidth > 0 {
			w.bracket(pad(r.Subject, t.opts.SubjectWidth))
		} else {
			w.bracket(r.Subject)
		}
	}
	if r.Message != "" {
		w.str(r.Message)
		w.byte(' ')
	}
	w.fields(r.Sorted())
	return w.flush()
}

// fit renders into a buffer, retrying with a larger one until the formatter
// stops short of the end. It is what keeps Format from losing a field when the
// first guess at the size is too small.
func fit(r Record, formatInto func([]byte, Record) int) []byte {
	size := estimate(r)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, 0, size)
		n := formatInto(buf, r)
		if n < cap(buf) {
			return buf[:n]
		}
		size *= 2
	}
	buf := make([]byte, 0, size)
	n := formatInto(buf, r)
	return buf[:n]
}

// estimate is a starting size for Format, so a heap caller rarely regrows.
func estimate(r Record) int {
	size := 96 + len(r.Subject) + len(r.Message) + len(r.StampText)
	for _, f := range r.Fields {
		size += len(f.Key) + len(f.Value) + 4
	}
	return size
}

// Sorted returns the record's fields in key order, so two records with the same
// content always render the same bytes.
func (r Record) Sorted() []Field {
	if len(r.Fields) < 2 {
		return r.Fields
	}
	out := make([]Field, len(r.Fields))
	copy(out, r.Fields)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

/* -------------------------------------------------------------------------- */
/*  Writer                                                                    */
/* -------------------------------------------------------------------------- */

// writer appends into a fixed buffer, dropping anything that does not fit. It is
// the whole reason a caller without an allocator can still format.
type writer struct {
	buf []byte
}

// str appends a string, ignoring overflow.
func (w *writer) str(s string) {
	if len(w.buf)+len(s) > cap(w.buf) {
		return
	}
	w.buf = append(w.buf, s...)
}

// byte appends one byte, ignoring overflow.
func (w *writer) byte(c byte) {
	if len(w.buf)+1 > cap(w.buf) {
		return
	}
	w.buf = append(w.buf, c)
}

// bracket writes [value].
func (w *writer) bracket(value string) {
	w.byte('[')
	w.str(value)
	w.byte(']')
	w.byte(' ')
}

// word writes a bare value followed by a space.
func (w *writer) word(value string) {
	w.str(value)
	w.byte(' ')
}

// fields writes the structured pairs as key=value, quoting a value only when
// it needs it, so a plain console line stays readable.
func (w *writer) fields(fields []Field) {
	for _, f := range fields {
		w.space()
		w.str(f.Key)
		w.byte('=')
		w.str(QuoteValue(f.Value))
	}
}

// space writes a single separator, unless one was just written, so a message
// followed by fields does not gain a double space.
func (w *writer) space() {
	if n := len(w.buf); n > 0 && w.buf[n-1] != ' ' {
		w.byte(' ')
	}
}

// stamp writes the bracketed time.
func (w *writer) stamp(r Record, layout string) {
	switch {
	case r.StampText != "":
		w.bracket(r.StampText)
	case r.Stamp > 0:
		w.bracket(formatStamp(r.Stamp, layout))
	}
}

// flush returns the number of bytes written, which may be short of what was
// offered when the buffer filled.
func (w *writer) flush() int { return len(w.buf) }

// pad right-pads a value to a width, for columns that should line up.
func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

/* -------------------------------------------------------------------------- */
/*  Timestamps                                                                */
/* -------------------------------------------------------------------------- */

// DefaultStampLayout is the RFC3339 form with milliseconds.
const DefaultStampLayout = "2006-01-02T15:04:05.000Z07:00"

// formatStamp renders Unix milliseconds with a layout. It is deliberately
// small: a layout is only meaningful for a host, and a board that has a
// pre-rendered stamp supplies StampText instead.
func formatStamp(millis int64, layout string) string {
	if layout == "" {
		layout = DefaultStampLayout
	}
	return formatTime(millis, layout)
}

/* -------------------------------------------------------------------------- */
/*  Values                                                                    */
/* -------------------------------------------------------------------------- */

// QuoteValue makes a field value safe to read in a log line.
//
// A value that is plain stays plain, because quoting every field would make a
// console unreadable. A value with a space or a quote is wrapped in double
// quotes with the quotes and backslashes inside it escaped.
func QuoteValue(value string) string {
	if value == "" {
		return `""`
	}
	if !needsQuoting(value) {
		return value
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(value[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

func needsQuoting(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c <= ' ' || c == '"' || c == '\\' || c == 0x7f {
			return true
		}
	}
	return false
}

// FormatInt renders an integer, which several formatters need.
func FormatInt(value int64) string { return strconv.FormatInt(value, 10) }
