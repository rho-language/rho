//
// @file logger.go
// @brief The Rho logging component.
//
// A logger is a subject, a level and a sink. The subject is stamped into every
// record, so a component that depends on this one gets its own name in the log
// without repeating it at each call site.
//
//	log := rhoLog.New("rho-lsp")
//	log.Info("listening", "port", 2080)
//
// Sinks decide where records go, and adding one is adding a type rather than
// changing the logger: stdout for a developer, a rotating file for a device
// that has to survive a reboot, and later a plugin that forwards to a collector.
//
// A Logger is safe to share between goroutines, because a component's log is
// normally package level and the language server handles requests on many at
// once.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package log

import (
	"os"
	"sync"

	"github.com/rho/rho-fmt/src/logfmt"
)

// Sink is where a formatted record goes.
//
// A sink receives the already-rendered line, so it never has to know the
// format. That is what lets a file, a socket and a collector be swapped without
// touching the logger.
type Sink interface {
	// Write takes one rendered record, newline included.
	Write(line []byte) error
	// Close releases whatever the sink holds.
	Close() error
}

// Logger writes records for one subject.
type Logger struct {
	subject string
	minimum logfmt.Level
	format  logfmt.Formatter
	mu      sync.Mutex
	sinks   []Sink
}

// Options configure a logger.
type Options struct {
	// Level is the lowest severity that is written. Anything quieter is
	// dropped before a sink ever sees it.
	Level logfmt.Level
	// Formatter decides the rendered shape. Text by default.
	Formatter logfmt.Formatter
	// Sinks receive the records. A logger with no sink discards them, which
	// keeps a component that is not configured from failing at run time.
	Sinks []Sink
}

// New returns a logger that stamps every record with a subject.
//
// The subject is normally the component's own name, so a log line says where
// it came from without the call site having to say so.
func New(subject string, options Options) *Logger {
	format := options.Formatter
	if format == nil {
		format = logfmt.NewText(logfmt.DefaultOptions())
	}
	logger := &Logger{
		subject: subject,
		minimum: options.Level,
		format:  format,
	}
	logger.sinks = append(logger.sinks, options.Sinks...)
	return logger
}

// Default returns a logger writing to standard error.
//
// Standard error, not standard output, and the reason is that standard output
// belongs to whatever the command was asked for. `rho doctor --json` puts a report
// there, a debug adapter puts a protocol there, `rho doc` puts a document there,
// and a line of diagnostics in front of any of them makes it unparseable — for a
// program that reads it, and for a person who pipes it. A developer still sees every
// line, because a terminal shows both.
func Default(subject string) *Logger {
	return New(subject, Options{
		Level: logfmt.LevelInfo,
		Sinks: []Sink{&WriterSink{Writer: os.Stderr}},
	})
}

// Subject is the component name written into every record.
func (l *Logger) Subject() string { return l.subject }

// SetLevel changes the lowest severity that is written.
func (l *Logger) SetLevel(level logfmt.Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minimum = level
}

// Level reports the lowest severity that is written.
func (l *Logger) Level() logfmt.Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.minimum
}

// AddSink attaches another destination, so a component can log to the console
// and a file at once.
func (l *Logger) AddSink(sink Sink) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sinks = append(l.sinks, sink)
}

// WithSubject returns a logger that stamps a different subject but shares this
// one's sinks and level. A component can use it to separate its own concerns
// without constructing a second sink chain.
func (l *Logger) WithSubject(subject string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{subject: subject, minimum: l.minimum, format: l.format, sinks: l.sinks}
}

// log renders and writes one record, unless it is quieter than the level.
func (l *Logger) log(level logfmt.Level, message string, fields []logfmt.Field) {
	l.mu.Lock()
	minimum := l.minimum
	sinks := append([]Sink{}, l.sinks...)
	format := l.format
	l.mu.Unlock()

	if level < minimum {
		return
	}

	record := logfmt.Record{
		Stamp:   logfmt.Now(),
		Level:   level,
		Subject: l.subject,
		Message: message,
		Fields:  fields,
	}
	line := append(format.Format(record), '\n')

	for _, sink := range sinks {
		// A failing sink must not take the component down with it; a logger
		// that is misconfigured should not be a reason to lose the program.
		_ = sink.Write(line)
	}
}

// Trace, Debug, Info, Warn, Error and Fatal write at their level. The key and
// value pairs are collected in order and sorted when rendered.
func (l *Logger) Trace(message string, kv ...any) { l.log(logfmt.LevelTrace, message, pairs(kv)) }
func (l *Logger) Debug(message string, kv ...any) { l.log(logfmt.LevelDebug, message, pairs(kv)) }
func (l *Logger) Info(message string, kv ...any)  { l.log(logfmt.LevelInfo, message, pairs(kv)) }
func (l *Logger) Warn(message string, kv ...any)  { l.log(logfmt.LevelWarn, message, pairs(kv)) }
func (l *Logger) Error(message string, kv ...any) { l.log(logfmt.LevelError, message, pairs(kv)) }

// Close releases every sink, which a component should do as it shuts down so
// a buffered file is flushed.
func (l *Logger) Close() error {
	l.mu.Lock()
	sinks := l.sinks
	l.mu.Unlock()
	for _, sink := range sinks {
		_ = sink.Close()
	}
	return nil
}

// pairs turns alternating keys and values into fields. A value that cannot be
// rendered becomes a placeholder rather than being dropped, because a missing
// field is harder to read than an odd one.
func pairs(kv []any) []logfmt.Field {
	fields := make([]logfmt.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			key = "?"
		}
		fields = append(fields, logfmt.Field{Key: key, Value: render(kv[i+1])})
	}
	return fields
}
