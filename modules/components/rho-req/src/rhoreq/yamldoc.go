//
// @file yamldoc.go
// @brief A YAML reader that remembers where every value is.
//
// The requirement files are written by people and edited by this tool, and both
// need the same thing from a parse: a validation error has to name the line it is
// about, and an edit has to touch the lines of one value and no others. The
// toolchain's loader (`rho/src/yaml`) answers neither — it returns values without
// positions, and it reads `- { name: SYS }` as a key called `{ name` — so the files
// this component owns are read here, by a parser for the subset they use:
//
//   - block mappings and block sequences, by indentation
//   - plain, single- and double-quoted scalars, and plain scalars continued
//     on more deeply indented lines
//   - `|` and `>` block scalars
//   - flow sequences and flow mappings, `[a, b]` and `{k: v}`, also across lines
//   - comments, on their own line or after a value
//
// Anchors, tags, multi-document streams and complex keys are not part of it, and
// are reported as errors with a line rather than read as something else.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind is the shape of a value.
type Kind int

const (
	// Null is a key written with nothing after it.
	Null Kind = iota
	// Scalar is a string; the reader does not type numbers or booleans, the
	// schema decides what a value means.
	Scalar
	// Map is a mapping, block or flow.
	Map
	// List is a sequence, block or flow.
	List
)

func (k Kind) String() string {
	switch k {
	case Scalar:
		return "a scalar"
	case Map:
		return "a mapping"
	case List:
		return "a list"
	}
	return "nothing"
}

// Node is one value and the lines it occupies.
type Node struct {
	Kind  Kind
	Value string
	// Style is how a scalar or a collection was written: 0 for plain or block,
	// '"' and '\'' for quoted scalars, '|' and '>' for block scalars, '[' and '{'
	// for flow collections.
	Style byte
	// Line and EndLine are 1-based and inclusive.
	Line, EndLine int
	Entries       []*Entry
	Items         []*Node
}

// Entry is one key of a mapping.
type Entry struct {
	Key string
	// Line is the key's line; Col its 0-based column.
	Line, Col int
	// ValueCol is the column where the value's text starts on the key's line, or
	// -1 when the key's line has nothing after the colon.
	ValueCol int
	Value    *Node
}

// EndLine is the last line the entry occupies.
func (e *Entry) EndLine() int {
	if e.Value != nil && e.Value.EndLine > e.Line {
		return e.Value.EndLine
	}
	return e.Line
}

// Get is the entry for a key, or nil.
func (n *Node) Get(key string) *Entry {
	if n == nil || n.Kind != Map {
		return nil
	}
	for _, entry := range n.Entries {
		if entry.Key == key {
			return entry
		}
	}
	return nil
}

// Str is the scalar value of a key, or "".
func (n *Node) Str(key string) string {
	if entry := n.Get(key); entry != nil && entry.Value.Kind == Scalar {
		return entry.Value.Value
	}
	return ""
}

// Strings is the list of scalars under a key.
func (n *Node) Strings(key string) []string {
	entry := n.Get(key)
	if entry == nil || entry.Value.Kind != List {
		return nil
	}
	out := []string{}
	for _, item := range entry.Value.Items {
		if item.Kind == Scalar {
			out = append(out, item.Value)
		}
	}
	return out
}

// ParseError is a parse failure with the line it happened on.
type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Message) }

type parser struct {
	lines  []string
	offset int // the number of the first line, minus one
}

// ParseYAML reads a document. An empty document is an empty mapping.
func ParseYAML(src string) (*Node, error) {
	return parseLines(splitLines(src), 0)
}

// splitLines is the document's lines, without their terminators.
func splitLines(src string) []string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// parseLines reads lines that start at line offset+1 of a file.
func parseLines(lines []string, offset int) (node *Node, err error) {
	p := &parser{lines: lines, offset: offset}
	defer func() {
		if recovered := recover(); recovered != nil {
			if parseErr, ok := recovered.(*ParseError); ok {
				node, err = nil, parseErr
				return
			}
			panic(recovered)
		}
	}()
	for index, line := range lines {
		if strings.Contains(leadingSpace(line), "\t") {
			p.fail(index, "a tab in indentation; indent with spaces")
		}
		if strings.TrimSpace(line) == "---" || strings.HasPrefix(line, "%") {
			p.fail(index, "multi-document streams and directives are not supported here")
		}
	}
	first := p.next(0)
	if first < 0 {
		return &Node{Kind: Map, Line: offset + 1, EndLine: offset + 1}, nil
	}
	indent := indentOf(lines[first])
	node, after := p.block(first, indent)
	if rest := p.next(after); rest >= 0 {
		p.fail(rest, "this line is not part of the document's structure (check its indentation)")
	}
	return node, nil
}

func (p *parser) fail(index int, format string, args ...any) {
	panic(&ParseError{Line: index + 1 + p.offset, Message: fmt.Sprintf(format, args...)})
}

// next is the index of the first line at or after i that holds content, or -1.
func (p *parser) next(i int) int {
	for ; i < len(p.lines); i++ {
		trimmed := strings.TrimSpace(p.lines[i])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return i
		}
	}
	return -1
}

func leadingSpace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func indentOf(line string) int { return len(leadingSpace(line)) }

// isDash reports whether text (at its start) is a block sequence entry.
func isDash(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "-\t")
}

// block reads the collection that starts on line i at the given indent.
func (p *parser) block(i, indent int) (*Node, int) {
	text := p.lines[i][indent:]
	if isDash(text) {
		return p.list(i, indent)
	}
	if !mapKey.MatchString(text) {
		p.fail(i, "expected a key or a list item, found %q", strings.TrimSpace(text))
	}
	return p.mapping(i, indent)
}

// mapKey matches the start of `key:` — a plain or quoted key followed by a colon
// and a space or the end of the line.
var mapKey = regexp.MustCompile(`^("(?:[^"\\]|\\.)*"|'(?:[^']|'')*'|[^\s#'"\[\]{},&*!|>%@` + "`" + `-][^:#]*?|-[^\s:#][^:#]*?)\s*:(\s|$)`)

// mapping reads a block mapping whose first key starts at column col of line i.
// The first line may be a list item's line, `- id: X`, with the key after the dash.
func (p *parser) mapping(i, col int) (*Node, int) {
	node := &Node{Kind: Map, Line: i + 1 + p.offset}
	seen := map[string]bool{}
	for {
		text := p.lines[i][col:]
		match := mapKey.FindStringSubmatchIndex(text)
		if match == nil {
			p.fail(i, "expected a key, found %q", strings.TrimSpace(text))
		}
		key := strings.TrimSpace(text[match[2]:match[3]])
		if len(key) > 0 && (key[0] == '"' || key[0] == '\'') {
			key = unquoteScalar(key)
		}
		if seen[key] {
			p.fail(i, "the key %q is written twice", key)
		}
		seen[key] = true
		restCol := col + match[1]
		if match[4] < match[5] { // the separator was a space; the value starts after it
			restCol = col + match[1]
		}
		entry := &Entry{Key: key, Line: i + 1 + p.offset, Col: col, ValueCol: -1}
		value, after := p.value(i, col, restCol, entry)
		entry.Value = value
		node.Entries = append(node.Entries, entry)
		node.EndLine = entry.EndLine()

		j := p.next(after)
		if j < 0 {
			return node, len(p.lines)
		}
		indent := indentOf(p.lines[j])
		if indent < col {
			return node, j
		}
		if indent > col {
			p.fail(j, "unexpected indentation")
		}
		if isDash(p.lines[j][indent:]) {
			// A list at the mapping's own column belongs to no key of it.
			return node, j
		}
		i = j
	}
}

// value reads what follows a key's colon. restCol is where the text after the
// colon starts on line i, keyCol the key's column (which continuation lines and
// block content must be indented beyond).
func (p *parser) value(i, keyCol, restCol int, entry *Entry) (*Node, int) {
	line := p.lines[i]
	rest := ""
	if restCol < len(line) {
		rest = line[restCol:]
	}
	trimmedRest := strings.TrimLeft(rest, " ")
	valueCol := restCol + (len(rest) - len(trimmedRest))
	rest = strings.TrimSpace(stripComment(trimmedRest))
	if rest == "" {
		j := p.next(i + 1)
		if j >= 0 {
			indent := indentOf(p.lines[j])
			if indent > keyCol {
				return p.block(j, indent)
			}
			if indent == keyCol && entry != nil && isDash(p.lines[j][indent:]) {
				return p.list(j, indent)
			}
		}
		return &Node{Kind: Null, Line: i + 1 + p.offset, EndLine: i + 1 + p.offset}, i + 1
	}
	if entry != nil {
		entry.ValueCol = valueCol
	}
	return p.inline(i, keyCol, valueCol, rest)
}

// inline reads a value that starts on line i at column col: a scalar, a block
// scalar header, or a flow collection.
func (p *parser) inline(i, keyCol, col int, rest string) (*Node, int) {
	lineNo := i + 1 + p.offset
	switch rest[0] {
	case '|', '>':
		return p.blockScalar(i, keyCol, rest)
	case '[', '{':
		text := p.lines[i][col:]
		end := i
		for !flowBalanced(text) {
			end++
			if end >= len(p.lines) {
				p.fail(i, "a flow collection that is never closed")
			}
			text += "\n" + p.lines[end]
		}
		value, consumed, err := parseFlow(text, 0)
		if err != nil {
			p.fail(i, "%v", err)
		}
		if tail := strings.TrimSpace(stripComment(text[consumed:])); tail != "" {
			p.fail(end, "unexpected %q after a flow collection", tail)
		}
		setLines(value, lineNo, end+1+p.offset)
		return value, end + 1
	case '"', '\'':
		text := p.lines[i][col:]
		closing := closingQuote(text)
		if closing < 0 {
			p.fail(i, "a quoted value that is not closed on its line")
		}
		if tail := strings.TrimSpace(stripComment(text[closing+1:])); tail != "" {
			p.fail(i, "unexpected %q after a quoted value", tail)
		}
		return &Node{Kind: Scalar, Value: unquoteScalar(text[:closing+1]), Style: rest[0], Line: lineNo, EndLine: lineNo}, i + 1
	case '&', '*', '!':
		p.fail(i, "anchors, aliases and tags are not supported here")
	}
	// A plain scalar, possibly continued on more deeply indented lines.
	value := rest
	end := i
	for j := i + 1; j < len(p.lines); j++ {
		trimmed := strings.TrimSpace(p.lines[j])
		if trimmed == "" {
			break
		}
		if strings.HasPrefix(trimmed, "#") || indentOf(p.lines[j]) <= keyCol || mapKey.MatchString(trimmed) {
			// A `key:` line is not a continuation; left alone, it is reported as
			// indentation that describes no structure.
			break
		}
		value += " " + strings.TrimSpace(stripComment(trimmed))
		end = j
	}
	return &Node{Kind: Scalar, Value: value, Line: lineNo, EndLine: end + 1 + p.offset}, end + 1
}

// blockScalar reads a `|` or `>` scalar whose header is on line i.
func (p *parser) blockScalar(i, keyCol int, header string) (*Node, int) {
	style := header[0]
	chomp := byte(0)
	for _, r := range header[1:] {
		switch {
		case r == '-' || r == '+':
			chomp = byte(r)
		case r >= '1' && r <= '9':
		default:
			p.fail(i, "unexpected %q in a block scalar header", header)
		}
	}
	content := []string{}
	end := i
	contentIndent := -1
	for j := i + 1; j < len(p.lines); j++ {
		line := p.lines[j]
		if strings.TrimSpace(line) == "" {
			content = append(content, "")
			continue
		}
		indent := indentOf(line)
		if indent <= keyCol {
			break
		}
		if contentIndent < 0 {
			contentIndent = indent
		}
		if indent < contentIndent {
			break
		}
		content = append(content, line[contentIndent:])
		end = j
	}
	// Trailing blank lines are not the scalar's.
	kept := end - i
	if kept < len(content) {
		content = content[:kept]
	}
	var value string
	if style == '|' {
		value = strings.Join(content, "\n")
	} else {
		value = foldLines(content)
	}
	if chomp != '-' && value != "" {
		value += "\n"
	}
	lineNo := i + 1 + p.offset
	return &Node{Kind: Scalar, Value: value, Style: style, Line: lineNo, EndLine: end + 1 + p.offset}, end + 1
}

// foldLines joins a folded scalar's lines: a line break becomes a space, an
// empty line a line break.
func foldLines(lines []string) string {
	var b strings.Builder
	for index, line := range lines {
		if index > 0 {
			if line == "" || lines[index-1] == "" {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(line)
	}
	return strings.ReplaceAll(b.String(), "\n\n", "\n")
}

// list reads a block sequence whose dashes are at column indent.
func (p *parser) list(i, indent int) (*Node, int) {
	node := &Node{Kind: List, Line: i + 1 + p.offset}
	for {
		line := p.lines[i]
		after := line[indent+1:]
		contentCol := indent + 1 + (len(after) - len(strings.TrimLeft(after, " ")))
		content := strings.TrimSpace(stripComment(strings.TrimLeft(after, " ")))
		var item *Node
		next := i + 1
		switch {
		case content == "":
			j := p.next(i + 1)
			if j >= 0 && indentOf(p.lines[j]) > indent {
				item, next = p.block(j, indentOf(p.lines[j]))
				item.Line = i + 1 + p.offset
			} else {
				item = &Node{Kind: Null, Line: i + 1 + p.offset, EndLine: i + 1 + p.offset}
			}
		case isDash(content):
			item, next = p.list(i, contentCol)
		case mapKey.MatchString(line[contentCol:]) && content[0] != '[' && content[0] != '{':
			item, next = p.mapping(i, contentCol)
		default:
			item, next = p.inline(i, indent, contentCol, content)
		}
		node.Items = append(node.Items, item)
		node.EndLine = item.EndLine

		j := p.next(next)
		if j < 0 {
			return node, len(p.lines)
		}
		at := indentOf(p.lines[j])
		if at < indent {
			return node, j
		}
		if at > indent {
			p.fail(j, "unexpected indentation")
		}
		if !isDash(p.lines[j][at:]) {
			return node, j
		}
		i = j
	}
}

func setLines(n *Node, line, end int) {
	n.Line, n.EndLine = line, end
	for _, item := range n.Items {
		setLines(item, line, end)
	}
	for _, entry := range n.Entries {
		entry.Line = line
		setLines(entry.Value, line, end)
	}
}

// stripComment removes a `#` comment that follows white space, outside quotes.
func stripComment(text string) string {
	return text[:commentStart(text)]
}

// commentStart is the index where a trailing comment starts, or len(text).
func commentStart(text string) int {
	var quote byte
	for index := 0; index < len(text); index++ {
		c := text[index]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				index++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			if index == 0 || strings.ContainsRune(" \t[{,:", rune(text[index-1])) {
				quote = c
			}
		case c == '#':
			if index == 0 || text[index-1] == ' ' || text[index-1] == '\t' {
				return index
			}
		}
	}
	return len(text)
}

// closingQuote is the index of the quote that closes the one text starts with.
func closingQuote(text string) int {
	quote := text[0]
	for index := 1; index < len(text); index++ {
		switch {
		case quote == '"' && text[index] == '\\':
			index++
		case text[index] == quote:
			if quote == '\'' && index+1 < len(text) && text[index+1] == '\'' {
				index++
				continue
			}
			return index
		}
	}
	return -1
}

// unquoteScalar is the value a quoted scalar stands for.
func unquoteScalar(text string) string {
	if len(text) < 2 {
		return text
	}
	switch text[0] {
	case '\'':
		return strings.ReplaceAll(text[1:len(text)-1], "''", "'")
	case '"':
		var b strings.Builder
		inner := text[1 : len(text)-1]
		for index := 0; index < len(inner); index++ {
			c := inner[index]
			if c != '\\' || index+1 >= len(inner) {
				b.WriteByte(c)
				continue
			}
			index++
			switch inner[index] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"', '\\', '/':
				b.WriteByte(inner[index])
			default:
				b.WriteByte('\\')
				b.WriteByte(inner[index])
			}
		}
		return b.String()
	}
	return text
}

// flowBalanced reports whether every bracket opened in text is closed.
func flowBalanced(text string) bool {
	depth := 0
	var quote byte
	for index := 0; index < len(text); index++ {
		c := text[index]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				index++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && (index == 0 || text[index-1] == ' '):
			// A comment runs to the end of its line.
			for index < len(text) && text[index] != '\n' {
				index++
			}
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return depth == 0
}

// parseFlow reads a flow value at text[pos:] and returns where it ended.
func parseFlow(text string, pos int) (*Node, int, error) {
	pos = skipFlowSpace(text, pos)
	if pos >= len(text) {
		return nil, pos, fmt.Errorf("a flow collection ends early")
	}
	switch text[pos] {
	case '[':
		node := &Node{Kind: List, Style: '['}
		pos = skipFlowSpace(text, pos+1)
		if pos < len(text) && text[pos] == ']' {
			return node, pos + 1, nil
		}
		for {
			item, next, err := parseFlow(text, pos)
			if err != nil {
				return nil, next, err
			}
			node.Items = append(node.Items, item)
			pos = skipFlowSpace(text, next)
			if pos >= len(text) {
				return nil, pos, fmt.Errorf("a flow list that is never closed")
			}
			if text[pos] == ']' {
				return node, pos + 1, nil
			}
			if text[pos] != ',' {
				return nil, pos, fmt.Errorf("expected ',' or ']' in a flow list, found %q", text[pos])
			}
			pos = skipFlowSpace(text, pos+1)
			if pos < len(text) && text[pos] == ']' { // a trailing comma
				return node, pos + 1, nil
			}
		}
	case '{':
		node := &Node{Kind: Map, Style: '{'}
		pos = skipFlowSpace(text, pos+1)
		if pos < len(text) && text[pos] == '}' {
			return node, pos + 1, nil
		}
		for {
			key, next, err := flowScalar(text, pos, true)
			if err != nil {
				return nil, next, err
			}
			pos = skipFlowSpace(text, next)
			if pos >= len(text) || text[pos] != ':' {
				return nil, pos, fmt.Errorf("expected ':' after %q in a flow mapping", key.Value)
			}
			if node.Get(key.Value) != nil {
				return nil, pos, fmt.Errorf("the key %q is written twice", key.Value)
			}
			value, after, err := parseFlow(text, pos+1)
			if err != nil {
				return nil, after, err
			}
			node.Entries = append(node.Entries, &Entry{Key: key.Value, ValueCol: -1, Value: value})
			pos = skipFlowSpace(text, after)
			if pos >= len(text) {
				return nil, pos, fmt.Errorf("a flow mapping that is never closed")
			}
			if text[pos] == '}' {
				return node, pos + 1, nil
			}
			if text[pos] != ',' {
				return nil, pos, fmt.Errorf("expected ',' or '}' in a flow mapping, found %q", text[pos])
			}
			pos = skipFlowSpace(text, pos+1)
			if pos < len(text) && text[pos] == '}' {
				return node, pos + 1, nil
			}
		}
	}
	return flowScalar(text, pos, false)
}

// flowScalar reads a scalar inside a flow collection.
func flowScalar(text string, pos int, isKey bool) (*Node, int, error) {
	pos = skipFlowSpace(text, pos)
	if pos < len(text) && (text[pos] == '"' || text[pos] == '\'') {
		closing := closingQuote(text[pos:])
		if closing < 0 {
			return nil, pos, fmt.Errorf("a quoted value that is never closed")
		}
		return &Node{Kind: Scalar, Style: text[pos], Value: unquoteScalar(text[pos : pos+closing+1])}, pos + closing + 1, nil
	}
	start := pos
	for pos < len(text) {
		c := text[pos]
		if c == ',' || c == ']' || c == '}' || c == '[' || c == '{' || c == '\n' {
			break
		}
		if c == ':' && (isKey || pos+1 >= len(text) || text[pos+1] == ' ') {
			break
		}
		if c == '#' && pos > start && text[pos-1] == ' ' {
			break
		}
		pos++
	}
	value := strings.TrimSpace(text[start:pos])
	if value == "" {
		return nil, pos, fmt.Errorf("an empty entry in a flow collection")
	}
	return &Node{Kind: Scalar, Value: value}, pos, nil
}

func skipFlowSpace(text string, pos int) int {
	for pos < len(text) {
		switch text[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		case '#':
			if pos == 0 || text[pos-1] == ' ' || text[pos-1] == '\n' {
				for pos < len(text) && text[pos] != '\n' {
					pos++
				}
				continue
			}
			return pos
		default:
			return pos
		}
	}
	return pos
}
