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
// SPDX-License-Identifier: Apache-2.0
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

	cg.emitStructs(m.Structs)
	cg.emitAliases(m.Aliases)
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
		if fn.Extern {
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

// orderedStructs returns the structs so that one is emitted after every struct
// its fields name. C needs a type defined before it is used, and a nested
// specialisation — `Box[Box[u32]]` — refers to one built from it. Go does not
// care, and reads the same list either way.
func orderedStructs(structs []ir.StructIR) []ir.StructIR {
	emitted := map[string]bool{}
	out := make([]ir.StructIR, 0, len(structs))
	pending := append([]ir.StructIR(nil), structs...)
	for len(pending) > 0 {
		progress := false
		kept := pending[:0:0]
		for _, s := range pending {
			ready := true
			for _, field := range packedFields(s) {
				// The type is compared as C spells it, because that is the name
				// the definition has to come before: a field of `Box[u32]` is
				// written `Box_u32` in the emitted C.
				spelling := cTypeFor(field.Type)
				for _, other := range structs {
					if other.Name != s.Name && containsWord(spelling, other.Name) && !emitted[other.Name] {
						ready = false
						break
					}
				}
				if !ready {
					break
				}
			}
			if ready {
				out = append(out, s)
				emitted[s.Name] = true
				progress = true
			} else {
				kept = append(kept, s)
			}
		}
		pending = kept
		if !progress {
			// A cycle of value types cannot be laid out; emit what is left so
			// the error is the C compiler's about the cycle rather than silence.
			out = append(out, pending...)
			break
		}
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

func (cg *Codegen) emitStructs(structs []ir.StructIR) {
	for _, s := range orderedStructs(structs) {
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
			continue
		}
		cg.b.WriteString("typedef struct {\n")
		for _, f := range packedFields(s) {
			cg.b.WriteString("    ")
			cg.b.WriteString(cTypeFor(f.Type))
			cg.b.WriteString(" ")
			cg.b.WriteString(f.Name)
			cg.b.WriteString(";\n")
		}
		cg.b.WriteString("} ")
		cg.b.WriteString(s.Name)
		cg.b.WriteString(";\n\n")
	}
}

// emitAliases declares each type alias as a typedef, so a name a program gave
// a type is a name the C compiler knows. The Go backend writes the same aliases
// as Go aliases; here they are C typedefs.
func (cg *Codegen) emitAliases(aliases []ir.AliasIR) {
	for _, alias := range aliases {
		if alias.CTarget == "" {
			continue
		}
		cg.b.WriteString("typedef ")
		cg.b.WriteString(alias.CTarget)
		cg.b.WriteString(" ")
		cg.b.WriteString(alias.Name)
		cg.b.WriteString(";\n\n")
	}
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
			cg.b.EmitLine(instr.Dest + operator + instr.Args[0] + ";")
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
			line += " = " + value
		}
		cg.b.EmitLine(line + ";")

	case ir.OpExpr:
		if len(instr.Args) == 1 {
			cg.b.EmitLine(cg.rewriteForeign(instr.Args[0]) + ";")
		}

	case ir.OpIf:
		cg.b.EmitLine("if (" + instr.Condition + ") {")
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
		cg.b.EmitLine("while (" + instr.Condition + ") {")
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

	case ir.OpRaw:
		// `OpRaw` carries text that is already a whole line, so it is emitted at
		// the current depth rather than being written verbatim: a raw statement
		// inside a block belongs at that block's indentation like any other.
		cg.b.EmitLine(instr.Dest)

	default:
		cg.b.EmitLine("/* unknown op */")
	}
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
			// A mangled pointer name is a pointer, not a structure. `Ptr_X` and
			// `X_Ptr` are the same pointer written two ways — one is what a
			// source writes and the other is what a foreign declaration
			// carries — and the C a declaration has to be given is `X*`. Spelling
			// it as a structure names a type no backend emitted, and the
			// declaration that uses it does not compile.
			if pointer, ok := mangledPointerCType(t.Name); ok {
				return pointer + suffix
			}
			return strings.NewReplacer("[", "_", "]", "", ",", "_").Replace(t.Name) + suffix
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
	case ir.TypePtr:
		// A pointer is a pointer. A pointer to nothing — `Ptr[Unit]`, which is
		// how a foreign handle is declared — is what C calls void*, and spelling
		// it as a mangled name would make every handle undeclareable.
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
func (cg *Codegen) renameForeignInInstr(instr *ir.Instr) {
	for i := range instr.Args {
		instr.Args[i] = cg.rewriteForeign(instr.Args[i])
	}
	instr.Condition = cg.rewriteForeign(instr.Condition)
	for i := range instr.Then {
		inner := instr.Then[i]
		cg.renameForeignInInstr(&inner)
		instr.Then[i] = inner
	}
	for i := range instr.Else {
		inner := instr.Else[i]
		cg.renameForeignInInstr(&inner)
		instr.Else[i] = inner
	}
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
