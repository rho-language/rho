//
// @file yaml_loader.go
// @brief Rho YAML Loader.
//
// Minimal deterministic YAML loader for Rho config files.
// Supports only:
//   - key: value
//   - nested maps
//   - lists using "- item"
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
	"bufio"
	"fmt"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// Load parses a minimal YAML document into a Value (KindMap).
func Load(src string) (Value, error) {
	root := Value{Kind: KindMap, Map: map[string]Value{}}
	scanner := bufio.NewScanner(strings.NewReader(src))

	type frame struct {
		indent int
		node   *Value
		parent *Value
		key    string
	}
	stack := []frame{{indent: -1, node: &root}}

	for scanner.Scan() {
		raw := strings.TrimRight(scanner.Text(), " \t")
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
		current := stack[len(stack)-1].node

		// key: value
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "-") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			if key == "" {
				return Value{}, fmt.Errorf("invalid YAML key in line: %q", line)
			}

			if val == ">" || val == "|" {
				current.Map[key] = Value{Kind: KindString, Str: ""}
				continue
			}
			if val == "" {
				child := &Value{Kind: KindMap, Map: map[string]Value{}}
				if key == "list" || key == "features" || key == "depends" || key == "include_dirs" || key == "source_dirs" || key == "sources" || key == "headers" || key == "required" || key == "optional" || key == "tags" || key == "test_dirs" || key == "test_files" || key == "kconfig_fragments" || key == "hooks" || key == "components" || key == "plugins" {
					child.Kind = KindList
					child.Map = nil
					child.List = []Value{}
				}
				if _, exists := current.Map[key]; exists {
					return Value{}, fmt.Errorf("duplicate YAML key: %q", key)
				}
				current.Map[key] = *child
				stack = append(stack, frame{
					indent: indent,
					node:   child,
					parent: current,
					key:    key,
				})
			} else {
				if _, exists := current.Map[key]; exists {
					return Value{}, fmt.Errorf("duplicate YAML key: %q", key)
				}
				// Strip surrounding quotes if present
				if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
					val = val[1 : len(val)-1]
				}
				current.Map[key] = Value{Kind: KindString, Str: val}
			}
			continue
		}

		// list item: "- item"
		if strings.HasPrefix(line, "-") {
			if current.Kind != KindList {
				return Value{}, fmt.Errorf("list item without a list: %q", line)
			}
			item := strings.TrimSpace(line[1:])
			if strings.Contains(item, ":") {
				parts := strings.SplitN(item, ":", 2)
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if key == "" || value == "" {
					return Value{}, fmt.Errorf("invalid YAML list map item: %q", line)
				}
				// Strip surrounding quotes if present
				if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
					value = value[1 : len(value)-1]
				}
				entry := Value{Kind: KindMap, Map: map[string]Value{key: {Kind: KindString, Str: value}}}
				current.List = append(current.List, entry)
				stack = append(stack, frame{indent: indent, node: &current.List[len(current.List)-1]})
				if frame := stack[len(stack)-2]; frame.parent != nil {
					frame.parent.Map[frame.key] = *current
				}
				continue
			}
			// Strip surrounding quotes if present
			if len(item) >= 2 && ((item[0] == '"' && item[len(item)-1] == '"') || (item[0] == '\'' && item[len(item)-1] == '\'')) {
				item = item[1 : len(item)-1]
			}
			current.List = append(current.List, Value{Kind: KindString, Str: item})
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

	return root, scanner.Err()
}
