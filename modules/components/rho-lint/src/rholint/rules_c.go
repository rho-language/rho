//
// @file rules_c.go
// @brief The rules over the C a build compiles.
//
// This engine is what makes one component cover both kinds of project. A Rho
// application and a hand-written C program differ in the language a person wrote
// them in and in nothing else a lint run can see: after codegen both are a set of C
// files and a compilation database, and both are read here by the same rules against
// the same findings. That is why `examples/c/hello_world` and
// `examples/rho/hello_world` are covered by one engine rather than two — the second
// one simply also arrives with its source.
//
// What it is *not* is a C front end. Everything below is decidable from the text,
// which is why no rule here needs the preprocessor, a symbol table or a type
// inference pass — and which is also why nothing here overlaps with what clang-tidy
// and cppcheck exist for. These are the checks that survive not knowing what the code
// means, and each of them is a defect the compiler does not report because the
// compiler was asked to compile, not to notice.
//
// Two decisions shape the scans below, and both of them exist because of a specific
// false positive:
//
// A comment and a string literal are blanked rather than removed, and the result is
// the same length as the file. Every finding's position is then an offset into that
// text read as a line and a column in the original — which matters because a header's
// banner is a comment, so a scan that cannot tell code from banner cannot tell where
// the code starts.
//
// The scans walk the whole text rather than line by line. A function's brace is very
// often on the line after its parameter list, which is the ordinary style in C and
// would hide every `static inline` function in a header — the single most common
// thing the header rules are for.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"fmt"
	"sort"
	"strings"
)

// The rules over C. Every name says which engine it is from.
const (
	// CUnbalancedBraces is a source whose braces do not balance.
	CUnbalancedBraces = "c-unbalanced-braces"
	// CDefinitionInHeader is a function defined in a header.
	CDefinitionInHeader = "c-definition-in-header"
	// CMissingIncludeGuard is a header with declarations and no guard.
	CMissingIncludeGuard = "c-missing-include-guard"
	// CStaticInHeader is a `static` definition in a header.
	CStaticInHeader = "c-static-in-header"
	// CTodoComment is a marker a person left in a comment.
	CTodoComment = "c-todo-comment"
	// CEmptyBody is a function whose body is empty.
	CEmptyBody = "c-empty-body"
)

// CRules is what the `c` engine checks.
//
// Ordered by how much a finding costs to be wrong about: the thing that stops every
// other analysis, then the things that are wrong, then the things worth knowing.
func CRules() RuleSet {
	return RuleSet{
		Name:     EngineC,
		Language: LanguageC,
		Rules: []Check{
			{
				ID:          CUnbalancedBraces,
				Severity:    SeverityError,
				Description: "A source whose braces do not balance, which every other rule here then reads wrongly",
				Apply:       cUnbalancedBraces,
			},
			{
				ID:          CDefinitionInHeader,
				Severity:    SeverityError,
				Description: "A function defined in a header, which every translation unit including it defines again",
				Apply:       cDefinitionInHeader,
			},
			{
				ID:          CMissingIncludeGuard,
				Severity:    SeverityWarning,
				Description: "A header with declarations in it and no include guard",
				Apply:       cMissingIncludeGuard,
			},
			{
				ID:          CStaticInHeader,
				Severity:    SeverityWarning,
				Description: "A static definition in a header, which every translation unit gets its own copy of",
				Apply:       cStaticInHeader,
			},
			{
				ID:          CTodoComment,
				Severity:    SeverityNote,
				Description: "A TODO, FIXME, XXX or HACK somebody left in a comment",
				Apply:       cTodo,
			},
			{
				ID:          CEmptyBody,
				Severity:    SeverityNote,
				Description: "A function whose body is empty, which reads as one that was meant to do something",
				Apply:       cEmptyBody,
			},
		},
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading the text                                                            */
/* -------------------------------------------------------------------------- */

// cleaned is a source with its comments and string literals blanked to spaces.
//
// Same length as the original, and that is the load-bearing property: everything
// downstream positions itself by byte offset into this and then reads the line and
// column out of the *original*. A shorter cleaned text would put every offset after
// the first comment in the wrong place, and every finding after it would point at
// the wrong line of a file that has comments in it — which is every header in an
// embedded project.
//
// Newlines are kept so that a line survives as a line and a line count in this text
// is a line number in the file.
func cleaned(source Source) string {
	out := []byte(source.Text)
	blanks := make([]bool, len(out))

	quotedHere := false
	escaped := false
	blockComment := false
	for index := 0; index < len(out); index++ {
		switch {
		case blockComment:
			blanks[index] = out[index] != '\n'
			if out[index] == '*' && index+1 < len(out) && out[index+1] == '/' {
				blanks[index+1] = true
				blockComment = false
				index++
			}
		case escaped:
			blanks[index] = out[index] != '\n'
			escaped = false
		case quotedHere && out[index] == '\\':
			blanks[index] = true
			escaped = true
		case out[index] == '"':
			quotedHere = !quotedHere
			blanks[index] = true
		case quotedHere:
			blanks[index] = out[index] != '\n'
		case index+1 < len(out) && out[index] == '/' && out[index+1] == '*':
			blanks[index] = true
			blockComment = true
		case index+1 < len(out) && out[index] == '/' && out[index+1] == '/':
			for index < len(out) && out[index] != '\n' {
				blanks[index] = true
				index++
			}
			index--
		}
	}
	for index := range out {
		if blanks[index] {
			out[index] = ' '
		}
	}
	return string(out)
}

// place is an offset in a cleaned text, as a line and a column in the original.
type place struct {
	Line   int
	Column int
}

// placeAt is an offset as a line and a column.
//
// Read out of the original text rather than counted in the cleaned one, because the
// two are the same length but a column in a report has to be a column a person can
// see in their editor — and in their editor the comment is still there.
func placeAt(source Source, offset int) place {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source.Text) {
		offset = len(source.Text)
	}
	line := strings.Count(source.Text[:offset], "\n") + 1
	lastNewline := strings.LastIndex(source.Text[:offset], "\n")
	return place{Line: line, Column: offset - lastNewline}
}

/* -------------------------------------------------------------------------- */
/*  Braces                                                                     */
/* -------------------------------------------------------------------------- */

// cUnbalancedBraces is a source whose braces do not balance.
//
// The first rule because it is the one that invalidates the others: the header and
// body scans below decide what is at brace depth zero by counting braces, and a file
// where the count never returns to zero makes every one of them answer about a
// structure that is not there. Reporting it as an error rather than letting the others
// misfire is the difference between a run that says something is wrong and a run that
// invents findings.
//
// An error rather than a warning because this is what the compiler reports too, at
// the same place — and the compiler's version stops at the first one.
func cUnbalancedBraces(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CUnbalancedBraces)

	code := cleaned(source)
	depth := 0
	openedAt := 0
	for index := 0; index < len(code); index++ {
		switch code[index] {
		case '{':
			if depth == 0 {
				openedAt = index
			}
			depth++
		case '}':
			depth--
			if depth < 0 {
				at := placeAt(source, index)
				return []Diagnostic{set.Report(rule, source, at.Line, at.Column,
					"this closes a brace that was never opened")}
			}
		}
	}
	if depth > 0 {
		at := placeAt(source, openedAt)
		return []Diagnostic{set.Report(rule, source, at.Line, at.Column, fmt.Sprintf(
			"there are %s open at the end of this file, so whatever is inside them is not what "+
				"it looks like", Plural(depth, "brace")))}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Headers                                                                    */
/* -------------------------------------------------------------------------- */

// IsHeader reports whether a path is a C header, which is what the header rules are
// for. The set is the one CMake and every C compiler agree on.
func IsHeader(path string) bool {
	return HasSuffix(path, ".h", ".hh", ".hpp", ".hxx", ".inl")
}

// cDefinitionInHeader is a function *defined* in a header.
//
// An error, and it is the only rule here that is about a build failing rather than
// about code being wrong. A definition in a header is emitted into every translation
// unit that includes it, so two units produce two copies of the same symbol and the
// link fails with a duplicate — in a project where the header is included by eleven
// units, at a link step, with a message naming a symbol rather than the header.
//
// The distinction that matters is definition against declaration: `void f(void);` in
// a header is correct and is the ordinary case. A body is what makes it a definition,
// which is why this looks for the brace after the parameter list and not for the
// parameter list.
//
// A `static` definition is left to the static rule, and not because two findings for
// one line is too noisy. A static definition does not produce a duplicate symbol —
// every translation unit gets its own copy, with internal linkage, and the link
// succeeds — so the sentence this rule writes would be false about it. Two claims
// about a line is normal; one of them being wrong is not.
func cDefinitionInHeader(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CDefinitionInHeader)
	if !IsHeader(source.Path) {
		return nil
	}
	found := []Diagnostic{}
	for _, one := range definitionsIn(source, "") {
		if one.Name == "" || declaredWeak(cleaned(source), one.Offset) ||
			hasStorageClass(cleaned(source), one.Offset, one.Body, "static") {
			continue
		}
		found = append(found, set.Report(rule, source, one.At.Line, one.At.Column, fmt.Sprintf(
			"%s defines %s here, so every translation unit that includes this header defines it again",
			headerName(source.Path), one.Name)))
	}
	return found
}

// cStaticInHeader is a `static` definition in a header.
//
// A warning rather than an error, because it links. A `static` symbol in a header is
// emitted into every unit that includes it, each with internal linkage and its own
// copy — and that is a real defect in its own right: a `static` buffer is duplicated
// per unit, and a `static` function is duplicated per unit so the compiler drops the
// ones nothing calls, which is how a debug build stops being the build you debugged.
//
// Functions and objects are reported by one rule with different sentences, because the
// fixes are different — one is "move it to a .c", the other is "make it a declaration
// and put the definition in a .c" — and a report that merged them would give somebody
// the wrong one.
func cStaticInHeader(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CStaticInHeader)
	if !IsHeader(source.Path) {
		return nil
	}
	// Both scans, then one list in file order — because the two are two shapes of one
	// rule rather than two rules, and a report that listed every function before every
	// object would read as though the objects were all at the end of the file.
	located := definitionsIn(source, "static")
	for _, one := range staticsIn(source) {
		located = append(located, one)
	}
	sort.Slice(located, func(i, j int) bool {
		if located[i].At.Line != located[j].At.Line {
			return located[i].At.Line < located[j].At.Line
		}
		return located[i].At.Column < located[j].At.Column
	})

	found := []Diagnostic{}
	for _, one := range located {
		if one.Name == "" {
			continue
		}
		message := fmt.Sprintf(
			"%s is static here, so every translation unit that includes this header gets its own copy",
			one.Name)
		if one.Body < 0 {
			message = fmt.Sprintf(
				"%s is a static object here, so every translation unit that includes this header "+
					"gets its own copy", one.Name)
		}
		found = append(found, set.Report(rule, source, one.At.Line, one.At.Column, message))
	}
	return found
}

// cEmptyBody is a function whose body is empty.
//
// A note, and only for a definition in a source file — not a declaration, and not a
// header, because a header that declares a function and does not define it is correct
// and saying otherwise would be a rule that fires on every interface header in a
// vendor tree.
//
// An empty body is legitimate in exactly one shape, an `extern "C"` shim whose
// implementation is supplied elsewhere. That shape is not detected, and the rule says
// so in its own description rather than pretending: a false positive here costs a
// `// NOLINT`, and a false negative costs a function that silently does nothing.
func cEmptyBody(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CEmptyBody)
	if IsHeader(source.Path) || !HasSuffix(source.Path, ".c", ".cpp", ".cc") {
		return nil
	}
	code := cleaned(source)
	found := []Diagnostic{}
	for _, one := range definitionsIn(source, "") {
		if one.Name == "" || !emptyBodyAt(code, one.Body) {
			continue
		}
		// A `weak` definition with an empty body is the C idiom, not a stub: it
		// exists so a strong definition elsewhere can override it, and it does
		// nothing when nothing does. `rho`'s own generated runtime has four of them —
		// `_init` and `_fini` in a bare-metal build, where `-nostartfiles` excludes
		// the C library's own and the startup reaches them through the library's
		// initializer. Reporting those would be four findings per Rho project about
		// code nobody wrote, in a file that is not supposed to have anything in it.
		if declaredWeak(code, one.Offset) {
			continue
		}
		found = append(found, set.Report(rule, source, one.At.Line, one.At.Column, fmt.Sprintf(
			"%s has an empty body, which is either a stub or a body that was meant to be written",
			one.Name)))
	}
	return found
}

// declaredWeak reports whether a declaration carries `__attribute__((weak))`.
//
// Read backwards from the name rather than forwards from it, because that is where
// C puts an attribute: `__attribute__((weak)) void _init(void) {}` has it in front of
// the whole declarator. The search stops at the previous statement boundary so a
// `weak` belonging to whatever came before cannot make this declaration look weak.
func declaredWeak(code string, at int) bool {
	if at < 0 || at > len(code) {
		return false
	}
	for index := at - 1; index >= 0; index-- {
		switch code[index] {
		case ';', '{', '}':
			// The first boundary going backwards *is* the start of this declaration,
			// and the search stops there. Carrying on past it would find the `weak` of
			// whatever came before and make this declaration look weak too.
			return strings.Contains(code[index+1:at], "weak")
		}
	}
	return strings.Contains(code[:at], "weak")
}

// cMissingIncludeGuard is a header with declarations in it and no guard.
//
// The most common real defect in embedded C and the one most easily got wrong by a
// scan, so the two decisions are stated here rather than left in the code.
//
// `#pragma once` counts, and is checked first: a project that has adopted it has said
// what it means, and a rule that reported it would be telling the project it did the
// wrong thing.
//
// A header with nothing in it does not need a guard and gets no finding. Otherwise
// every forward-declaration header in a vendor tree is a finding, and an STM32 project
// has thousands of those — a rule that fires that often is a rule that gets turned off.
func cMissingIncludeGuard(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CMissingIncludeGuard)
	if !IsHeader(source.Path) {
		return nil
	}
	lines := strings.Split(cleaned(source), "\n")
	if !declaresSomething(lines) {
		return nil
	}
	if pragmaOnce(lines) {
		return nil
	}
	if includeGuard(lines) {
		return nil
	}
	// Pointed at the first declaration rather than at line one, because line one of a
	// generated header is a banner and telling somebody their banner needs a guard is
	// not a finding.
	for number, line := range lines {
		if isDeclaration(line) {
			return []Diagnostic{set.Report(rule, source, number+1, 1, fmt.Sprintf(
				"%s has declarations in it and no include guard, so a second translation unit "+
					"including it will define them again", headerName(source.Path)))}
		}
	}
	return nil
}

// headerName is a header's file name, which is what a report names rather than its
// path: a person looking at this finding is looking at one file among many.
func headerName(path string) string {
	base := path
	if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
		base = base[slash+1:]
	}
	return base
}

/* -------------------------------------------------------------------------- */
/*  Declarations                                                               */
/* -------------------------------------------------------------------------- */

// declaresSomething reports whether a header declares anything at all.
func declaresSomething(lines []string) bool {
	for _, line := range lines {
		if isDeclaration(line) {
			return true
		}
	}
	return false
}

// isDeclaration reports whether a line declares something at file scope: a type, a
// function, an object, an enum, a struct, a union or a typedef.
//
// The keyword list rather than a grammar, and the list is the one C's own keywords
// allow before a declarator. A bare `rh_u32 count;` with no keyword is covered by the
// two shapes below, which is the case a keyword list alone would miss and which is
// most of what a header declares.
func isDeclaration(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	for _, keyword := range []string{
		"extern ", "typedef ", "struct ", "union ", "enum ", "class ",
		"static ", "const ", "volatile ", "inline ", "register ",
	} {
		if strings.HasPrefix(trimmed, keyword) {
			return true
		}
	}
	if strings.HasPrefix(trimmed, "extern") && len(trimmed) > len("extern") {
		return true
	}
	// `type name(` at file scope is a function declaration or definition, and the
	// name is the *last* word before the paren: `uint32_t led_pin(void);` declares
	// something and requiring all of `uint32_t led_pin` to be one identifier would say
	// it does not, which is the shape of most of what a header declares.
	if open := strings.Index(trimmed, "("); open > 0 {
		return isIdentifierish(lastWord(trimmed[:open]))
	}
	// `type name;` or `type name = value;` at file scope is an object declaration.
	if semicolon := strings.Index(trimmed, ";"); semicolon > 0 {
		name := trimmed[:semicolon]
		if star := strings.Index(name, "*"); star >= 0 {
			name = name[:star]
		}
		if equals := strings.Index(name, "="); equals >= 0 {
			name = name[:equals]
		}
		return isIdentifierish(lastWord(name))
	}
	return false
}

// lastWord is the last identifier in a text, which is where C puts a declarator's
// name: `uint32_t *const led` declares `led`.
func lastWord(text string) string {
	words := wordsOf(text)
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

func isIdentifierish(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if index := strings.IndexAny(trimmed, " \t"); index >= 0 {
		return false
	}
	for index, character := range trimmed {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character == '_', character == ':', character == '.':
		case index > 0 && character >= '0' && character <= '9':
		default:
			return false
		}
	}
	return true
}

func pragmaOnce(lines []string) bool {
	for _, line := range lines {
		fields := directiveFields(line)
		if len(fields) >= 2 && fields[0] == "pragma" && fields[1] == "once" {
			return true
		}
	}
	return false
}

// includeGuard reports whether the file wraps its declarations in `#ifndef` /
// `#define`.
//
// Checked as a pair appearing in that order with the same macro, and before any
// declaration rather than anywhere in the file — because a guard in the middle of a
// header is not a guard, and a scan that accepted one anywhere would call a header
// guarded when the second inclusion of it redefines everything above the `#ifndef`.
func includeGuard(lines []string) bool {
	ifndef := ""
	for _, line := range lines {
		fields := directiveFields(line)
		if len(fields) == 0 {
			if ifndef == "" && isDeclaration(line) {
				return false
			}
			continue
		}
		switch fields[0] {
		case "ifndef":
			if len(fields) != 2 || ifndef != "" {
				return false
			}
			ifndef = fields[1]
		case "define":
			if ifndef == "" || len(fields) < 2 || fields[1] != ifndef {
				return false
			}
			return true
		case "include":
			// An `#include` between the `#ifndef` and the `#define` is the ordinary
			// shape and says nothing about whether the guard is complete.
			if ifndef == "" {
				return false
			}
		default:
			return false
		}
	}
	return false
}

// directiveFields is a preprocessor line as its `#` and its words.
func directiveFields(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return nil
	}
	return strings.Fields(strings.TrimPrefix(trimmed, "#"))
}

/* -------------------------------------------------------------------------- */
/*  Finding definitions                                                        */
/* -------------------------------------------------------------------------- */

// definition is one definition found at file scope.
type definition struct {
	// Name is what is defined, or "" when the declaration names nothing — a brace
	// scope rather than a finding.
	Name string
	// At is where the name is written.
	At place
	// Offset is the same thing as a byte offset, which is what the attribute search
	// needs — a position that cannot be turned back into an index is no use to it.
	Offset int
	// Body is the offset of the opening brace, or -1 for a definition with no body.
	Body int
}

// definitionsIn is the function *definitions* in a source, and `wanted` filters them
// by storage class.
//
// Filtered rather than one scan per rule because the two scans are the same scan: a
// `static` definition in a header is both a static definition and a definition in a
// header, and finding it once is both cheaper and more honest than finding it twice
// and then explaining the overlap in the report.
//
// Objects are not here. A `static` object in a header is the same defect and is
// reported by `staticsIn`, because it is a *different* defect: one is a copy per
// translation unit of something meant to be one, the other is a duplicate symbol at
// link time. Calling a data definition a "definition in a header" would hand a reader
// the wrong one of those two sentences.
func definitionsIn(source Source, wanted string) []definition {
	code := cleaned(source)
	found := []definition{}
	depth := 0
	for index := 0; index < len(code); index++ {
		switch code[index] {
		case '{':
			depth++
			continue
		case '}':
			depth--
			if depth < 0 {
				// The balance rule owns this file's problems; a scan that carried on
				// past one would report every brace after it at the wrong depth.
				return found
			}
			continue
		}
		if depth != 0 || !isWordStart(code, index) {
			continue
		}
		end := wordEnd(code, index)
		name := code[index:end]
		open := skipSpace(code, end)
		if open >= len(code) || code[open] != '(' {
			index = end - 1
			continue
		}
		close := matchParen(code, open)
		if close < 0 {
			index = end - 1
			continue
		}
		body := skipSpace(code, close+1)
		if body < len(code) && code[body] == '{' {
			if wanted == "" || hasStorageClass(code, index, body, wanted) {
				found = append(found, definition{
					Name: name, At: placeAt(source, index), Offset: index, Body: body,
				})
			}
		}
		index = close
	}
	return found
}

// staticsIn is the `static` objects a source declares at file scope.
//
// Separate from `definitionsIn` because an object has no brace to find and so never
// appears in that scan, while somebody who wrote `static uint32_t count;` in a header
// has committed exactly the defect the static rule is for.
func staticsIn(source Source) []definition {
	code := cleaned(source)
	found := []definition{}
	depth := 0
	for index := 0; index < len(code); index++ {
		switch code[index] {
		case '{':
			depth++
			continue
		case '}':
			depth--
			if depth < 0 {
				return found
			}
			continue
		}
		if depth != 0 || !isWordStart(code, index) {
			continue
		}
		end := wordEnd(code, index)
		if code[index:end] != "static" {
			index = end - 1
			continue
		}
		semicolon := strings.IndexByte(code[end:], ';')
		if semicolon < 0 {
			return found
		}
		if name := staticName(code[index : end+semicolon]); name != "" {
			at := end + strings.LastIndex(code[end:end+semicolon], name)
			found = append(found, definition{Name: name, At: placeAt(source, at), Offset: at, Body: -1})
		}
		index += end + semicolon
	}
	return found
}

// emptyBodyAt reports whether the body opening at an offset closes immediately.
func emptyBodyAt(code string, body int) bool {
	if body < 0 || body+1 >= len(code) {
		return false
	}
	after := skipSpace(code, body+1)
	return after < len(code) && code[after] == '}'
}

// hasStorageClass reports whether the declaration between two offsets carries a
// storage class.
//
// The whole declaration, from the statement boundary in front of the name to the
// opening brace behind it, rather than the words in front of the name alone. C puts
// the class in both places depending on the declarator — `static inline void bump`
// has it two words ahead of the name and `int (*get(void))(void) { }` has it after
// the parameter list — and reading only one direction misses the other. Here it
// misses `static inline`, which is the single most common thing the header rules
// are for.
func hasStorageClass(code string, nameAt int, bodyAt int, class string) bool {
	start := 0
	for index := nameAt - 1; index >= 0; index-- {
		switch code[index] {
		case ';', '{', '}':
			// The first boundary going backwards *is* the start of this declaration,
			// and the search stops there. Carrying on past it would find the `static`
			// of whatever came before and make this declaration look static too.
			start = index + 1
			index = 0
		}
		if start > 0 {
			break
		}
	}
	end := bodyAt
	if end < 0 || end > len(code) {
		end = len(code)
	}
	for _, word := range wordsOf(code[start:end]) {
		if word == class {
			return true
		}
	}
	return false
}

// isWordStart reports whether an offset begins an identifier.
//
// The preceding-character test is what keeps `RoboHubGPIO::write` and `stm32_gpio.c`
// from being split into two words: an identifier that is already part of one, by a
// `::` or a `.`, is not the start of a declaration.
func isWordStart(code string, index int) bool {
	if index < 0 || index >= len(code) {
		return false
	}
	character := code[index]
	letter := character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' || character == '_'
	if !letter {
		return false
	}
	if index > 0 {
		switch previous := code[index-1]; {
		case previous == '_', previous == ':', previous == '.':
			return false
		case previous >= 'a' && previous <= 'z',
			previous >= 'A' && previous <= 'Z',
			previous >= '0' && previous <= '9':
			return false
		}
	}
	return true
}

// wordEnd is the offset just past the identifier beginning at an offset.
func wordEnd(code string, index int) int {
	end := index
	for end < len(code) {
		character := code[end]
		if character == '_' || character == ':' || character == '.' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' {
			end++
			continue
		}
		break
	}
	return end
}

// wordsOf is the identifiers in a text, in order.
func wordsOf(text string) []string {
	out := []string{}
	for index := 0; index < len(text); {
		if !isWordStart(text, index) {
			index++
			continue
		}
		end := wordEnd(text, index)
		out = append(out, text[index:end])
		index = end
	}
	return out
}

// matchParen is the offset of the `)` closing the one at an open offset, or -1.
//
// Stopping at a semicolon is what keeps a macro invocation — `HAL_Init(&config);` —
// from being read as a definition: its parentheses do close, but a declaration that
// ends before them is not one. Without it, every configuration call in an embedded
// project would look like a function definition.
func matchParen(code string, open int) int {
	depth := 0
	for index := open; index < len(code); index++ {
		switch code[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		case ';':
			if depth == 1 {
				return -1
			}
		}
	}
	return -1
}

// skipSpace is the offset of the next character that is not whitespace.
func skipSpace(code string, from int) int {
	for from < len(code) {
		switch code[from] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			from++
			continue
		}
		break
	}
	return from
}

// staticName is the name in a static object declaration.
//
// The last identifier before the initialiser, skipping the type keywords — so
// `static const char *const name` reports `name` and not `const`.
func staticName(declaration string) string {
	rest := strings.TrimSpace(declaration)
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "static"))
	if rest == "" {
		return ""
	}
	if end := strings.IndexAny(rest, "=[;,("); end >= 0 {
		rest = rest[:end]
	}
	words := wordsOf(rest)
	for index := len(words) - 1; index >= 0; index-- {
		if isTypeWord(words[index]) {
			continue
		}
		return words[index]
	}
	return ""
}

// isTypeWord reports whether a word is a keyword rather than a name.
func isTypeWord(word string) bool {
	switch word {
	case "const", "volatile", "register", "extern", "static", "unsigned", "signed",
		"struct", "union", "enum", "inline", "auto":
		return true
	default:
		return false
	}
}

/* -------------------------------------------------------------------------- */
/*  Comments                                                                   */
/* -------------------------------------------------------------------------- */

// cTodo is a marker left in a comment.
//
// The same walk as the Rho rule and for the same reason: a `}` or a `//` inside a
// string literal must not be read as code, and `"set the TODO flag"` must not be a
// finding. Both languages share one implementation of the answer, because the answer
// is the same and two implementations of it would drift.
//
// Unlike Rho's, a block comment *is* scanned, and that is the one place the two
// differ. A Rho file carries its banner in `//` and its own markers in `//`, while a C
// file's banner is a `/** ... */` block and a marker is as likely to be inside one as
// beside it. A Rho rule that read block comments would fire on this toolchain's own
// generated headers.
func cTodo(source Source) []Diagnostic {
	set := CRules()
	rule, _ := set.Find(CTodoComment)
	found := []Diagnostic{}

	quotedNow := false
	escaped := false
	inBlockComment := false
	for index := 0; index < len(source.Text); index++ {
		character := source.Text[index]
		switch {
		case inBlockComment:
			if character == '*' && index+1 < len(source.Text) && source.Text[index+1] == '/' {
				inBlockComment = false
				index++
				continue
			}
			marker := todoMarker.FindStringIndex(source.Text[index:])
			if marker == nil {
				continue
			}
			at := placeAt(source, index+marker[0])
			found = append(found, set.Report(rule, source, at.Line, at.Column,
				fmt.Sprintf("%s left in a comment; it is either done or it is a comment",
					source.Text[index+marker[0]:index+marker[1]])))
			// Past the marker, and then on to the end of the comment or the end of the
			// line, whichever comes first. Both halves are needed: without the first,
			// this same marker is found again on the very next step of the loop; and
			// without the second, the walk would leave the comment and read the code
			// after it as though it were still commented.
			stop := index + marker[1]
			closed := false
			for stop < len(source.Text) && source.Text[stop] != '\n' {
				if source.Text[stop] == '*' && stop+1 < len(source.Text) && source.Text[stop+1] == '/' {
					stop += 2
					closed = true
					break
				}
				stop++
			}
			// The closing `*/` was stepped over rather than read by the loop above, so
			// the comment has to be closed here. Leaving it open would make every line
			// after this one part of the comment — and the marker after that is the
			// word in a string literal, which is exactly what this rule must not report.
			inBlockComment = !closed
			index = stop - 1
		case escaped:
			escaped = false
		case character == '\\' && quotedNow:
			escaped = true
		case character == '"':
			quotedNow = !quotedNow
		case quotedNow:
		case character == '/' && index+1 < len(source.Text) && source.Text[index+1] == '/':
			marker := todoMarker.FindStringIndex(source.Text[index+2:])
			if marker != nil {
				at := placeAt(source, index+2+marker[0])
				found = append(found, set.Report(rule, source, at.Line, at.Column,
					fmt.Sprintf("%s left in a comment; it is either done or it is a comment",
						source.Text[index+2+marker[0]:index+2+marker[1]])))
			}
			// To the end of the line, which is where a line comment ends.
			for index+1 < len(source.Text) && source.Text[index+1] != '\n' {
				index++
			}
		case character == '/' && index+1 < len(source.Text) && source.Text[index+1] == '*':
			inBlockComment = true
			index++
		}
	}
	return found
}
