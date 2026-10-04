//
// @file sink.go
// @brief Sinks: where a log record goes.
//
// A sink takes a rendered line, so the shape of a record is decided in one
// place and every destination agrees. Adding a plugin later means adding a Sink
// rather than changing the logger.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// render turns a value into its log form. Numbers and booleans read naturally;
// anything else is shown with its type, so a caller passing the wrong thing
// sees it in the log rather than discovering it later.
func render(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case time.Duration:
		return v.String()
	case error:
		return v.Error()
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%v", v)
	}
}

/* -------------------------------------------------------------------------- */
/*  Writer Sink                                                               */
/* -------------------------------------------------------------------------- */

// WriterSink sends records to any writer, which covers standard output, a
// serial console and a test buffer.
//
// It is used through a pointer, because a sink that serialises its writes holds
// a lock and a lock copied with the value it belongs to is a lock that
// protects nothing: each call would take its own copy of it, and two goroutines
// writing at once would both find it free.
type WriterSink struct {
	Writer io.Writer
	// Locked serialises writes when several goroutines share the sink.
	Locked bool

	mu sync.Mutex
}

func (s *WriterSink) Write(line []byte) error {
	if s.Locked {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	_, err := s.Writer.Write(line)
	return err
}

func (s *WriterSink) Close() error { return nil }

// Discard is a sink that throws records away. A component that is not
// configured uses it rather than failing at run time.
type Discard struct{}

func (Discard) Write([]byte) error { return nil }
func (Discard) Close() error       { return nil }

/* -------------------------------------------------------------------------- */
/*  File Sink                                                                 */
/* -------------------------------------------------------------------------- */

// FileOptions configure a rotating log file.
type FileOptions struct {
	// Path is the file to write.
	Path string
	// MaxBytes is the size at which the file is cycled. Zero disables
	// rotation, which suits a console.
	MaxBytes int64
	// Keep is how many previous files to retain, named path.1, path.2 and so
	// on. Zero keeps one.
	Keep int
}

// FileSink appends records to a file, cycling it when it grows past a limit.
//
// Cycling rather than deleting is deliberate: a device that has just rebooted
// has usually not written anything yet, and the previous run is the interesting
// one. The files are renamed rather than copied, so writing stays cheap and
// there is never a half-written file left behind.
type FileSink struct {
	options FileOptions

	mu   sync.Mutex
	file *os.File
	size int64
}

// NewFile returns a sink writing to a file, creating the folder if needed.
func NewFile(options FileOptions) (*FileSink, error) {
	if options.Path == "" {
		return nil, fmt.Errorf("log: a file sink needs a path")
	}
	if options.Keep <= 0 {
		options.Keep = 1
	}
	if dir := filepath.Dir(options.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	sink := &FileSink{options: options}
	if err := sink.open(); err != nil {
		return nil, err
	}
	return sink, nil
}

// open attaches to the current file and records how much is already in it, so
// the size limit counts the whole file rather than only this run's writes.
func (s *FileSink) open() error {
	file, err := os.OpenFile(s.options.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.file = file
	if info, err := file.Stat(); err == nil {
		s.size = info.Size()
	}
	return nil
}

func (s *FileSink) Write(line []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		if err := s.open(); err != nil {
			return err
		}
	}
	if s.options.MaxBytes > 0 && s.size+int64(len(line)) > s.options.MaxBytes {
		if err := s.cycle(); err != nil {
			return err
		}
	}
	n, err := s.file.Write(line)
	s.size += int64(n)
	return err
}

// cycle renames the current file out of the way and starts a fresh one, dropping
// the oldest beyond the retention count.
func (s *FileSink) cycle() error {
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			return err
		}
		s.file = nil
	}

	// Drop the archive past the retention count, then shift the rest along.
	// Renaming upward from Keep-1 keeps exactly Keep archives: with Keep of 2
	// the result is .1 and .2, and nothing beyond.
	_ = os.Remove(s.archivePath(s.options.Keep))
	for i := s.options.Keep - 1; i >= 1; i-- {
		if _, err := os.Stat(s.archivePath(i)); err != nil {
			continue
		}
		if err := os.Rename(s.archivePath(i), s.archivePath(i+1)); err != nil {
			return err
		}
	}
	if err := os.Rename(s.options.Path, s.archivePath(1)); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.size = 0
	return s.open()
}

func (s *FileSink) archivePath(index int) string {
	return s.options.Path + "." + strconv.Itoa(index)
}

func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
