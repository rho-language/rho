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

			if val == "" {
				child := &Value{Kind: KindMap, Map: map[string]Value{}}
				if key == "list" {
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
			current.List = append(current.List, Value{Kind: KindString, Str: item})
			if frame := stack[len(stack)-1]; frame.parent != nil {
				frame.parent.Map[frame.key] = *current
			}
			continue
		}

		return Value{}, fmt.Errorf("invalid YAML line: %q", line)
	}

	return root, scanner.Err()
}
