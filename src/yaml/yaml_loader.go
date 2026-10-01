//
// @file yaml_loader.go
// @brief Rho YAML Loader.
//
// Minimal deterministic YAML loader for Rho config files.
// Supports only:
//   - key: value
//   - nested maps
//   - lists using "- item"
//   - block scalars using ">" and "|"
//
// No anchors, no tags, no advanced YAML features.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package yaml

import (
	"fmt"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// Load parses a minimal YAML document into a Value (KindMap).
func Load(src string) (Value, error) {
	root := Value{Kind: KindMap, Map: map[string]Value{}}
	lines := strings.Split(src, "\n")

	type frame struct {
		indent int
		node   *Value
		parent *Value
		key    string
		// isItem marks a frame pushed for a "- item" entry rather than for
		// a "key:" declaration. Item frames are popped when the next item
		// starts at the same indent; key frames are not.
		isItem bool
	}
	stack := []frame{{indent: -1, node: &root}}

	for i := 0; i < len(lines); i++ {
		raw := strings.TrimRight(lines[i], " \t")
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		popAtSameIndent := !strings.HasPrefix(line, "-")
		for len(stack) > 1 && (indent < stack[len(stack)-1].indent ||
			(popAtSameIndent && indent <= stack[len(stack)-1].indent)) {
			stack = stack[:len(stack)-1]
		}

		// A key declared with no value stays KindNull until content
		// follows. Resolve it from the first line it actually owns, so
		// list-vs-map is decided by the document instead of a fixed
		// table of key names.
		if top := &stack[len(stack)-1]; top.node.Kind == KindNull {
			if strings.HasPrefix(line, "-") {
				top.node.Kind = KindList
				top.node.Map = nil
				top.node.List = []Value{}
			} else {
				top.node.Kind = KindMap
				top.node.Map = map[string]Value{}
			}
			if top.parent != nil {
				top.parent.Map[top.key] = *top.node
			}
		}

		current := stack[len(stack)-1].node

		// A `key:` line only means something under a mapping. Reaching one with
		// a scalar or a list on top of the stack means the document's
		// indentation does not describe the shape it claims — a continuation line
		// left behind under a block scalar, most often, which is what a mangled
		// manifest looks like.
		//
		// This used to write into a nil map and crash. A loader that panics on a
		// file it cannot read takes the process with it, so the caller gets an
		// error naming the line instead: a person can act on that, and a build
		// can report it.
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "-") &&
			current.Kind != KindMap {
			return Value{}, fmt.Errorf("YAML line %d: %q is indented as content of a %s, but is a key",
				i+1, strings.TrimSpace(line), kindName(current.Kind))
		}

		// key: value
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "-") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			if key == "" {
				return Value{}, fmt.Errorf("invalid YAML key in line: %q", line)
			}
			if _, exists := current.Map[key]; exists {
				return Value{}, fmt.Errorf("duplicate YAML key: %q", key)
			}

			if style, ok := blockScalarStyle(val); ok {
				// A block scalar owns every following line indented deeper
				// than its key, so multi-line descriptions survive parsing.
				content, next := readBlockScalar(lines, i, indent)
				current.Map[key] = Value{Kind: KindString, Str: joinBlockScalar(content, style)}
				i = next - 1
				continue
			}

			if items, ok := inlineSequence(val); ok {
				current.Map[key] = items
				continue
			}

			if val == "" {
				// Defer the map-vs-list decision to the content below.
				child := &Value{Kind: KindNull}
				current.Map[key] = *child
				stack = append(stack, frame{
					indent: indent,
					node:   child,
					parent: current,
					key:    key,
				})
			} else {
				current.Map[key] = Value{Kind: KindString, Str: unquote(val)}
			}
			continue
		}

		// list item: "- item"
		if strings.HasPrefix(line, "-") {
			// A new item at the same indent as the previous one closes that
			// item, so pop its frame and return to the owning list.
			for len(stack) > 1 && stack[len(stack)-1].isItem && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			current = stack[len(stack)-1].node
			if current.Kind != KindList {
				return Value{}, fmt.Errorf("list item without a list: %q", line)
			}
			item := strings.TrimSpace(line[1:])
			// A quoted item is a string, whatever it contains. A target name
			// like `rho::robohub-common` is quoted precisely because of its
			// colons, and reading that as a key and a value turned a declared name
			// into a one-entry map — which then failed a schema that had said it
			// expected a string.
			if strings.Contains(item, ":") && !isQuoted(item) {
				parts := strings.SplitN(item, ":", 2)
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if key == "" || value == "" {
					return Value{}, fmt.Errorf("invalid YAML list map item: %q", line)
				}
				entry := Value{Kind: KindMap, Map: map[string]Value{key: {Kind: KindString, Str: unquote(value)}}}
				current.List = append(current.List, entry)
				stack = append(stack, frame{indent: indent, node: &current.List[len(current.List)-1], isItem: true})
				if frame := stack[len(stack)-2]; frame.parent != nil {
					frame.parent.Map[frame.key] = *current
				}
				continue
			}
			current.List = append(current.List, Value{Kind: KindString, Str: unquote(item)})
			if frame := stack[len(stack)-1]; frame.parent != nil {
				frame.parent.Map[frame.key] = *current
			}
			continue
		}

		if indent > 0 {
			continue
		}
		return Value{}, fmt.Errorf("invalid YAML line: %q", line)
	}

	return root, nil
}

/* -------------------------------------------------------------------------- */
/*  Block Scalars                                                             */
/* -------------------------------------------------------------------------- */

// blockScalarStyle reports whether a value opens a block scalar and returns
// the folding style. ">" folds lines with spaces, "|" keeps line breaks.
func blockScalarStyle(val string) (string, bool) {
	switch strings.TrimSpace(val) {
	case ">", ">-", ">+":
		return ">", true
	case "|", "|-", "|+":
		return "|", true
	default:
		return "", false
	}
}

// readBlockScalar collects the lines indented deeper than the owning key.
// It returns the content and the index of the first line after the block.
func readBlockScalar(lines []string, keyIndex, keyIndent int) ([]string, int) {
	content := []string{}
	i := keyIndex + 1
	for ; i < len(lines); i++ {
		raw := strings.TrimRight(lines[i], " \t")
		if strings.TrimSpace(raw) == "" {
			content = append(content, "")
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent <= keyIndent {
			break
		}
		content = append(content, strings.TrimSpace(raw))
	}
	// Trailing blank lines belong to the separator, not the content.
	for len(content) > 0 && content[len(content)-1] == "" {
		content = content[:len(content)-1]
	}
	return content, i
}

// joinBlockScalar renders collected block lines using the given style.
func joinBlockScalar(content []string, style string) string {
	if style == "|" {
		return strings.Join(content, "\n")
	}
	// Folded style: blank lines become newlines, everything else joins with
	// a single space, which is how a multi-line description reads.
	var b strings.Builder
	for i, line := range content {
		if i > 0 {
			if line == "" || content[i-1] == "" {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(line)
	}
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Inline Sequences                                                          */
/* -------------------------------------------------------------------------- */

// inlineSequence parses a flow-style list such as `[]` or `[a, b]` into a
// list value. Manifests use it for short lists, most often empty ones.
func inlineSequence(val string) (Value, bool) {
	if len(val) < 2 || val[0] != '[' || val[len(val)-1] != ']' {
		return Value{}, false
	}
	inner := strings.TrimSpace(val[1 : len(val)-1])
	list := Value{Kind: KindList, List: []Value{}}
	if inner == "" {
		return list, true
	}
	for _, item := range strings.Split(inner, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, ":") && !isQuoted(item) {
			// A flow entry written as `key: value` becomes a single-entry map
			// so the shape matches the block form.
			parts := strings.SplitN(item, ":", 2)
			key := strings.TrimSpace(parts[0])
			value := unquote(strings.TrimSpace(parts[1]))
			if key == "" {
				return Value{}, false
			}
			list.List = append(list.List, Value{Kind: KindMap, Map: map[string]Value{key: {Kind: KindString, Str: value}}})
			continue
		}
		list.List = append(list.List, Value{Kind: KindString, Str: unquote(item)})
	}
	return list, true
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// unquote strips one layer of matching surrounding quotes.
// Unquote removes the quotes of a quoted scalar, and only those. A reader that
// takes a value out of a document uses this, so every reader agrees on what a
// quoted value is.
func Unquote(val string) string { return unquote(val) }

// unquote removes the quotes of a quoted scalar, and only those.
//
// A value whose first and last characters happen to be quotes is not a quoted
// scalar: a command that runs a program with arguments is written
// `run: "prog" --flag "arg"`, and the quotes belong to the arguments, not to
// the value. A quoted scalar carries no unescaped quote of its own between the
// pair, which is what tells the two apart.
func unquote(val string) string {
	if len(val) < 2 {
		return val
	}
	first, last := val[0], val[len(val)-1]
	if first != last || (first != '"' && first != '\'') {
		return val
	}
	for index := 1; index < len(val)-1; index++ {
		switch {
		case val[index] == '\\' && index+1 < len(val):
			index++
		case val[index] == first:
			return val
		}
	}
	return val[1 : len(val)-1]
}

// kindName is a node's kind as a word, for a message that says what a line
// landed under.
func kindName(kind Kind) string {
	switch kind {
	case KindMap:
		return "mapping"
	case KindList:
		return "list"
	case KindNull:
		return "key with no value"
	default:
		return "value"
	}
}

// isQuoted reports whether a scalar is quoted, and therefore a string whatever
// it contains.
//
// Quoting is how a value says what it is. A colon inside quotes is a character,
// not a mapping, and a value whose whole purpose is to contain one — a CMake
// target name, a URL, a timestamp — is quoted for that reason alone.
func isQuoted(value string) bool {
	if len(value) < 2 {
		return false
	}
	first := value[0]
	return (first == '"' || first == '\'') && value[len(value)-1] == first
}
