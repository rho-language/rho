//
// @file format.go
// @brief How target output is presented.
//
// A firmware's output is not written for a terminal, so the two things this adds
// are the two things a person watching a board needs and the firmware cannot know
// it should print: when a line arrived, and how bad it was.
//
// Both are read from the text rather than asked of the target. The target's log
// format belongs to the target — asking it to emit timestamps in this component's
// shape would mean every firmware had to be built to suit a tool — and a level is
// the one word in a line that says how bad it was, which every log format already
// has and none of them spells the same way.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level is how bad a line sounds.
//
// Four, not seven. A serial console is 115200 baud and a trace port is worse, so
// anything a firmware prints is already expensive to produce; a level scheme with
// seven values spends the budget on distinctions nobody reads off a board. These
// are the four that change what a person does next.
type Level int

// The levels, in the order they are printed.
const (
	// LevelTrace is ordinary running output.
	LevelTrace Level = iota
	// LevelDebug is something a firmware says while working out what it is doing.
	LevelDebug
	// LevelInfo is something that happened.
	LevelInfo
	// LevelWarn is something that happened and will matter.
	LevelWarn
	// LevelError is something that went wrong.
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "TRACE"
	}
}

// ANSI colours, as the eight the terminal has had since forever.
const (
	ansiReset  = "\x1b[0m"
	ansiGrey   = "\x1b[90m"
	ansiBlue   = "\x1b[34m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

// Color is the colour a level is written in.
func (l Level) Color() string {
	switch l {
	case LevelDebug:
		return ansiGrey
	case LevelInfo:
		return ansiBlue
	case LevelWarn:
		return ansiYellow
	case LevelError:
		return ansiRed
	default:
		return ansiGreen
	}
}

// levelWords are the words a firmware writes to say how bad a line is.
//
// Uppercase, whole words. That is the convention every firmware logger follows —
// `ERROR`, `WARN`, `INFO`, `DEBUG` — and insisting on it is what keeps prose out of
// the result: a line reading "the error rate was measured" is a sentence about a
// measurement, and a case-insensitive scan calls it an error report, which colours
// a healthy run as a broken one.
//
// The other two spellings are matched on their punctuation, which is what marks a
// word as a tag rather than as English: a tag in brackets anywhere in the line, and
// a tag with a colon at the start of it. `warning: clock drifted` is a level and
// "the error rate was measured" is a sentence, and the difference is the colon and
// the position.
var levelWords = []struct {
	pattern *regexp.Regexp
	level   Level
}{
	{regexp.MustCompile(`\b(ERROR|ERR|FATAL|FAILED|FAILURE)\b`), LevelError},
	{regexp.MustCompile(`\b(WARN|WARNING)\b`), LevelWarn},
	{regexp.MustCompile(`\b(INFO|NOTICE)\b`), LevelInfo},
	{regexp.MustCompile(`\b(DEBUG|DBG|TRACE|VERBOSE)\b`), LevelDebug},
	// A tag in brackets, in any case: `[error]`, `[WARN]`, `[warn]`. The brackets
	// are what say it is a tag and not a word in a sentence.
	{regexp.MustCompile(`(?i)^\s*(error|err|fatal|fail(?:ed|ure)?)\s*:`), LevelError},
	{regexp.MustCompile(`(?i)^\s*(warn|warning)\s*:`), LevelWarn},
	{regexp.MustCompile(`(?i)^\s*(info|notice)\s*:`), LevelInfo},
	{regexp.MustCompile(`(?i)^\s*(debug|dbg|trace|verbose)\s*:`), LevelDebug},
	// The bracketed forms, which are a tag wherever in the line they appear.
	{regexp.MustCompile(`(?i)\[(error|err|fatal|fail(?:ed|ure)?)\]`), LevelError},
	{regexp.MustCompile(`(?i)\[(warn|warning)\]`), LevelWarn},
	{regexp.MustCompile(`(?i)\[(info|notice)\]`), LevelInfo},
	{regexp.MustCompile(`(?i)\[(debug|dbg|trace|verbose)\]`), LevelDebug},
}

// LevelOf is how bad a line sounds, read from its text.
//
// A line that says nothing in particular is ordinary running output, which is the
// right default: a firmware that prints values with no level at all is printing
// values, and calling that a warning would be a monitor inventing a problem.
func LevelOf(line string) Level {
	for _, word := range levelWords {
		if word.pattern.MatchString(line) {
			return word.level
		}
	}
	return LevelTrace
}

// Formatter turns a line of target output into what is written to the terminal.
type Formatter struct {
	options Formatting
	// now is the clock, replaceable so a timestamp is testable.
	now func() time.Time
	// out is where the formatted lines go.
	out io.Writer
	// log is the copy written to a file, when there is one. Held as the file
	// rather than as an io.Writer because closing it is part of ending a run.
	log *os.File
	// count is called for every line written, when a run is counting them.
	count *Counter
}

// NewFormatter is a formatter writing to a terminal and, optionally, to a file.
func NewFormatter(out io.Writer, options Formatting) *Formatter {
	return &Formatter{options: options, now: time.Now, out: out}
}

// Count has the formatter record every line it writes, so a run can report what
// it read rather than only that it ended.
func (f *Formatter) Count(counter *Counter) { f.count = counter }

// SetClock replaces the clock, which is what makes a timestamp assertable.
func (f *Formatter) SetClock(now func() time.Time) { f.now = now }

// AttachLog writes a copy of everything to a file as well.
//
// Beside the terminal rather than instead of it: a log file is for after the
// moment has passed, and a person watching a board needs the terminal now. The
// file is opened when the run starts and appended to, because a run that truncated
// it would destroy the output of the run before it — which is the run somebody
// wanted to compare against.
func (f *Formatter) AttachLog(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if directory := filepath.Dir(path); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", directory, err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open the log file: %w", err)
	}
	f.log = file
	return nil
}

// Close releases the log file.
func (f *Formatter) Close() error {
	if f.log == nil {
		return nil
	}
	err := f.log.Close()
	f.log = nil
	return err
}

// Line is one line of target output, as it was written.
type Line struct {
	// Text is the line, without its newline.
	Text string
	// Level is how bad it sounds.
	Level Level
	// At is when it arrived.
	At time.Time
	// Partial is whether this was a fragment rather than a finished line — a
	// firmware that prints a heartbeat without a newline produces a stream of
	// them, and printing each fragment as a line would double the output.
	Partial bool
}

// Format is one line as it is written to the terminal.
func (f *Formatter) Format(line Line) string {
	out := &strings.Builder{}
	if f.options.Timestamps {
		stamp := line.At.Format("15:04:05.000")
		if f.options.Color {
			fmt.Fprintf(out, "%s%s%s ", ansiGrey, stamp, ansiReset)
		} else {
			fmt.Fprintf(out, "%s ", stamp)
		}
	}
	body := line.Text
	if f.options.ColorLevels && f.options.Color && line.Level > LevelTrace {
		fmt.Fprintf(out, "%s%s%s", line.Level.Color(), body, ansiReset)
	} else {
		out.WriteString(body)
	}
	out.WriteString("\n")
	return out.String()
}

// Write is one line, to the terminal and to the log.
//
// The log file gets the plain line with its timestamp and nothing else: a file
// full of escape sequences is a file `grep` cannot read, and the terminal is the
// only place colour belongs.
func (f *Formatter) Write(line Line) error {
	if f.count != nil {
		f.count.Count()
	}
	if f.log != nil {
		entry := line.Text
		if f.options.Timestamps {
			entry = fmt.Sprintf("%s %s", line.At.Format("15:04:05.000"), entry)
		}
		if _, err := fmt.Fprintln(f.log, entry); err != nil {
			return err
		}
	}
	_, err := io.WriteString(f.out, f.Format(line))
	return err
}

// Tail reads a stream and formats every line that arrives.
//
// A line-oriented reader, because a console is line-oriented: a firmware that
// prints without a newline would otherwise produce nothing at all, since nothing
// is ever a complete line. So a newline ends a line, and the wait between lines
// does not — whatever has arrived when the stream ends is printed as a line of its
// own, which is what a heartbeat actually is.
//
// The reading happens in its own goroutine and this loop chooses between what it
// found and being told to stop. That is not tidiness: a read on a serial port
// blocks in the kernel and closing the device from another goroutine does not
// reliably wake it, so a monitor that waited for its own read to return would
// ignore `--duration` and Ctrl-C on exactly the backend people use it for. The
// goroutine left blocked is abandoned when the process exits, which costs nothing
// and is the only way to stop a blocked read portably.
func (f *Formatter) Tail(stream Stream, done <-chan struct{}) error {
	lines := make(chan Line, 64)
	partial := &partialLine{}
	go readLines(stream, f.now, partial, lines)

	for {
		select {
		case <-done:
			// Whatever the target had said and not finished is output, not
			// nothing: a firmware whose last line was cut off by somebody pressing
			// Ctrl-C still said it.
			if held := partial.take(); held != "" {
				return f.Write(Line{Text: held, Level: LevelOf(held), At: f.now(), Partial: true})
			}
			return nil
		case line, open := <-lines:
			if !open {
				if held := partial.take(); held != "" {
					return f.Write(Line{Text: held, Level: LevelOf(held), At: f.now(), Partial: true})
				}
				return nil
			}
			if err := f.Write(line); err != nil {
				return err
			}
		}
	}
}

// readLines is the blocking half of Tail: a stream read into lines.
func readLines(stream Stream, now func() time.Time, partial *partialLine, out chan<- Line) {
	defer close(out)
	reader := bufio.NewReader(stream)
	carry := &strings.Builder{}
	for {
		chunk, err := reader.ReadString('\n')
		if chunk != "" {
			carry.WriteString(chunk)
		}
		complete := strings.HasSuffix(chunk, "\n")
		if complete || err != nil {
			text := strings.TrimRight(carry.String(), "\r\n")
			carry.Reset()
			if text == "" && !complete {
				// The stream ended mid-nothing; there is no line to report.
				return
			}
			partial.set(text)
			out <- Line{Text: text, Level: LevelOf(text), At: now(), Partial: !complete}
			partial.set("")
		}
		if err != nil {
			return
		}
	}
}

// partialLine is the text a reader has read and not yet reported as a line, shared
// between the goroutine that reads and the loop that writes.
type partialLine struct {
	mutex sync.Mutex
	held  string
}

func (p *partialLine) set(text string) {
	p.mutex.Lock()
	p.held = text
	p.mutex.Unlock()
}

func (p *partialLine) take() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	out := p.held
	p.held = ""
	return out
}

// Counter is how many lines a run printed.
//
// Exported because a run that read a board for a minute and printed nothing is a
// run that went wrong, and the log entry saying "finished" does not say that. The
// count is what tells a silent board from a broken monitor.
type Counter struct {
	mutex sync.Mutex
	lines int
}

// Count records that one line was written.
func (c *Counter) Count() {
	c.mutex.Lock()
	c.lines++
	c.mutex.Unlock()
}

// Lines is how many lines have been written.
func (c *Counter) Lines() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.lines
}
