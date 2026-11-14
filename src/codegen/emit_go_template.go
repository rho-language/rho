//
// @file emit_go_template.go
// @brief Template rendering for the Go backend.
//
// A Rho project keeps its markup in template files rather than in string
// literals, and the descriptor names the templates a target carries. The
// backend embeds them and supplies a small renderer, so a handler renders a
// page without writing HTML in Rho.
//
// The syntax is deliberately small:
//
//	{{ name }}      substitute a value, HTML-escaped
//	{{{ name }}}    substitute a value verbatim, for trusted markup
//	{# note #}      a comment, removed
//
// Values are passed as name/value pairs because Rho has no runtime maps, and
// a pair whose name is empty is ignored, so a caller can leave one unset.
//
// Rendering escapes by default: a value that reaches a page unescaped has to be
// asked for explicitly with the triple-brace form.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/ir"
)

// TemplateFileName is the generated file holding the embedded templates.
const TemplateFileName = "templates.go"

// Template is one template carried into a generated project.
type Template struct {
	// Name is how Rho requests the template, for example "index".
	Name string
	// Content is the template source.
	Content string
}

// EmitGoTemplates renders the embedded templates and the renderer.
func EmitGoTemplates(contents map[string]string) string {
	if len(contents) == 0 {
		return ""
	}

	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(goBanner(irModuleForBanner(), TemplateFileName, "Generated Rho page templates."))
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t\"fmt\"\n\t\"strings\"\n)\n\n")
	b.WriteString("// The templates a Rho handler renders, keyed by name.\n")
	b.WriteString("var rhoTemplates = map[string]string{\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\t%q: %s,\n", name, goQuoted(contents[name]))
	}
	b.WriteString("}\n\n")
	b.WriteString(goTemplateRenderer())
	return b.String()
}

// goTemplateRenderer emits template lookup and the placeholder renderer.
func goTemplateRenderer() string {
	return `// templateGet returns a template's source, or "" when it is absent.
func templateGet(name string) string {
	return rhoTemplates[name]
}

// templateHas reports whether a template was carried into the project.
func templateHas(name string) bool {
	_, ok := rhoTemplates[name]
	return ok
}

// templateSep separates the encoded name/value list. It is a non-printing byte,
// so a value may contain spaces, tabs and newlines.
const templateSep = "\x1f"

// templateFill renders a template with the values in an encoded list.
//
// The list is the concatenation of name, value, name, value pairs, each
// preceded by templateSep, which is what std/web/template.rho's vars builds.
// Because the list is a single string, a template can gain a placeholder
// without any signature here changing.
//
// {{ name }} is HTML-escaped, which is what a page needs by default. The
// triple-brace form {{{ name }}} inserts a value verbatim, for markup a Rho
// handler built itself. A name that is not in the list is left as written, so
// a typo shows up in the page rather than silently disappearing.
//
// Blocks compose and nest: {{#if name}}...{{else}}...{{/if}} and
// {{#unless name}}...{{else}}...{{/unless}} choose a branch, {{#each name}}
// repeats its body once per line with {{.}} / {{this}}, {{@index}},
// {{@index1}}, {{@first}}, {{@last}}, {{@count}} and, for a tab-separated
// item, {{@0}}, {{@1}} ... bound, and
// {{#with prefix}} scopes lookups to prefix.* (or binds {{this}} to the value)
// inside its body. A block with no items or scope may carry {{else}}.
// Partials: {{> name }} inserts another carried template. Comments:
// {# note #} is removed. A default: {{ name ?? fallback }} or
// {{ name | "default" }}. A tag may absorb the whitespace around it with
// {{- ... -}} (or {{~ ... ~}}), so conditional markup does not leave blank
// lines. These forms compose, so a page can be built from small partials
// rather than one giant string.
func templateFill(source, pairs string) string {
	names, values := decodeTemplatePairs(pairs)
	return renderTemplate(source, names, values, 0)
}

// templateMaxDepth bounds partial and block nesting, so a template that
// includes itself stops rather than exhausting the stack.
const templateMaxDepth = 64

// trimTemplateWhitespace removes the whitespace a tag asks to absorb. A tag
// written {{- ... }} absorbs the whitespace before it, and ... -}} absorbs the
// whitespace after it. The same markers work on comments and block tags, and
// "~" is accepted in place of "-". The markers are removed before rendering,
// so the rest of the engine only ever sees ordinary tags.
func trimTemplateWhitespace(source string) string {
	var out strings.Builder
	i := 0
	for i < len(source) {
		open := strings.Index(source[i:], "{{")
		comment := strings.Index(source[i:], "{#")
		if open < 0 && comment < 0 {
			out.WriteString(source[i:])
			break
		}
		at := 0
		isComment := false
		if comment >= 0 && (open < 0 || comment < open) {
			at = i + comment
			isComment = true
		} else {
			at = i + open
		}
		rest := source[at:]
		opener := "{{"
		closer := "}}"
		if isComment {
			opener = "{#"
			closer = "#}"
		} else if strings.HasPrefix(rest, "{{{") {
			opener = "{{{"
			closer = "}}}"
		}
		openLen := len(opener)
		closeOffset := strings.Index(rest[openLen:], closer)
		if closeOffset < 0 {
			out.WriteString(source[i:])
			break
		}
		closeAt := openLen + closeOffset
		inner := rest[openLen:closeAt]
		leftTrim := len(inner) > 0 && (inner[0] == '-' || inner[0] == '~')
		rightTrim := len(inner) > 0 && (inner[len(inner)-1] == '-' || inner[len(inner)-1] == '~')
		if leftTrim {
			inner = inner[1:]
		}
		if rightTrim && len(inner) > 0 {
			inner = inner[:len(inner)-1]
		}
		text := source[i:at]
		if leftTrim {
			text = strings.TrimRight(text, " \t\r\n")
		}
		out.WriteString(text)
		out.WriteString(opener)
		out.WriteString(inner)
		out.WriteString(closer)
		i = at + closeAt + len(closer)
		if rightTrim {
			for i < len(source) && (source[i] == ' ' || source[i] == '\t' || source[i] == '\r' || source[i] == '\n') {
				i = i + 1
			}
		}
	}
	return out.String()
}

// renderTemplate renders one template body against the supplied pairs. It
// recurses for partials and blocks, and for a loop's body once per item.
func renderTemplate(source string, names []string, values []string, depth int) string {
	if depth > templateMaxDepth {
		return source
	}
	source = trimTemplateWhitespace(source)
	var out strings.Builder
	i := 0
	for i < len(source) {
		open := strings.Index(source[i:], "{{")
		comment := strings.Index(source[i:], "{#")
		// A comment carries no value and disappears. It is found by its own
		// opener, because "{#" does not begin with "{{".
		if comment >= 0 && (open < 0 || comment < open) {
			start := i + comment
			out.WriteString(source[i:start])
			rest := source[start:]
			close := strings.Index(rest[2:], "#}")
			if close < 0 {
				out.WriteString(source[start:])
				break
			}
			i = start + 2 + close + 2
			continue
		}
		if open < 0 {
			out.WriteString(source[i:])
			break
		}
		start := i + open
		out.WriteString(source[i:start])

		rest := source[start:]

		// A partial inserts another carried template, rendered against the
		// same values, so a page can be built from small pieces.
		if strings.HasPrefix(rest, "{{>") {
			close := strings.Index(rest, "}}")
			if close < 0 {
				out.WriteString(source[start:])
				break
			}
			key := strings.TrimSpace(rest[3:close])
			if tmpl, ok := rhoTemplates[key]; ok {
				out.WriteString(renderTemplate(tmpl, names, values, depth+1))
			}
			i = start + close + 2
			continue
		}

		// A block directive opens a body, closes with {{/kind}}, and may carry
		// an {{else}} branch. The matching close is the one at the same depth,
		// so a block of one kind can nest inside another of the same kind.
		if strings.HasPrefix(rest, "{{#") {
			close := strings.Index(rest, "}}")
			if close < 0 {
				out.WriteString(source[start:])
				break
			}
			directive := strings.TrimSpace(rest[3:close])
			fields := strings.Fields(directive)
			kind := ""
			key := ""
			if len(fields) > 0 {
				kind = fields[0]
			}
			if len(fields) > 1 {
				key = fields[1]
			}
			bodyStart := close + 2
			inner, otherwise, hasElse, consumed, ok := templateBlock(rest[bodyStart:], kind)
			if !ok {
				out.WriteString(source[start:])
				break
			}

			switch kind {
			case "each":
				out.WriteString(templateEach(inner, otherwise, hasElse, key, names, values, depth))
			case "with":
				out.WriteString(templateWith(inner, otherwise, hasElse, key, names, values, depth))
			default:
				cond := false
				if value, found := lookupPair(names, values, key); found {
					cond = value != ""
				}
				if kind == "unless" {
					cond = !cond
				}
				if cond {
					out.WriteString(renderTemplate(inner, names, values, depth+1))
				} else if hasElse {
					out.WriteString(renderTemplate(otherwise, names, values, depth+1))
				}
			}
			i = start + bodyStart + consumed
			continue
		}

		// A placeholder: {{ name }} is escaped, {{{ name }}} is verbatim.
		raw := false
		openWidth := 2
		if strings.HasPrefix(rest, "{{{") {
			raw = true
			openWidth = 3
		}
		close := strings.Index(rest[openWidth:], "}}")
		if close < 0 {
			out.WriteString(source[start:])
			break
		}
		rawKey := strings.TrimSpace(rest[openWidth : openWidth+close])
		if rawKey == "t" || strings.HasPrefix(rawKey, "t ") {
			out.WriteString(templateTranslate(rawKey, names, values))
		} else {
			key := rawKey
			fallback := ""
			hasFallback := false
			if idx := strings.Index(rawKey, "??"); idx >= 0 {
				key = strings.TrimSpace(rawKey[:idx])
				fallback = strings.TrimSpace(rawKey[idx+2:])
				hasFallback = true
			} else if idx := strings.Index(rawKey, "|"); idx >= 0 {
				key = strings.TrimSpace(rawKey[:idx])
				fallback = strings.TrimSpace(rawKey[idx+1:])
				hasFallback = true
			}
			// unquote a quoted fallback
			if hasFallback && len(fallback) >= 2 && (fallback[0] == '"' || fallback[0] == '\'') {
				fallback = fallback[1 : len(fallback)-1]
			}
			if !strings.HasPrefix(key, "#") {
				value, found := lookupPair(names, values, key)
				if !found || value == "" {
					if hasFallback {
						value = fallback
						found = true
					}
				}
				if found {
					if raw {
						out.WriteString(value)
					} else {
						out.WriteString(escapeHtml(value))
					}
				}
			}
		}
		closeWidth := 2
		if raw && openWidth+close+2 < len(rest) && rest[openWidth+close+2] == '}' {
			closeWidth = 3
		}
		i = start + openWidth + close + closeWidth
	}
	return out.String()
}

// templateTranslate resolves {{t "key" name=value}}. The active language is
// the reserved __locale value, falling back to the compiled default. A missing
// key prints the key, so a missing translation is visible rather than blank.
// Substituted values are HTML-escaped; the catalog itself is trusted markup
// only as text.
func templateTranslate(expression string, names []string, values []string) string {
	locale := i18nDefaultLocale
	if value, found := lookupPair(names, values, "__locale"); found && value != "" {
		locale = value
	}
	rest := strings.TrimSpace(strings.TrimPrefix(expression, "t"))
	key, rest := templateQuoted(rest)
	if key == "" {
		return ""
	}
	text, found := i18nText(locale, key)
	if !found {
		return key
	}
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			break
		}
		equals := strings.Index(rest, "=")
		if equals < 0 {
			break
		}
		name := strings.TrimSpace(rest[:equals])
		rest = rest[equals+1:]
		if name == "" {
			break
		}
		value, remaining := templateTranslationValue(rest, names, values)
		text = strings.ReplaceAll(text, "{{"+name+"}}", escapeHtml(value))
		rest = remaining
	}
	return text
}

// templateQuoted reads a single- or double-quoted token and returns it with
// the rest of the expression.
func templateQuoted(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	quote := value[0]
	if quote != '"' && quote != '\'' {
		return "", value
	}
	var out strings.Builder
	index := 1
	for index < len(value) {
		ch := value[index]
		if ch == '\\' && index+1 < len(value) {
			index = index + 1
			out.WriteByte(value[index])
			index = index + 1
			continue
		}
		if ch == quote {
			return out.String(), value[index+1:]
		}
		out.WriteByte(ch)
		index = index + 1
	}
	return "", value
}

// templateTranslationValue reads a quoted literal or a variable name and
// returns its value with the rest of the expression.
func templateTranslationValue(rest string, names []string, values []string) (string, string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}
	if rest[0] == '"' || rest[0] == '\'' {
		return templateQuoted(rest)
	}
	end := 0
	for end < len(rest) {
		ch := rest[end]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			break
		}
		end = end + 1
	}
	name := rest[:end]
	value := ""
	if found, ok := lookupPair(names, values, name); ok {
		value = found
	}
	return value, rest[end:]
}

// templateBlock splits a block body at the {{else}} that belongs to it and
// reports where its own close tag ends. Nesting is tracked for every kind, so
// an else inside a nested block is not mistaken for the outer one.
func templateBlock(body string, kind string) (inner string, otherwise string, hasElse bool, consumed int, ok bool) {
	stack := []string{kind}
	elseAt := -1
	elseStart := -1
	i := 0
	for i < len(body) {
		open := strings.Index(body[i:], "{{")
		comment := strings.Index(body[i:], "{#")
		if comment >= 0 && (open < 0 || comment < open) {
			close := strings.Index(body[i+comment+2:], "#}")
			if close < 0 {
				return "", "", false, 0, false
			}
			i = i + comment + 2 + close + 2
			continue
		}
		if open < 0 {
			return "", "", false, 0, false
		}
		at := i + open
		rest := body[at:]
		close := strings.Index(rest, "}}")
		if close < 0 {
			return "", "", false, 0, false
		}
		token := strings.TrimSpace(rest[2:close])
		switch {
		case strings.HasPrefix(token, "#"):
			name := ""
			if fields := strings.Fields(strings.TrimSpace(token[1:])); len(fields) > 0 {
				name = fields[0]
			}
			if name != "" {
				stack = append(stack, name)
			}
		case strings.HasPrefix(token, "/"):
			name := strings.TrimSpace(token[1:])
			if name == "" {
				break
			}
			found := false
			for j := len(stack) - 1; j >= 0; j-- {
				if stack[j] == name {
					stack = stack[:j]
					found = true
					break
				}
			}
			if !found {
				break
			}
			if len(stack) == 0 {
				end := at
				if hasElse {
					return body[:elseAt], body[elseStart:end], true, at + close + 2, true
				}
				return body[:end], "", false, at + close + 2, true
			}
		case token == "else":
			if len(stack) == 1 && elseAt < 0 {
				elseAt = at
				elseStart = at + close + 2
				hasElse = true
			}
		}
		i = at + close + 2
	}
	return "", "", false, 0, false
}

// templateEach renders a loop body once per line of the named value. The item
// bindings are placed before the outer names, so an inner loop shadows an
// outer one and the index helpers describe the innermost item.
func templateEach(inner string, otherwise string, hasElse bool, key string, names []string, values []string, depth int) string {
	list := ""
	if value, found := lookupPair(names, values, key); found {
		list = value
	}
	items := templateLines(list)
	if len(items) == 0 {
		if hasElse {
			return renderTemplate(otherwise, names, values, depth+1)
		}
		return ""
	}
	var out strings.Builder
	for index, item := range items {
		first := ""
		if index == 0 {
			first = "true"
		}
		last := ""
		if index == len(items)-1 {
			last = "true"
		}
		augNames := []string{".", "this", "@index", "@index1", "@first", "@last", "@count"}
		augValues := []string{item, item, fmt.Sprintf("%d", index), fmt.Sprintf("%d", index+1), first, last, fmt.Sprintf("%d", len(items))}
		// A tab-separated item can be read field by field, so a listing of
		// rows does not need a Rho loop to be turned into markup.
		for fieldIndex, fieldValue := range strings.Split(item, "\t") {
			augNames = append(augNames, fmt.Sprintf("@%d", fieldIndex))
			augValues = append(augValues, fieldValue)
		}
		augNames = append(augNames, names...)
		augValues = append(augValues, values...)
		out.WriteString(renderTemplate(inner, augNames, augValues, depth+1))
	}
	return out.String()
}

// templateWith scopes lookups to prefix.* inside its body, or binds {{this}}
// to the named value when it is a plain value rather than a prefix. Scoped
// names come first, so they shadow the outer ones; an outer name remains
// reachable when the scope does not define it.
func templateWith(inner string, otherwise string, hasElse bool, key string, names []string, values []string, depth int) string {
	prefix := key + "."
	scopedNames := []string{}
	scopedValues := []string{}
	for index, name := range names {
		if strings.HasPrefix(name, prefix) {
			scopedNames = append(scopedNames, name[len(prefix):])
			if index < len(values) {
				scopedValues = append(scopedValues, values[index])
			} else {
				scopedValues = append(scopedValues, "")
			}
		}
	}
	if value, found := lookupPair(names, values, key); found {
		scopedNames = append(scopedNames, ".", "this")
		scopedValues = append(scopedValues, value, value)
	}
	if len(scopedNames) == 0 {
		if hasElse {
			return renderTemplate(otherwise, names, values, depth+1)
		}
		return ""
	}
	allNames := append([]string{}, scopedNames...)
	allValues := append([]string{}, scopedValues...)
	allNames = append(allNames, names...)
	allValues = append(allValues, values...)
	return renderTemplate(inner, allNames, allValues, depth+1)
}

// templateLines splits a loop value into its items. A trailing newline is a
// terminator rather than an empty item, and CRLF line endings are accepted.
func templateLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// decodeTemplatePairs splits an encoded list into parallel name and value
// slices. The first field is empty, because every pair is preceded by the
// separator, so names land on odd positions.
func decodeTemplatePairs(pairs string) ([]string, []string) {
	fields := strings.Split(pairs, templateSep)
	names := []string{}
	values := []string{}
	for i := 1; i+1 < len(fields); i += 2 {
		names = append(names, fields[i])
		values = append(values, fields[i+1])
	}
	return names, values
}

// lookupPair finds a name among the supplied pairs.
func lookupPair(names []string, values []string, key string) (string, bool) {
	for i, name := range names {
		if name == key && i < len(values) {
			return values[i], true
		}
	}
	return "", false
}

// escapeHtml makes a value safe to place in element text or an attribute.
func escapeHtml(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		case '\'':
			out.WriteString("&#39;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

`
}

// irModuleForBanner is the module identity used in the generated header.
func irModuleForBanner() ir.ModuleIR { return ir.ModuleIR{Name: "templates"} }

// goQuoted renders a template as a Go string literal, so the generated file
// carries the markup verbatim.
func goQuoted(content string) string {
	// A backquoted literal cannot contain a backquote, so fall back to an
	// interpreted one when the template uses one.
	if !strings.Contains(content, "`") {
		return "`" + content + "`"
	}
	return fmt.Sprintf("%q", content)
}
