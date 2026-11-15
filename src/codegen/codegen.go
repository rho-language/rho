//
// @file codegen.go
// @brief Rho Compiler - C backend code generator.
//
// This file implements the C backend for the Rho compiler. It consumes
// the Rho IR (ModuleIR, StructIR, FuncIR, etc.) and emits deterministic,
// portable C code. The backend is intentionally simple and explicit,
// with no hidden runtime or magic.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"fmt"
	"strings"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Codegen Type                                                              */
/* -------------------------------------------------------------------------- */

// Codegen is the main C backend for the Rho compiler.
type Codegen struct {
	// b is the emitter, not a builder: it knows how deep it is, which is what
	// stops indentation from being four spaces written at every call site — and
	// what removed the child generator that used to re-indent a nested block by
	// splitting what it had written and prefixing every line.
	b *Emitter
	// foreignHeaders is ir.ModuleIR.ForeignHeaders for the module being emitted.
	foreignHeaders bool
	// foreign maps a qualified function name to the symbol its C header declares.
	// A call site is lowered once for every backend and says the qualified name;
	// this is where C reads it as the name that exists.
	foreign map[string]string
	// hostMain is the function being emitted is a `main` declared void, which C
	// spells `int` so that something can call it and read an exit status.
	//
	// It is here rather than passed down because the consequence is at the
	// instruction: Rho's `main` has nothing to return, so its `return` carries no
	// value, while the C function it became is expected to give one back.
	hostMain bool
	// runtime is the C the compiled program calls into, assembled from the
	// fragments it actually uses. Kept beside the program rather than inside it
	// because they are two translation units, and a caller that wants them
	// separately should not have to cut one file apart at a marker.
	runtime string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new C code generator.
func New() *Codegen {
	return &Codegen{b: NewEmitter()}
}

// EmitModule generates C code for a given IR module and returns it as a string.
func (cg *Codegen) EmitModule(m ir.ModuleIR) string {
	cg.b.Reset()

	// A call site is lowered once, for every backend, and carries the qualified
	// name. Go wants that — every module of a program compiles into one package,
	// where two same-named functions would collide. C cannot use it, because an
	// extern's symbol is whatever its header's compiler agreed on.
	//
	// The rename is done over the whole program before anything is written,
	// rather than at each place an expression is emitted. Every instruction that
	// carries lowered text would otherwise have to remember: a call in a `let`, in
	// a bare statement, in an `if` condition, in a `while`, in a return. Missing
	// one produces an undefined reference to a name that looks like a typo rather
	// than a missing binding, which is a hard thing to read back to this.
	cg.foreign = foreignCallNames(m.Funcs)
	m.Funcs = cg.renameForeignInFuncs(m.Funcs)

	cg.emitTypes(m.Structs, m.Aliases)
	cg.emitFuncValueHelpers(m)
	cg.foreignHeaders = m.ForeignHeaders
	cg.emitFuncPrototypes(m.Funcs)
	cg.emitFuncs(m.Funcs)

	// The runtime, assembled from the fragments the program actually calls.
	//
	// Detected from what was just emitted rather than from the manifest's feature
	// list, because the question is not "what did the project say it uses" but
	// "what does this program call". A program whose declaration is incomplete
	// still gets what it uses, and a program that declares a feature it does not
	// use does not carry the code.
	cg.runtime = runtimeFor(cg.b.String())

	// The header goes in front, once the program is known, so that the runtime
	// prototypes it needs can be gated to the ones it calls.
	body := cg.b.String()
	// Every function value is one C type, declared before any struct that
	// holds one.
	if strings.Contains(body, "RhoFn") || strings.Contains(body, "RHO_FN_NIL") {
		body = funcValueTypedef + body
	}
	cg.b.Reset()
	cg.emitHeader(m)
	if decls := runtimeDecls(body); decls != "" {
		cg.b.WriteString("\n")
		cg.b.WriteString(decls)
	}
	cg.b.WriteString(body)

	return cg.b.String()
}

// Runtime is the C the compiled program calls into, feature-gated.
func (cg *Codegen) Runtime() string { return cg.runtime }

func (cg *Codegen) emitFuncPrototypes(funcs []ir.FuncIR) {
	for _, fn := range funcs {
		// An extern is declared by its header, and that header is included by
		// the generated interface. A prototype here would be the same function
		// declared twice — and declared in two languages, because this one is
		// written in the types Rho mapped the C ones to. Any C type spelled
		// differently in the two, which is every enum and every typedef, makes
		// the pair a compile error over a signature that was already correct.
		//
		// The standard library's own foreign functions are the exception: no
		// header declares them, because the library is where their signature is
		// written, and a platform implements them to it. Without a prototype the
		// call was an implicit declaration — an error in C99 and in GCC 14, and a
		// wrong return type for anything that is not an int.
		//
		// A program's own extern is the same case when nothing declares it: no
		// connected library and no rho-cimport shim. One the C library or the
		// runtime already declares is left to that declaration, which spells the
		// C types (`size_t`, `void*`) a prototype written in Rho's would
		// contradict.
		if fn.Extern && !stdForeignFunction(fn) && !(!cg.foreignHeaders && ownForeignFunction(fn)) {
			continue
		}
		cg.emitFunctionSignature(fn)
		cg.b.WriteString(";\n")
	}
	if len(funcs) > 0 {
		cg.b.WriteString("\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Header                                                          */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitHeader(m ir.ModuleIR) {
	features := make([]string, 0, len(m.Features))
	for _, feature := range m.Features {
		features = append(features,
			strings.ToUpper(strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(feature)))
	}
	// The preamble is a real C header in the repository, embedded and rendered.
	// It is a template with two substitutions and no logic, because it has no
	// logic: a file an editor can highlight and clang-format can rewrite is worth
	// more than one assembled from Go strings, and the parts that do have logic
	// are the parts that are emitted programmatically.
	cg.b.WriteString("/* Rho \u2192 C backend: module ")
	cg.b.WriteString(m.Name)
	cg.b.WriteString(" */\n\n")
	cg.b.WriteString(renderPreamble(m.Name, features))
	cg.b.WriteString("\n")
}

// typeDecl is one type the C program declares before its functions: a struct
// (a declared one or a generic specialisation) or an alias. They are ordered as
// one list, because either may name the other — std.strings.string declares
// `String = Slice[u8]` and then uses `Slice[String]`, whose field is `String*`.
type typeDecl struct {
	name string
	// refs is the C text the declaration is written from: its fields' types,
	// or the alias's target. A name occurring in it is a type that has to be
	// declared first.
	refs []string
	// exactly one of these is set
	structDecl *ir.StructIR
	aliasDecl  *ir.AliasIR
}

// orderedTypeDecls returns the structs and aliases so that each is emitted
// after every type its text names. C needs a type defined before it is used,
// and an anonymous `typedef struct` cannot be declared ahead of its definition;
// a nested specialisation — `Box[Box[u32]]` — refers to one built from it, and
// a specialisation over an alias refers to the alias. Go does not care, and
// reads the same lists either way.
//
// The order is topological and otherwise stable: of the declarations whose
// dependencies are out, the earliest in the input goes first, so a program
// whose types are already in order is emitted as it was.
func orderedTypeDecls(structs []ir.StructIR, aliases []ir.AliasIR) []typeDecl {
	decls := make([]typeDecl, 0, len(structs)+len(aliases))
	for i := range structs {
		s := &structs[i]
		d := typeDecl{name: s.Name, structDecl: s}
		for _, field := range packedFields(*s) {
			// The type is compared as C spells it, because that is the name
			// the definition has to come before: a field of `Box[u32]` is
			// written `Box_u32` in the emitted C.
			d.refs = append(d.refs, cTypeFor(field.Type))
		}
		decls = append(decls, d)
	}
	for i := range aliases {
		a := &aliases[i]
		if a.CTarget == "" {
			continue
		}
		decls = append(decls, typeDecl{name: a.Name, refs: []string{a.CTarget}, aliasDecl: a})
	}

	// deps[i] are the indices of the declarations decls[i] names.
	deps := make([][]int, len(decls))
	for i, d := range decls {
		for j, other := range decls {
			if i == j || other.name == d.name {
				continue
			}
			for _, ref := range d.refs {
				if containsWord(ref, other.name) {
					deps[i] = append(deps[i], j)
					break
				}
			}
		}
	}

	emitted := make([]bool, len(decls))
	out := make([]typeDecl, 0, len(decls))
	for len(out) < len(decls) {
		progress := false
		for i := range decls {
			if emitted[i] {
				continue
			}
			ready := true
			for _, j := range deps[i] {
				if !emitted[j] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			out = append(out, decls[i])
			emitted[i] = true
			progress = true
			// Start again from the top, so a declaration that was waiting on
			// this one goes before a later one that waited on nothing.
			break
		}
		if !progress {
			// A cycle of value types cannot be laid out; emit what is left so
			// the error is the C compiler's about the cycle rather than silence.
			for i := range decls {
				if !emitted[i] {
					out = append(out, decls[i])
					emitted[i] = true
				}
			}
		}
	}
	return out
}

// orderedStructs is orderedTypeDecls over structs alone.
func orderedStructs(structs []ir.StructIR) []ir.StructIR {
	out := make([]ir.StructIR, 0, len(structs))
	for _, d := range orderedTypeDecls(structs, nil) {
		out = append(out, *d.structDecl)
	}
	return out
}

// containsWord reports whether word occurs in text as a whole identifier, so
// `Box_u32` matches inside `Box_u32*` and not inside `Box_u320`.
func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	for index := 0; index+len(word) <= len(text); {
		relative := strings.Index(text[index:], word)
		if relative < 0 {
			return false
		}
		start := index + relative
		end := start + len(word)
		beforeOK := start == 0 || !isIdentPart(text[start-1])
		afterOK := end == len(text) || !isIdentPart(text[end])
		if beforeOK && afterOK {
			return true
		}
		index = start + 1
	}
	return false
}

// packedFields are the fields a struct is stored as: a run of bit fields is one
// integer, everything else is itself. A struct with no bit fields stores its
// fields as declared.
func packedFields(s ir.StructIR) []ir.FieldIR {
	if len(s.Packed) > 0 {
		return s.Packed
	}
	return s.Fields
}

// emitTypes declares the structs and the aliases in the order they refer to one
// another (orderedTypeDecls).
func (cg *Codegen) emitTypes(structs []ir.StructIR, aliases []ir.AliasIR) {
	for _, d := range orderedTypeDecls(structs, aliases) {
		if d.aliasDecl != nil {
			cg.emitAlias(*d.aliasDecl)
		} else {
			cg.emitStruct(*d.structDecl)
		}
	}
}

func (cg *Codegen) emitStruct(s ir.StructIR) {
	// An optional pointer is the pointer. It is declared as the C type it
	// is represented as rather than as a struct with a tag and a payload,
	// because that is what makes a value returned by a C library usable
	// without a translation step in either direction.
	if payload, ok := optionalPointerCType(s.Name); ok {
		cg.b.WriteString("typedef ")
		cg.b.WriteString(payload)
		cg.b.WriteString(" ")
		cg.b.WriteString(s.Name)
		cg.b.WriteString(";\n\n")
		return
	}
	cg.b.WriteString("typedef struct {\n")
	fields, variants, shared := enumLayout(packedFields(s))
	for _, f := range fields {
		cg.b.WriteString("    ")
		cg.b.WriteString(cDeclaration(f.Type, f.Name))
		cg.b.WriteString(";\n")
	}
	// An enum's variants share their storage: a value is one variant at a time,
	// so the values of each are a struct in a union, and the enum is as large as
	// its largest variant and its tag.
	if len(variants) > 0 {
		cg.b.WriteString("    union {\n")
		for _, variant := range variants {
			cg.b.WriteString("        struct {")
			for _, f := range variant.fields {
				cg.b.WriteString(" ")
				cg.b.WriteString(cDeclaration(f.Type, f.Name))
				cg.b.WriteString(";")
			}
			cg.b.WriteString(" } ")
			cg.b.WriteString(variant.name)
			cg.b.WriteString(";\n")
		}
		cg.b.WriteString("    } as;\n")
	}
	// Values stored by their own names share one anonymous union, so each is
	// reached as a member of the struct, as it is in Go.
	if len(shared) > 0 {
		cg.b.WriteString("    union {\n")
		for _, f := range shared {
			cg.b.WriteString("        ")
			cg.b.WriteString(cDeclaration(f.Type, f.Name))
			cg.b.WriteString(";\n")
		}
		cg.b.WriteString("    };\n")
	}
	cg.b.WriteString("} ")
	cg.b.WriteString(s.Name)
	cg.b.WriteString(";\n\n")
}

// enumVariant is the fields of one variant of an enum, in declared order.
type enumVariant struct {
	name   string
	fields []ir.FieldIR
}

// enumLayout splits a struct's fields into its own, those of each variant, and
// those stored by name in shared storage, for an enum. A struct has neither of
// the last two, and its fields come back as they are.
func enumLayout(fields []ir.FieldIR) ([]ir.FieldIR, []enumVariant, []ir.FieldIR) {
	own := []ir.FieldIR{}
	shared := []ir.FieldIR{}
	variants := []enumVariant{}
	at := map[string]int{}
	for _, field := range fields {
		if field.Shared {
			shared = append(shared, field)
			continue
		}
		if field.Variant == "" {
			own = append(own, field)
			continue
		}
		index, seen := at[field.Variant]
		if !seen {
			index = len(variants)
			at[field.Variant] = index
			variants = append(variants, enumVariant{name: field.Variant})
		}
		variants[index].fields = append(variants[index].fields, field)
	}
	return own, variants, shared
}

// emitAlias declares a type alias as a typedef, so a name a program gave a type
// is a name the C compiler knows. The Go backend writes the same aliases as Go
// aliases; here they are C typedefs.
func (cg *Codegen) emitAlias(alias ir.AliasIR) {
	if alias.CTarget == "" {
		return
	}
	cg.b.WriteString("typedef ")
	cg.b.WriteString(alias.CTarget)
	cg.b.WriteString(" ")
	cg.b.WriteString(alias.Name)
	cg.b.WriteString(";\n\n")
}

/* -------------------------------------------------------------------------- */
/*  Internal: Functions                                                       */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitFuncs(funcs []ir.FuncIR) {
	for _, fn := range funcs {
		if fn.Extern {
			// An extern is declared by the header it came from, and that header
			// is included by the generated interface. Restating it here would be
			// redundant at best and wrong at worst: the declaration here is in
			// terms of the types Rho mapped the C ones to, and any C type that is
			// not spelled the same way in both — an enum, a typedef, anything
			// whose width was inferred — makes the two declarations conflict and
			// fails the compile over a signature that was already correct.
			//
			// So nothing is emitted. The name it is called by is decided at the
			// call site, and the declaration is the header's.
			continue
		}
		// signature
		returnType := fn.ReturnType
		hostMain := fn.Name == "main" && returnType.Kind == ir.TypeVoid
		if hostMain {
			cg.b.WriteString("int")
		} else {
			cg.b.WriteString(cTypeFor(returnType))
		}
		cg.b.WriteString(" ")
		name := cSymbolFor(fn)
		cg.b.WriteString(name)
		cg.b.WriteString("(")

		for i, p := range fn.Params {
			if i > 0 {
				cg.b.WriteString(", ")
			}
			ptype := cTypeFor(p.Type)
			if arrayIdx := strings.IndexByte(ptype, '['); arrayIdx >= 0 {
				cg.b.WriteString(ptype[:arrayIdx])
				cg.b.WriteString(" ")
				cg.b.WriteString(p.Name)
				cg.b.WriteString(ptype[arrayIdx:])
			} else {
				cg.b.WriteString(ptype)
				cg.b.WriteString(" ")
				cg.b.WriteString(p.Name)
			}
		}

		cg.b.WriteString(")")
		cg.b.EmitLine(" {")
		// The body is emitted one level in, which is the emitter's whole job: the
		// instructions no longer carry their own leading spaces, and a nested block
		// is deeper because the emitter was told it was, not because every line it
		// wrote was prefixed again on the way out.
		cg.b.Indent()

		// body (Rho 0.1: minimal, based on IR instructions)
		cg.hostMain = hostMain
		for _, instr := range fn.Body {
			cg.emitInstr(instr)
		}
		cg.hostMain = false
		if hostMain {
			// A void `main` is emitted as `int` so that a real entry point can
			// call it, which means the C compiler expects a value back. Rho's
			// `main` cannot return one — it is declared void — so falling off the
			// end means it ran to completion, which is success.
			//
			// Without this the function returns whatever happened to be in the
			// return register, and that becomes the process's exit status: a
			// program that printed exactly what it should have still reported
			// failure to whatever ran it.
			cg.b.EmitLine("return 0;")
		} else if len(fn.Body) == 0 && fn.ReturnType.Kind != ir.TypeVoid {
			cg.b.EmitLine("return " + defaultValueFor(returnType) + ";")
		}

		cg.b.Dedent()
		cg.b.EmitLine("}")
		cg.b.EmitLine("")
	}
}

func (cg *Codegen) emitFunctionSignature(fn ir.FuncIR) {
	name := cSymbolFor(fn)
	if fn.Name == "main" && fn.ReturnType.Kind == ir.TypeVoid {
		cg.b.WriteString("int")
	} else {
		cg.b.WriteString(cTypeFor(fn.ReturnType))
	}
	cg.b.WriteString(" ")
	cg.b.WriteString(name)
	cg.b.WriteString("(")
	for i, p := range fn.Params {
		if i > 0 {
			cg.b.WriteString(", ")
		}
		ptype := cTypeFor(p.Type)
		if arrayIdx := strings.IndexByte(ptype, '['); arrayIdx >= 0 {
			cg.b.WriteString(ptype[:arrayIdx])
			cg.b.WriteString(" ")
			cg.b.WriteString(p.Name)
			cg.b.WriteString(ptype[arrayIdx:])
		} else {
			cg.b.WriteString(ptype)
			cg.b.WriteString(" ")
			cg.b.WriteString(p.Name)
		}
	}
	cg.b.WriteString(")")
}

// cZeroInitializer is what a local of a C type starts as when the program gave
// it no value: 0 for a number, "" for a string (Go's zero string, and what the
// runtime treats a NULL string as), NULL for a pointer and `{0}` for an array, a
// struct or an enum that carries data.
func cZeroInitializer(cType string) string {
	trimmed := strings.TrimSpace(cType)
	switch {
	case strings.Contains(trimmed, "[0]"):
		// An array of no elements has nothing to set, and C allows no
		// initialiser element for it.
		return ""
	case strings.Contains(trimmed, "["):
		return "{0}"
	case trimmed == "const char*" || trimmed == "const char *":
		return "\"\""
	case strings.HasSuffix(trimmed, "*"):
		return "NULL"
	}
	switch trimmed {
	case "bool", "uint8_t", "uint16_t", "uint32_t", "uint64_t", "int8_t", "int16_t", "int32_t", "int64_t",
		"float", "double", "size_t", "uintptr_t", "int", "unsigned", "char":
		return "0"
	}
	return "{0}"
}

func defaultValueFor(t ir.Type) string {
	switch t.Kind {
	case ir.TypeBool, ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeI64, ir.TypeF64, ir.TypeF32:
		return "0"
	case ir.TypeString:
		return "NULL"
	case ir.TypeStruct:
		return "(" + t.Name + "){0}"
	default:
		return "0"
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Instructions                                                    */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitInstr(instr ir.Instr) {
	// Every line goes through `EmitLine`, which puts it at the current depth. The
	// four spaces that used to start every one of these strings were the whole of
	// the indentation scheme, and they had to be written again at each site — and
	// again, differently, by the child generator that re-indented a nested block by
	// prefixing what it had already written.
	switch instr.Op {
	case ir.OpReturn:
		// A `return` with no expression is lowered with one operand, and that
		// operand is empty — so the operand's emptiness is what says whether there
		// is a value here, not how many operands arrived.
		value := ""
		if len(instr.Args) == 1 {
			value = strings.TrimSpace(instr.Args[0])
		}
		switch {
		case value != "":
			cg.b.EmitLine("return " + value + ";")
		case cg.hostMain:
			// A `main` declared void has nothing to hand back, and the `int` it was
			// emitted as is expected to hand something back. Zero, because reaching
			// the end of a void `main` is the same thing as saying so: the program
			// ran to completion, which is success.
			cg.b.EmitLine("return 0;")
		default:
			cg.b.EmitLine("return;")
		}

	case ir.OpAssign:
		if len(instr.Args) == 1 {
			operator := " = "
			if strings.HasSuffix(instr.Dest, "=") {
				operator = " "
			}
			cg.b.EmitLine(instr.Dest + operator + arenaKept(instr.Arena, instr.Args[0]) + ";")
		}

	case ir.OpLet:
		// An array type like "uint8_t[1024]" needs the dimension after the
		// variable name, not after the type: C spells it "uint8_t buf[1024];".
		line := instr.Type + " " + instr.Dest
		if arrayIdx := strings.IndexByte(instr.Type, '['); arrayIdx >= 0 {
			line = instr.Type[:arrayIdx] + " " + instr.Dest + instr.Type[arrayIdx:]
		}
		if len(instr.Args) == 1 && instr.Args[0] != "" {
			value := cg.rewriteForeign(instr.Args[0])
			// A dynamic array `[T]` is a pointer in C, and a pointer cannot be
			// initialised from a brace list. The literal becomes a compound
			// array whose address the pointer takes, which lives as long as the
			// block it is written in just as the binding does.
			if strings.HasPrefix(strings.TrimSpace(value), "{") && strings.HasSuffix(instr.Type, "*") {
				value = "(" + strings.TrimSuffix(instr.Type, "*") + "[])" + value
			}
			line += " = " + arenaKept(instr.Arena, value)
		} else if zero := cZeroInitializer(instr.Type); zero != "" {
			// A local declared without a value is the zero of its type, as it is
			// in Go: `var x: u32` reads 0 and `var s: Shape` its first variant,
			// never what was on the stack.
			line += " = " + zero
		}
		cg.b.EmitLine(line + ";")

	case ir.OpExpr:
		if len(instr.Args) == 1 {
			cg.b.EmitLine(cg.rewriteForeign(instr.Args[0]) + ";")
		}

	case ir.OpIf:
		condition := instr.Condition
		if instr.Arena != "" {
			// The condition's temporaries are given back once it is evaluated;
			// the branch it chooses reads none of them.
			condition = "rho_arena_release_cond(" + instr.Arena + ", " + condition + ")"
		}
		cg.b.EmitLine("if (" + condition + ") {")
		cg.b.Indent()
		cg.emitBlock(instr.Then)
		cg.b.Dedent()
		if len(instr.Else) > 0 {
			cg.b.EmitLine("} else {")
			cg.b.Indent()
			cg.emitBlock(instr.Else)
			cg.b.Dedent()
		}
		cg.b.EmitLine("}")

	case ir.OpWhile:
		condition := instr.Condition
		if instr.Arena != "" {
			// A loop whose iterations cannot be released still gives back what
			// each test of its condition built: a mark before the test and a
			// release after it, every time it is evaluated.
			condition = "(" + instr.Arena + " = rho_arena_mark(), rho_arena_release_cond(" + instr.Arena + ", " + condition + "))"
		}
		cg.b.EmitLine("while (" + condition + ") {")
		cg.b.Indent()
		cg.emitBlock(instr.Then)
		cg.b.Dedent()
		cg.b.EmitLine("}")

	case ir.OpBreak:
		cg.b.EmitLine("break;")

	case ir.OpContinue:
		cg.b.EmitLine("continue;")

	case ir.OpCall:
		line := ""
		if instr.Dest != "" {
			line = instr.Dest + " = "
		}
		if len(instr.Args) > 0 {
			line += instr.Args[0] + "(" + strings.Join(instr.Args[1:], ", ") + ");"
			cg.b.EmitLine(line)
		}

	case ir.OpNop:
		cg.b.EmitLine("/* nop */")

	case ir.OpArenaMark:
		cg.b.EmitLine("size_t " + instr.Dest + " = rho_arena_mark();")

	case ir.OpArenaRelease:
		if len(instr.Args) > 0 {
			// The strings the loop keeps are moved down to the mark, and the
			// variables holding them updated; everything else is given back.
			cg.b.EmitLine(fmt.Sprintf("rho_arena_keep_many(%s, (void*[]){%s}, %d);",
				instr.Dest, strings.Join(instr.Args, ", "), len(instr.Args)))
			break
		}
		cg.b.EmitLine("rho_arena_release(" + instr.Dest + ");")

	case ir.OpRaw:
		// `OpRaw` carries text that is already a whole line, so it is emitted at
		// the current depth rather than being written verbatim: a raw statement
		// inside a block belongs at that block's indentation like any other.
		cg.b.EmitLine(instr.Dest)

	default:
		cg.b.EmitLine("/* unknown op */")
	}
}

// arenaKept is a stored string kept past the arena mark its statement took:
// everything else the statement built is given back, and the string is moved
// down to the mark. With no mark it is the value as it stands.
func arenaKept(mark, value string) string {
	if mark == "" {
		return value
	}
	return "rho_arena_keep(" + mark + ", " + value + ")"
}

// emitBlock is a nested block of instructions, at the caller's indentation.
//
// It used to need a generator of its own and a pass over what that child had
// written, prefixing four spaces to every line to move it inwards. An emitter that
// knows its depth needs neither: the instructions are emitted where they are, at
// the depth the caller set.
func (cg *Codegen) emitBlock(instructions []ir.Instr) {
	for _, instruction := range instructions {
		cg.emitInstr(instruction)
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Type Mapping                                                    */
/* -------------------------------------------------------------------------- */

// isUnitType reports whether a type is the synthesised empty struct that means
// "no value". A pointer to it is a pointer to nothing.
func isUnitType(t ir.Type) bool {
	if t.Kind == ir.TypeVoid {
		return true
	}
	return t.Kind == ir.TypeStruct && (t.Name == "Unit" || t.Name == "RhoUnit")
}

// cDeclaration declares a name of a type in C, where a fixed array's length
// follows the name: `uint32_t items[4]`, not `uint32_t[4] items`.
func cDeclaration(t ir.Type, name string) string {
	ctype := cTypeFor(t)
	if open := strings.IndexByte(ctype, '['); open >= 0 && strings.HasSuffix(ctype, "]") {
		return ctype[:open] + " " + name + ctype[open:]
	}
	return ctype + " " + name
}

// cTypeFor maps IR types to C types.
func cTypeFor(t ir.Type) string {
	suffix := ""
	if t.ArrayLen > 0 {
		suffix = fmt.Sprintf("[%d]", t.ArrayLen)
	}
	switch t.Kind {
	case ir.TypeVoid:
		return "void" + suffix
	case ir.TypeBool:
		return "bool" + suffix
	case ir.TypeU8:
		return "uint8_t" + suffix
	case ir.TypeU16:
		return "uint16_t" + suffix
	case ir.TypeU32:
		return "uint32_t" + suffix
	case ir.TypeU64:
		return "uint64_t" + suffix
	case ir.TypeI8:
		return "int8_t" + suffix
	case ir.TypeI16:
		return "int16_t" + suffix
	case ir.TypeI32:
		return "int32_t" + suffix
	case ir.TypeI64:
		return "int64_t" + suffix
	case ir.TypeF64:
		return "double" + suffix
	case ir.TypeF32:
		return "float" + suffix
	case ir.TypeString:
		return "const char*" + suffix
	case ir.TypeStruct:
		if t.Name != "" {
			// The written name of an array of structs carries its length,
			// `P[4]`; the element is P and the length follows the declared
			// name, so the length is cut from the name before it is mangled.
			name := t.Name
			if t.ArrayLen > 0 {
				name = strings.TrimSuffix(name, suffix)
			}
			return strings.NewReplacer("[", "_", "]", "", ",", "_", " ", "").Replace(name) + suffix
		}
		return "struct /* anonymous */"
	case ir.TypeSlice:
		// A dynamic array is a pointer to its element in C: both are `T*`, and
		// what differs — the length — is what the program tracks. The element
		// spells the pointer.
		if len(t.Args) > 0 {
			return cTypeFor(t.Args[0]) + "*" + suffix
		}
		return "uint8_t*" + suffix
	case ir.TypeFunc:
		return "RhoFn" + suffix
	case ir.TypePtr:
		// `Ptr[T]` is `T*`, wherever it was written. A pointer to nothing —
		// `Ptr[Unit]`, which is how a foreign handle is declared — is what C
		// calls void*.
		if len(t.Args) == 0 || isUnitType(t.Args[0]) {
			return "void*"
		}
		return cTypeFor(t.Args[0]) + "*"
	default:
		return "void /* invalid */"
	}
}

/* -------------------------------------------------------------------------- */
/*  Debug Helpers                                                             */
/* -------------------------------------------------------------------------- */

// Dump returns the current buffer content (for debugging).
func (cg *Codegen) Dump() string {
	return cg.b.String()
}

// EmitModuleToStdout is a convenience helper for quick experiments.
func (cg *Codegen) EmitModuleToStdout(m ir.ModuleIR) {
	fmt.Println(cg.EmitModule(m))
}

// renameForeignInFuncs returns the functions with every lowered expression
// renamed to the C symbols that exist.
func (cg *Codegen) renameForeignInFuncs(funcs []ir.FuncIR) []ir.FuncIR {
	if len(cg.foreign) == 0 {
		return funcs
	}
	out := make([]ir.FuncIR, len(funcs))
	copy(out, funcs)
	for index := range out {
		body := make([]ir.Instr, len(out[index].Body))
		copy(body, out[index].Body)
		for i := range body {
			cg.renameForeignInInstr(&body[i])
		}
		out[index].Body = body
	}
	return out
}

// renameForeignInInstr renames every piece of lowered text one instruction
// carries.
//
// The instruction's slices are copied before they are written: the IR is
// shared with every other backend emitted from it, and renaming in place made
// a Go build see C's names when C was emitted first, and its own when it was
// not — which is how a Go-only build lost the runtime's names.
func (cg *Codegen) renameForeignInInstr(instr *ir.Instr) {
	renameForeignIn(instr, cg.rewriteForeign)
}

// renameForeignIn rewrites every piece of lowered text an instruction carries,
// on copies of its slices.
func renameForeignIn(instr *ir.Instr, rewrite func(string) string) {
	if len(instr.Args) > 0 {
		args := make([]string, len(instr.Args))
		for i, arg := range instr.Args {
			args[i] = rewrite(arg)
		}
		instr.Args = args
	}
	instr.Condition = rewrite(instr.Condition)
	if len(instr.Then) > 0 {
		then := make([]ir.Instr, len(instr.Then))
		copy(then, instr.Then)
		for i := range then {
			renameForeignIn(&then[i], rewrite)
		}
		instr.Then = then
	}
	if len(instr.Else) > 0 {
		otherwise := make([]ir.Instr, len(instr.Else))
		copy(otherwise, instr.Else)
		for i := range otherwise {
			renameForeignIn(&otherwise[i], rewrite)
		}
		instr.Else = otherwise
	}
}

// renameRuntimeExterns is the Go backend's half: a runtime function an
// imported module declares extern (`rho_fd_read` of std.os.sys) is called by
// its own name, which the Go runtime supplies as rhoFdRead, not by the
// qualified name its call site was lowered with.
func renameRuntimeExterns(funcs []ir.FuncIR) []ir.FuncIR {
	names := map[string]string{}
	for qualified, plain := range foreignCallNames(funcs) {
		if strings.HasPrefix(plain, "rho_") {
			names[qualified] = plain
		}
	}
	if len(names) == 0 {
		return funcs
	}
	rewrite := func(text string) string {
		if !strings.Contains(text, "_") {
			return text
		}
		for qualified, plain := range names {
			text = replaceWord(text, qualified, plain)
		}
		return text
	}
	out := make([]ir.FuncIR, len(funcs))
	copy(out, funcs)
	for index := range out {
		body := make([]ir.Instr, len(out[index].Body))
		copy(body, out[index].Body)
		for i := range body {
			renameForeignIn(&body[i], rewrite)
		}
		out[index].Body = body
	}
	return out
}

// rewriteForeign reads a lowered expression as C.
//
// A call site is lowered once, for every backend, and carries the qualified name
// that tells two same-named functions apart — which Go needs and C cannot use,
// because an extern's symbol is whatever its header's compiler agreed on. This is
// where a C expression says the name that exists.
//
// Only a whole word is replaced. The text is not parsed, and a shorter name that
// happens to be a prefix of a longer one must not rewrite the longer one.
func (cg *Codegen) rewriteForeign(text string) string {
	if len(cg.foreign) == 0 || !strings.Contains(text, "_") {
		return text
	}
	for qualified, foreign := range cg.foreign {
		text = replaceWord(text, qualified, foreign)
	}
	return text
}

// replaceWord replaces whole-word occurrences, delimited by anything that cannot
// be part of an identifier.
func replaceWord(text, from, to string) string {
	var out strings.Builder
	for {
		at := strings.Index(text, from)
		if at < 0 {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:at])
		before := byte(' ')
		if at > 0 {
			before = text[at-1]
		}
		after := byte(' ')
		if at+len(from) < len(text) {
			after = text[at+len(from)]
		}
		if isIdentifierByte(before) || isIdentifierByte(after) {
			out.WriteString(text[at : at+len(from)])
		} else {
			out.WriteString(to)
		}
		text = text[at+len(from):]
	}
}

func isIdentifierByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// foreignCallNames maps each extern's qualified name to the name its header gave
// it.
//
// A call site is lowered once, for every backend, and carries the qualified name
// because that is what distinguishes two functions of the same name — which Go
// needs, since every module of a program compiles into one package. C does not
// need it and cannot use it: an extern's symbol is whatever its header's
// compiler agreed on, and the header knows nothing about this language.
//
// So the qualified name is what the call site says, and this is where C turns it
// back into the symbol that exists.
func foreignCallNames(funcs []ir.FuncIR) map[string]string {
	if len(funcs) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, fn := range funcs {
		if !fn.Extern || fn.CName == "" || fn.CName == fn.Name {
			continue
		}
		names[fn.CName] = fn.Name
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// stdForeignFunction reports whether an extern is one the standard library
// declares for a platform to implement — `gpio_hw_init` of std.embedded.gpio —
// rather than one a C header or the runtime already declares. The runtime's own
// (`rho_*`) are declared by its preamble.
// ownForeignFunction reports whether an extern is the program's own: not the
// runtime's (`rho_*` and every symbol a runtime fragment provides) and not the C
// library's, both of which are declared before the program.
func ownForeignFunction(fn ir.FuncIR) bool {
	name := fn.CName
	if name == "" {
		name = fn.Name
	}
	if strings.HasPrefix(name, "rho_") || strings.HasPrefix(fn.Name, "rho_") || cLibraryFunctions[name] || runtimeSymbol(name) {
		return false
	}
	return true
}

// runtimeSymbol reports whether a runtime fragment provides a symbol.
func runtimeSymbol(name string) bool {
	for _, fragment := range runtimeFragments {
		for _, provided := range fragment.Provides {
			if provided == name {
				return true
			}
		}
	}
	return false
}

// cLibraryFunctions are the C library functions the generated C's own includes
// (stdio.h, stdlib.h, string.h, and math.h in the runtime) declare, which a
// program may name as an extern.
var cLibraryFunctions = map[string]bool{
	"abort": true, "exit": true, "_Exit": true, "atexit": true, "malloc": true, "calloc": true, "realloc": true, "free": true,
	"printf": true, "fprintf": true, "sprintf": true, "snprintf": true, "vprintf": true, "puts": true, "putchar": true,
	"getchar": true, "fputs": true, "fputc": true, "fgets": true, "fgetc": true, "fflush": true, "fopen": true, "fclose": true,
	"fread": true, "fwrite": true, "fseek": true, "ftell": true, "remove": true, "rename": true, "perror": true,
	"strlen": true, "strcmp": true, "strncmp": true, "strcpy": true, "strncpy": true, "strcat": true, "strncat": true,
	"strchr": true, "strrchr": true, "strstr": true, "strdup": true, "strerror": true, "strtok": true,
	"memcpy": true, "memmove": true, "memset": true, "memcmp": true, "memchr": true,
	"atoi": true, "atol": true, "atof": true, "strtol": true, "strtoul": true, "strtoll": true, "strtoull": true, "strtod": true,
	"rand": true, "srand": true, "abs": true, "labs": true, "llabs": true, "qsort": true, "bsearch": true, "getenv": true, "system": true,
	"sqrt": true, "sin": true, "cos": true, "tan": true, "atan": true, "atan2": true, "floor": true, "ceil": true, "pow": true,
	"fabs": true, "exp": true, "log": true, "log10": true, "fmod": true, "round": true, "trunc": true,
	"sqrtf": true, "sinf": true, "cosf": true, "floorf": true, "fabsf": true, "powf": true,
	"time": true, "clock": true,
}

func stdForeignFunction(fn ir.FuncIR) bool {
	return fn.Extern && strings.HasPrefix(fn.CName, "std_") && !strings.HasPrefix(fn.Name, "rho_")
}

// cSymbolFor is the name a function has in C.
//
// It is the name a header gave it when the function is an extern, and a name Rho
// chose — carrying its module — when it is not.
//
// The difference is the whole of a foreign binding. An extern is a function some
// C header already declared, and its symbol is whatever that header's compiler
// agreed on; the header does not know this language exists and could not have
// avoided the name. Prefixing it produces a call to a function that was never
// defined, and the error names a symbol that looks like a typo rather than a
// missing binding.
//
// Qualification is not redundant where it is applied. Every module of a Rho
// program is emitted into one C file, so two modules declaring their own `init`
// would otherwise collide, and a name Rho chose is one Rho may namespace.
func cSymbolFor(fn ir.FuncIR) string {
	if fn.Extern {
		return fn.Name
	}
	if fn.CName != "" {
		return fn.CName
	}
	return fn.Name
}

// funcValueTypedef is what every function value is in C: the function, called
// with the environment first, and the environment (types/closures.go). The
// function is held as a generic function pointer and converted back to its own
// type where it is called, which is the one conversion C defines for one.
const funcValueTypedef = `/* A function value: a function that takes its environment first, and the
 * environment, NULL for a named function. */
typedef struct {
    void (*fn)(void);
    void* env;
} RhoFn;
#define RHO_FN_NIL ((RhoFn){ NULL, NULL })

`

// emitFuncValueHelpers writes the helpers function values are made, called
// and captured with (ir.ModuleIR.FuncSigs and Envs), after the structs whose
// types they name.
func (cg *Codegen) emitFuncValueHelpers(m ir.ModuleIR) {
	for _, env := range m.Envs {
		cg.b.WriteString("static inline void* " + env + "_make(" + env + " v)\n{\n")
		cg.b.WriteString("    " + env + "* e = (" + env + "*)rho_env_alloc(sizeof(" + env + "));\n")
		cg.b.WriteString("    *e = v;\n    return e;\n}\n")
		cg.b.WriteString("static inline " + env + " " + env + "_at(void* e)\n{\n    return *(" + env + "*)e;\n}\n")
	}
	for _, sig := range m.FuncSigs {
		cg.b.WriteString("typedef RhoFn " + sig.Name + ";\n")
	}
	for _, sig := range m.FuncSigs {
		params := sig.Type.FuncParams()
		result := sig.Type.FuncResult()
		returns := cTypeFor(result)
		if isUnitType(result) {
			returns = "void"
		}
		types := []string{"void*"}
		named := []string{"RhoFn f"}
		args := []string{"f.env"}
		for i, param := range params {
			name := fmt.Sprintf("a%d", i)
			types = append(types, cTypeFor(param))
			named = append(named, cTypeFor(param)+" "+name)
			args = append(args, name)
		}
		pointer := returns + " (*)(" + strings.Join(types, ", ") + ")"
		cg.b.WriteString("static inline " + returns + " " + sig.Name + "_call(" + strings.Join(named, ", ") + ")\n{\n")
		cg.b.WriteString("    if (f.fn == NULL) {\n        rho_panic_fn();\n    }\n")
		call := "((" + pointer + ")f.fn)(" + strings.Join(args, ", ") + ")"
		if returns == "void" {
			cg.b.WriteString("    " + call + ";\n}\n")
		} else {
			cg.b.WriteString("    return " + call + ";\n}\n")
		}
		cg.b.WriteString("static inline RhoFn " + sig.Name + "_bind(" + returns + " (*fn)(" + strings.Join(types, ", ") + "), void* env)\n{\n")
		cg.b.WriteString("    RhoFn f;\n    f.fn = (void (*)(void))fn;\n    f.env = env;\n    return f;\n}\n")
	}
	if len(m.Envs) > 0 || len(m.FuncSigs) > 0 {
		cg.b.WriteString("\n")
	}
}
