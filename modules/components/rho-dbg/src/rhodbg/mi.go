//
// @file mi.go
// @brief The GDB Machine Interface: what a debugger says, and how it is read.
//
// MI is a line protocol with a grammar rather than a free format, and a session
// that treats it as one is a session that works until a filename contains a comma
// or a program's output happens to start with a bracket. So it is parsed here,
// once, into values a session can ask questions of — which is what makes the
// translation from an editor's request to a debugger's command something that can
// be tested without a debugger anywhere in sight.
//
// The grammar, as far as a session needs it:
//
//	[token] ^done,threads=[{id="1"},{id="2"}]
//	[token] *stopped,reason="breakpoint-hit",disp="keep",bkptno="1"
//	~"the program's own output\n"
//
// A record is a token, a kind character, a class, and a list of results. Results
// are `name=value` pairs whose values are strings, tuples or lists, which is
// JSON with different punctuation — so the same parser reads both, and a debugger
// extension that adds a field does not need one added here.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Record is one line from a debugger.
type Record struct {
	// Token is the number this command was answered with, or empty for a record
	// that answers nothing — a notification, a line of console output, a stop.
	Token string
	// Kind is the character that says what sort of record it is: `^` for a result,
	// `=` for a notification, `*` for an async stop, `+` for a status report, `~`
	// for the target's console stream, `&` for the debugger's log stream.
	Kind byte
	// Class is what it says: `done`, `running`, `error`, `exit`, `connected`, or
	// the name of a notification.
	Class string
	// Results is what it says about it.
	Results Result
}

// Value is one result's value: a string, a tuple, or a list of values.
type Value any

// Tuple is a `{}` value: named values, as MI's maps.
type Tuple map[string]Value

// Result is the `name=value` pairs that follow a class.
type Result map[string]Value

// The record kinds. Named so a reader does not have to know the characters.
const (
	KindResult    byte = '^'
	KindNotify    byte = '='
	KindAsyncStop byte = '*'
	KindStatus    byte = '+'
	KindTargetOut byte = '~'
	KindLogOut    byte = '&'
	KindTargetErr byte = '@'
)

// Parse reads one MI record.
//
// An empty line is not a record and is reported as one: a debugger that prints a
// blank line to its console — which a program under test does whenever it prints
// one — must not be read as a malformed record that invalidates everything after
// it.
func Parse(line string) (Record, error) {
	trimmed := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return Record{}, fmt.Errorf("no record: the line is empty")
	}
	if IsPrompt(trimmed) {
		return Record{}, fmt.Errorf("no record: %q is a prompt", trimmed)
	}

	// Results is never nil: a record with no results is `^connected`, which is an
	// answer in its own right, and a caller that has to check for nil before asking
	// a record anything is a caller that will forget.
	record := Record{Results: Result{}}
	cursor := 0

	// A token, if there is one: digits before the kind character.
	for cursor < len(trimmed) && trimmed[cursor] >= '0' && trimmed[cursor] <= '9' {
		record.Token += string(trimmed[cursor])
		cursor++
	}
	if cursor >= len(trimmed) {
		return Record{}, fmt.Errorf("no record: %q is only a token", trimmed)
	}

	record.Kind = trimmed[cursor]
	cursor++

	// The class is everything up to the first comma or brace that starts the
	// results — except for the stream records, where the whole rest is a string.
	switch record.Kind {
	case KindTargetOut, KindLogOut, KindTargetErr:
		value, consumed, err := ReadValue(trimmed[cursor:])
		if err != nil {
			return Record{}, err
		}
		record.Results = Result{"output": value}
		cursor += consumed
	default:
		end := cursor
		for end < len(trimmed) && trimmed[end] != ',' {
			end++
		}
		record.Class = trimmed[cursor:end]
		cursor = end
	}

	if cursor < len(trimmed) {
		if trimmed[cursor] != ',' {
			return Record{}, fmt.Errorf("no record: %q has %q where a result list should start", trimmed, trimmed[cursor:cursor+1])
		}
		cursor++
		results, err := readResults(trimmed[cursor:])
		if err != nil {
			return Record{}, fmt.Errorf("no record: %q: %w", trimmed, err)
		}
		record.Results = results
	}

	if record.Class == "" && record.Kind != KindTargetOut && record.Kind != KindLogOut && record.Kind != KindTargetErr {
		return Record{}, fmt.Errorf("no record: %q says what happened but not how", trimmed)
	}
	return record, nil
}

// IsPrompt is whether a line is gdb's prompt rather than a record.
//
// MI2 prints `(gdb)` when asynchronous reporting is off, and for the start-up
// commands a debugger was given rather than the ones it was sent. It is not a
// record and it is not the program's output either: a reader that shows it puts a
// line in the console for every command, which is how a session that is working
// perfectly comes to look like one that is not.
func IsPrompt(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "(") || !strings.HasSuffix(trimmed, ")") {
		return false
	}
	// `(gdb)` and `(gdb) ` are the same line; anything inside the brackets that is
	// not the debugger's own name is a record after all.
	return !strings.Contains(trimmed[1:len(trimmed)-1], "\"")
}

// readResults reads a `name=value` list up to the end of the line.
func readResults(input string) (Result, error) {
	results := Result{}
	parser := &parser{input: input}
	for parser.more() {
		name, err := parser.name()
		if err != nil {
			return nil, err
		}
		value, err := parser.value()
		if err != nil {
			return nil, fmt.Errorf("the result %q: %w", name, err)
		}
		results[name] = value
		parser.comma()
	}
	return results, nil
}

// parser walks a MI value list.
type parser struct {
	input string
	at    int
}

func (p *parser) more() bool {
	p.skip()
	return p.at < len(p.input)
}

func (p *parser) skip() {
	for p.at < len(p.input) && (p.input[p.at] == ' ' || p.input[p.at] == ',') {
		p.at++
	}
}

func (p *parser) comma() {
	p.skip()
}

// name reads a bare identifier: a run of characters that are not a delimiter.
func (p *parser) name() (string, error) {
	start := p.at
	for p.at < len(p.input) {
		c := p.input[p.at]
		if c == '=' || c == ' ' || c == ',' {
			break
		}
		p.at++
	}
	if p.at >= len(p.input) || p.input[p.at] != '=' {
		return "", fmt.Errorf("expected a name and an =, got %q", p.input[start:])
	}
	name := p.input[start:p.at]
	if name == "" {
		return "", fmt.Errorf("a result has no name")
	}
	p.at++
	return name, nil
}

// value reads one value: a quoted string, a tuple, or a list.
func (p *parser) value() (Value, error) {
	p.skip()
	if p.at >= len(p.input) {
		return nil, fmt.Errorf("expected a value, got nothing")
	}
	switch p.input[p.at] {
	case '"':
		return p.text()
	case '{':
		return p.tuple()
	case '[':
		return p.list()
	}
	// A bare value. MI has none, but a debugger extension has been known to add
	// one, and refusing the whole record over it would cost more than reading it.
	start := p.at
	for p.at < len(p.input) && p.input[p.at] != ',' && p.input[p.at] != ' ' {
		p.at++
	}
	return p.input[start:p.at], nil
}

// text reads a C-style quoted string.
func (p *parser) text() (string, error) {
	if p.input[p.at] != '"' {
		return "", fmt.Errorf("expected a string, got %q", p.input[p.at:p.at+1])
	}
	p.at++
	out := &strings.Builder{}
	for p.at < len(p.input) {
		c := p.input[p.at]
		if c == '"' {
			p.at++
			return out.String(), nil
		}
		if c != '\\' {
			out.WriteByte(c)
			p.at++
			continue
		}
		p.at++
		if p.at >= len(p.input) {
			break
		}
		switch p.input[p.at] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '0':
			out.WriteByte(0)
		case '\\', '"', '\'', '`':
			out.WriteByte(p.input[p.at])
		default:
			// An escape this does not know: MI allows `x` followed by digits, so
			// the escape is the character itself unless it is a known one.
			out.WriteByte(p.input[p.at])
		}
		p.at++
	}
	return "", fmt.Errorf("a string is not closed")
}

// tuple reads a `{}` value.
func (p *parser) tuple() (Value, error) {
	p.at++ // '{'
	out := Tuple{}
	for {
		p.skip()
		if p.at >= len(p.input) {
			return nil, fmt.Errorf("a tuple is not closed")
		}
		if p.input[p.at] == '}' {
			p.at++
			return out, nil
		}
		name, err := p.name()
		if err != nil {
			return nil, err
		}
		value, err := p.value()
		if err != nil {
			return nil, fmt.Errorf("the value of %q: %w", name, err)
		}
		out[name] = value
		p.comma()
	}
}

// list reads a `[]` value.
//
// An element may be a bare value or a named one — `name=value` — because MI has
// both and uses them for different things. A list of threads is a list of bare
// tuples, while a list of frames is a list of *named* values: `frame={...}`, one
// per frame, where the name says what kind of entry it is. A parser that reads
// only bare values loses every frame, which is the shape a stack is always in.
func (p *parser) list() (Value, error) {
	p.at++ // '['
	out := []Value{}
	for {
		p.skip()
		if p.at >= len(p.input) {
			return nil, fmt.Errorf("a list is not closed")
		}
		if p.input[p.at] == ']' {
			p.at++
			return out, nil
		}
		// A named element is an identifier followed by `=`; a bare one is anything
		// else, including a quoted string, which is why this is decided by what the
		// `=` follows rather than by the first character.
		if name, isName := p.takeName(); isName {
			value, err := p.value()
			if err != nil {
				return nil, fmt.Errorf("the value of %q: %w", name, err)
			}
			out = append(out, Tuple{name: value})
			p.comma()
			continue
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		p.comma()
	}
}

// takeName consumes an identifier and the `=` after it, and says whether that is
// what was there.
//
// Decided by scanning ahead rather than by the first character, because a bare
// value may be a quoted string, a tuple or a list, and only the identifier form has
// an `=` in it. Read ahead and not consumed, so that a value which turns out not to
// have a name after it is read whole.
func (p *parser) takeName() (string, bool) {
	index := p.at
	for index < len(p.input) {
		switch c := p.input[index]; c {
		case '=':
			if index == p.at {
				return "", false
			}
			name := p.input[p.at:index]
			p.at = index + 1
			return name, true
		case ' ', ',', '"', '{', '[':
			return "", false
		}
		index++
	}
	return "", false
}

// ReadValue reads a value from the start of a string, and says how much of it it
// used. It is what a caller uses when a value is not the whole line — a frame's
// fields, a variable's value.
func ReadValue(input string) (Value, int, error) {
	p := &parser{input: input}
	value, err := p.value()
	if err != nil {
		return nil, 0, err
	}
	return value, p.at, nil
}

/* -------------------------------------------------------------------------- */
/*  Reading results                                                            */
/* -------------------------------------------------------------------------- */

// String is a result read as text.
//
// Results that are not there are empty rather than an error: a field a debugger
// only sends for some records is a field this session asks for and gets, and a
// session that failed on every absent field would fail on every command.
func (r Result) String(name string) string {
	value, found := r[name]
	if !found {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case Tuple, []Value:
		return render(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// Int is a result read as a number, and whether it was one.
func (r Result) Int(name string) (int, bool) {
	value := strings.TrimSpace(r.String(name))
	if value == "" {
		return 0, false
	}
	// MI renders an integer as a string, and a hexadecimal one with a 0x in it.
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "-0x") {
		parsed, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimPrefix(value, "-"), "0x"), 16, 64)
		if err != nil {
			return 0, false
		}
		if strings.HasPrefix(value, "-") {
			parsed = -parsed
		}
		return int(parsed), true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// Tuple is a result read as a tuple.
func (r Result) Tuple(name string) (Tuple, bool) {
	value, found := r[name].(Tuple)
	return value, found
}

// List is a result read as a list.
func (r Result) List(name string) ([]Value, bool) {
	value, found := r[name].([]Value)
	return value, found
}

// Text reads a string out of a tuple, the shape most MI results are: a frame is a
// tuple, a thread is a tuple, a breakpoint is a tuple.
func (t Tuple) Text(name string) string { return Result(t).String(name) }

// Number reads a number out of a tuple.
func (t Tuple) Number(name string) (int, bool) { return Result(t).Int(name) }

// Int is a number read out of a tuple.
func (t Tuple) Int(name string) (int, bool) { return Result(t).Int(name) }

// Nested is a tuple inside a tuple.
func (t Tuple) Nested(name string) (Tuple, bool) {
	value, found := t[name].(Tuple)
	return value, found
}

// Sequence is a list inside a tuple.
func (t Tuple) Sequence(name string) ([]Value, bool) {
	value, found := t[name].([]Value)
	return value, found
}

// render is a value as the debugger would have written it, for a field read as
// text that is really a structure.
func render(value Value) string {
	out := &strings.Builder{}
	writeRendered(out, value)
	return out.String()
}

func writeRendered(out *strings.Builder, value Value) {
	switch typed := value.(type) {
	case string:
		out.WriteString(typed)
	case Tuple:
		out.WriteString("{")
		first := true
		for _, key := range sortedTupleKeys(typed) {
			if !first {
				out.WriteString(", ")
			}
			first = false
			out.WriteString(key)
			out.WriteString("=")
			writeRendered(out, typed[key])
		}
		out.WriteString("}")
	case []Value:
		out.WriteString("[")
		for index, one := range typed {
			if index > 0 {
				out.WriteString(", ")
			}
			writeRendered(out, one)
		}
		out.WriteString("]")
	default:
		fmt.Fprint(out, typed)
	}
}

/* -------------------------------------------------------------------------- */
/*  Writing commands                                                           */
/* -------------------------------------------------------------------------- */

// Quote renders a value as an MI constant, escaping what a constant has to escape.
//
// Every value this sends is quoted rather than bare, including ones that look
// numeric. MI's bare form is a debugger extension rather than the grammar, and a
// path with a space in it — which a build directory has on some machines and does
// not on others — is the difference between a session that starts and one that
// fails with a syntax error in a line the editor never shows.
func Quote(value string) string {
	out := &strings.Builder{}
	out.WriteByte('"')
	for _, c := range value {
		switch c {
		case '"':
			out.WriteString("\\\"")
		case '\\':
			out.WriteString("\\\\")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			out.WriteRune(c)
		}
	}
	out.WriteByte('"')
	return out.String()
}

// Command is one MI command: a name and the arguments that go with it.
//
// Built from parts rather than assembled from a string, because MI's own shape is
// the thing that goes wrong. A command is a token, a name, and arguments separated
// by spaces — *not* a comma after the name and `name=value` pairs, which is what
// the result records use and what an implementer writes first. `gdb` answers the
// difference with a usage message, so each command below is written in the form
// gdb reads it.
type Command struct {
	name string
	args []string
}

// Of is a command with no arguments.
func Of(name string) Command { return Command{name: name} }

// With is a command with one text argument.
func With(name string, value string) Command {
	return Command{name: name}.Arg(value)
}

// Arg adds an argument as it is written.
//
// Quoted unless it is a number or an already-formed option: gdb splits its command
// line on whitespace, so a path with a space in it has to arrive quoted and a
// number must not be quoted where a number is expected.
func (c Command) Arg(value string) Command {
	c.args = append(c.args, renderArgument(value))
	return c
}

// Opt adds an option and the value it takes, as gdb's options are written: the flag,
// then its value.
func (c Command) Opt(flag string, value string) Command {
	c.args = append(c.args, flag, renderArgument(value))
	return c
}

// Num adds a number, which is never quoted.
func (c Command) Num(number int) Command {
	c.args = append(c.args, strconv.Itoa(number))
	return c
}

// Word adds an argument exactly as it is written, with no quoting.
//
// For the arguments gdb reads as words rather than as strings: a remote target's
// address, for one, is `host:port` and gdb takes the colon as a separator, so a
// quoted `localhost:3333` is looked up as the name of a target that does not exist.
func (c Command) Word(value string) Command {
	c.args = append(c.args, value)
	return c
}

// Location is a file and a line as gdb's breakpoints are written: `main.c:21`.
//
// A location rather than the `-source`/`-line` pair that another debugger's protocol
// accepts, because gdb's own does not: it answers `-source` with a usage message.
func Location(file string, line int) string {
	return fmt.Sprintf("%s:%d", file, line)
}

// renderArgument is one argument as MI writes it.
func renderArgument(value string) string {
	if value == "" {
		return `""`
	}
	// A number, a signed number, or a hexadecimal one: left as it is, because a
	// command that expects a number refuses a string.
	for _, c := range value {
		if (c >= '0' && c <= '9') || c == 'x' || c == '-' || c == '+' {
			continue
		}
		return Quote(value)
	}
	return value
}

// String renders the command as it is sent, with the token in front of it.
func (c Command) String(token string) string {
	out := &strings.Builder{}
	if token != "" {
		out.WriteString(token)
	}
	out.WriteString(c.name)
	for _, argument := range c.args {
		out.WriteString(" ")
		out.WriteString(argument)
	}
	return out.String()
}

// Arguments is what the command would send without its token, for a reader that
// wants to know what was asked.
func (c Command) Arguments() string {
	return strings.TrimPrefix(c.String(""), c.name)
}

// sortedTupleKeys are a tuple's keys in name order, which is what a tuple rendered
// as text is read in: MI does not say in which order a debugger writes them, and an
// unordered rendering of the same record is a difference in every comparison
// against one.
func sortedTupleKeys(value Tuple) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
