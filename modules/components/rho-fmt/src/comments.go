//
// @file comments.go
// @brief Keeping comments through a format.
//
// The tree has no comments in it, so the printer puts them back from the
// source. The lexer hands over every comment with where it is, and the printer
// walks the source tokens beside the tree: before it prints a declaration, a
// field, a variant, a statement or a match arm, the comments written before
// that node's first token are printed above it; when it ends a line, the
// comments written after that line's last token, on the same source line, stay
// at the end of it; before a closing brace, the comments written before it are
// printed inside the block.
//
// A comment the printer has no place for in the line it is printing — one in
// the middle of an expression the printer puts on one line — is printed on a
// line of its own above that line, or below it when a closure in the line has
// already been printed. It is never dropped: what is not placed by the time the
// file ends is printed at the end of it, and FormatSource refuses to answer if
// the comments it printed are not the comments it read.
//
// A blank line next to a comment is kept (one, however many were written), so a
// comment that is separated from the declaration below it stays separated, and
// a doc comment stays directly above what it documents.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

// commentSet is the source's tokens and comments, and how far through the
// comments the printer is.
type commentSet struct {
	toks []lexer.Token
	list []lexer.Comment
	next int
	// blankBefore and blankAfter say whether a blank line was written between
	// each comment and what comes before it, and after it.
	blankBefore []bool
	blankAfter  []bool
	at          map[[2]int]int
	pair        map[int]int
	// tests are the functions declared `test fn`, by where their `fn` is.
	tests map[[2]int]bool
}

// comments is the set being printed, nil when a node is printed without a
// source behind it.
var comments *commentSet

// newCommentSet indexes a source's tokens and comments.
func newCommentSet(toks []lexer.Token, list []lexer.Comment) *commentSet {
	cs := &commentSet{
		toks:        toks,
		list:        list,
		blankBefore: make([]bool, len(list)),
		blankAfter:  make([]bool, len(list)),
		at:          make(map[[2]int]int, len(toks)),
		pair:        map[int]int{},
	}
	var stack []int
	for i, t := range toks {
		cs.at[[2]int{t.Line, t.Column}] = i
		switch t.Kind {
		case lexer.TokenLBrace:
			stack = append(stack, i)
		case lexer.TokenRBrace:
			if len(stack) > 0 {
				open := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cs.pair[open], cs.pair[i] = i, open
			}
		}
	}
	// What ends before a comment and what begins after it, by line: the token
	// or comment nearest it on either side.
	tok := 0
	prevEnd := 0
	for i, c := range list {
		for tok < len(toks) && tokenBefore(toks[tok], c) {
			if toks[tok].Line > prevEnd {
				prevEnd = toks[tok].Line
			}
			tok++
		}
		cs.blankBefore[i] = prevEnd > 0 && c.Line-prevEnd >= 2
		prevEnd = c.EndLine
		nextLine := 0
		if i+1 < len(list) {
			nextLine = list[i+1].Line
		}
		if tok < len(toks) && toks[tok].Kind != lexer.TokenEOF && (nextLine == 0 || toks[tok].Line < nextLine) {
			nextLine = toks[tok].Line
		}
		cs.blankAfter[i] = nextLine > 0 && nextLine-c.EndLine >= 2
	}
	return cs
}

// tokenBefore reports whether a token begins before a comment.
func tokenBefore(t lexer.Token, c lexer.Comment) bool {
	return t.Line < c.Line || (t.Line == c.Line && t.Column < c.Column)
}

// commentBefore reports whether a comment begins before a token.
func commentBefore(c lexer.Comment, t lexer.Token) bool {
	return c.Line < t.Line || (c.Line == t.Line && c.Column < t.Column)
}

// commentText is a comment as it is printed: a line comment without the white
// space it ended with, a block comment exactly as written.
func commentText(c lexer.Comment) string {
	if c.Block() {
		return c.Text
	}
	return strings.TrimRight(c.Text, " \t\r")
}

// tokOf is the index of the token a node begins with, or -1.
func tokOf(pos ast.Position) int {
	if comments == nil || !pos.Valid() {
		return -1
	}
	if i, ok := comments.at[[2]int{pos.Line, pos.Column}]; ok {
		return i
	}
	return -1
}

// kindAt is the kind of the token at an index, or EOF when there is none.
func kindAt(i int) lexer.Kind {
	if comments == nil || i < 0 || i >= len(comments.toks) {
		return lexer.TokenEOF
	}
	return comments.toks[i].Kind
}

// pairOf is the brace that closes or opens the one at an index, or -1.
func pairOf(i int) int {
	if comments == nil || i < 0 {
		return -1
	}
	if j, ok := comments.pair[i]; ok {
		return j
	}
	return -1
}

// braceFrom is the first `{` after a token that is not inside parentheses or
// brackets: the brace that opens a declaration's or a statement's body. A
// condition cannot hold a struct literal, so its first such brace is the body's.
func braceFrom(start int) int {
	if comments == nil || start < 0 {
		return -1
	}
	depth := 0
	for i := start + 1; i < len(comments.toks); i++ {
		switch comments.toks[i].Kind {
		case lexer.TokenLParen, lexer.TokenLBracket:
			depth++
		case lexer.TokenRParen, lexer.TokenRBracket:
			depth--
		case lexer.TokenLBrace:
			if depth == 0 {
				return i
			}
		case lexer.TokenRBrace:
			if depth == 0 {
				return -1
			}
		case lexer.TokenEOF:
			return -1
		}
	}
	return -1
}

// findFrom is the first token of a kind after start and before end that is not
// nested in any bracket, or -1. A lexeme narrows it, `=` among the assignments.
func findFrom(start, end int, kind lexer.Kind, lex string) int {
	if comments == nil || start < 0 {
		return -1
	}
	if end < 0 || end >= len(comments.toks) {
		end = len(comments.toks) - 1
	}
	depth := 0
	for i := start + 1; i <= end; i++ {
		t := comments.toks[i]
		if depth == 0 && t.Kind == kind && (lex == "" || t.Lex == lex) {
			return i
		}
		switch t.Kind {
		case lexer.TokenLParen, lexer.TokenLBracket, lexer.TokenLBrace:
			depth++
		case lexer.TokenRParen, lexer.TokenRBracket, lexer.TokenRBrace:
			depth--
			if depth < 0 {
				return -1
			}
		}
	}
	return -1
}

// hasClosure reports whether the tokens after start, up to end, hold a function
// literal: a `fn` with a brace after it. A function type has no brace.
func hasClosure(start, end int) bool {
	if comments == nil || start < 0 || end < 0 {
		return false
	}
	sawFn := false
	for i := start + 1; i <= end && i < len(comments.toks); i++ {
		switch comments.toks[i].Kind {
		case lexer.TokenFn:
			sawFn = true
		case lexer.TokenLBrace:
			if sawFn {
				return true
			}
		}
	}
	return false
}

// unitEnds is where each of a list of units ends: just before the next one
// begins, the last one just before the brace that closes the list. An end that
// cannot be known is -1.
func unitEnds(starts []int, close int) []int {
	ends := make([]int, len(starts))
	for i := range starts {
		next := close
		if i+1 < len(starts) {
			next = starts[i+1]
		}
		if next < 0 || starts[i] < 0 {
			ends[i] = -1
			continue
		}
		ends[i] = next - 1
	}
	return ends
}

// stmtStarts is the first token of each statement in a list.
func stmtStarts(list []ast.Stmt) []int {
	starts := make([]int, len(list))
	for i, stmt := range list {
		starts[i] = tokOf(stmt.Position())
	}
	return starts
}

// fieldStarts finds the first token of each field in a braced field list, the
// tree not saying where a struct field is: a name followed by a colon, at the
// top of the braces, that begins the body, follows a separator or begins a
// line. Any disagreement with the tree's count gives no starts at all.
func fieldStarts(open, close, count int) []int {
	starts := make([]int, count)
	for i := range starts {
		starts[i] = -1
	}
	if comments == nil || open < 0 || close < 0 {
		return starts
	}
	var found []int
	depth := 0
	for i := open + 1; i < close; i++ {
		t := comments.toks[i]
		switch t.Kind {
		case lexer.TokenLParen, lexer.TokenLBracket, lexer.TokenLBrace:
			depth++
			continue
		case lexer.TokenRParen, lexer.TokenRBracket, lexer.TokenRBrace:
			depth--
			continue
		}
		if depth != 0 || comments.toks[i+1].Kind != lexer.TokenColon {
			continue
		}
		prev := comments.toks[i-1]
		switch {
		case i == open+1, prev.Kind == lexer.TokenComma, prev.Kind == lexer.TokenSemicolon:
			found = append(found, i)
		case prev.Line < t.Line && prev.Kind != lexer.TokenColon:
			found = append(found, i)
		}
	}
	if len(found) != count {
		return starts
	}
	return found
}

// wantsBlank reports whether a blank line may be written here: after a line,
// not after another blank line, and not first in a block.
func wantsBlank(b *strings.Builder) bool {
	s := b.String()
	return len(s) > 0 && strings.HasSuffix(s, "\n") && !strings.HasSuffix(s, "\n\n") && !strings.HasSuffix(s, "{\n")
}

// writeComment prints a comment on a line of its own. A block comment's later
// lines are its own and are kept as written.
func writeComment(b *strings.Builder, c lexer.Comment, depth int) {
	b.WriteString(strings.Repeat(indent, depth) + commentText(c) + "\n")
}

// flushBefore prints, on lines of their own, the comments written before the
// token at an index. blankAfter keeps a blank line written between the last of
// them and that token, which is what separates a detached comment from the
// declaration below it.
func flushBefore(b *strings.Builder, before, depth int, blankAfter bool) {
	if comments == nil || before < 0 || before >= len(comments.toks) {
		return
	}
	cs := comments
	target := cs.toks[before]
	last := -1
	for cs.next < len(cs.list) && commentBefore(cs.list[cs.next], target) {
		if cs.blankBefore[cs.next] && wantsBlank(b) {
			b.WriteString("\n")
		}
		writeComment(b, cs.list[cs.next], depth)
		last = cs.next
		cs.next++
	}
	if last >= 0 && blankAfter && cs.blankAfter[last] && target.Kind != lexer.TokenRBrace && target.Kind != lexer.TokenEOF &&
		(cs.next >= len(cs.list) || !commentBefore(cs.list[cs.next], target)) {
		b.WriteString("\n")
	}
}

// endLine ends the line being printed, whose last source token is at an index.
// The comments after that token on its source line stay at the end of the line;
// any comment still waiting from inside the line, which the printer could not
// put above it, goes on a line of its own below it.
func endLine(b *strings.Builder, end, depth int) {
	if comments == nil || end < 0 || end >= len(comments.toks) {
		b.WriteString("\n")
		return
	}
	cs := comments
	last := cs.toks[end]
	var leftovers []lexer.Comment
	for cs.next < len(cs.list) && commentBefore(cs.list[cs.next], last) {
		leftovers = append(leftovers, cs.list[cs.next])
		cs.next++
	}
	for cs.next < len(cs.list) {
		c := cs.list[cs.next]
		if c.Line != last.Line || commentBefore(c, last) {
			break
		}
		if end+1 < len(cs.toks) && !commentBefore(c, cs.toks[end+1]) {
			break
		}
		b.WriteString(" " + commentText(c))
		cs.next++
	}
	b.WriteString("\n")
	for _, c := range leftovers {
		writeComment(b, c, depth)
	}
}

// flushRest prints every comment not printed yet, at the end of the file.
func flushRest(b *strings.Builder) {
	if comments == nil {
		return
	}
	cs := comments
	for ; cs.next < len(cs.list); cs.next++ {
		if cs.blankBefore[cs.next] && wantsBlank(b) {
			b.WriteString("\n")
		}
		writeComment(b, cs.list[cs.next], 0)
	}
}

// commentTexts is the comments of a source, as they are compared: each as it
// is printed, sorted, so two sources with the same comments in a different
// order compare equal.
func commentTexts(list []lexer.Comment) []string {
	texts := make([]string, len(list))
	for i, c := range list {
		texts[i] = commentText(c)
	}
	sort.Strings(texts)
	return texts
}

// sameComments checks that a formatted source has the comments its original
// had, and says which one is missing or extra when it does not.
func sameComments(before []lexer.Comment, formatted string) error {
	_, after, err := lexer.LexComments(formatted)
	if err != nil {
		return fmt.Errorf("the formatted source does not lex: %w", err)
	}
	want, got := commentTexts(before), commentTexts(after)
	i, j := 0, 0
	for i < len(want) || j < len(got) {
		switch {
		case j >= len(got) || (i < len(want) && want[i] < got[j]):
			return fmt.Errorf("formatting would lose the comment %q", want[i])
		case i >= len(want) || got[j] < want[i]:
			return fmt.Errorf("formatting would add the comment %q", got[j])
		default:
			i++
			j++
		}
	}
	return nil
}
