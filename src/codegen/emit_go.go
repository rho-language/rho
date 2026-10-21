//
// @file emit_go.go
// @brief Rho compiler - Go emission backend.
//
// Emits a Go module from Rho IR. The backend mirrors the C backend's
// decomposition so both targets share the same project roles:
//
//	app.go      structs and functions (role: implementation)
//	runtime.go  runtime helpers actually referenced (role: runtime)
//	main.go     host entry point (role: entrypoint)
//	go.mod      module definition (role: build)
//
// Rho's `fn main() void` becomes AppMain so that main.go stays the only
// place that defines Go's `main`, which mirrors app_main in the C target.
//
// Expressions arrive in C syntax and are rewritten by ToGoExpr. The typechecker
// inserts explicit casts for mismatched numeric operands, so no implicit
// promotion is left to the host language.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/ir"
)

// AppMainName is the Go function generated from a Rho `main`.
const AppMainName = "AppMain"

// AppInitName is the Go function called before AppMain.
const AppInitName = "AppInit"

// EmitGo generates the implementation file (app.go) for an IR module.
func EmitGo(m ir.ModuleIR) string {
	return EmitGoForTarget(m, "cli")
}

// EmitGoForTarget generates the implementation file for a Go target. A server
// target receives Request and Response from its own server file, so those are
// not repeated here; every other target needs the definitions emitted.
func EmitGoForTarget(m ir.ModuleIR, kind string) string {
	isServer := kind == "server"
	var body strings.Builder

	if flags := goFeatureFlags(m.Features); len(flags) > 0 {
		body.WriteString("// Feature flags derived from the Rho manifest.\nconst (\n")
		for _, flag := range flags {
			fmt.Fprintf(&body, "\tRHO_FEATURE_%s = 1\n", flag)
		}
		body.WriteString(")\n\n")
	}

	for _, structure := range m.Structs {
		emitGoStructForTarget(&body, structure, isServer)
	}
	if len(m.Structs) > 0 {
		body.WriteString("\n")
	}

	// A type alias is defined as one, so a value declared under the name a
	// program gave a type has that name to refer to. An alias whose target is
	// itself an alias is resolved to the type behind it, because Go has no
	// chain of aliases and a program that followed one would not compile.
	for _, alias := range m.Aliases {
		// The target is the written type, so a dynamic array stays a slice
		// rather than becoming the pointer its C spelling would name.
		target := goTypeForRhoName(alias.Target)
		if target == "" {
			target = goTypeForCName(alias.Target)
		}
		if target == "" {
			continue
		}
		fmt.Fprintf(&body, "// %s is the name this module gives %s.\n", alias.Name, target)
		fmt.Fprintf(&body, "type %s = %s\n", goTypeName(alias.Name), target)
	}
	if len(m.Aliases) > 0 {
		body.WriteString("\n")
	}

	hasInit := hasGoInit(m)
	if hasInit {
		fmt.Fprintf(&body, "// %s runs Rho module initialisation before %s.\nfunc %s() {\n", AppInitName, AppMainName, AppInitName)
		for _, fn := range m.Funcs {
			if fn.Name == "init" && !fn.Extern && len(fn.Params) == 0 {
				fmt.Fprintf(&body, "\t%s()\n", goFuncName(irFuncName(fn)))
			}
		}
		body.WriteString("}\n\n")
	}

	sawMain := false
	emittedMain := false
	for _, fn := range m.Funcs {
		if fn.Extern {
			continue
		}
		// Definitions must use the same identity the typechecker used for call
		// sites, which is the qualified name for imported functions.
		name := goFuncName(irFuncName(fn))
		if fn.Name == "main" {
			sawMain = true
			if fn.ReturnType.Kind != ir.TypeVoid {
				// A main that returns a value has no Go equivalent. Report it
				// rather than quietly generating a program that ignores the
				// Rho entry point.
				return ""
			}
			name = AppMainName
			emittedMain = true
		}
		emitGoFunc(&body, fn, name)
	}

	if !emittedMain && !sawMain {
		// Keep the generated project buildable when the Rho module has no
		// entry point at all, which is normal for a library.
		fmt.Fprintf(&body, "// %s is a stub: the Rho module declares no main function.\nfunc %s() {}\n", AppMainName, AppMainName)
	}

	var b strings.Builder
	b.WriteString(goBanner(m, "app.go", "Generated Rho application implementation."))
	b.WriteString("package main\n")
	// Imports follow actual use so the generated file never carries an
	// unused import.
	imports := []string{}
	if strings.Contains(body.String(), "fmt.") {
		imports = addGoImport(imports, "fmt")
	}
	// A pointer is carried as an unsafe.Pointer, because a Rho pointer is a raw
	// address rather than a Go reference: it may point into memory a foreign
	// library owns, and the language does not promise it points at anything Go
	// keeps alive. A module that declares one needs the import; importing it
	// unconditionally would be an unused import in every module that does not.
	// The import follows what the file actually says. A pointer whose element
	// is known is spelled `*T` and needs no import; what needs one is a pointer
	// the file touches as an address — a slice's data, a cast of a raw value.
	// A module that only ever names a pointer's element would otherwise carry
	// an import nothing uses, and an unused import does not compile.
	if moduleHasPointer(m) && goBodyUsesUnsafe(body.String()) {
		imports = addGoImport(imports, "unsafe")
	}
	if len(imports) == 1 {
		// A single import is written on one line, because a block around one
		// name is the shape a reader does not expect to see in generated code.
		fmt.Fprintf(&b, "\nimport %q\n", imports[0])
	} else if len(imports) > 1 {
		fmt.Fprintf(&b, "\nimport (\n")
		for _, name := range imports {
			fmt.Fprintf(&b, "\t%q\n", name)
		}
		b.WriteString(")\n")
	}

	// RhoUnit is what a Rho value that means "no value" is in Go: a real,
	// nameable and empty type, so a program that binds one can hold it. It goes
	// after the imports, because Go reads the file in order.
	b.WriteString("\n// RhoUnit is the type of a Rho value that means \"no value\".\n")
	b.WriteString("type RhoUnit struct{}\n")
	b.WriteString("\n")

	// PtrUnit is a raw address a program cannot look inside, which is what a
	// handle and a slice's data both are. It is defined when a declaration names
	// it rather than always, because a module that holds neither has no reason
	// to carry a type nothing in it refers to.
	if moduleUsesPtrUnit(m) {
		b.WriteString("// PtrUnit is an address Rho can carry and not read: what a slice's\n")
		b.WriteString("// data is, and what an engine handle is.\n")
		b.WriteString("type PtrUnit struct {\n\traw uint64\n}\n")
		b.WriteString("\n")
	}
	b.WriteString(body.String())
	return b.String()
}

// bodyUsesPtrUnit reports whether the emitted body names the opaque address.
//
// It is asked of the text rather than the declarations because a slice's data
// field names it without any declaration of its own: the field is a pointer into
// a buffer, and the buffer's address is a thing a program can hold and not read.
func moduleUsesPtrUnit(m ir.ModuleIR) bool {
	// A struct field is where it first appears: a slice's data is one, and a
	// value built from that slice is one too. The declarations are asked rather
	// than the emitted text, because the struct that names it is written after
	// the point at which the decision has to be made.
	found := false
	var visit func(typ ir.Type)
	visit = func(typ ir.Type) {
		// A pointer to nothing renders as PtrUnit, which is the one pointer the
		// backend gives a name to. The type arrives as a pointer with no element
		// or an empty one — a slice's data, a handle — so it is the element that
		// is asked about rather than the name, which is decided here.
		if typ.Kind == ir.TypePtr && (len(typ.Args) == 0 ||
			typ.Args[0].Kind == ir.TypeVoid ||
			typ.Args[0].Name == "Unit" || typ.Args[0].Name == "RhoUnit" ||
			typ.Args[0].Name == "") {
			found = true
			return
		}
		if typ.Name == "PtrUnit" || typ.Name == "PtrVoid" {
			found = true
			return
		}
		for _, argument := range typ.Args {
			visit(argument)
		}
	}
	for _, structure := range m.Structs {
		for _, field := range structure.Fields {
			visit(field.Type)
		}
	}
	for _, fn := range m.Funcs {
		for _, param := range fn.Params {
			visit(param.Type)
		}
		visit(fn.ReturnType)
	}
	return found
}

func emitGoStruct(b *strings.Builder, structure ir.StructIR) {
	emitGoStructForTarget(b, structure, false)
}

func emitGoStructForTarget(b *strings.Builder, structure ir.StructIR, isServer bool) {
	// The server target defines the HTTP protocol types itself, because it has
	// to decode a net/http request into them. Emitting them again would
	// redeclare them.
	if isServer && isProtocolType(structure.Name) {
		return
	}
	// Name is the spelling type references use; CName carries the owner
	// prefix, which is only for cross-module C linkage. Matching Name keeps
	// definitions and references on the same identifier.
	name := goTypeName(structure.Name)
	if name == "" {
		name = goTypeName(structure.CName)
	}
	if name == "" {
		name = "anon"
	}
	fmt.Fprintf(b, "// %s mirrors the Rho struct of the same name.\ntype %s struct {\n", name, name)
	// Go has no bit fields, so a run of them is one storage integer here; the
	// shift and mask a read and a write need are written where the field is
	// used.
	fields := structure.Packed
	if len(fields) == 0 {
		fields = structure.Fields
	}
	for _, field := range fields {
		goType := goTypeForIR(field.Type)
		if goType == "" {
			goType = "any"
		}
		fmt.Fprintf(b, "\t%s %s\n", goFieldName(field.Name), goType)
	}
	b.WriteString("}\n")
}

func emitGoFunc(b *strings.Builder, fn ir.FuncIR, goName string) {
	returnType := goTypeForIR(fn.ReturnType)

	// Names are tracked so a repeated assignment reuses `=` instead of `:=`,
	// which Go rejects for an already-declared variable.
	declared := map[string]bool{}
	for _, param := range fn.Params {
		declared[goFieldName(param.Name)] = true
	}

	if returnType != "" {
		fmt.Fprintf(b, "func %s(%s) %s {\n", goName, goParamList(fn.Params), returnType)
	} else {
		fmt.Fprintf(b, "func %s(%s) {\n", goName, goParamList(fn.Params))
	}

	// Go rejects an unused local, while C only warns. A Rho program may leave a
	// binding unread, so those names are explicitly discarded. The discards go
	// before the final return because Go's terminating-statement rule only
	// accepts a return as the last statement.
	prefix, suffix := fn.Body, []ir.Instr(nil)
	if len(fn.Body) > 0 && fn.Body[len(fn.Body)-1].Op == ir.OpReturn {
		prefix, suffix = fn.Body[:len(fn.Body)-1], fn.Body[len(fn.Body)-1:]
	}
	for _, instr := range prefix {
		emitGoInstr(b, instr, 1, declared)
	}
	for _, name := range unusedLocals(prefix, declared) {
		fmt.Fprintf(b, "\t_ = %s\n", name)
	}
	for _, instr := range suffix {
		emitGoInstr(b, instr, 1, declared)
	}

	// Go requires a terminating return on non-void paths.
	if returnType != "" && !hasTerminatingReturn(fn.Body) {
		fmt.Fprintf(b, "\treturn %s\n", goZeroValue(fn.ReturnType))
	}
	b.WriteString("}\n\n")
}

// unusedLocals returns the declared names that are never read in the body.
//
// Identifier spellings are taken from the translated Go, because the IR text
// still uses the C names that ToGoExpr rewrites.
func unusedLocals(instructions []ir.Instr, declared map[string]bool) []string {
	declarations := map[string]bool{}
	used := map[string]bool{}

	var walk func([]ir.Instr)
	walk = func(list []ir.Instr) {
		for _, instr := range list {
			switch instr.Op {
			case ir.OpLet:
				// The destination is a declaration, not a read.
				if instr.Dest != "" {
					declarations[goFieldName(instr.Dest)] = true
				}
			case ir.OpCall:
				if instr.Dest != "" {
					declarations[goFieldName(instr.Dest)] = true
				}
			case ir.OpAssign:
				if instr.Dest != "" {
					used[goFieldName(strings.TrimSuffix(instr.Dest, "="))] = true
				}
			}
			for _, arg := range instr.Args {
				collectWords(ToGoExpr(arg), used)
			}
			collectWords(ToGoExpr(instr.Condition), used)
			collectWords(ToGoExpr(instr.Type), used)
			walk(instr.Then)
			walk(instr.Else)
		}
	}
	walk(instructions)

	out := []string{}
	for name := range declarations {
		if declared[name] && !used[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// collectWords records every identifier-like token in a lowered expression.
func collectWords(text string, into map[string]bool) {
	src := []byte(text)
	for i := 0; i < len(src); {
		// A string literal's contents are not identifiers: `print("x")` does
		// not use a local named `x`, and counting it as one left the binding
		// undiscarded and Go rejected it as unused.
		if src[i] == '"' || src[i] == '\'' {
			i = scanStringLiteral(src, i)
			continue
		}
		if !isIdentStart(src[i]) {
			i++
			continue
		}
		start := i
		for i < len(src) && isIdentPart(src[i]) {
			i++
		}
		into[text[start:i]] = true
	}
}

func emitGoInstr(b *strings.Builder, instr ir.Instr, depth int, declared map[string]bool) {
	pad := strings.Repeat("\t", depth)

	switch instr.Op {
	case ir.OpReturn:
		if len(instr.Args) == 1 {
			fmt.Fprintf(b, "%sreturn %s\n", pad, ToGoExpr(instr.Args[0]))
		} else {
			fmt.Fprintf(b, "%sreturn\n", pad)
		}

	case ir.OpAssign:
		if len(instr.Args) != 1 {
			return
		}
		// The IR carries the operator in the destination, as the C backend
		// expects. An assignment is always `name = value` in Go; a
		// declaration is OpLet, not a trailing "=" here.
		dest := strings.TrimSpace(instr.Dest)
		dest = strings.TrimRight(dest, "=+-*/")
		fmt.Fprintf(b, "%s%s = %s\n", pad, goFieldName(strings.TrimSpace(dest)), ToGoExpr(instr.Args[0]))

	case ir.OpLet:
		// The Rho type is preferred when it was carried, because a dynamic
		// array `[T]` and a pointer `Ptr[T]` share the C spelling `T*` and
		// differ in Go, where the slice is `[]T`. Reading the C spelling alone
		// would declare a slice as a pointer.
		goType := ""
		if instr.TypeName != "" {
			goType = goTypeForRhoName(instr.TypeName)
		}
		if goType == "" {
			goType = goTypeForCName(instr.Type)
		}
		if goType == "" {
			goType = "any"
		}
		name := goFieldName(instr.Dest)
		declared[name] = true
		// Go has no `T name = v` form, so a typed local always needs `var`.
		if len(instr.Args) == 1 && instr.Args[0] != "" {
			value := ToGoExpr(instr.Args[0])
			// A bare initializer needs its type in Go: C writes `{ 0 }` where Go
			// writes `[N]T{ 0 }`, and the type is what the declaration names.
			if strings.HasPrefix(strings.TrimSpace(value), "{") {
				value = goType + value
			}
			fmt.Fprintf(b, "%svar %s %s = %s\n", pad, name, goType, value)
			return
		}
		fmt.Fprintf(b, "%svar %s %s\n", pad, name, goType)

	case ir.OpExpr:
		if len(instr.Args) == 1 {
			fmt.Fprintf(b, "%s%s\n", pad, ToGoExpr(instr.Args[0]))
		}

	case ir.OpIf:
		fmt.Fprintf(b, "%sif %s {\n", pad, ToGoExpr(instr.Condition))
		emitGoNested(b, instr.Then, depth+1, declared)
		if len(instr.Else) > 0 {
			b.WriteString(pad + "} else {\n")
			emitGoNested(b, instr.Else, depth+1, declared)
		}
		fmt.Fprintf(b, "%s}\n", pad)

	case ir.OpWhile:
		fmt.Fprintf(b, "%sfor %s {\n", pad, ToGoExpr(instr.Condition))
		emitGoNested(b, instr.Then, depth+1, declared)
		fmt.Fprintf(b, "%s}\n", pad)

	case ir.OpBreak:
		fmt.Fprintf(b, "%sbreak\n", pad)

	case ir.OpContinue:
		fmt.Fprintf(b, "%scontinue\n", pad)

	case ir.OpCall:
		if len(instr.Args) > 0 {
			args := make([]string, 0, len(instr.Args)-1)
			for _, arg := range instr.Args[1:] {
				args = append(args, ToGoExpr(arg))
			}
			call := fmt.Sprintf("%s(%s)", ToGoExpr(instr.Args[0]), strings.Join(args, ", "))
			if instr.Dest != "" {
				name := goFieldName(instr.Dest)
				// A second assignment to the same name must reuse `=`.
				operator := "="
				if !declared[name] {
					operator = ":="
					declared[name] = true
				}
				fmt.Fprintf(b, "%s%s %s %s\n", pad, name, operator, call)
				return
			}
			fmt.Fprintf(b, "%s%s\n", pad, call)
		}

	case ir.OpRaw:
		b.WriteString(instr.Dest + "\n")

	default:
		fmt.Fprintf(b, "%s// unsupported instruction op %d\n", pad, int(instr.Op))
	}
}

func emitGoNested(b *strings.Builder, instructions []ir.Instr, depth int, declared map[string]bool) {
	for _, instruction := range instructions {
		emitGoInstr(b, instruction, depth, declared)
	}
}

// goFeatureFlags normalises Rho feature names into constant suffixes,
// dropping duplicates so the generated const block never redeclares a name.
func goFeatureFlags(features []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(features))
	for _, feature := range features {
		if feature == "" {
			continue
		}
		name := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(feature))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// goParamList builds a Go function parameter list. Go writes `name Type`,
// the opposite of the C spelling the IR uses elsewhere.
func goParamList(params []ir.ParamIR) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		goType := goTypeForIR(param.Type)
		if goType == "" {
			goType = "any"
		}
		parts = append(parts, goFieldName(param.Name)+" "+goType)
	}
	return strings.Join(parts, ", ")
}

func goZeroValue(t ir.Type) string {
	switch t.Kind {
	case ir.TypeBool:
		return "false"
	case ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeI64, ir.TypeF64, ir.TypeF32:
		return "0"
	case ir.TypeString:
		return "\"\""
	case ir.TypeStruct:
		name := goTypeName(t.Name)
		if name == "" {
			return "nil"
		}
		return name + "{}"
	case ir.TypePtr:
		return "nil"
	default:
		return "nil"
	}
}

// hasTerminatingReturn reports whether the body ends in a return.
func hasTerminatingReturn(body []ir.Instr) bool {
	for i := len(body) - 1; i >= 0; i-- {
		switch body[i].Op {
		case ir.OpReturn:
			return true
		case ir.OpBreak, ir.OpContinue, ir.OpNop:
			continue
		default:
			return false
		}
	}
	return false
}

// irFuncName returns the identity the typechecker used for a function, which
// is what both the definition and its call sites must agree on.
func irFuncName(fn ir.FuncIR) string {
	if fn.CName != "" {
		return fn.CName
	}
	return fn.Name
}

// goFuncName maps a Rho function name to its Go spelling.
func goFuncName(name string) string {
	if name == "" {
		return ""
	}
	if mapped, ok := goTypeNames[name]; ok && mapped != "" {
		return mapped
	}
	if escaped := escapeGoName(name); escaped != name {
		return escaped
	}
	return lowerCamel(name)
}

// goFieldName maps a Rho identifier to its Go spelling. Field names keep
// their case so exported struct fields stay reachable.
func goFieldName(name string) string {
	if name == "" {
		return ""
	}
	if escaped := escapeGoName(name); escaped != name {
		return escaped
	}
	return lowerCamel(name)
}

// EmitGoEntrypoint returns main.go for a generated Go project.
func EmitGoEntrypoint(m ir.ModuleIR) string {
	var b strings.Builder
	b.WriteString(goBanner(m, "main.go", "Generated native entry point for a Rho application."))
	b.WriteString("package main\n\n")
	fmt.Fprintf(&b, "// main is the Go entry point for the generated Rho project.\nfunc main() {\n")
	if hasGoInit(m) {
		fmt.Fprintf(&b, "\t%s()\n", AppInitName)
	}
	fmt.Fprintf(&b, "\t%s()\n}\n", AppMainName)
	return b.String()
}

func hasGoInit(m ir.ModuleIR) bool {
	for _, fn := range m.Funcs {
		if fn.Name == "init" && !fn.Extern && len(fn.Params) == 0 {
			return true
		}
	}
	return false
}

// EmitGoRuntime returns runtime.go containing only the helpers the generated
// implementation actually references, so the file never carries an unused
// import. The abort and panic intrinsics are always present: they back the
// `extern fn abort()` and `panic()` builtins, and a test module reaches them
// through the assert library.
func EmitGoRuntime(implementation string) string {
	var body strings.Builder
	imports := []string{}

	// The abort and panic intrinsics. println writes to stderr and needs no
	// import; os.Exit is the only other dependency.
	body.WriteString("// abort ends the program with a non-zero status, backing Rho's abort().\n")
	body.WriteString("func abort() {\n\tprintln(\"rho: aborted\")\n\tos.Exit(1)\n}\n\n")
	body.WriteString("// panicf ends the program with a message, backing Rho's panic().\n")
	body.WriteString("func panicf(msg string) {\n\tprintln(\"rho: \" + msg)\n\tos.Exit(1)\n}\n\n")
	imports = addGoImport(imports, "os")

	// String and environment intrinsics, so the same Rho code builds and
	// behaves the same for every Go target.
	body.WriteString("// rhoStrlen returns the length of s in bytes.\n")
	body.WriteString("func rhoStrlen(s string) uint32 { return uint32(len(s)) }\n\n")
	body.WriteString("// rhoStrStartsWith reports whether s begins with prefix.\n")
	body.WriteString("func rhoStrStartsWith(s, prefix string) bool { return strings.HasPrefix(s, prefix) }\n\n")
	body.WriteString("// rhoStrTrimPrefix removes prefix from s when present.\n")
	body.WriteString("func rhoStrTrimPrefix(s, prefix string) string { return strings.TrimPrefix(s, prefix) }\n\n")
	body.WriteString("// rhoStrContains reports whether haystack contains needle.\n")
	body.WriteString("func rhoStrContains(haystack, needle string) bool { return strings.Contains(haystack, needle) }\n\n")
	body.WriteString("// rhoStrAt returns the byte at an index, or 0 past the end.\n")
	body.WriteString("func rhoStrAt(s string, i uint32) byte {\n")
	body.WriteString("\tif int(i) >= len(s) {\n\t\treturn 0\n\t}\n")
	body.WriteString("\treturn s[i]\n}\n\n")
	body.WriteString("// rhoStrIndexOf finds a substring, or -1 when it is absent.\n")
	body.WriteString("func rhoStrIndexOf(haystack, needle string) int32 { return int32(strings.Index(haystack, needle)) }\n\n")
	body.WriteString("// rhoStrSlice takes a run of bytes, clamped to the string.\n")
	body.WriteString("func rhoStrSlice(s string, start uint32, length uint32) string {\n")
	body.WriteString("\tif int(start) >= len(s) {\n\t\treturn \"\"\n\t}\n")
	body.WriteString("\tend := int(start) + int(length)\n")
	body.WriteString("\tif end > len(s) {\n\t\tend = len(s)\n\t}\n")
	body.WriteString("\treturn s[start:end]\n}\n\n")
	body.WriteString("// rhoIntToStr renders a number in base ten.\n")
	body.WriteString("func rhoIntToStr(n uint32) string { return strconv.FormatUint(uint64(n), 10) }\n\n")
	body.WriteString("// rhoIntFromStr reads a run of digits, returning 0 for anything else.\n")
	body.WriteString("func rhoIntFromStr(s string) uint32 {\n")
	body.WriteString("\tn, err := strconv.ParseUint(s, 10, 32)\n")
	body.WriteString("\tif err != nil {\n\t\treturn 0\n\t}\n")
	body.WriteString("\treturn uint32(n)\n}\n\n")
	body.WriteString("// rhoStrFromByte renders one byte as a one-character string.\n")
	body.WriteString("func rhoStrFromByte(b byte) string { return string([]byte{b}) }\n\n")
	body.WriteString("// rhoEnvOr returns an environment variable, or fallback.\n")
	body.WriteString("func rhoEnvOr(name, fallback string) string {\n")
	body.WriteString("\tif value, ok := os.LookupEnv(name); ok {\n\t\treturn value\n\t}\n")
	body.WriteString("\treturn fallback\n}\n\n")
	imports = addGoImport(imports, "strconv")
	imports = addGoImport(imports, "strings")

	// File and argument builtins.
	body.WriteString("// rhoReadFile reads a whole file into a byte slice, or nil when it cannot.\n")
	body.WriteString("func rhoReadFile(path string) []byte {\n")
	body.WriteString("\tdata, err := os.ReadFile(path)\n")
	body.WriteString("\tif err != nil {\n\t\treturn nil\n\t}\n")
	body.WriteString("\treturn data\n}\n\n")
	body.WriteString("// rhoWriteFile writes a byte slice to a file.\n")
	body.WriteString("func rhoWriteFile(path string, data []byte) {\n")
	body.WriteString("\tos.WriteFile(path, data, 0644)\n}\n\n")
	body.WriteString("// rhoGetArgs returns the command-line arguments as one space-joined string.\n")
	body.WriteString("func rhoGetArgs() string {\n")
	body.WriteString("\treturn strings.Join(os.Args[1:], \" \")\n}\n\n")
	body.WriteString("// rhoPrintI32 writes a signed number to stdout.\n")
	body.WriteString("func rhoPrintI32(value int32) { fmt.Print(value) }\n\n")
	imports = addGoImport(imports, "fmt")

	needed := []struct {
		call string
		// needs are the helpers this one is written in terms of. A helper that
		// names another is emitted with it, and the set is closed over that:
		// a program that calls one of them gets the ones under it as well.
		needs   []string
		define  string
		imports []string
	}{
		{
			// The text of a file.
			//
			// A registry or a manifest is read as text and understood by the
			// program that reads it, and a program that writes one has to be
			// able to say whether it did: a file that was not written is a
			// registry that was not changed.
			call: "rhoReadText(",
			define: "func rhoReadText(path string) string {\n" +
				"\tdata, err := os.ReadFile(path)\n" +
				"\tif err != nil {\n" +
				"\t\treturn \"\"\n" +
				"\t}\n" +
				"\treturn string(data)\n" +
				"}\n",
		},
		{
			call: "rhoWriteText(",
			define: "func rhoWriteText(path string, text string) bool {\n" +
				"\treturn os.WriteFile(path, []byte(text), 0644) == nil\n" +
				"}\n",
		},
		{
			// One value out of a YAML file, named by its path.
			//
			// The path is dotted: `board.connectors` is the connectors of the
			// board, and `module.version` is the version of the module. A
			// program that reads a document asks for the thing it wants by the
			// name of the thing, rather than by the name of a reader it carries.
			call:  "rhoYamlValue(",
			needs: []string{"rhoYamlLookup("},
			define: "func rhoYamlValue(path string, key string) string {\n" +
				"\tvalue, _ := rhoYamlLookup(path, key)\n" +
				"\treturn value\n" +
				"}\n",
		},
		{
			// What a value is: `string`, `number`, `boolean`, `list`, `map`, or
			// `missing`.
			//
			// A program that reads a document needs to know what it read: a
			// manifest that says a list where it should say a map is a manifest
			// the reader cannot use, and a reader that cannot say what it read
			// cannot report that.
			call:  "rhoYamlKind(",
			needs: []string{"rhoYamlLookup("},
			define: "func rhoYamlKind(path string, key string) string {\n" +
				"\t_, kind := rhoYamlLookup(path, key)\n" +
				"\treturn kind\n" +
				"}\n",
		},
		{
			// The keys under a key, one per line.
			//
			// A program that reads a document needs to know what is in it: a
			// registry is read by asking for each of the things it holds, and a
			// program that cannot ask what is there cannot read what is.
			call:  "rhoYamlKeys(",
			needs: []string{"rhoYamlLookup("},
			define: "func rhoYamlKeys(path string, key string) string {\n" +
				"\tdata, err := os.ReadFile(path)\n" +
				"\tif err != nil {\n" +
				"\t\treturn \"\"\n" +
				"\t}\n" +
				"\tlines := strings.Split(string(data), \"\\n\")\n" +
				"\tindent := -1\n" +
				"\tindex := 0\n" +
				"\tfor _, part := range strings.Split(key, \".\") {\n" +
				"\t\tfound := false\n" +
				"\t\tfor index < len(lines) {\n" +
				"\t\t\traw := lines[index]\n" +
				"\t\t\ttrimmed := strings.TrimLeft(raw, \" \")\n" +
				"\t\t\tcurrent := len(raw) - len(trimmed)\n" +
				"\t\t\tif trimmed == \"\" || strings.HasPrefix(trimmed, \"#\") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tif indent >= 0 && current <= indent {\n" +
				"\t\t\t\treturn \"\"\n" +
				"\t\t\t}\n" +
				"\t\t\tif strings.HasPrefix(trimmed, \"- \") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tif !strings.HasPrefix(trimmed, part+\":\") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tindex++\n" +
				"\t\t\tindent = current\n" +
				"\t\t\tfound = true\n" +
				"\t\t\tbreak\n" +
				"\t\t}\n" +
				"\t\tif !found {\n" +
				"\t\t\treturn \"\"\n" +
				"\t\t}\n" +
				"\t}\n" +
				"\tnames := []string{}\n" +
				"\tchild := -1\n" +
				"\tfor index < len(lines) {\n" +
				"\t\traw := lines[index]\n" +
				"\t\ttrimmed := strings.TrimLeft(raw, \" \")\n" +
				"\t\tcurrent := len(raw) - len(trimmed)\n" +
				"\t\tif trimmed == \"\" || strings.HasPrefix(trimmed, \"#\") {\n" +
				"\t\t\tindex++\n" +
				"\t\t\tcontinue\n" +
				"\t\t}\n" +
				"\t\tif current <= indent {\n" +
				"\t\t\tbreak\n" +
				"\t\t}\n" +
				"\t\tif child < 0 {\n" +
				"\t\t\tchild = current\n" +
				"\t\t}\n" +
				"\t\tif current == child && !strings.HasPrefix(trimmed, \"- \") {\n" +
				"\t\t\tname := trimmed\n" +
				"\t\t\tif colon := strings.Index(name, \":\"); colon >= 0 {\n" +
				"\t\t\t\tname = name[:colon]\n" +
				"\t\t\t}\n" +
				"\t\t\tif name != \"\" {\n" +
				"\t\t\t\tnames = append(names, strings.TrimSpace(name))\n" +
				"\t\t\t}\n" +
				"\t\t}\n" +
				"\t\tindex++\n" +
				"\t}\n" +
				"\treturn strings.Join(names, \"\\n\")\n" +
				"}\n",
		},
		{
			// The lookup the two share: a dotted path into a YAML document, and
			// the value and the kind found there.
			//
			// The document is walked by its indentation, which is what a YAML
			// mapping is: a key, and the block below it that belongs to it. A
			// path that leaves the block it is in is a path that names nothing,
			// which is what `missing` says.
			call:  "rhoYamlLookup(",
			needs: []string{"rhoYamlKindOf("},
			define: "func rhoYamlLookup(path string, key string) (string, string) {\n" +
				"\tdata, err := os.ReadFile(path)\n" +
				"\tif err != nil {\n" +
				"\t\treturn \"\", \"missing\"\n" +
				"\t}\n" +
				"\tlines := strings.Split(string(data), \"\\n\")\n" +
				"\tparts := strings.Split(key, \".\")\n" +
				"\tvalue := \"\"\n" +
				"\tkind := \"missing\"\n" +
				"\tindent := -1\n" +
				"\tindex := 0\n" +
				"\tfor _, part := range parts {\n" +
				"\t\tfound := false\n" +
				"\t\tfor index < len(lines) {\n" +
				"\t\t\traw := lines[index]\n" +
				"\t\t\ttrimmed := strings.TrimLeft(raw, \" \")\n" +
				"\t\t\tcurrent := len(raw) - len(trimmed)\n" +
				"\t\t\tif trimmed == \"\" || strings.HasPrefix(trimmed, \"#\") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tif indent >= 0 && current <= indent {\n" +
				"\t\t\t\treturn \"\", \"missing\"\n" +
				"\t\t\t}\n" +
				"\t\t\tif strings.HasPrefix(trimmed, \"- \") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tif !strings.HasPrefix(trimmed, part+\":\") {\n" +
				"\t\t\t\tindex++\n" +
				"\t\t\t\tcontinue\n" +
				"\t\t\t}\n" +
				"\t\t\tvalue = strings.TrimSpace(strings.TrimPrefix(trimmed, part+\":\"))\n" +
				"\t\t\tvalue = strings.Trim(value, \"\\\"\")\n" +
				"\t\t\tindex++\n" +
				"\t\t\tkind = rhoYamlKindOf(value, lines, index, current)\n" +
				"\t\t\tindent = current\n" +
				"\t\t\tfound = true\n" +
				"\t\t\tbreak\n" +
				"\t\t}\n" +
				"\t\tif !found {\n" +
				"\t\t\treturn \"\", \"missing\"\n" +
				"\t\t}\n" +
				"\t}\n" +
				"\treturn value, kind\n" +
				"}\n",
			imports: []string{"strconv"},
		},
		{
			// What a value is, from the line it is on and the block below it.
			//
			// A value on its own line is a scalar, and which scalar it is is
			// said by what it looks like. A value that is a block is a list when
			// the block's first item is a dash, and a map when it is a key.
			call: "rhoYamlKindOf(",
			define: "func rhoYamlKindOf(value string, lines []string, index int, indent int) string {\n" +
				"\tif value != \"\" {\n" +
				"\t\tif _, err := strconv.Atoi(value); err == nil {\n" +
				"\t\t\treturn \"number\"\n" +
				"\t\t}\n" +
				"\t\tif value == \"true\" || value == \"false\" {\n" +
				"\t\t\treturn \"boolean\"\n" +
				"\t\t}\n" +
				"\t\tif strings.HasPrefix(value, \"[\") {\n" +
				"\t\t\treturn \"list\"\n" +
				"\t\t}\n" +
				"\t\tif strings.HasPrefix(value, \"{\") {\n" +
				"\t\t\treturn \"map\"\n" +
				"\t\t}\n" +
				"\t\treturn \"string\"\n" +
				"\t}\n" +
				"\tfor index < len(lines) {\n" +
				"\t\traw := lines[index]\n" +
				"\t\ttrimmed := strings.TrimLeft(raw, \" \")\n" +
				"\t\tcurrent := len(raw) - len(trimmed)\n" +
				"\t\tif trimmed == \"\" || strings.HasPrefix(trimmed, \"#\") {\n" +
				"\t\t\tindex++\n" +
				"\t\t\tcontinue\n" +
				"\t\t}\n" +
				"\t\tif current <= indent {\n" +
				"\t\t\tbreak\n" +
				"\t\t}\n" +
				"\t\tif strings.HasPrefix(trimmed, \"- \") || trimmed == \"-\" {\n" +
				"\t\t\treturn \"list\"\n" +
				"\t\t}\n" +
				"\t\treturn \"map\"\n" +
				"\t}\n" +
				"\treturn \"missing\"\n" +
				"}\n",
		},
		{
			// One command-line argument, by its position. The arguments as one
			// string are what a program is given; a program with a command in
			// them needs to pick the command out of it.
			call: "rhoArg(",
			define: "func rhoArg(index int32) string {\n" +
				"\tif index < 0 || int(index) >= len(os.Args)-1 {\n" +
				"\t\treturn \"\"\n" +
				"\t}\n" +
				"\treturn os.Args[index+1]\n" +
				"}\n",
		},
		{
			// Whether a path is there. A program that walks a tree asks about
			// every path it meets, and a program that cannot ask is a program
			// that has to guess.
			call: "rhoPathExists(",
			define: "func rhoPathExists(path string) bool {\n" +
				"\t_, err := os.Stat(path)\n" +
				"\treturn err == nil\n" +
				"}\n",
		},
		{
			// The names in a directory, one per line. A program that walks a tree
			// needs to know what is in it, and a list is what a directory is.
			call: "rhoListDir(",
			define: "func rhoListDir(path string) string {\n" +
				"\tentries, err := os.ReadDir(path)\n" +
				"\tif err != nil {\n" +
				"\t\treturn \"\"\n" +
				"\t}\n" +
				"\tnames := make([]string, 0, len(entries))\n" +
				"\tfor _, entry := range entries {\n" +
				"\t\tnames = append(names, entry.Name())\n" +
				"\t}\n" +
				"\treturn strings.Join(names, \"\\n\")\n" +
				"}\n",
		},
		{
			// Running a program. A tool that orchestrates other tools has to run
			// them, and a program that cannot is a program that can only describe.
			//
			// The output goes to a file rather than back to the caller, because
			// what a program prints is not a string a caller can be trusted to
			// have room for, and a file is what a reader can read in pieces.
			call: "rhoSplitArgs(",
			define: "func rhoSplitArgs(arguments string) []string {\n" +
				"\targs := []string{}\n" +
				"\tcurrent := strings.Builder{}\n" +
				"\tquoted := false\n" +
				"\tfor _, character := range arguments {\n" +
				"\t\tswitch {\n" +
				"\t\tcase character == 34:\n" +
				"\t\t\tquoted = !quoted\n" +
				"\t\tcase character == ' ' && !quoted:\n" +
				"\t\t\tif current.Len() > 0 {\n" +
				"\t\t\t\targs = append(args, current.String())\n" +
				"\t\t\t\tcurrent.Reset()\n" +
				"\t\t\t}\n" +
				"\t\tdefault:\n" +
				"\t\t\tcurrent.WriteRune(character)\n" +
				"\t\t}\n" +
				"\t}\n" +
				"\tif current.Len() > 0 {\n" +
				"\t\targs = append(args, current.String())\n" +
				"\t}\n" +
				"\treturn args\n" +
				"}\n",
		},
		{
			// Running a program.
			//
			// The arguments are split on whitespace and honouring quotes, so
			// that a name with a space in it is one argument and not two.
			call:  "rhoExec(",
			needs: []string{"rhoSplitArgs("},
			define: "func rhoExec(program string, arguments string, outputPath string) uint32 {\n" +
				"\t// The report is written where it was asked for, and the place it\n" +
				"\t// was asked for is made first: a run that cannot write its own\n" +
				"\t// report is a run whose findings nobody can read.\n" +
				"\t_ = os.MkdirAll(filepath.Dir(outputPath), 0755)\n" +
				"\tcommand := exec.Command(program, rhoSplitArgs(arguments)...)\n" +
				"\toutput, err := command.CombinedOutput()\n" +
				"\tif err != nil {\n" +
				"\t\t// The output a program wrote before it failed is still its\n" +
				"\t\t// output, and a reader needs it to say why it failed.\n" +
				"\t\t_ = os.WriteFile(outputPath, output, 0644)\n" +
				"\t\tif exit, ok := err.(*exec.ExitError); ok {\n" +
				"\t\t\treturn uint32(exit.ExitCode())\n" +
				"\t\t}\n" +
				"\t\t// The program could not be run at all, which is not an exit\n" +
				"\t\t// status: it is the one answer that is not a status.\n" +
				"\t\treturn 255\n" +
				"\t}\n" +
				"\t_ = os.WriteFile(outputPath, output, 0644)\n" +
				"\treturn 0\n" +
				"}\n",
			imports: []string{"os/exec", "path/filepath"},
		},
		{
			call:   "rhoStrEq(",
			define: "func rhoStrEq(a, b string) bool {\n\treturn a == b\n}\n",
		},
		{
			// An if expression, which Go has no syntax for: both branches have
			// the same type, so one generic function answers for every one.
			call:   "rhoSelect(",
			define: "func rhoSelect[T any](condition bool, ifTrue, ifFalse T) T {\n\tif condition {\n\t\treturn ifTrue\n\t}\n\treturn ifFalse\n}\n",
		},
		{
			// The bit cast a wire protocol needs: a float on the wire is its
			// IEEE-754 bits little-endian, and Rho has no cast to a word.
			call:    "rhoF32ToBits(",
			define:  "func rhoF32ToBits(value float32) uint32 { return math.Float32bits(value) }\n",
			imports: []string{"math"},
		},
		{
			call:    "rhoF32FromBits(",
			define:  "func rhoF32FromBits(bits uint32) float32 { return math.Float32frombits(bits) }\n",
			imports: []string{"math"},
		},
		{
			call:    "rhoF64ToBits(",
			define:  "func rhoF64ToBits(value float64) uint64 { return math.Float64bits(value) }\n",
			imports: []string{"math"},
		},
		{
			call:    "rhoF64FromBits(",
			define:  "func rhoF64FromBits(bits uint64) float64 { return math.Float64frombits(bits) }\n",
			imports: []string{"math"},
		},
		{
			call:    "rhoStringFromI32(",
			define:  "func rhoStringFromI32(value int32) string {\n\treturn strconv.Itoa(int(value))\n}\n",
			imports: []string{"strconv"},
		},
		{
			// The name of a program's configuration: the one under the `user:`
			// section, which is where a configuration keeps it.
			//
			// It is read through the same walk as everything else, so that a
			// program which asks for `user.name` and a program which loads a
			// configuration are asking the same question — on either backend.
			call:  "rhoYamlLoad(",
			needs: []string{"rhoYamlLookup("},
			define: "func rhoYamlLoad(path string) string {\n" +
				"\tvalue, kind := rhoYamlLookup(path, \"user.name\")\n" +
				"\tif kind == \"missing\" {\n" +
				"\t\treturn \"unknown\"\n" +
				"\t}\n" +
				"\treturn value\n" +
				"}\n",
		},
	}

	// A helper is emitted when the program names it, and when it is written in
	// terms of one that is: the set is closed over that, so a helper that needs
	// another brings it with it.
	wanted := map[string]bool{}
	for _, helper := range needed {
		if strings.Contains(implementation, helper.call) {
			wanted[helper.call] = true
		}
	}
	changed := true
	for changed {
		changed = false
		for _, helper := range needed {
			if !wanted[helper.call] {
				continue
			}
			for _, need := range helper.needs {
				if !wanted[need] {
					wanted[need] = true
					changed = true
				}
			}
		}
	}
	for _, helper := range needed {
		if !wanted[helper.call] {
			continue
		}
		body.WriteString(helper.define)
		body.WriteString("\n")
		for _, name := range helper.imports {
			imports = addGoImport(imports, name)
		}
	}

	var b strings.Builder
	b.WriteString(goBanner(ir.ModuleIR{Name: "runtime"}, "runtime.go", "Generated Rho Go runtime support."))
	b.WriteString("package main\n")
	if len(imports) > 0 {
		b.WriteString("\nimport (\n")
		for _, name := range imports {
			fmt.Fprintf(&b, "\t%q\n", name)
		}
		b.WriteString(")\n")
	}
	if body.Len() > 0 {
		b.WriteString("\n")
		b.WriteString(body.String())
	}
	return b.String()
}

// addGoImport adds a package to a generated file's import list, and leaves the
// list alone if the package is already on it.
//
// The list is built from more than one place — what the runtime body always
// needs, plus what each intrinsic a program happens to call pulls in — so the
// same package can be asked for twice. That is not a cosmetic duplicate: Go
// rejects a file that imports the same package under one name twice, so a
// program using two of these intrinsics produced a runtime that would not
// compile, and the failure named a duplicate import rather than the program that
// caused it.
func addGoImport(imports []string, name string) []string {
	for _, existing := range imports {
		if existing == name {
			return imports
		}
	}
	return append(imports, name)
}

// EmitGoMod returns the go.mod file for a generated Go project.
func EmitGoMod(m ir.ModuleIR) string {
	name := goModulePath(m.Name)
	return fmt.Sprintf("module %s\n\ngo 1.22\n", name)
}

// EmitGoWork returns the go.work file for a generated Go project.
//
// A generated project is a project of its own, and it is generated into a
// directory that is often inside somebody else's checkout — a repository, a
// monorepo, this toolchain's own examples. If a go.work exists anywhere above
// it, that workspace owns the build, and the go tool refuses packages the
// workspace does not list: "main module does not contain package ...". So the
// documented way to run a generated project, `cd build/generated/go && go run .`,
// fails for a reason that has nothing to do with the project.
//
// A go.work naming only the project itself ends that. Go stops at the nearest
// go.work going up, so the generated project becomes its own workspace and is
// built by its own go.mod, wherever it was written.
func EmitGoWork(m ir.ModuleIR) string {
	return fmt.Sprintf("go 1.22\n\nuse .\n")
}

// goModulePath turns a Rho module name into a valid Go module path.
func goModulePath(name string) string {
	if name == "" {
		return "rhoapp"
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 32)
		case c == '-' || c == '_' || c == '.':
			b.WriteByte('-')
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "rhoapp"
	}
	return out
}

func goBanner(m ir.ModuleIR, file, brief string) string {
	return fmt.Sprintf("//\n// @file %s\n// @brief %s\n//\n// @project %s\n// @version 0.1\n// @date 2026\n//\n// SPDX-License-Identifier: Apache-2.0\n//\n\n",
		file, brief, m.Name)
}

// moduleHasPointer reports whether any of the module's declarations mention a
// pointer.
//
// It is asked of the declarations rather than of the emitted text, because a
// pointer in a signature reaches the generated file whether or not a body
// happens to use it, and a module whose only pointer is a parameter — a handle
// passed to a foreign call, which is the usual shape — would otherwise be
// emitted without the import its own signature needs.
// goBodyUsesUnsafe reports whether the emitted body reaches the unsafe package.
//
// The import exists for the places a value crosses into Go's own pointer world:
// building a slice from an address, or converting a raw one. A pointer that is
// only ever named as `*T` is ordinary Go and needs no import.
func goBodyUsesUnsafe(body string) bool {
	return strings.Contains(body, "unsafe.")
}

func moduleHasPointer(m ir.ModuleIR) bool {
	found := false
	var visit func(typ ir.Type)
	visit = func(typ ir.Type) {
		// A pointer to nothing is a handle, and a handle lowers to the
		// `PtrUnit` struct rather than to an `unsafe.Pointer`. It is the one
		// pointer that is not a raw address, so it does not bring the import
		// with it — and importing it anyway would be an unused import in every
		// module that only holds handles.
		if typ.Kind == ir.TypePtr {
			// A handle arrives spelled either way: as a pointer to nothing, or
			// already carrying the name the backend gives it. Both are the same
			// type, and neither is a raw address.
			if typ.Name == "PtrUnit" || typ.Name == "PtrVoid" {
				return
			}
			if len(typ.Args) > 0 {
				element := typ.Args[0]
				if element.Name == "Unit" || element.Name == "" ||
					element.Kind == ir.TypeVoid {
					return
				}
			}
			found = true
			return
		}
		for _, argument := range typ.Args {
			visit(argument)
		}
	}
	for _, structure := range m.Structs {
		for _, field := range structure.Fields {
			visit(field.Type)
		}
	}
	for _, fn := range m.Funcs {
		for _, param := range fn.Params {
			visit(param.Type)
		}
		visit(fn.ReturnType)
	}
	return found
}
