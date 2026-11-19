//
// @file edit.go
// @brief Edits that change one value and leave every other byte where it was.
//
// The requirement files and the ADR headers belong to the people who write them:
// their comments, blank lines, key order and quoting are theirs. So an edit is not
// a load, a change and a write — that would be a reformat — but a change to the
// lines of one value, found through a parse that knows which lines each value
// occupies:
//
//   - a scalar is replaced in place on its line, keeping what came before it and
//     the comment after it; a block scalar keeps its `|` or `>` header and indent
//   - a flow list is rewritten in place; a block list gains or loses the one
//     `- item` line
//   - a missing key is inserted after the key that precedes it in the canonical
//     order, at the indentation of its neighbours
//
// After every change the document is parsed again, so the next change works from
// what the file now says rather than from line numbers that have moved.
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

// Doc is a file being edited.
type Doc struct {
	Lines []string
	// FrontMatter limits the YAML to a Markdown file's front matter.
	FrontMatter bool
	crlf        bool
	final       bool
}

// NewDoc opens a document's text for editing.
func NewDoc(text string, frontMatter bool) *Doc {
	return &Doc{
		Lines:       splitLines(text),
		FrontMatter: frontMatter,
		crlf:        strings.Contains(text, "\r\n"),
		final:       text == "" || strings.HasSuffix(text, "\n"),
	}
}

// String is the document's text.
func (d *Doc) String() string {
	newline := "\n"
	if d.crlf {
		newline = "\r\n"
	}
	text := strings.Join(d.Lines, newline)
	if d.final && len(d.Lines) > 0 {
		text += newline
	}
	return text
}

// Root parses the document (or its front matter).
func (d *Doc) Root() (*Node, error) {
	if !d.FrontMatter {
		return parseLines(d.Lines, 0)
	}
	start, end, ok := frontMatter(d.Lines)
	if !ok {
		return nil, fmt.Errorf("no front matter between two --- lines")
	}
	return parseLines(d.Lines[start:end], start)
}

// Locator finds the mapping an edit applies to.
type Locator func(root *Node) (*Node, error)

// RootMap locates the document's top-level mapping.
func RootMap(root *Node) (*Node, error) {
	if root.Kind != Map {
		return nil, fmt.Errorf("the document is not a mapping")
	}
	return root, nil
}

// RequirementItem locates the requirement with an id.
func RequirementItem(id string) Locator {
	return func(root *Node) (*Node, error) {
		entry := root.Get("requirements")
		if entry == nil || entry.Value.Kind != List {
			return nil, fmt.Errorf("the file has no requirements list")
		}
		for _, item := range entry.Value.Items {
			if item.Kind == Map && item.Str("id") == id {
				if item.Style == '{' {
					return nil, fmt.Errorf("%s is written as a flow mapping; edit it by hand", id)
				}
				return item, nil
			}
		}
		return nil, fmt.Errorf("%s is not in this file", id)
	}
}

func (d *Doc) locate(loc Locator) (*Node, error) {
	root, err := d.Root()
	if err != nil {
		return nil, err
	}
	return loc(root)
}

func (d *Doc) splice(from, to int, replacement []string) {
	// from and to are 0-based, to exclusive.
	out := append([]string{}, d.Lines[:from]...)
	out = append(out, replacement...)
	d.Lines = append(out, d.Lines[to:]...)
}

/* -------------------------------------------------------------------------- */
/*  Scalars                                                                   */
/* -------------------------------------------------------------------------- */

// Get is the scalar value of a key in the located mapping.
func (d *Doc) Get(loc Locator, key string) (string, bool, error) {
	node, err := d.locate(loc)
	if err != nil {
		return "", false, err
	}
	entry := node.Get(key)
	if entry == nil {
		return "", false, nil
	}
	return entry.Value.Value, true, nil
}

// Set gives a key a scalar value, inserting the key when it is missing.
func (d *Doc) Set(loc Locator, key, value string, order []string) error {
	node, err := d.locate(loc)
	if err != nil {
		return err
	}
	entry := node.Get(key)
	if entry == nil {
		return d.insert(node, key, formatScalar(value), order)
	}
	switch entry.Value.Kind {
	case Map, List:
		if entry.Value.Style == 0 {
			return fmt.Errorf("%s holds %s; it cannot be given a single value", key, entry.Value.Kind)
		}
	}
	if entry.Value.Kind == Scalar && (entry.Value.Style == '|' || entry.Value.Style == '>') {
		d.replaceBlockScalar(entry, value)
		return nil
	}
	d.replaceInline(entry, formatScalar(value))
	return nil
}

// replaceInline writes text as a key's value on the key's line, keeping the
// comment after the old value, and drops any lines the old value continued on.
func (d *Doc) replaceInline(entry *Entry, text string) {
	index := entry.Line - 1
	line := d.Lines[index]
	var updated string
	if entry.ValueCol < 0 {
		colon := entry.Col + strings.Index(line[entry.Col:], ":")
		rest := line[colon+1:]
		if strings.TrimSpace(rest) == "" {
			rest = ""
		}
		updated = line[:colon+1] + " " + text + rest
	} else {
		prefix := line[:entry.ValueCol]
		after := line[entry.ValueCol:]
		suffix := ""
		if entry.EndLine() == entry.Line {
			cut := commentStart(after)
			old := after[:cut]
			suffix = old[len(strings.TrimRight(old, " \t")):] + after[cut:]
			if cut == len(after) {
				suffix = ""
			}
		}
		updated = prefix + text + suffix
	}
	d.splice(index, entry.EndLine(), []string{updated})
}

// replaceBlockScalar writes a value under a `|` or `>` header, at the indent the
// old content had.
func (d *Doc) replaceBlockScalar(entry *Entry, value string) {
	header := entry.Line - 1
	indent := strings.Repeat(" ", entry.Col+2)
	if entry.Value.EndLine > entry.Line {
		indent = leadingSpace(d.Lines[header+1])
		for index := header + 1; index < entry.Value.EndLine; index++ {
			if strings.TrimSpace(d.Lines[index]) != "" {
				indent = leadingSpace(d.Lines[index])
				break
			}
		}
	}
	value = strings.TrimRight(value, "\n")
	content := []string{}
	paragraphs := strings.Split(value, "\n")
	for index, line := range paragraphs {
		if entry.Value.Style == '>' && index > 0 {
			content = append(content, "")
		}
		if line == "" {
			content = append(content, "")
			continue
		}
		content = append(content, indent+line)
	}
	d.splice(header+1, entry.EndLine(), content)
}

// insert adds a key after the key that precedes it in order, or after the last.
func (d *Doc) insert(node *Node, key, text string, order []string) error {
	if len(node.Entries) == 0 {
		return fmt.Errorf("cannot add %s to an empty mapping", key)
	}
	col := node.Entries[len(node.Entries)-1].Col
	after := node.Entries[len(node.Entries)-1]
	if position := indexOf(order, key); position >= 0 {
		after = node.Entries[0]
		for _, entry := range node.Entries {
			if at := indexOf(order, entry.Key); at >= 0 && at < position {
				after = entry
			}
		}
		// A key that comes before every key present still goes after the first,
		// which may be on a list item's dash line.
	}
	line := strings.Repeat(" ", col) + key + ": " + text
	end := after.EndLine()
	d.splice(end, end, []string{line})
	return nil
}

// Delete removes a key and its value.
func (d *Doc) Delete(loc Locator, key string) error {
	node, err := d.locate(loc)
	if err != nil {
		return err
	}
	entry := node.Get(key)
	if entry == nil {
		return nil
	}
	if entry.Line == node.Line && node.Entries[0] == entry && node.Line != 1 {
		if strings.HasPrefix(strings.TrimSpace(d.Lines[entry.Line-1]), "-") {
			return fmt.Errorf("%s is on the item's first line and cannot be removed", key)
		}
	}
	d.splice(entry.Line-1, entry.EndLine(), nil)
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Lists                                                                     */
/* -------------------------------------------------------------------------- */

// List is the scalars of a list key.
func (d *Doc) List(loc Locator, key string) ([]string, error) {
	node, err := d.locate(loc)
	if err != nil {
		return nil, err
	}
	return node.Strings(key), nil
}

// SetList replaces a list. A flow list is rewritten in place; a block list keeps
// its style.
func (d *Doc) SetList(loc Locator, key string, values []string, order []string) error {
	node, err := d.locate(loc)
	if err != nil {
		return err
	}
	entry := node.Get(key)
	if entry == nil {
		return d.insert(node, key, formatFlow(values), order)
	}
	if entry.Value.Kind == List && entry.Value.Style == 0 && len(values) > 0 {
		first := entry.Value.Items[0].Line - 1
		indent := leadingSpace(d.Lines[first])
		lines := []string{}
		for _, value := range values {
			lines = append(lines, indent+"- "+formatFlowItem(value))
		}
		d.splice(first, entry.Value.EndLine, lines)
		return nil
	}
	if entry.Value.Kind == List && entry.Value.Style == 0 {
		// An emptied block list becomes `key: []`.
		index := entry.Line - 1
		line := d.Lines[index]
		colon := entry.Col + strings.Index(line[entry.Col:], ":")
		d.splice(index, entry.EndLine(), []string{line[:colon+1] + " []"})
		return nil
	}
	if entry.Value.Kind == Map {
		return fmt.Errorf("%s is a mapping, not a list", key)
	}
	d.replaceInline(entry, formatFlow(values))
	return nil
}

// Add appends a value to a list, unless it is there already.
func (d *Doc) Add(loc Locator, key, value string, order []string) error {
	node, err := d.locate(loc)
	if err != nil {
		return err
	}
	entry := node.Get(key)
	if entry == nil {
		return d.insert(node, key, formatFlow([]string{value}), order)
	}
	current := node.Strings(key)
	if contains(current, value) {
		return nil
	}
	if entry.Value.Kind == List && entry.Value.Style == 0 && len(entry.Value.Items) > 0 {
		last := entry.Value.Items[len(entry.Value.Items)-1]
		indent := leadingSpace(d.Lines[last.Line-1])
		d.splice(last.EndLine, last.EndLine, []string{indent + "- " + formatFlowItem(value)})
		return nil
	}
	if entry.Value.Kind == Scalar && entry.Value.Value != "" {
		return fmt.Errorf("%s holds a single value, not a list", key)
	}
	return d.SetList(loc, key, append(current, value), order)
}

// Remove takes a value out of a list.
func (d *Doc) Remove(loc Locator, key, value string) error {
	node, err := d.locate(loc)
	if err != nil {
		return err
	}
	entry := node.Get(key)
	current := node.Strings(key)
	if entry == nil || !contains(current, value) {
		return fmt.Errorf("%s is not in %s", value, key)
	}
	if entry.Value.Kind == List && entry.Value.Style == 0 && len(entry.Value.Items) > 1 {
		for _, item := range entry.Value.Items {
			if item.Kind == Scalar && item.Value == value {
				d.splice(item.Line-1, item.EndLine, nil)
				return nil
			}
		}
	}
	kept := []string{}
	for _, item := range current {
		if item != value {
			kept = append(kept, item)
		}
	}
	return d.SetList(loc, key, kept, nil)
}

/* -------------------------------------------------------------------------- */
/*  Appending a requirement                                                   */
/* -------------------------------------------------------------------------- */

// KV is a key and its already-formatted value.
type KV struct{ Key, Text string }

// AppendItem adds a mapping to the end of the top-level list under listKey,
// written in the style of the items already there.
func (d *Doc) AppendItem(listKey string, fields []KV) error {
	root, err := d.Root()
	if err != nil {
		return err
	}
	entry := root.Get(listKey)
	itemLines := func(dash, col int) []string {
		lines := []string{}
		for index, field := range fields {
			lead := strings.Repeat(" ", col)
			if index == 0 {
				lead = strings.Repeat(" ", dash) + "-" + strings.Repeat(" ", col-dash-1)
			}
			lines = append(lines, lead+field.Key+": "+field.Text)
		}
		return lines
	}
	switch {
	case entry == nil:
		lines := []string{listKey + ":"}
		lines = append(lines, itemLines(2, 4)...)
		d.Lines = append(d.Lines, lines...)
		d.final = true
	case entry.Value.Kind == Null:
		d.splice(entry.Line, entry.Line, itemLines(entry.Col+2, entry.Col+4))
	case entry.Value.Kind == List && entry.Value.Style == '[':
		if len(entry.Value.Items) > 0 {
			return fmt.Errorf("%s is written as a flow list; write it as a block list to add to it", listKey)
		}
		index := entry.Line - 1
		line := d.Lines[index]
		after := line[entry.ValueCol:]
		closing := strings.Index(after, "]")
		d.Lines[index] = strings.TrimRight(line[:entry.ValueCol], " ") + after[closing+1:]
		d.splice(entry.Line, entry.Line, itemLines(entry.Col+2, entry.Col+4))
	case entry.Value.Kind == List:
		items := entry.Value.Items
		first := items[0]
		dash := indentOf(d.Lines[first.Line-1])
		col := dash + 2
		if first.Kind == Map && len(first.Entries) > 0 {
			col = first.Entries[0].Col
		}
		lines := itemLines(dash, col)
		separated := false
		if len(items) > 1 {
			for index := items[0].EndLine; index < items[1].Line-1; index++ {
				if strings.TrimSpace(d.Lines[index]) == "" {
					separated = true
				}
			}
		}
		if separated {
			lines = append([]string{""}, lines...)
		}
		end := entry.Value.EndLine
		d.splice(end, end, lines)
	default:
		return fmt.Errorf("%s is not a list", listKey)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Formatting                                                                */
/* -------------------------------------------------------------------------- */

var (
	numberLike = regexp.MustCompile(`^[-+]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][-+]?[0-9]+)?$`)
	reserved   = map[string]bool{"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true, "null": true, "~": true, "y": true, "n": true}
)

// formatScalar writes a value plain when YAML reads it back as the same string,
// and double-quoted otherwise.
func formatScalar(value string) string {
	if needsQuotes(value, false) {
		return quote(value)
	}
	return value
}

func needsQuotes(value string, inFlow bool) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return true
	}
	if strings.ContainsAny(value[:1], "-?:,[]{}#&*!|>'\"%@`") {
		return true
	}
	if strings.Contains(value, ": ") || strings.Contains(value, " #") || strings.ContainsAny(value, "\n\t") || strings.HasSuffix(value, ":") {
		return true
	}
	if inFlow && strings.ContainsAny(value, ",[]{}") {
		return true
	}
	return reserved[strings.ToLower(value)] || numberLike.MatchString(value)
}

func quote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func formatFlowItem(value string) string {
	if needsQuotes(value, true) {
		return quote(value)
	}
	return value
}

func formatFlow(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatFlowItem(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func indexOf(list []string, value string) int {
	for index, item := range list {
		if item == value {
			return index
		}
	}
	return -1
}
