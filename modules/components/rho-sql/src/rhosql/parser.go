//
// @file parser.go
// @brief The reader a service's SQL is read with.
//
// A parser, not a pattern. The difference shows up the first time a service
// writes a string containing a semicolon, or a column named `order`, or a
// default of `now()`: a pattern reads those as structure, and a service whose
// schema parses into a different shape than it was written is a service whose
// generated code is wrong in a way nobody can see from the SQL.
//
// What is here is deliberately small. A schema is a list of `CREATE TABLE`
// statements, a query file is a list of commented statements, and both are
// separated by semicolons that are not inside a string. Nothing more is
// claimed, and a construct that is not claimed is reported with its line rather
// than guessed at.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import "strings"

// parser reads one file.
type parser struct {
	name  string
	lines []string
	// at is the line being read, counted from one, which is what a diagnostic
	// names because a line is what a person can go and look at.
	at      int
	pos     int
	last    string
	diags   []Diagnostic
	stopped bool
}

// newParser prepares a file for reading.
func newParser(name, text string) *parser {
	return &parser{
		name:  name,
		lines: strings.Split(text, "\n"),
		pos:   0,
	}
}

// atEnd reports whether there is nothing left to read.
func (p *parser) atEnd() bool { return p.pos >= len(p.lines) }

// line returns the line being read.
func (p *parser) line() string {
	if p.atEnd() {
		return ""
	}
	return p.lines[p.pos]
}

// report records a problem against the line being read.
func (p *parser) report(level, message string) {
	p.diags = append(p.diags, Diagnostic{
		Level:   level,
		File:    p.name,
		Line:    p.pos + 1,
		Message: message,
	})
}

// skipSeparators steps over the blank lines and the comments between
// statements, keeping the last comment seen: it is where a statement's name and
// its prose come from.
func (p *parser) skipSeparators() {
	for !p.atEnd() {
		trimmed := strings.TrimSpace(p.line())
		switch {
		case trimmed == "":
			p.pos++
		case strings.HasPrefix(trimmed, "--"):
			p.last = commentText(trimmed)
			p.pos++
		default:
			return
		}
	}
}

// lastComment is the prose the last comment block carried.
func (p *parser) lastComment() string { return p.last }

// commentText is a comment's text with its dashes removed.
func commentText(line string) string {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "--"))
	return text
}

// namedStatement reads the comment block that introduces a statement and
// returns the name it declares and the prose above it.
//
// The convention is one the tree already uses for hook declarations:
//
//	-- name: find_customer
//	-- Returns the customer at this address.
//
// A comment with a `name:` opens a declaration; the lines above it are its
// description, and the lines below it before the next `name:` are the next
// statement's. That is what makes a query file readable as prose with the
// statements named inside it.
// namedStatement reads the comment block that introduces a statement and
// returns the name it declares and the prose that describes it.
//
// The convention is one the tree already uses for hook declarations:
//
//	-- name: find_customer
//	-- Returns the customer at this address.
//
//	-- The address is compared as written: a collation is an engine's
//	-- decision.
//	SELECT id, name FROM customers WHERE address = :address;
//
// The prose may be above the name or below it, because both are how a person
// writes one: the name first when the description is a heading, the description
// first when it is a sentence. Both are collected, and the blank line that ends
// the block ends the prose — so the block of the *next* statement, which begins
// after that blank line, is not swallowed into this one's description.
func (p *parser) namedStatement() (string, string) {
	// A run of comments before anything has been named is the file talking about
	// itself — the block a project opens a queries file with. It is kept as the
	// file's description and not handed to the first statement, because a reader
	// of a generated function should find that statement's description there and
	// not the file's header.
	prose := []string{}
	name := ""
	// A blank line before any name separates the file's header from the first
	// statement; a blank line after a name separates two statements. `sawName`
	// records which of the two a blank line is, which is the only difference
	// between them.
	sawName := false
	for !p.atEnd() {
		trimmed := strings.TrimSpace(p.line())
		if trimmed == "" {
			if !sawName && len(prose) > 0 {
				p.last = strings.Join(prose, "\n")
				prose = nil
			}
			p.pos++
			continue
		}
		if !strings.HasPrefix(trimmed, "--") {
			break
		}
		text := commentText(trimmed)
		if rest, found := cutPrefix(text, "name:"); found {
			name = strings.TrimSpace(rest)
			// What came before the first name is the file's header, and a
			// reader of the first function should not find it there.
			if !sawName && len(prose) > 0 {
				p.last = strings.Join(prose, "\n")
				prose = nil
			}
			sawName = true
			p.pos++
			continue
		}
		prose = append(prose, text)
		p.pos++
	}
	p.last = strings.Join(prose, "\n")
	return name, p.last
}

// readStatement reads the text of one statement, up to the semicolon that ends
// it, and steps past it.
func (p *parser) readStatement() string {
	var b strings.Builder
	for !p.atEnd() {
		line := p.line()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			// A comment inside a statement does not end it and is not part of
			// it: the text handed to an engine has to be runnable.
			p.pos++
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
		p.pos++
		if endsStatement(trimmed) {
			break
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.String()), ";"))
}

// endsStatement reports whether a line ends a statement, which it does at a
// semicolon that is not inside a string.
func endsStatement(line string) bool {
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'', '"', '`':
			i = skipLiteral(runes, i)
		case ';':
			return true
		}
	}
	return false
}

// skipStatement steps over a statement without reading it, which is what a
// parser does with a construct it has already reported: stopping there would
// report every line of it as well.
func (p *parser) skipStatement() {
	for !p.atEnd() {
		line := strings.TrimSpace(p.line())
		p.pos++
		if endsStatement(line) {
			return
		}
	}
}

// describe names what the parser is looking at, for a message.
func (p *parser) describe() string {
	if p.atEnd() {
		return "the end of the file"
	}
	trimmed := strings.TrimSpace(p.line())
	if trimmed == "" {
		return "a blank line"
	}
	if len(trimmed) > 30 {
		trimmed = trimmed[:30] + "..."
	}
	return "`" + trimmed + "`"
}

// peekKeyword reports whether the line being read begins with a word.
func (p *parser) peekKeyword(word string) bool {
	fields := strings.Fields(p.line())
	return len(fields) > 0 && strings.EqualFold(fields[0], word)
}

// peekAnyKeyword reports whether the line opens with one of these words.
//
// It is how a schema's statements that define no column are recognised in one
// place, rather than a list of comparisons growing in the reader.
func (p *parser) peekAnyKeyword(words []string) bool {
	fields := strings.Fields(p.line())
	if len(fields) == 0 {
		return false
	}
	for _, word := range words {
		if strings.EqualFold(fields[0], word) {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Table declarations                                                          */
/* -------------------------------------------------------------------------- */

// parseTable reads one `CREATE TABLE` declaration.
//
// The shape is the one every engine agrees on: a name, a parenthesised list of
// column definitions, and optionally the table constraints. What is inside a
// type is carried as written, because the spelling is the engine's to interpret
// and this shape has no business deciding it.
func (p *parser) parseTable() (Table, bool) {
	table := Table{Columns: []Column{}}
	fields := strings.Fields(p.line())
	if len(fields) < 3 || !strings.EqualFold(fields[1], "TABLE") {
		return table, false
	}
	// CREATE TABLE [IF NOT EXISTS] name (...)
	for i := 2; i < len(fields); i++ {
		if strings.EqualFold(fields[i], "if") && i+2 < len(fields) &&
			strings.EqualFold(fields[i+1], "not") && strings.EqualFold(fields[i+2], "exists") {
			table.IfNotExists = true
			i += 2
			continue
		}
		table.Name = unquote(fields[i])
		break
	}
	if table.Name == "" {
		return table, false
	}
	comment := p.tableComment()
	table.Comment = strings.Join(comment, " ")

	body, tail, ok := p.readParenthesis()
	if !ok {
		p.report("error", "the table "+table.Name+" has no column list")
		return table, false
	}
	// Whatever follows the closing bracket is a table constraint rather than a
	// column, and the one that matters is the key.
	if tail == "" {
		// A constraint may be written on the line after the bracket, which is
		// where a long column list usually ends.
		tail = p.readContinuation()
	}
	if key, found := primaryKeyIn(tail); found {
		table.PrimaryKey = key
	}
	table.Columns = p.parseColumns(body, &table)
	return table, true
}

// cutPrefix is strings.CutPrefix, spelled here because a component builds
// with Go 1.18, where that function arrived later.
func cutPrefix(text, prefix string) (string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return text[len(prefix):], true
}

// tableComment is the prose written above a table, which is what a reader of the
// generated code is looking for.
func (p *parser) tableComment() []string {
	out := []string{}
	// The comments were stepped over to reach this line, so they are read back
	// off the file rather than remembered: a schema's prose sits above the
	// declaration and is separated from it by nothing.
	for i := p.pos - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(p.lines[i])
		if trimmed == "" {
			if len(out) == 0 {
				continue
			}
			break
		}
		if !strings.HasPrefix(trimmed, "--") {
			break
		}
		out = append([]string{commentText(trimmed)}, out...)
	}
	return out
}

// readParenthesis reads the parenthesised body of a declaration, spanning lines
// until the brackets balance, and steps past the closing one. It returns the
// body and whatever the line carrying the closing bracket said after it, which
// is where a trailing table constraint is written.
//
// It starts one bracket deep, because the caller has already read the opening
// bracket on the line it was reading when it called. Starting at zero would
// balance against the first nested pair it met — the `(10, 2)` of a numeric
// type — and return half a column list. The line it starts on is the one
// carrying that opening bracket, so its brackets are not counted again.
func (p *parser) readParenthesis() (string, string, bool) {
	depth := 1
	var b strings.Builder
	first := true
	for !p.atEnd() {
		line := p.line()
		counted := line
		if first {
			counted = strings.TrimSpace(line[strings.LastIndex(line, "(")+1:])
			first = false
		}
		for _, r := range counted {
			switch r {
			case '(':
				depth++
			case ')':
				depth--
			}
		}
		if b.Len() > 0 {
			// Lines are kept apart rather than folded into one. A comment ends
			// at the end of its line, so a body whose lines were folded would
			// have every comment in it run to the end of the column list.
			b.WriteString("\n")
		}
		b.WriteString(line)
		p.pos++
		if depth <= 0 {
			// Everything up to the closing bracket is the body; the bracket is
			// not, and what the same line says after it is a constraint.
			return strings.TrimSpace(trimParens(b.String())), tailAfterParen(line), true
		}
	}
	return "", "", false
}

// trimParens removes the bracket that opened the declaration and the one that
// closed it.
//
// The first bracket is the declaration's own, on the line that opened it, so it
// is the one that is trimmed; the rest of the line after it is already inside
// the body. A type's own brackets are left alone, which is why the type of a
// column is the whole of `numeric(10, 2)` rather than half of it.
func trimParens(text string) string {
	open := strings.Index(text, "(")
	if open < 0 {
		return text
	}
	runes := []rune(text[open+1:])
	depth := 0
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'':
			i = skipLiteral(runes, i)
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return strings.TrimSpace(string(runes[:i]))
			}
			depth--
		}
	}
	return strings.TrimSpace(string(runes))
}

// readContinuation reads a statement's next line when the line before it ended
// the declaration's brackets without ending the statement.
//
// A table constraint may be written on the line after its column list, which is
// what happens when the list is long enough to be worth spreading. Reading one
// line is enough: a constraint longer than that is a constraint this shape does
// not claim to read, and a second one would be the next statement's.
func (p *parser) readContinuation() string {
	if p.atEnd() {
		return ""
	}
	line := strings.TrimSpace(p.line())
	if line == "" || strings.HasPrefix(line, "--") {
		return ""
	}
	p.pos++
	return line
}

// tailAfterParen is what a line says after the bracket that closes a
// declaration, which is a constraint or the end of the statement.
func tailAfterParen(line string) string {
	at := strings.LastIndex(line, ")")
	if at < 0 {
		return ""
	}
	return strings.TrimSpace(line[at+1:])
}

// primaryKeyIn reads the key from a table's trailing constraints.
func primaryKeyIn(tail string) ([]string, bool) {
	upper := strings.ToUpper(tail)
	at := strings.Index(upper, "PRIMARY KEY")
	if at < 0 {
		return nil, false
	}
	rest := tail[at+len("PRIMARY KEY"):]
	open := strings.Index(rest, "(")
	close := strings.Index(rest, ")")
	if open < 0 || close <= open {
		return nil, false
	}
	return splitList(rest[open+1 : close]), true
}

// parseColumns reads a column list.
//
// A comma inside brackets does not end a column, which is what lets a type be
// written as `numeric(10, 2)` rather than being two columns, and a `--` comment
// is text about the column rather than part of it, which is why it is taken off
// each entry after the split rather than being read as structure.
func (p *parser) parseColumns(body string, table *Table) []Column {
	out := []Column{}
	for _, part := range splitTopLevel(body, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// The comment comes off the definition, and it documents the column the
		// service wrote it beside.
		definition, comment := splitComment(part)
		part = strings.TrimSpace(definition)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		// The line a declaration ends on carries its closing bracket and
		// whatever followed it. A fragment that is only that is the end of the
		// column list, not a column of it.
		if strings.HasPrefix(part, ")") {
			continue
		}
		upper := strings.ToUpper(part)
		// A table constraint is not a column, and a column whose type is
		// written as one is.
		if strings.HasPrefix(upper, "PRIMARY KEY") ||
			strings.HasPrefix(upper, "FOREIGN KEY") ||
			strings.HasPrefix(upper, "UNIQUE") ||
			strings.HasPrefix(upper, "CHECK") ||
			strings.HasPrefix(upper, "CONSTRAINT") {
			if key, found := primaryKeyIn(part); found {
				table.PrimaryKey = key
			}
			continue
		}
		column := Column{Name: unquote(fields[0])}
		if len(fields) > 1 {
			column.Type = columnType(part[len(fields[0]):])
		}
		column.NotNull = containsWord(upper, "NOT NULL")
		// An inline `PRIMARY KEY` comes after the type, as in `id integer PRIMARY
		// KEY`, so it is found anywhere in the definition rather than only at the
		// front.
		column.PrimaryKey = containsWord(upper, "PRIMARY KEY")
		column.Unique = strings.Contains(upper, "UNIQUE")
		// A default is read from the text as written, not from the upper case
		// used for matching: a string default is a value, and upper-casing a
		// value changes it.
		if _, rest, found := cutWord(part, "DEFAULT"); found {
			column.Default = strings.TrimSpace(defaultText(rest))
		}
		// The comment is the column's description, and it is what a reader of
		// the generated struct is looking for.
		column.Comment = comment
		out = append(out, column)
	}
	return out
}

// columnType is a column's type as written.
//
// A type may contain a space — `numeric(10, 2)`, `double precision` — so it is
// read as the run of words up to the first constraint word rather than to the
// first space. A type is what a service wrote and an engine interprets, so it is
// kept whole.
func columnType(text string) string {
	words := strings.Fields(text)
	end := len(words)
	for i, word := range words {
		if isTypeConstraint(strings.ToUpper(word)) {
			end = i
			break
		}
	}
	return strings.Join(words[:end], " ")
}

// isTypeConstraint reports whether a word in a column definition is a
// constraint rather than part of the type.
func isTypeConstraint(word string) bool {
	switch word {
	case "NOT", "NULL", "PRIMARY", "KEY", "UNIQUE", "DEFAULT", "CHECK", "REFERENCES",
		"COLLATE", "AUTOINCREMENT", "GENERATED", "AS", "ON", "UPDATE", "DEFERRABLE",
		"INITIALLY":
		return true
	}
	return false
}

// splitTopLevel splits on a separator that is not inside brackets or a string.
//
// A `--` comment is kept and travels with the entry it follows. SQL writes a
// column's comment after its comma, so `id integer, -- the id` puts the comment
// between two separators and it documents the piece that was just finished. The
// split therefore does not close a piece at a comma that is followed only by a
// comment: the comment is read first, attached back to the piece it documents,
// and the split happens at the next real separator.
//
// A comma inside brackets does not split, which is what lets a type be written
// as `numeric(10, 2)` rather than being two columns.
func splitTopLevel(text string, separator rune) []string {
	out := []string{}
	depth := 0
	current := strings.Builder{}
	leading := strings.Builder{}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'', '"', '`':
			end := skipLiteral(runes, i)
			current.WriteString(string(runes[i : end+1]))
			i = end
			continue
		case '(':
			depth++
		case ')':
			depth--
		case '-':
			// A `--` is a comment unless it is part of a value, which a lone
			// minus is not: `-1` is a number, `--` is prose.
			if i+1 < len(runes) && runes[i+1] == '-' && depth == 0 {
				start := i
				end := i
				for end < len(runes) && runes[end] != '\n' {
					end++
				}
				comment := strings.TrimSpace(string(runes[start:end]))
				// Which column a comment documents is decided by where it is
				// written, and SQL writes it both ways: beside the column it
				// describes, on that column's own line after its comma, or above
				// the column when the description does not fit beside it. So a
				// comment that starts its own line belongs to the column that
				// follows, and one that shares a line with what came before
				// belongs to that.
				// A comment is on its own line when nothing but whitespace
				// precedes it back to the start of its line, and an inline
				// comment after a comma has `PRIMARY KEY,` in front of it — so
				// it documents the column it was written beside, while one on
				// its own line documents the column that follows it.
				ownLine := true
				for i := start - 1; i >= 0; i-- {
					if runes[i] == '\n' {
						break
					}
					if runes[i] != ' ' && runes[i] != '\t' {
						ownLine = false
						break
					}
				}
				switch {
				case strings.TrimSpace(current.String()) != "":
					// On the same line as what has been read, it documents it.
					current.WriteString(" " + comment)
					out = append(out, leading.String()+current.String())
					leading.Reset()
					current.Reset()
				case ownLine && strings.TrimSpace(leading.String()) == "":
					// On its own line after a column was closed: it documents
					// the column that follows, which is the one a service writes
					// when a description is too long to sit beside the type.
					leading.WriteString(comment + "\n")
				case len(out) > 0:
					// After a comma on the same line, it documents the column
					// that was just closed.
					out[len(out)-1] += " " + comment
				default:
					leading.WriteString(comment + "\n")
				}
				i = end - 1
				continue
			}
		}
		if runes[i] == separator && depth == 0 {
			if strings.TrimSpace(current.String()) != "" {
				out = append(out, leading.String()+current.String())
				leading.Reset()
			}
			current.Reset()
			continue
		}
		current.WriteRune(runes[i])
	}
	if strings.TrimSpace(current.String()) != "" {
		out = append(out, leading.String()+current.String())
	}
	return out
}

// containsWord reports whether a word appears in a statement, matched on
// boundaries so a column named `nullable` is not `NULL`.
func containsWord(text, word string) bool {
	_, _, found := cutWord(text, word)
	return found
}

// cutWord finds a word in a statement and returns what is around it.
func cutWord(text, word string) (before string, after string, found bool) {
	at := 0
	for {
		index := strings.Index(text[at:], word)
		if index < 0 {
			return text, "", false
		}
		start := at + index
		end := start + len(word)
		before, after = text[:start], text[end:]
		// A word is a word when it is not inside a longer one.
		if (start == 0 || !isNameByte(rune(text[start-1]))) &&
			(end == len(text) || !isNameByte(rune(text[end]))) {
			return before, after, true
		}
		at = end
	}
}

// splitComment separates a column definition from the comments around it.
//
// A comment may be on the line above the column, on the same line after it, or
// both, which is how a service writes a column whose description does not fit
// beside it. Both are returned, joined, because both document the same thing.
func splitComment(text string) (definition string, comment string) {
	runes := []rune(text)
	lines := []string{}
	comments := []string{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			comments = append(comments, commentText(trimmed))
			continue
		}
		// A comment on the same line as the definition, after it.
		if at := strings.Index(line, "--"); at >= 0 {
			// Only when it is not inside a string, which a type or a default
			// with a `--` inside it would otherwise trip over.
			if !insideString(runes[:len([]rune(line[:at]))]) {
				comments = append(comments, commentText(line[at:]))
				line = line[:at]
			}
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.TrimSpace(strings.Join(lines, " ")), strings.Join(comments, " ")
}

// insideString reports whether a prefix of a line is inside an unclosed string
// literal, which is what a `--` in a default like `'a--b'` looks like.
func insideString(prefix []rune) bool {
	quote := rune(0)
	for _, r := range prefix {
		if quote == 0 {
			switch r {
			case '\'', '"', '`':
				quote = r
			}
			continue
		}
		if r == quote {
			quote = 0
		}
	}
	return quote != 0
}

// defaultText is a column's default as written, up to the comma that ends the
// column or the constraint that follows it.
//
// A default may be a number, a quoted string, or an expression — `now()`,
// `(0 + 1)` — so it is read as text and stopped at the first comma that is not
// inside brackets or a string. The whole of `'a;b;c'` is one default; splitting
// it at the semicolon would be splitting a string.
func defaultText(text string) string {
	runes := []rune(text)
	depth := 0
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'':
			i = skipLiteral(runes, i)
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(string(runes[:i]))
			}
		}
	}
	return strings.TrimSpace(text)
}
