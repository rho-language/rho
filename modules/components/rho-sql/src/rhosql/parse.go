//
// @file parse.go
// @brief Reading a service's SQL into the shape engines are given.
//
// Two files, and they are read differently because they mean different things.
// `schema.sql` is a list of tables, and what it says is structural. `queries.sql`
// is a list of named statements, and what it says is behaviour: a comment, a
// name, and a statement.
//
// The lexer is a real one rather than a set of patterns over the text, because a
// SQL string may contain the word `select`, or a `;`, or a brace, and a pattern
// cannot tell those apart from the grammar. The parser is a recursive descent
// over the tokens, and it reports the line every problem is on, because a
// service that cannot be read is a service nobody can fix without being told
// where to look.
//
// What is not parsed is the expression language: a `where` clause is carried as
// the text the service wrote, and only the parameters inside it are read. That is
// a decision about who knows what. An engine is the thing that knows how to plan
// a predicate, and a shape that re-derived one would be a second, worse
// implementation of the same thing.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhosql

import (
	"strconv"
	"strings"
)

// ParseSchema reads a schema file.
//
// A table declaration is what a schema is read for: it is the only statement
// that says what a column is called and what it holds, which is what a result is
// typed from.
//
// A schema also holds statements that define no shape — an index over a column,
// a row of seed data, a pragma an engine wants at start-up. Those are passed
// over rather than refused, because the whole file is what an engine is handed
// and a parser that rejected them would make a schema that cannot be indexed
// the only schema this command reads. What is not passed over is a statement
// that neither says what a table is nor is one of the statements a schema is
// known to hold: that is a mistake, and reporting it is how it is found.
func ParseSchema(name, text string) (Schema, []Diagnostic) {
	schema := Schema{Version: Version, Tables: []Table{}}
	parser := newParser(name, text)
	for !parser.atEnd() {
		parser.skipSeparators()
		if parser.atEnd() {
			break
		}
		if parser.peekKeyword("create") {
			table, ok := parser.parseTable()
			if !ok {
				// A `CREATE` that is not a table — an index, a view, a trigger —
				// defines no column, so there is nothing here to type a result
				// from. It is stepped over so the reading moves on.
				parser.skipStatement()
				continue
			}
			schema.Tables = append(schema.Tables, table)
			continue
		}
		if parser.peekAnyKeyword(schemaFillerKeywords) {
			// Seed data, a pragma, a drop: statements a schema holds that say
			// nothing about a table's shape.
			parser.skipStatement()
			continue
		}
		parser.report("error", "expected CREATE TABLE, found "+parser.describe())
		parser.skipStatement()
	}
	return schema, parser.diags
}

// schemaFillerKeywords are the statements a schema may hold that define no
// column, and are therefore passed over rather than typed.
var schemaFillerKeywords = []string{
	"insert", "pragma", "drop", "alter", "set", "begin", "commit", "delete",
	"update", "grant", "comment", "analyze", "vacuum",
}

// ParseQueries reads a query file.
//
// Every statement is introduced by a comment that names it:
//
//	-- name: find_customer
//	-- Returns the customer with this address, or no rows.
//	SELECT id, name FROM customers WHERE address = :address;
//
// The name is what generated code and a caller use, so a statement with no name
// is reported: a query nobody can call is a query nobody meant to write.
func ParseQueries(name, text string) (Schema, []Diagnostic) {
	schema := Schema{Version: Version, Queries: []Query{}}
	parser := newParser(name, text)
	comment := parser.lastComment()

	for !parser.atEnd() {
		// The comment block that introduces a statement is read by
		// namedStatement, which is what takes the name and the prose from it —
		// so nothing steps over it first.
		queryName, prose := parser.namedStatement()
		if parser.atEnd() {
			break
		}
		if queryName == "" {
			parser.report("error", "every statement needs a comment naming it, as `-- name: thing`")
			parser.skipStatement()
			continue
		}
		body := parser.readStatement()
		if body == "" {
			parser.report("error", "the statement named "+queryName+" is empty")
			continue
		}
		query, _ := parseQuery(queryName, prose, body, parser)
		schema.Queries = append(schema.Queries, query)
		comment = prose
	}
	_ = comment
	if len(schema.Queries) == 0 && len(parser.diags) == 0 {
		parser.report("warning", "the file declares no queries")
	}
	return schema, parser.diags
}

/* -------------------------------------------------------------------------- */
/*  One statement                                                               */
/* -------------------------------------------------------------------------- */

// parseQuery reads one named statement: what it does, what it takes, and what
// it writes.
func parseQuery(name, comment, body string, parser *parser) (Query, []Diagnostic) {
	query := Query{Name: name, Comment: comment, SQL: body, Parameters: []Parameter{}}
	trimmed := strings.TrimSpace(body)
	verb, rest := firstWord(trimmed)
	switch Verb(strings.ToLower(verb)) {
	case VerbSelect:
		query.Verb = VerbSelect
	case VerbInsert:
		query.Verb = VerbInsert
	case Verb("into"):
		// `INSERT INTO` reads as two words, and the shape has one verb.
		query.Verb = VerbInsert
		rest = strings.TrimSpace(strings.TrimPrefix(trimmed, verb))
	case VerbUpdate:
		query.Verb = VerbUpdate
	case VerbDelete:
		query.Verb = VerbDelete
	case Verb("with"):
		// A `WITH` is a select that starts with a common table expression, and
		// it is a select as far as a caller is concerned. The verb is the first
		// word, so this is the word to match: matching `from` refused every one
		// of them, because `from` is never the first word of anything.
		query.Verb = VerbSelect
	default:
		parser.report("error", "a query must be a SELECT, an INSERT, an UPDATE or a DELETE, found "+verb)
		return query, parser.diags
	}

	query.Table = tableOf(query.Verb, rest)
	if len(query.Table) == 0 {
		parser.report("error", "could not read the table "+name+" works on")
	}
	query.Returning = returningOf(trimmed)
	query.Parameters = parametersOf(rest, parser)
	return query, parser.diags
}

// tableOf is the table a statement reads or writes.
func tableOf(verb Verb, rest string) string {
	fields := strings.Fields(rest)
	switch verb {
	case VerbSelect:
		// The table is what follows FROM, skipping over a join.
		for i, field := range fields {
			if strings.EqualFold(field, "from") && i+1 < len(fields) {
				return unquote(fields[i+1])
			}
		}
		return ""
	case VerbInsert:
		if len(fields) > 1 && strings.EqualFold(fields[0], "into") {
			return unquote(fields[1])
		}
	case VerbUpdate:
		if len(fields) > 0 {
			return unquote(fields[0])
		}
	case VerbDelete:
		for i, field := range fields {
			if strings.EqualFold(field, "from") && i+1 < len(fields) {
				return unquote(fields[i+1])
			}
		}
	}
	return ""
}

// returningOf is the columns a statement was written to return, which an engine
// that cannot return them has to work around rather than lose.
// returningOf is the columns a statement was written to return, which an engine
// that cannot return them has to work around rather than lose.
//
// The list is read to the end of the statement rather than to the next space:
// a RETURNING clause is a comma-separated list, and splitting on whitespace
// would read `RETURNING id, name` as one column and stop.
func returningOf(statement string) []string {
	runes := []rune(statement)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'', '"', '`':
			i = skipLiteral(runes, i)
			continue
		}
		if !atWord(runes, i, "RETURNING") {
			continue
		}
		rest := strings.TrimSpace(string(runes[i+len("RETURNING"):]))
		// A list ends at the statement's end or at a clause that is not part of
		// it, so what follows the last column is not read as one.
		if end := clauseAfter(rest); end >= 0 {
			rest = rest[:end]
		}
		return splitList(rest)
	}
	return nil
}

// atWord reports whether a word is written at a position, on word boundaries.
func atWord(runes []rune, at int, word string) bool {
	end := at + len(word)
	if end > len(runes) {
		return false
	}
	if !strings.EqualFold(string(runes[at:end]), word) {
		return false
	}
	before := at == 0 || !isNameByte(runes[at-1])
	after := end == len(runes) || !isNameByte(runes[end])
	return before && after
}

// clauseAfter is where a clause that ends a column list begins, or -1 when the
// list runs to the end of the statement.
func clauseAfter(text string) int {
	runes := []rune(text)
	for _, clause := range []string{" ON ", " WHERE ", " RETURNING ", " ORDER ", " LIMIT "} {
		if at := indexClause(runes, clause); at >= 0 {
			return at
		}
	}
	return -1
}

// indexClause is where a clause begins in a list of columns.
func indexClause(runes []rune, clause string) int {
	for i := 0; i+len(clause) <= len(runes); i++ {
		if strings.HasPrefix(string(runes[i:i+len(clause)]), clause) {
			return i
		}
	}
	return -1
}

// splitList splits a comma-separated list, dropping the spaces around each entry.
func splitList(text string) []string {
	out := []string{}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part != "" && part != ";" {
			out = append(out, unquote(part))
		}
	}
	return out
}

// unquote removes the quoting a name may carry, so a lookup finds the table
// whatever engine convention wrote it.
//
// The closing quote is looked for *after* the opening one. Looking from the
// start finds the opening quote itself, which is never a closing quote, so
// `"order"` would come back with its quotes and a column of that name would be a
// column nobody wrote.
func unquote(text string) string {
	text = strings.TrimSpace(text)
	for _, quote := range []string{`"`, "`", "[", "("} {
		if strings.HasPrefix(text, quote) {
			closing := map[string]string{`"`: `"`, "`": "`", "[": "]", "(": ")"}[quote]
			if end := strings.Index(text[len(quote):], closing); end > 0 {
				return text[len(quote) : len(quote)+end]
			}
		}
	}
	return text
}

// firstWord is the leading word of a statement, in upper case, and the rest.
func firstWord(text string) (string, string) {
	fields := strings.SplitN(strings.TrimSpace(text), " ", 2)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], fields[1]
}

/* -------------------------------------------------------------------------- */
/*  The parameter reader                                                        */
/* -------------------------------------------------------------------------- */

// parametersOf reads the values a statement takes.
//
// What is read is the markers and where they appear, which is all this file can
// know: a marker's *type* is the column it is compared against, and a column is
// a fact about the schema, which is read from another file. The generator puts
// the two together, because it has both.
//
// A `where` clause is carried as written. Re-deriving a predicate here would be a
// second implementation of the one thing an engine is best placed to do.
func parametersOf(statement string, parser *parser) []Parameter {
	out := []Parameter{}
	seen := map[string]bool{}

	for _, marker := range markersIn(statement) {
		ordinal := 0
		for _, existing := range out {
			if existing.Ordinal > ordinal {
				ordinal = existing.Ordinal
			}
		}
		parameter := Parameter{
			Name:    marker.name,
			Ordinal: len(out) + 1,
			Marker:  marker.kind,
		}
		if marker.kind == MarkerNumbered {
			if number, err := strconv.Atoi(strings.TrimPrefix(marker.name, "$")); err == nil {
				parameter.Ordinal = number
			}
		}
		if seen[parameter.Name+string(parameter.Marker)] {
			continue
		}
		seen[parameter.Name+string(parameter.Marker)] = true
		out = append(out, parameter)
	}

	// The order a caller passes values in is the order the markers appear, which
	// is what the shape carries; a dialect that names its parameters is matched
	// by name instead.
	sortByAppearance(out, statement)
	for i := range out {
		out[i].Ordinal = i + 1
	}
	return out
}

// aMarker is one parameter marker found in a statement's text.
type aMarker struct {
	name string
	kind Marker
	// at is where the marker starts, so a reader that wants to know what it is
	// compared against can look around it without scanning the statement again.
	at int
}

// markersIn finds every parameter marker in a statement.
//
// String literals are stepped over, so an address that happens to contain a
// colon is text rather than a parameter. That is the whole reason this walks the
// text rather than searching it.
func markersIn(statement string) []aMarker {
	out := []aMarker{}
	runes := []rune(statement)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'', '"', '`':
			i = skipLiteral(runes, i)
		case '?':
			out = append(out, aMarker{name: "?", kind: MarkerPositional, at: i})
		case '$':
			if i+1 < len(runes) && isDigit(runes[i+1]) {
				start := i
				i++
				for i+1 < len(runes) && isDigit(runes[i+1]) {
					i++
				}
				out = append(out, aMarker{name: string(runes[start : i+1]), kind: MarkerNumbered, at: start})
			}
		case ':', '@':
			// `::` is a cast and `:=` is an assignment; neither is a parameter.
			if i+1 < len(runes) && !isNameByte(runes[i+1]) {
				continue
			}
			if i+1 < len(runes) && runes[i+1] == ':' {
				continue
			}
			start := i
			i++
			for i < len(runes) && isNameByte(runes[i]) {
				i++
			}
			i--
			out = append(out, aMarker{name: string(runes[start+1 : i+1]), kind: MarkerNamed, at: start})
		}
	}
	return out
}

// skipLiteral returns the index of the closing quote of a literal starting at
// open, so a marker inside one is not read as a marker.
func skipLiteral(runes []rune, open int) int {
	quote := runes[open]
	for i := open + 1; i < len(runes); i++ {
		if runes[i] == quote {
			return i
		}
	}
	return len(runes)
}

// sortByAppearance puts a statement's parameters in the order their markers
// appear, which is the order a positional caller has to pass them in.
func sortByAppearance(parameters []Parameter, statement string) {
	positions := make(map[string]int, len(parameters))
	for i, parameter := range parameters {
		positions[parameter.Name+string(parameter.Marker)] = i
	}
	// A stable sort on the index each marker was found at, so two markers of the
	// same kind keep the order they were written in.
	order := map[string]int{}
	for i, marker := range markersIn(statement) {
		key := marker.name + string(marker.kind)
		if _, seen := order[key]; !seen {
			order[key] = i
		}
	}
	insertionSort(parameters, func(a, b Parameter) bool {
		left, leftSeen := order[a.Name+string(a.Marker)]
		right, rightSeen := order[b.Name+string(b.Marker)]
		if !leftSeen {
			return false
		}
		if !rightSeen {
			return true
		}
		return left < right
	})
	_ = positions
}

// insertionSort is a stable sort, which is what keeping two markers of the same
// name in written order needs.
func insertionSort(values []Parameter, less func(a, b Parameter) bool) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && less(values[j], values[j-1]); j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isNameByte(r rune) bool {
	return r == '_' || isDigit(r) ||
		('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}
