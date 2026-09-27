//
// @file parse.go
// @brief The MyST-style Markdown parser.
//
// The source is read as MyST: Markdown, plus the Sphinx constructs a reference
// needs. Each is a small rule on top of the Markdown block grammar:
//
//	# Title {#target}        a heading with an explicit target
//	:::{note} … :::          an admonition, or any other directive
//	{eval-rst} … {/eval-rst} a block of reStructuredText kept as source
//	[text]{ref}`label`       a cross-reference role
//	:ref:`label`             a role in the classic spelling
//
// Everything else is Markdown: paragraphs, ATX headings, fenced and indented
// code, ordered and bullet lists, block quotes, tables, and the usual inline
// spans. A line the parser does not understand becomes a paragraph, so a source
// always yields the text its author wrote.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Parse reads a doc-as-code source into a document.
func Parse(path, source string) *Document {
	lines := splitLines(source)
	p := &parser{path: path, lines: lines, line: 1, targets: map[string]bool{}}
	document := &Document{Path: path, Target: defaultTarget(path)}
	document.Blocks = p.parseBlocks(0)
	document.Diagnostics = p.diagnostics
	document.collectHeadings()
	document.diagnose()
	return document
}

// defaultTarget is the identifier a document is addressed by when its author
// names none: its file name without directories or extension.
func defaultTarget(path string) string {
	name := path
	if index := strings.LastIndexAny(name, "/\\"); index >= 0 {
		name = name[index+1:]
	}
	name = strings.TrimSuffix(name, ".md")
	return headingSlug(name)
}

// splitLines splits a source into lines, dropping a UTF-8 byte order mark and
// a final empty line so the last line is not read twice.
func splitLines(source string) []string {
	source = strings.TrimPrefix(source, "\ufeff")
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type parser struct {
	path         string
	lines        []string
	line         int
	diagnostics  []Diagnostic
	targets      map[string]bool
	usedExplicit bool
}

// parseBlocks reads blocks until the lines run out or a fence closes at the
// given indentation. It stops before a blank-line-separated run of a shape the
// caller handles, which is how nested content is found.
func (p *parser) parseBlocks(indent int) []*Node {
	blocks := []*Node{}
	for p.line <= len(p.lines) {
		raw := p.lines[p.line-1]
		trimmed := strings.TrimSpace(raw)
		switch {
		case trimmed == "":
			p.line++
		case isDirectiveOpen(trimmed):
			if node := p.parseDirective(indent); node != nil {
				blocks = append(blocks, node)
			}
		case isHeading(trimmed):
			blocks = append(blocks, p.parseHeading(indent))
		case isFence(trimmed):
			blocks = append(blocks, p.parseCode(indent))
		case isTableRow(trimmed) && p.nextLineIsTableDelimiter():
			// Read as a table rather than as prose: the row is the header, and
			// the delimiter below it says so. Deciding here keeps the header
			// from being emitted twice, once as a paragraph and once as a row.
			blocks = append(blocks, p.parseTable(indent))
		case isQuote(trimmed):
			blocks = append(blocks, p.parseQuote(indent))
		case listMarker(trimmed) != "":
			blocks = append(blocks, p.parseList(indent))
		default:
			if node := p.parseParagraph(indent); node != nil {
				blocks = append(blocks, node)
			}
		}
	}
	return blocks
}

/* -------------------------------------------------------------------------- */
/*  Headings                                                                  */
/* -------------------------------------------------------------------------- */

// isHeading reports whether a line is an ATX heading.
func isHeading(line string) bool {
	if !strings.HasPrefix(line, "#") {
		return false
	}
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	return level <= 6 && level < len(line) && line[level] == ' '
}

// parseHeading reads a heading, taking the explicit target from a trailing
// {#id} when the author wrote one.
func (p *parser) parseHeading(indent int) *Node {
	raw := p.lines[p.line-1]
	line := p.line
	p.line++

	trimmed := strings.TrimSpace(raw)
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}

	text := strings.TrimSpace(trimmed[level:])
	target := ""
	if index := strings.LastIndex(text, "{#"); index >= 0 && strings.HasSuffix(text, "}") {
		target = strings.TrimSpace(text[index+2 : len(text)-1])
		text = strings.TrimSpace(text[:index])
	}
	// The closing hashes of a closed ATX heading are decoration.
	text = strings.TrimSpace(strings.TrimRight(text, "#"))

	node := &Node{
		Kind:     BlockHeading,
		Level:    level,
		Text:     text,
		Target:   target,
		Line:     line,
		Children: parseInline(text),
	}
	if target == "" {
		node.ID = uniqueID(headingSlug(text), p.targets)
	} else {
		node.ID = target
		if p.targets[target] {
			p.warn(line, "target %q is used more than once; the first one wins", target)
		}
		p.targets[target] = true
	}
	return node
}

// uniqueID keeps derived identifiers distinct, the way a documentation tool
// does when two headings in a page reduce to the same slug.
func uniqueID(slug string, used map[string]bool) string {
	if slug == "" {
		slug = "section"
	}
	if !used[slug] {
		used[slug] = true
		return slug
	}
	for suffix := 2; ; suffix++ {
		candidate := slug + "-" + itoa(suffix)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Directives and admonitions                                                 */
/* -------------------------------------------------------------------------- */

// isDirectiveOpen reports whether a line opens a MyST directive: the fence is
// three or more colons followed by a braced name, or the classic `.. name::`.
func isDirectiveOpen(line string) bool {
	if strings.HasPrefix(line, ".. ") && strings.Contains(line, "::") {
		return true
	}
	colons := 0
	for colons < len(line) && line[colons] == ':' {
		colons++
	}
	if colons < 3 || colons >= len(line) || line[colons] != '{' {
		return false
	}
	rest := line[colons:]
	return !strings.HasPrefix(rest, "{::") && strings.Contains(rest, "}")
}

// parseDirective reads a directive and its body. An admonition is a directive
// with a name the renderer knows how to frame, so both travel the same path.
func (p *parser) parseDirective(indent int) *Node {
	raw := p.lines[p.line-1]
	trimmed := strings.TrimSpace(raw)
	line := p.line

	colons := 0
	for colons < len(trimmed) && trimmed[colons] == ':' {
		colons++
	}
	// The name and its options sit between the braces; the rest of the opening
	// line is the directive's arguments, as in `:::{code-block} rho`.
	inner := strings.TrimSpace(trimmed[colons+1 : strings.LastIndex(trimmed, "}")])
	arguments := strings.TrimSpace(trimmed[strings.LastIndex(trimmed, "}")+1:])
	name, options := inner, ""
	if space := strings.IndexAny(inner, " \t"); space > 0 {
		name, options = inner[:space], strings.TrimSpace(inner[space+1:])
	}
	if arguments == "" {
		arguments = options
	}
	if name == "" {
		name = "note"
	}
	p.line++

	bodyIndent := indent + colons
	// A directive that holds code holds it verbatim: parsing its body as prose
	// would act on what the author wrote as an example, which is exactly what a
	// code block is for showing.
	if isCodeDirective(name) {
		return p.parseCodeDirective(name, arguments, line)
	}
	node := &Node{Kind: BlockDirective, Anchor: name, Text: arguments, Line: line, ID: headingSlug(name)}
	if isAdmonition(name) {
		node.Kind = BlockAdmonition
	}
	// The body ends at the closing fence: a run of colons at least as long as
	// the opening one that opens nothing, so a nested directive's own fence
	// does not close its parent.
	body := []string{}
	for p.line <= len(p.lines) {
		current := strings.TrimSpace(p.lines[p.line-1])
		if isDirectiveClose(current, colons) {
			p.line++
			break
		}
		body = append(body, p.lines[p.line-1])
		p.line++
	}
	// The body is parsed at the indentation of the directive, so nested lists
	// and code keep their structure.
	indentOfBody := leadingIndent(body)
	if indentOfBody < bodyIndent {
		bodyIndent = indentOfBody
	}
	inner1 := &parser{path: p.path, lines: dedent(body, bodyIndent), line: 1, targets: map[string]bool{}}
	node.Children = inner1.parseBlocks(bodyIndent)
	for _, diagnostic := range inner1.diagnostics {
		p.diagnostics = append(p.diagnostics, diagnostic)
	}
	return node
}

func isDirectiveClose(line string, opening int) bool {
	colons := 0
	for colons < len(line) && line[colons] == ':' {
		colons++
	}
	if colons < opening {
		return false
	}
	// A fence followed by a name opens a nested directive rather than closing
	// this one.
	return colons >= len(line) || line[colons] != '{'
}

// isCodeDirective reports whether a directive's body is code.
func isCodeDirective(name string) bool {
	switch strings.ToLower(name) {
	case "code", "code-block", "sourcecode", "eval-rst", "literalinclude", "raw":
		return true
	}
	return false
}

// parseCodeDirective reads a directive whose body is code, and answers a code
// block holding it as it was written. The opener has already been consumed.
func (p *parser) parseCodeDirective(name, language string, line int) *Node {
	body := []string{}
	for p.line <= len(p.lines) {
		current := strings.TrimSpace(p.lines[p.line-1])
		if isDirectiveClose(current, 3) {
			p.line++
			break
		}
		body = append(body, p.lines[p.line-1])
		p.line++
	}
	trimmed := dedent(body, leadingIndent(body))
	return &Node{
		Kind:     BlockCode,
		Lang:     strings.TrimSpace(language),
		Line:     line,
		Anchor:   name,
		Children: []*Node{{Kind: BlockHTML, Text: strings.Join(trimmed, "\n")}},
	}
}

// isAdmonition reports whether a directive is one of the framed notes a
// documentation tool gives a name to.
func isAdmonition(name string) bool {
	switch strings.ToLower(name) {
	case "note", "warning", "tip", "important", "caution", "danger", "hint", "attention", "error":
		return true
	}
	return false
}

func leadingIndent(lines []string) int {
	smallest := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if smallest < 0 || indent < smallest {
			smallest = indent
		}
	}
	if smallest < 0 {
		return 0
	}
	return smallest
}

func dedent(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) >= width {
			out = append(out, line[width:])
		} else {
			out = append(out, strings.TrimLeft(line, " \t"))
		}
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Code                                                                      */
/* -------------------------------------------------------------------------- */

// isFence reports whether a line opens or closes a code block.
func isFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func (p *parser) parseCode(indent int) *Node {
	trimmed := strings.TrimSpace(p.lines[p.line-1])
	line := p.line
	marker := trimmed[:3]
	language := strings.TrimSpace(trimmed[3:])
	language = strings.Trim(language, "{}")
	p.line++

	body := []string{}
	for p.line <= len(p.lines) {
		current := strings.TrimSpace(p.lines[p.line-1])
		if strings.HasPrefix(current, marker) {
			p.line++
			break
		}
		body = append(body, p.lines[p.line-1])
		p.line++
	}
	return &Node{Kind: BlockCode, Lang: language, Line: line, Children: []*Node{{Kind: BlockHTML, Text: strings.Join(body, "\n")}}}
}

/* -------------------------------------------------------------------------- */
/*  Lists                                                                     */
/* -------------------------------------------------------------------------- */

// listMarker returns the marker of a list item, or "" when the line is not one.
func listMarker(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
		return "-"
	}
	if strings.HasPrefix(trimmed, "* ") || trimmed == "*" {
		return "*"
	}
	if strings.HasPrefix(trimmed, "+ ") || trimmed == "+" {
		return "+"
	}
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits+1 >= len(trimmed) {
		return ""
	}
	// The marker carries its separator, because the item body is everything
	// after it: a marker of "1" would leave the "." in the item.
	if trimmed[digits] == '.' && (trimmed[digits+1] == ' ' || digits+1 == len(trimmed)) {
		return trimmed[:digits+1]
	}
	if trimmed[digits] == ')' && (trimmed[digits+1] == ' ' || digits+1 == len(trimmed)) {
		return trimmed[:digits+1]
	}
	return ""
}

func (p *parser) parseList(indent int) *Node {
	line := p.line
	ordered := false
	if marker := listMarker(strings.TrimSpace(p.lines[p.line-1])); marker != "" && marker[0] >= '0' && marker[0] <= '9' {
		ordered = true
	}
	list := &Node{Kind: BlockList, Ordered: ordered, Line: line}
	for p.line <= len(p.lines) {
		raw := p.lines[p.line-1]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// A blank line ends the list unless the next line continues it.
			if p.line+1 > len(p.lines) {
				break
			}
			next := strings.TrimSpace(p.lines[p.line])
			if listMarker(next) == "" && !isIndented(p.lines[p.line]) {
				break
			}
			p.line++
			continue
		}
		marker := listMarker(trimmed)
		if marker == "" {
			break
		}
		markerOrdered := marker[0] >= '0' && marker[0] <= '9'
		if markerOrdered != ordered {
			break
		}
		itemLine := p.line
		itemIndent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		// The item body is the rest of the marker line plus every following
		// line indented past the marker.
		body := []string{trimmed[len(marker):]}
		p.line++
		for p.line <= len(p.lines) {
			current := p.lines[p.line-1]
			if strings.TrimSpace(current) == "" {
				break
			}
			if len(current)-len(strings.TrimLeft(current, " \t")) <= itemIndent {
				break
			}
			body = append(body, current)
			p.line++
		}
		inner := &parser{path: p.path, lines: dedent(body, 0), line: 1, targets: map[string]bool{}}
		list.Children = append(list.Children, &Node{
			Kind:     BlockListItem,
			Line:     itemLine,
			Children: inner.parseBlocks(0),
		})
	}
	return list
}

func isIndented(line string) bool {
	return len(line)-len(strings.TrimLeft(line, " \t")) >= 2
}

/* -------------------------------------------------------------------------- */
/*  Quotes, tables, paragraphs                                                 */
/* -------------------------------------------------------------------------- */

func isQuote(line string) bool { return strings.HasPrefix(line, ">") }

func (p *parser) parseQuote(indent int) *Node {
	line := p.line
	body := []string{}
	for p.line <= len(p.lines) {
		current := strings.TrimSpace(p.lines[p.line-1])
		if !isQuote(current) {
			break
		}
		body = append(body, strings.TrimSpace(strings.TrimPrefix(current, ">")))
		p.line++
	}
	inner := &parser{path: p.path, lines: body, line: 1, targets: map[string]bool{}}
	return &Node{Kind: BlockQuote, Line: line, Children: inner.parseBlocks(0)}
}

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

func isTableDelimiter(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}
	for _, r := range trimmed {
		switch r {
		case '|', '-', ':', ' ':
		default:
			return false
		}
	}
	return strings.Contains(trimmed, "-")
}

// nextLineIsTableDelimiter reports whether the line after the current one is a
// table's delimiter, which is what makes the current row a header.
func (p *parser) nextLineIsTableDelimiter() bool {
	return p.line < len(p.lines) && isTableDelimiter(strings.TrimSpace(p.lines[p.line]))
}

// parseTable reads a table. The caller is on the header row: the delimiter
// below it is skipped, and every following row is read until the rows stop.
func (p *parser) parseTable(indent int) *Node {
	header := p.lines[p.line-1]
	line := p.line
	p.line += 2

	table := &Node{Kind: BlockTable, Line: line}
	table.Children = append(table.Children, p.tableRow(header, true))
	for p.line <= len(p.lines) {
		current := p.lines[p.line-1]
		if !isTableRow(current) {
			break
		}
		table.Children = append(table.Children, p.tableRow(current, false))
		p.line++
	}
	return table
}

func (p *parser) tableRow(line string, header bool) *Node {
	row := &Node{Kind: BlockRow, Line: p.line, Text: boolText(header)}
	for _, cell := range splitRow(line) {
		row.Children = append(row.Children, &Node{
			Kind:     BlockCell,
			Children: parseInline(cell),
		})
	}
	return row
}

// boolText names the row's role in the text field, so a node stays one struct
// rather than a row becoming a second kind the renderer has to know.
func boolText(value bool) string {
	if value {
		return "header"
	}
	return ""
}

// splitRow splits a table row into cells, dropping the outer pipes.
func splitRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	cells := []string{}
	current := strings.Builder{}
	for _, r := range trimmed {
		if r == '\\' {
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	cells = append(cells, strings.TrimSpace(current.String()))
	return cells
}

func (p *parser) parseParagraph(indent int) *Node {
	line := p.line
	body := []string{}
	for p.line <= len(p.lines) {
		current := p.lines[p.line-1]
		trimmed := strings.TrimSpace(current)
		if trimmed == "" || isHeading(trimmed) || isFence(trimmed) || isQuote(trimmed) ||
			isDirectiveOpen(trimmed) || listMarker(trimmed) != "" {
			break
		}
		// A row whose next line is a delimiter is a table's header, so the
		// paragraph stops there and the block reader reads the table.
		if isTableRow(trimmed) && p.nextLineIsTableDelimiter() {
			break
		}
		body = append(body, trimmed)
		p.line++
	}
	if len(body) == 0 {
		p.line++
		return nil
	}
	return &Node{
		Kind:     BlockParagraph,
		Line:     line,
		Children: parseInline(strings.Join(body, "\n")),
	}
}

/* -------------------------------------------------------------------------- */
/*  Inline spans                                                              */
/* -------------------------------------------------------------------------- */

// parseInline reads the inline spans of one paragraph of text.
//
// The order matters: code spans win over everything, because a backtick inside
// code is text, and roles are read before emphasis, because a role is written
// with the same brackets an author uses for emphasis.
func parseInline(text string) []*Node {
	nodes := []*Node{}
	rest := text
	for len(rest) > 0 {
		// A role is read before a code span, because the classic spelling
		// writes its label in backticks: `:ref:`target``. Read as code first,
		// the label would swallow the target and leave ":ref:" as prose.
		if node, consumed, ok := parseRole(rest); ok {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		if node, consumed, ok := parseCodeSpan(rest); ok {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		if node, consumed, ok := parseLink(rest); ok {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		if node, consumed, ok := parseImage(rest); ok {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		if node, consumed, ok := parseEmphasis(rest); ok {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		if node, consumed := parseHardBreak(rest); consumed > 0 {
			nodes = append(nodes, node)
			rest = rest[consumed:]
			continue
		}
		// One run of ordinary text, up to the next construct.
		next := nextConstruct(rest)
		if next < 0 {
			nodes = append(nodes, &Node{Kind: InlineText, Text: rest})
			break
		}
		if next > 0 {
			nodes = append(nodes, &Node{Kind: InlineText, Text: rest[:next]})
			rest = rest[next:]
			continue
		}
		// A marker character that no construct claimed. It is kept as text and
		// the scan moves past it: one stray character must not stop the rest of
		// the paragraph from being read.
		_, size := utf8.DecodeRuneInString(rest)
		if size <= 0 {
			break
		}
		nodes = append(nodes, &Node{Kind: InlineText, Text: rest[:size]})
		rest = rest[size:]
	}
	return nodes
}

// nextConstruct is the offset of the first character that starts an inline
// construct, or -1 when the text holds none.
func nextConstruct(text string) int {
	candidates := []int{}
	for _, marker := range []string{"`", "[", "!", "*", "_", "{", "}", "\n"} {
		if index := strings.Index(text, marker); index >= 0 {
			candidates = append(candidates, index)
		}
	}
	// A role is a construct too, and it opens with a brace or a colon. Without
	// this the scan would stop at the backtick of a classic role, and the label
	// would be read as code before the role could claim it.
	if index := roleStart(text); index >= 0 {
		candidates = append(candidates, index)
	}
	smallest := -1
	for _, index := range candidates {
		if smallest < 0 || index < smallest {
			smallest = index
		}
	}
	return smallest
}

// roleStart is the offset of an opening role, or -1 when the text opens none.
//
//	{role}`label`     the MyST spelling
//	:role:`label`     the classic spelling
func roleStart(text string) int {
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '{', ':':
		default:
			continue
		}
		opening := roleNameAt(text, index)
		if opening == "" {
			continue
		}
		// The label has to be there: an opening that is not closed is text.
		if strings.HasPrefix(text[index+len(opening):], "`") {
			return index
		}
	}
	return -1
}

// roleNameAt reads the role name opening at an offset, returning "" when the
// shape is not a role.
func roleNameAt(text string, index int) string {
	closer := byte('}')
	if text[index] == ':' {
		closer = ':'
	}
	length := 0
	for index+1+length < len(text) && isRoleNameChar(text[index+1+length]) {
		length++
	}
	if length == 0 || index+1+length >= len(text) || text[index+1+length] != closer {
		return ""
	}
	return text[index : index+1+length+1]
}

func isRoleNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '_'
}

func parseCodeSpan(text string) (*Node, int, bool) {
	if !strings.HasPrefix(text, "`") {
		return nil, 0, false
	}
	closing := strings.Index(text[1:], "`")
	if closing < 0 {
		return nil, 0, false
	}
	body := text[1 : 1+closing]
	// A double backtick carries a code span that itself holds a backtick.
	if strings.HasPrefix(body, "`") && strings.HasSuffix(body, "`") && len(body) > 1 {
		body = body[1 : len(body)-1]
	}
	return &Node{Kind: InlineCode, Text: body}, closing + 2, true
}

func parseLink(text string) (*Node, int, bool) {
	if !strings.HasPrefix(text, "[") {
		return nil, 0, false
	}
	label, rest, ok := readBracket(text)
	if !ok {
		return nil, 0, false
	}
	if !strings.HasPrefix(rest, "(") {
		return nil, 0, false
	}
	closing := strings.Index(rest, ")")
	if closing < 0 {
		return nil, 0, false
	}
	href := strings.TrimSpace(rest[1:closing])
	// A title is accepted and dropped: it carries nothing a page renders.
	if space := strings.IndexAny(href, " \t"); space > 0 {
		href = href[:space]
	}
	node := &Node{Kind: InlineLink, Href: href, Children: parseInline(label)}
	return node, len(text) - len(rest) + closing + 1, true
}

func parseImage(text string) (*Node, int, bool) {
	if !strings.HasPrefix(text, "![") {
		return nil, 0, false
	}
	alt, rest, ok := readBracket(text[1:])
	if !ok {
		return nil, 0, false
	}
	if !strings.HasPrefix(rest, "(") {
		return nil, 0, false
	}
	closing := strings.Index(rest, ")")
	if closing < 0 {
		return nil, 0, false
	}
	href := strings.TrimSpace(rest[1:closing])
	if space := strings.IndexAny(href, " \t"); space > 0 {
		href = href[:space]
	}
	return &Node{Kind: InlineLink, Href: href, Text: alt}, len(text) - len(rest) + closing + 1, true
}

// readBracket reads a bracketed run and returns what is inside it, with the
// text that follows the closing bracket.
func readBracket(text string) (inner string, rest string, ok bool) {
	depth := 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return text[1:index], text[index+1:], true
			}
		}
	}
	return "", text, false
}

// parseRole reads a cross-reference role in either spelling:
//
//	[text]{ref}`label`      the MyST spelling
//	:ref:`label`            the classic spelling
func parseRole(text string) (*Node, int, bool) {
	if strings.HasPrefix(text, ":") {
		closing := strings.Index(text, "`")
		if closing < 0 {
			return nil, 0, false
		}
		name := strings.TrimSuffix(text[1:closing], ":")
		if name == "" {
			return nil, 0, false
		}
		label := text[closing+1:]
		end := strings.Index(label, "`")
		if end < 0 {
			return nil, 0, false
		}
		target := label[:end]
		text := target
		if space := strings.Index(target, "<"); space > 0 && strings.HasSuffix(target, ">") {
			text = strings.TrimSpace(target[:space])
			target = target[space+1 : len(target)-1]
		}
		return &Node{Kind: InlineRole, Role: name, Text: text, Href: target}, closing + 1 + end + 1, true
	}

	if strings.HasPrefix(text, "{") {
		closing := strings.Index(text, "}")
		if closing < 0 {
			return nil, 0, false
		}
		name := text[1:closing]
		if !strings.HasPrefix(text[closing+1:], "`") {
			return nil, 0, false
		}
		label := text[closing+2:]
		end := strings.Index(label, "`")
		if end < 0 {
			return nil, 0, false
		}
		target := label[:end]
		// A role may carry an explicit target: {ref}`label <target>`.
		if space := strings.Index(target, "<"); space > 0 && strings.HasSuffix(target, ">") {
			label := strings.TrimSpace(target[:space])
			target = target[space+1 : len(target)-1]
			return &Node{Kind: InlineRole, Role: name, Text: label, Href: target}, closing + 2 + end + 1, true
		}
		return &Node{Kind: InlineRole, Role: name, Text: target, Href: target}, closing + 2 + end + 1, true
	}
	return nil, 0, false
}

func parseEmphasis(text string) (*Node, int, bool) {
	for _, marker := range []string{"**", "*", "__", "_"} {
		if !strings.HasPrefix(text, marker) {
			continue
		}
		closing := strings.Index(text[len(marker):], marker)
		if closing <= 0 {
			continue
		}
		body := text[len(marker) : len(marker)+closing]
		kind := InlineEmphasis
		if len(marker) == 2 {
			kind = InlineStrong
		}
		return &Node{Kind: kind, Children: parseInline(body)}, len(marker) + closing + len(marker), true
	}
	return nil, 0, false
}

func parseHardBreak(text string) (*Node, int) {
	if !strings.HasPrefix(text, "  \n") {
		return nil, 0
	}
	return &Node{Kind: InlineBreak}, 3
}

/* -------------------------------------------------------------------------- */
/*  Headings and diagnostics                                                  */
/* -------------------------------------------------------------------------- */

// collectHeadings fills the section list and the document title. The title is
// the first heading of level one, or the first heading at all, because a page
// may open with a level-two heading under a title it declares elsewhere.
func (d *Document) collectHeadings() {
	for _, block := range d.Blocks {
		d.collectFrom(block)
	}
	if d.Title == "" {
		for _, section := range d.Sections {
			if section.Level == 1 {
				d.Title = section.Title
				break
			}
		}
	}
	if d.Title == "" && len(d.Sections) > 0 {
		d.Title = d.Sections[0].Title
	}
}

func (d *Document) collectFrom(block *Node) {
	if block == nil {
		return
	}
	if block.Kind == BlockHeading {
		d.Sections = append(d.Sections, Section{
			ID:       block.ID,
			Title:    block.Text,
			Level:    block.Level,
			Line:     block.Line,
			Explicit: block.Target != "",
		})
	}
	for _, child := range block.Children {
		d.collectFrom(child)
	}
}

// diagnose reports the problems worth telling an author about: a page that
// declares no title, and a heading target written more than once.
func (d *Document) diagnose() {
	if d.Title == "" {
		d.Diagnostics = append(d.Diagnostics, Diagnostic{Path: d.Path, Line: 1, Message: "the page has no heading, so it has no title"})
	}
}

// warn records a problem found while parsing.
func (p *parser) warn(line int, format string, args ...any) {
	p.diagnostics = append(p.diagnostics, Diagnostic{Path: p.path, Line: line, Message: fmt.Sprintf(format, args...)})
}
