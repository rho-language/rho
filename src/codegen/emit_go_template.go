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
// SPDX-License-Identifier: MIT
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
	b.WriteString("import \"strings\"\n\n")
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
func templateFill(source, pairs string) string {
	names, values := decodeTemplatePairs(pairs)
	var out strings.Builder
	for i := 0; i < len(source); {
		open := strings.Index(source[i:], "{{")
		if open < 0 {
			out.WriteString(source[i:])
			break
		}
		start := i + open
		out.WriteString(source[i:start])

		raw := false
		openWidth := 2
		rest := source[start+2:]
		if strings.HasPrefix(rest, "{") {
			// The triple-brace form asks for a value verbatim.
			raw = true
			openWidth = 3
			rest = rest[1:]
		}
		closeWidth := 2
		close := strings.Index(rest, "}}")
		if close < 0 {
			// An unterminated placeholder is emitted as written rather than
			// silently dropped, so a typo is visible in the page.
			out.WriteString(source[start:])
			break
		}

		key := strings.TrimSpace(rest[:close])
		// A comment carries no value and disappears.
		if !strings.HasPrefix(key, "#") {
			if value, ok := lookupPair(names, values, key); ok {
				if raw {
					out.WriteString(value)
				} else {
					out.WriteString(escapeHtml(value))
				}
			}
		}
		// The closing braces match the opening form, so a verbatim
		// placeholder is consumed whole rather than leaving a stray "}}".
		if raw && strings.HasPrefix(rest[close+2:], "}") {
			closeWidth = 3
		}
		i = start + openWidth + close + closeWidth
	}
	return out.String()
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

// escapeHTML makes a value safe to place in element text or an attribute.
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
