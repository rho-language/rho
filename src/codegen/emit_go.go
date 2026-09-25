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
// SPDX-License-Identifier: MIT
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
	var body strings.Builder

	if flags := goFeatureFlags(m.Features); len(flags) > 0 {
		body.WriteString("// Feature flags derived from the Rho manifest.\nconst (\n")
		for _, flag := range flags {
			fmt.Fprintf(&body, "\tRHO_FEATURE_%s = 1\n", flag)
		}
		body.WriteString(")\n\n")
	}

	for _, structure := range m.Structs {
		emitGoStruct(&body, structure)
	}
	if len(m.Structs) > 0 {
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
	if strings.Contains(body.String(), "fmt.") {
		b.WriteString("\nimport \"fmt\"\n")
	}
	b.WriteString("\n")
	b.WriteString(body.String())
	return b.String()
}

func emitGoStruct(b *strings.Builder, structure ir.StructIR) {
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
	for _, field := range structure.Fields {
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
	for i := 0; i < len(text); {
		if !isIdentStart(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && isIdentPart(text[i]) {
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
		if strings.HasSuffix(instr.Dest, "=") {
			fmt.Fprintf(b, "%s%s %s\n", pad, goFieldName(strings.TrimSuffix(instr.Dest, "=")), ToGoExpr(instr.Args[0]))
			return
		}
		fmt.Fprintf(b, "%s%s = %s\n", pad, goFieldName(instr.Dest), ToGoExpr(instr.Args[0]))

	case ir.OpLet:
		goType := goTypeForCName(instr.Type)
		if goType == "" {
			goType = "any"
		}
		name := goFieldName(instr.Dest)
		declared[name] = true
		// Go has no `T name = v` form, so a typed local always needs `var`.
		if len(instr.Args) == 1 && instr.Args[0] != "" {
			fmt.Fprintf(b, "%svar %s %s = %s\n", pad, name, goType, ToGoExpr(instr.Args[0]))
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
	case ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeF64:
		return "0"
	case ir.TypeString:
		return "\"\""
	case ir.TypeStruct:
		name := goTypeName(t.Name)
		if name == "" {
			return "nil"
		}
		return name + "{}"
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
		case ir.OpBreak, ir.OpNop:
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
// import.
func EmitGoRuntime(implementation string) string {
	needed := []struct {
		call    string
		define  string
		imports []string
	}{
		{
			call:   "rhoStrEq(",
			define: "func rhoStrEq(a, b string) bool {\n\treturn a == b\n}\n",
		},
		{
			call:    "rhoStringFromI32(",
			define:  "func rhoStringFromI32(value int32) string {\n\treturn strconv.Itoa(int(value))\n}\n",
			imports: []string{"strconv"},
		},
		{
			call: "rhoYamlLoad(",
			define: "func rhoYamlLoad(path string) string {\n" +
				"\tdata, err := os.ReadFile(path)\n" +
				"\tif err != nil {\n\t\treturn \"unknown\"\n\t}\n" +
				"\tfor _, line := range strings.Split(string(data), \"\\n\") {\n" +
				"\t\ttrimmed := strings.TrimSpace(line)\n" +
				"\t\tif strings.HasPrefix(trimmed, \"name:\") {\n" +
				"\t\t\treturn strings.TrimSpace(strings.TrimPrefix(trimmed, \"name:\"))\n" +
				"\t\t}\n" +
				"\t}\n" +
				"\treturn \"unknown\"\n}\n",
			imports: []string{"os", "strings"},
		},
	}

	var body strings.Builder
	imports := []string{}
	for _, helper := range needed {
		if !strings.Contains(implementation, helper.call) {
			continue
		}
		body.WriteString(helper.define)
		body.WriteString("\n")
		imports = append(imports, helper.imports...)
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

// EmitGoMod returns the go.mod file for a generated Go project.
func EmitGoMod(m ir.ModuleIR) string {
	name := goModulePath(m.Name)
	return fmt.Sprintf("module %s\n\ngo 1.22\n", name)
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
	return fmt.Sprintf("//\n// @file %s\n// @brief %s\n//\n// @project %s\n// @version 0.1\n// @date 2026\n//\n// SPDX-License-Identifier: MIT\n//\n\n",
		file, brief, m.Name)
}
