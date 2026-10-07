//
// @file goexpr.go
// @brief Translation of Rho IR expressions from C syntax to Go syntax.
//
// Rho IR carries expressions in the syntax the C backend consumes: C compound
// literals, C casts, snake_case runtime helpers and C library calls. Emitting
// Go means rewriting those constructs into their Go equivalents before the Go
// backend writes them out.
//
// The rewrites applied here are:
//
//	(PID){ a, b }        ->  PID{a, b}          compound literal
//	(int32_t)x           ->  int32(x)            C cast
//	rho_concat(a, b)     ->  (a + b)             string/number join
//	printf(f, a)         ->  fmt.Printf(f, a)    C stdio
//	clamp_f64(a, b, c)   ->  clampF64(a, b, c)   snake_case helper
//	filter.previous_err  ->  filter.previousErr  snake_case field
//	NULL                 ->  nil
//
// String literals are copied verbatim, so escapes and format specifiers keep
// working across both backends.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strconv"
	"strings"

	"rho/src/ir"
)

// goTypeNames maps C type spellings to their Go equivalents.
var goTypeNames = map[string]string{
	"void":       "",
	"bool":       "bool",
	"_Bool":      "bool",
	"char":       "int8",
	"signed":     "int32",
	"unsigned":   "uint32",
	"int":        "int32",
	"long":       "int64",
	"short":      "int16",
	"float":      "float32",
	"double":     "float64",
	"int8_t":     "int8",
	"int16_t":    "int16",
	"int32_t":    "int32",
	"int64_t":    "int64",
	"uint8_t":    "uint8",
	"uint16_t":   "uint16",
	"uint32_t":   "uint32",
	"uint64_t":   "uint64",
	"size_t":     "uint",
	"const char": "string",
}

// goCasts maps a C cast to the Go conversion to emit. A cast whose target is
// not listed is dropped rather than mistranslated.
var goCasts = map[string]string{
	"void":          "",
	"bool":          "bool",
	"_Bool":         "bool",
	"char":          "int8",
	"signed":        "int32",
	"unsigned":      "uint32",
	"int":           "int32",
	"long":          "int64",
	"short":         "int16",
	"float":         "float32",
	"double":        "float64",
	"int8_t":        "int8",
	"int16_t":       "int16",
	"int32_t":       "int32",
	"int64_t":       "int64",
	"uint8_t":       "uint8",
	"uint16_t":      "uint16",
	"uint32_t":      "uint32",
	"uint64_t":      "uint64",
	"size_t":        "uint",
	"const char *":  "string",
	"const char*":   "string",
	"char *":        "string",
	"char*":         "string",
	"unsigned char": "uint8",
	"unsigned int":  "uint32",
	"unsigned long": "uint64",
	"signed char":   "int8",
	"long int":      "int64",
	"long long":     "int64",
	"long double":   "float64",
	"struct char":   "int8",
	// The language's own scalar names. A lowering spells a type in the
	// language's own vocabulary, and the backend has to read that rather than
	// guess: `i64` and `long long` are the same type, and a file that dropped
	// one of them would build a conversion of a type nothing declares.
	"i8":   "int8",
	"i16":  "int16",
	"i32":  "int32",
	"i64":  "int64",
	"u8":   "uint8",
	"u16":  "uint16",
	"u32":  "uint32",
	"u64":  "uint64",
	"f32":  "float32",
	"f64":  "float64",
	"f128": "float64",
	// A pointer to nothing is a handle, and the backend gives it a nameable
	// type so a program can hold one. A cast written `void*` and left as a cast
	// is not a Go expression at all, and every call that carried a handle would
	// fail to parse.
	"void *":  "PtrUnit",
	"void*":   "PtrUnit",
	"Unit *":  "PtrUnit",
	"Unit*":   "PtrUnit",
	"PtrUnit": "PtrUnit",
}

// cLibraryCalls maps the helpers the C backend emits to their Go equivalents.
//
// `rho_print` is in this list because the C backend emits it for `print` — it is
// the runtime's own sink, written so that a firmware does not pay for the C
// library's formatted output. The Go backend has no such constraint and has always
// had a print, so it maps to `fmt.Print`, which adds no newline and is therefore
// the same function rather than an approximation of it.
var cLibraryCalls = map[string]string{
	"rho_print": "fmt.Print",
	"printf":    "fmt.Printf",
	"sprintf":   "fmt.Sprintf",
	"fprintf":   "fmt.Fprintf",
	"puts":      "fmt.Println",
	"putchar":   "fmt.Print",
}

// goKeywords are reserved words that must never be rewritten. They cannot
// appear as Rho identifiers, so they pass through untouched.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// goLiterals are predeclared constants that must pass through untouched.
var goLiterals = map[string]bool{
	"true": true, "false": true, "nil": true, "iota": true,
}

// goMustEscape are predeclared identifiers that break when a Rho name shadows
// them. `error` is the common case: the control-flow stdlib declares locals
// called `error`.
//
// Other predeclared names (len, min, new, and the type names) are
// deliberately left alone: Go permits shadowing them, and escaping would turn
// a genuine call to a builtin into an undefined function.
var goMustEscape = map[string]bool{
	"error":      true,
	"any":        true,
	"comparable": true,
	// `init` is reserved by Go: a package initializer cannot be called, and
	// referencing it as a function is an error. Rho uses `init` for startup
	// code, so the emitted function is renamed and the caller names it here.
	"init": true,
}

// goPassThroughWords keep their C spelling so the call translator can still
// recognise them. `rho_concat` is rewritten as a whole call, not a name.
var goPassThroughWords = map[string]bool{
	"rho_concat": true,
	"read_file":  true,
	"write_file": true,
	"get_args":   true,
	"append":     true,
	"len":        true,
}

// goBuiltinCalls maps a Rho builtin to the Go call that carries it. The file
// and argument builtins are supplied by the Go runtime, so a call site has to
// name the runtime function rather than the Rho spelling.
var goBuiltinCalls = map[string]string{
	"read_file":  "rhoReadFile",
	"write_file": "rhoWriteFile",
	"get_args":   "rhoGetArgs",
	"append":     "append",
}

// escapeGoName makes a Rho name safe to use in generated Go.
func escapeGoName(name string) string {
	if name == "" {
		return ""
	}
	if goKeywords[name] || goLiterals[name] {
		return name
	}
	if goMustEscape[name] {
		return name + "_"
	}
	return name
}

// ToGoExpr rewrites a C-syntax expression into Go syntax.
func ToGoExpr(expr string) string {
	if expr == "" {
		return ""
	}
	var out strings.Builder
	src := []byte(expr)

	for i := 0; i < len(src); {
		c := src[i]

		// String literal: copy verbatim so escapes survive untouched.
		if c == '"' || c == '\'' {
			end := scanStringLiteral(src, i)
			out.WriteString(string(src[i:end]))
			i = end
			continue
		}

		// Call or cast opening parenthesis.
		if c == '(' {
			if consumed, replacement, ok := translateParen(src, i); ok {
				out.WriteString(replacement)
				i = consumed
				continue
			}
			out.WriteByte(c)
			i++
			continue
		}

		// Identifier or keyword.
		if isIdentStart(c) {
			end := scanIdent(src, i)
			word := string(src[i:end])
			// A call that is rewritten as a whole must be consumed here,
			// while the name is still visible, not at its parenthesis.
			if goPassThroughWords[word] {
				if consumed, replacement, ok := translateKnownCall(src, i, end, word); ok {
					out.WriteString(replacement)
					i = consumed
					continue
				}
			}
			out.WriteString(translateWord(word))
			i = end
			continue
		}

		out.WriteByte(c)
		i++
	}

	return strings.TrimSpace(out.String())
}

// translateWord maps a single identifier to its Go spelling.
func translateWord(word string) string {
	if word == "NULL" {
		return "nil"
	}
	// Keywords and literals keep their spelling.
	if goKeywords[word] || goLiterals[word] {
		return word
	}
	// rho_concat is rewritten as a whole call by translateParen, so the name
	// has to survive long enough to be matched there.
	if goPassThroughWords[word] {
		return word
	}
	if mapped, ok := cLibraryCalls[word]; ok {
		return mapped
	}
	if mapped, ok := goTypeNames[word]; ok && mapped != "" {
		return mapped
	}
	// A predeclared identifier that Go cannot tolerate shadowed must be
	// escaped, otherwise the generated Go would not compile.
	if goMustEscape[word] {
		return word + "_"
	}
	// Only rename names that actually use underscores, so already-idiomatic
	// identifiers and exported type names are left untouched.
	if strings.Contains(word, "_") && !strings.HasPrefix(word, "__") {
		return lowerCamel(word)
	}
	return word
}

// translateKnownCall rewrites a whole call whose name is spelled in C. It is
// invoked at the identifier, before the name can be renamed.
func translateKnownCall(src []byte, start, nameEnd int, name string) (int, string, bool) {
	pos := nameEnd
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	if pos >= len(src) || src[pos] != '(' {
		return 0, "", false
	}
	close := matchParen(src, pos)
	if close < 0 {
		return 0, "", false
	}
	args := splitTopLevel(string(src[pos+1 : close]))
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, ToGoExpr(arg))
	}

	if name == "rho_concat" {
		// Go's + covers both the numeric and the string case that rho_concat
		// joined in C.
		return close + 1, "(" + strings.Join(parts, " + ") + ")", true
	}
	if name == "len" {
		// Rho's len is a u32; Go's is an int, so the result is converted. A
		// program that compares it with a u32 needs the same width on both
		// sides.
		return close + 1, "uint32(len(" + strings.Join(parts, ", ") + "))", true
	}
	if goName, ok := goBuiltinCalls[name]; ok {
		return close + 1, goName + "(" + strings.Join(parts, ", ") + ")", true
	}
	return 0, "", false
}

// translateParen inspects a '(' and decides whether it opens a C cast, a
// compound literal, or an ordinary grouping. It returns the replacement text
// and the index to continue scanning from.
func translateParen(src []byte, start int) (int, string, bool) {
	// Try to read a C type immediately after '('.
	typeStart := skipSpaces(src, start+1)
	if typeStart >= len(src) || !isIdentStart(src[typeStart]) {
		return 0, "", false
	}
	firstEnd := scanIdent(src, typeStart)
	first := string(src[typeStart:firstEnd])

	// Qualifiers and width words form one type: `unsigned char`, `const char`.
	typeName := first
	wordEnd := firstEnd
	if isCTypeQualifier(first) {
		next := skipSpaces(src, firstEnd)
		if next < len(src) && isIdentStart(src[next]) {
			secondEnd := scanIdent(src, next)
			wordEnd = secondEnd
			typeName = first + " " + string(src[next:secondEnd])
		}
	}

	// A pointer type such as `const char *`.
	sawPointer := false
	afterType := skipSpaces(src, wordEnd)
	if afterType < len(src) && src[afterType] == '*' {
		sawPointer = true
		afterType++
	}
	if sawPointer {
		typeName += " *"
	}

	if afterType >= len(src) || src[afterType] != ')' {
		return 0, "", false
	}
	afterCast := afterType + 1

	// Compound literal: (Type){ ... } becomes Type{ ... }.
	next := skipSpaces(src, afterCast)
	if next < len(src) && src[next] == '{' {
		goType := goCompoundType(typeName)
		if goType == "" {
			return 0, "", false
		}
		return afterCast, goType, true
	}

	// A cast to a struct is a literal, not a conversion: `PtrUnit(0)` would be
	// a call of a struct, which is not a thing. A handle is built from its
	// address, and the struct's one field is that address.
	if goStruct := goCompoundType(typeName); goStruct != "" && isStructCastTarget(typeName) {
		operandStart := skipSpaces(src, afterCast)
		operandEnd, ok := scanPrimaryExpr(src, operandStart)
		if !ok {
			return 0, "", false
		}
		operand := ToGoExpr(string(src[operandStart:operandEnd]))
		return operandEnd, goStruct + "{raw: " + operand + "}", true
	}

	// Plain cast: (Type)expr becomes GoType(expr). The operand is a single
	// primary expression, so the rewrite stays local and balanced.
	if conversion, ok := goCasts[typeName]; ok {
		if conversion == "" {
			// A void cast carries no Go equivalent; drop it.
			return afterCast, "", true
		}
		operandStart := skipSpaces(src, afterCast)
		operandEnd, ok := scanPrimaryExpr(src, operandStart)
		if !ok {
			return 0, "", false
		}
		operand := ToGoExpr(string(src[operandStart:operandEnd]))
		return operandEnd, conversion + "(" + operand + ")", true
	}

	return 0, "", false
}

// isStructCastTarget reports whether a cast to this type builds a struct rather
// than converting a value.
//
// A pointer is the one that matters: a Rho handle is a pointer to nothing, which
// the backend spells as a struct carrying the address, and `PtrUnit(0)` written
// as a conversion is a call of a struct — not a thing Go can express.
func isStructCastTarget(typeName string) bool {
	trimmed := strings.TrimSpace(typeName)
	switch trimmed {
	case "void *", "void*", "Unit *", "Unit*", "PtrUnit", "PtrVoid":
		return true
	}
	return strings.HasPrefix(trimmed, "Ptr[")
}

// isCTypeQualifier reports whether a C type word is only a qualifier or width
// modifier that must be followed by a real type.
func isCTypeQualifier(word string) bool {
	switch word {
	case "const", "volatile", "unsigned", "signed", "struct", "long", "short":
		return true
	default:
		return false
	}
}

func skipSpaces(src []byte, pos int) int {
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	return pos
}

// scanPrimaryExpr returns the end index of a single primary expression: an
// identifier, a numeric literal, or a parenthesised group. Member accesses
// and index expressions are consumed as part of the operand, because a cast
// applies to the whole `(int32_t)filter.index` rather than to `filter`.
func scanPrimaryExpr(src []byte, start int) (int, bool) {
	if start >= len(src) {
		return 0, false
	}

	var end int
	switch {
	case src[start] == '(':
		closed := matchParen(src, start)
		if closed < 0 {
			return 0, false
		}
		end = closed + 1
	case isIdentStart(src[start]):
		end = scanIdent(src, start)
	case src[start] >= '0' && src[start] <= '9':
		i := start
		for i < len(src) && (isIdentPart(src[i]) || src[i] == '.') {
			// A '.' followed by a digit is a decimal point, not a selector.
			if src[i] == '.' && (i+1 >= len(src) || src[i+1] < '0' || src[i+1] > '9') {
				break
			}
			i++
		}
		end = i
	default:
		return 0, false
	}

	// Absorb selector and index suffixes so the cast covers the full operand.
	for end < len(src) {
		switch src[end] {
		case '.':
			next := end + 1
			if next >= len(src) || !isIdentStart(src[next]) {
				return end, true
			}
			end = scanIdent(src, next)
		case '[':
			closed := matchBracket(src, end)
			if closed < 0 {
				return end, true
			}
			end = closed + 1
		case '(':
			// A call suffix is part of the primary expression: in C the cast
			// `(int32_t)len(data)` applies to the whole call, because a
			// postfix call binds tighter than a cast.
			closed := matchParen(src, end)
			if closed < 0 {
				return end, true
			}
			end = closed + 1
		default:
			return end, true
		}
	}
	return end, true
}

// matchBracket returns the index of the ']' closing the '[' at open, or -1.
func matchBracket(src []byte, open int) int {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '"', '\'':
			i = scanStringLiteral(src, i) - 1
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// goCompoundType maps a C compound-literal type to its Go spelling.
func goCompoundType(cType string) string {
	if mapped, ok := goTypeNames[cType]; ok {
		return mapped
	}
	if goCasts[cType] != "" {
		return goCasts[cType]
	}
	// A user struct, possibly a mangled generic specialisation, uses the same
	// spelling as its declaration and parameter references.
	if isValidGoIdent(cType) {
		return goTypeName(cType)
	}
	return ""
}

// matchCall reports whether src[start:] opens a call to the given name and
// returns the indices of its argument parentheses.
func matchCall(src []byte, start int, name string) (int, int, bool) {
	pos := start + 1
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	if !hasWordAt(src, pos, name) {
		return 0, 0, false
	}
	pos += len(name)
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	if pos >= len(src) || src[pos] != '(' {
		return 0, 0, false
	}
	end := matchParen(src, pos)
	if end < 0 {
		return 0, 0, false
	}
	return pos, end, true
}

func hasWordAt(src []byte, pos int, word string) bool {
	if pos+len(word) > len(src) {
		return false
	}
	if string(src[pos:pos+len(word)]) != word {
		return false
	}
	if pos+len(word) < len(src) && isIdentPart(src[pos+len(word)]) {
		return false
	}
	return true
}

// matchParen returns the index of the ')' closing the '(' at open, or -1.
func matchParen(src []byte, open int) int {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '"', '\'':
			i = scanStringLiteral(src, i) - 1
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits a comma-separated argument list, respecting nesting and
// string literals.
func splitTopLevel(args string) []string {
	out := []string{}
	depth := 0
	start := 0
	inString := byte(0)
	for i := 0; i < len(args); i++ {
		c := args[i]
		if inString != 0 {
			if c == '\\' {
				i++
			} else if c == inString {
				inString = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inString = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(args[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(args[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func scanStringLiteral(src []byte, start int) int {
	quote := src[start]
	for i := start + 1; i < len(src); i++ {
		if src[i] == '\\' {
			i++
			continue
		}
		if src[i] == quote {
			return i + 1
		}
	}
	return len(src)
}

func scanIdent(src []byte, start int) int {
	i := start
	for i < len(src) && isIdentPart(src[i]) {
		i++
	}
	return i
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// lowerCamel converts a snake_case identifier to lowerCamelCase.
func lowerCamel(name string) string {
	parts := strings.Split(name, "_")
	var b strings.Builder
	upperNext := false
	for _, part := range parts {
		if part == "" {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteString(strings.ToUpper(part[:1]))
			b.WriteString(part[1:])
			upperNext = false
			continue
		}
		if b.Len() == 0 {
			b.WriteString(part)
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

func isValidGoIdent(name string) bool {
	if name == "" || !isIdentStart(name[0]) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isIdentPart(name[i]) {
			return false
		}
	}
	return escapeGoName(name) == name
}

// goTypeForIR maps an IR type to its Go spelling.
func goTypeForIR(t ir.Type) string {
	// A fixed-size array `T[N]` is `[N]T` in Go. The length is carried on the
	// type, and the element type is the same type with the length cleared.
	if t.ArrayLen > 0 {
		element := t
		element.ArrayLen = 0
		if open := strings.IndexByte(t.Name, '['); open >= 0 {
			element.Name = t.Name[:open]
		}
		return "[" + strconv.Itoa(t.ArrayLen) + "]" + goTypeForIR(element)
	}
	switch t.Kind {
	case ir.TypeBool:
		return "bool"
	case ir.TypeU8:
		return "uint8"
	case ir.TypeU16:
		return "uint16"
	case ir.TypeU32:
		return "uint32"
	case ir.TypeU64:
		return "uint64"
	case ir.TypeI32:
		return "int32"
	case ir.TypeI64:
		return "int64"
	case ir.TypeF64:
		return "float64"
	case ir.TypeF32:
		return "float32"
	case ir.TypeString:
		return "string"
	case ir.TypeStruct:
		return goTypeName(t.Name)
	case ir.TypePtr:
		// A pointer to nothing is a handle, and it is a nameable struct rather
		// than a raw pointer: a program has to be able to hold one, pass it and
		// get one back, and `*Unit` is a pointer to an empty struct that a cast
		// of 0 could not build.
		if len(t.Args) == 0 || t.Args[0].Kind == ir.TypeVoid ||
			t.Args[0].Name == "Unit" || t.Args[0].Name == "" {
			return "PtrUnit"
		}
		element := goTypeForIR(t.Args[0])
		if element == "" {
			return "unsafe.Pointer"
		}
		return "*" + element
	case ir.TypeSlice:
		// A dynamic array is a Go slice. This is the difference from a pointer
		// that makes the two kinds worth telling apart.
		if len(t.Args) == 0 {
			return "[]byte"
		}
		element := goTypeForIR(t.Args[0])
		if element == "" {
			return "[]byte"
		}
		return "[]" + element
	default:
		return ""
	}
}

// sanitizeGoTypeName normalises a struct type name for Go use.
func sanitizeGoTypeName(name string) string {
	return strings.NewReplacer("[", "_", "]", "", ",", "_", " ", "").Replace(name)
}

// goTypeName normalises a declared type name for Go. A name that already reads
// as an identifier is kept, so exported types such as PID survive; a mangled
// generic name such as Result_u32_u32 is lower-camel-cased. Definitions and
// type references must both use this so they resolve to the same identifier.
func goTypeName(name string) string {
	if name == "" {
		return ""
	}
	return lowerCamel(sanitizeGoTypeName(name))
}

// goTypeForCName maps a C type spelling appearing in IR text to Go.
func goTypeForCName(name string) string {
	trimmed := strings.TrimSpace(name)
	if mapped, ok := goTypeNames[trimmed]; ok {
		return mapped
	}
	if converted, ok := goCasts[trimmed]; ok {
		return converted
	}
	return goTypeName(trimmed)
}

// goRhoPrimitives maps a Rho primitive as written to the Go type that carries
// it. The C spellings differ — Rho writes `u8`, C writes `uint8_t` — so a name
// that reaches the backend as written needs its own table.
var goRhoPrimitives = map[string]string{
	"void":   "",
	"Unit":   "",
	"bool":   "bool",
	"u8":     "uint8",
	"u16":    "uint16",
	"u32":    "uint32",
	"u64":    "uint64",
	"i32":    "int32",
	"i64":    "int64",
	"f64":    "float64",
	"string": "string",
}

// goTypeForRhoName maps a Rho type as written to the Go type that carries it.
//
// It exists for the spellings the C form loses. A dynamic array `[T]` is a Go
// slice, while a pointer `Ptr[T]` is `*T`; both are `T*` in C, so a backend
// handed only the C spelling would declare one as the other.
func goTypeForRhoName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if mapped, ok := goRhoPrimitives[trimmed]; ok {
		return mapped
	}
	// Fixed-size array: T[N] becomes [N]T. The brackets trail the element type,
	// which is what tells it from the dynamic form.
	if open := strings.IndexByte(trimmed, '['); open > 0 && strings.HasSuffix(trimmed, "]") {
		count := trimmed[open+1 : len(trimmed)-1]
		if _, err := strconv.Atoi(count); err == nil {
			element := goTypeForRhoName(trimmed[:open])
			if element == "" {
				element = "byte"
			}
			return "[" + count + "]" + element
		}
	}
	// Dynamic array: [T] becomes []T.
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		element := goTypeForRhoName(inner)
		if element == "" {
			element = goTypeForCName(inner)
		}
		if element == "" {
			return "[]byte"
		}
		return "[]" + element
	}
	return goTypeForCName(trimmed)
}
