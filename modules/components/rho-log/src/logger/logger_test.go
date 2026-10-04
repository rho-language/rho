//
// @file logger_test.go
// @brief Tests for the logger and its sinks.
//
// The behaviour a component depends on is that its name reaches every record
// without being repeated at each call site, and that a quiet record never
// costs a write.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package log

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rho/rho-fmt/src/logfmt"
)

func capture(level logfmt.Level) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return New("rho-test", Options{
		Level: level,
		Sinks: []Sink{&WriterSink{Writer: buf}},
	}), buf
}

// A component's name is stamped into every record, which is what lets a
// component depend on the logger without repeating itself.
func TestSubjectIsStamped(t *testing.T) {
	logger, out := capture(logfmt.LevelTrace)
	logger.Info("one")
	logger.Warn("two")
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if !strings.Contains(line, "[rho-test") {
			t.Errorf("record is missing the component subject: %q", line)
		}
	}
}

func TestLevelFiltersQuieterRecords(t *testing.T) {
	logger, out := capture(logfmt.LevelWarn)
	logger.Debug("dropped")
	logger.Info("dropped")
	logger.Warn("kept")
	logger.Error("kept")
	body := out.String()
	if strings.Contains(body, "dropped") {
		t.Errorf("records quieter than the level were written:\n%s", body)
	}
	if strings.Count(body, "kept") != 2 {
		t.Errorf("expected both records at or above the level:\n%s", body)
	}
}

func TestFieldsAreRendered(t *testing.T) {
	logger, out := capture(logfmt.LevelInfo)
	logger.Info("listening", "port", 2080, "secure", true)
	body := out.String()
	for _, want := range []string{"port=2080", "secure=true", "[info]"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in %q", want, body)
		}
	}
}

// A value that is not a key and a value must not shift the pairs.
func TestOddArgumentsDoNotShift(t *testing.T) {
	logger, out := capture(logfmt.LevelInfo)
	logger.Info("msg", "lonely")
	if strings.Contains(out.String(), "=lonely") {
		t.Errorf("a dangling key should not produce a field: %q", out)
	}
}

// WithSubject shares the sinks but stamps a different name, so a component can
// separate its own concerns.
func TestWithSubjectSharesSinks(t *testing.T) {
	logger, out := capture(logfmt.LevelInfo)
	other := logger.WithSubject("sub")
	other.Info("scoped")
	if !strings.Contains(out.String(), "[sub") {
		t.Errorf("expected the derived subject: %q", out)
	}
}

func TestDiscardIsSafe(t *testing.T) {
	logger := New("rho-test", Options{Sinks: []Sink{Discard{}}})
	logger.Info("thrown away") // must not panic
}

// A logger with no sink at all discards, so an unconfigured component still runs.
func TestNoSinkIsSafe(t *testing.T) {
	New("rho-test", Options{}).Info("nowhere")
}

/* -------------------------------------------------------------------------- */
/*  File sink                                                                 */
/* -------------------------------------------------------------------------- */

func TestFileSinkWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "registry.log")
	sink, err := NewFile(FileOptions{Path: path})
	if err != nil {
		t.Fatalf("NewFile failed: %v", err)
	}
	defer sink.Close()

	logger := New("registry", Options{Sinks: []Sink{sink}})
	logger.Info("stored", "version", "1.0.0")
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the log file is missing: %v", err)
	}
	if !strings.Contains(string(data), "version=1.0.0") {
		t.Errorf("unexpected contents: %q", data)
	}
}

// The file cycles once it passes the limit, and the old run is kept.
func TestFileSinkCycles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.log")
	sink, err := NewFile(FileOptions{Path: path, MaxBytes: 200, Keep: 2})
	if err != nil {
		t.Fatal(err)
	}
	logger := New("registry", Options{Sinks: []Sink{sink}})
	for i := 0; i < 40; i++ {
		logger.Info("a reasonably long line to push the file past its limit", "i", i)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"registry.log", "registry.log.1", "registry.log.2"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s is missing: %v", name, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
	// Retention is honoured: nothing past the kept count survives.
	if _, err := os.Stat(filepath.Join(dir, "registry.log.3")); err == nil {
		t.Error("a file beyond the retention count was kept")
	}
}

// A sink that fails must not take the component down with it.
func TestFailingSinkDoesNotPanic(t *testing.T) {
	logger := New("rho-test", Options{Sinks: []Sink{failingSink{}}})
	logger.Info("still fine")
}

type failingSink struct{}

func (failingSink) Write([]byte) error { return os.ErrPermission }
func (failingSink) Close() error       { return nil }
