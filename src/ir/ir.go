//
// @file ir.go
// @brief Rho Compiler - Intermediate Representation (IR).
//
// This file defines the core Intermediate Representation (IR) used by the
// Rho compiler. The IR is a deterministic, low-level model derived from
// the AST and designed to map directly into C code. All IR nodes are
// explicit, predictable, and free of hidden runtime behavior.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package ir

/* -------------------------------------------------------------------------- */
/*  Basic Types                                                               */
/* -------------------------------------------------------------------------- */

// TypeKind represents the category of a type in the IR.
type TypeKind int

const (
	TypeInvalid TypeKind = iota
	TypeVoid
	TypeBool
	TypeU8
	TypeU16
	TypeU32
	TypeU64
	// TypeI8 and TypeI16 are the signed widths below 32 bits. They are kinds of
	// their own rather than i32 with a narrow range, because a value crosses a
	// foreign boundary at the width the C declaration named: an int16_t read as
	// an int32 reads the wrong bytes.
	TypeI8
	TypeI16
	TypeI32
	// TypeI64 is a signed 64-bit integer. A database column is one, and so is
	// anything read from a file or a clock: i32 is a width, and a value that
	// came from outside is not one the program chose.
	TypeI64
	TypeF64
	TypeString
	TypeStruct
	// TypePtr is a pointer to another type, `Ptr[T]`, with T as its one
	// argument. It is one type wherever it is written — a field, a parameter,
	// a local, a foreign signature — and each backend spells it once: C as `T*`
	// (`void*` for `Ptr[Unit]`), Go as `unsafe.Pointer`.
	TypePtr
	// TypeSlice is a dynamic array, written `[T]`. It is distinct from a pointer
	// because the two are the same in C — both are `T*` with a length the
	// program tracks — but not in Go, where a slice is `[]T` and a pointer is
	// `*T`. Keeping them apart is what lets both backends spell their own.
	TypeSlice
	// TypeF32 is a 32-bit float. It is a width of its own rather than the
	// default float, because a value that crosses a foreign boundary has the
	// width the C function declared: `float` and `double` return in different
	// registers on an ARM EABI target, so spelling an `f32` as a `double` is a
	// declaration that compiles and a call that reads the wrong register.
	TypeF32
)

// Type represents a concrete type in the IR.
type Type struct {
	Kind     TypeKind
	Name     string // for structs or user-defined types
	Args     []Type
	ArrayLen int
}

// SameType reports whether two types are the same one.
//
// A pointer is one type, `TypePtr`, wherever it was written, and it is the
// pointer at its element: two pointers are the same when they point at the same
// thing, whatever spelling of the element's name each carries in its own.
func SameType(a, b Type) bool {
	if a.Kind == TypePtr && b.Kind == TypePtr && len(a.Args) == 1 && len(b.Args) == 1 {
		return a.ArrayLen == b.ArrayLen && SameType(a.Args[0], b.Args[0])
	}
	if a.Kind != b.Kind || a.Name != b.Name || a.ArrayLen != b.ArrayLen {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for index := range a.Args {
		if !SameType(a.Args[index], b.Args[index]) {
			return false
		}
	}
	return true
}

/* -------------------------------------------------------------------------- */
/*  Module                                                                    */
/* -------------------------------------------------------------------------- */

// ModuleIR is the IR representation of a Rho module.
type ModuleIR struct {
	Name string
	// Version is the manifest version, so a generated service can report it.
	Version  string
	Features []string
	Funcs    []FuncIR
	Structs  []StructIR
	// Routes are the HTTP routes the module exposes, meaningful for a server
	// target.
	Routes []RouteIR
	// Aliases are the names a module gave to other types, as in
	// `type SqlDb = Ptr[Unit]`. They are carried because a backend has to
	// define them: a program may declare a value under the alias, and a file
	// that emitted only the type it names would leave that declaration referring
	// to a name nothing defines.
	Aliases []AliasIR
	// CRefusals are the things the program does that the C backend cannot
	// emit, each a sentence naming where. They are carried rather than raised
	// while lowering because one lowering serves every backend: a program built
	// only for Go is not refused for what it would mean in C. A build that
	// emits C fails on the first of them.
	CRefusals []string
}

// AliasIR is one type alias a module declares.
type AliasIR struct {
	// Name is the alias as the program wrote it.
	Name string
	// Target is the type it names, as the program wrote it. Each backend
	// translates it, because one type name can be two types: a dynamic array
	// `[u32]` is a Go slice and a C pointer.
	Target string
	// CTarget is the C spelling of the target, for the backend that needs it
	// without a translator of its own.
	CTarget string
}

/* -------------------------------------------------------------------------- */
/*  Structs                                                                   */
/* -------------------------------------------------------------------------- */

// StructIR represents a struct in IR.
type StructIR struct {
	Name   string
	CName  string
	Fields []FieldIR
	// Packed are the fields as they are stored: a run of bit fields becomes one
	// integer of the run's type, and every other field stands alone. A struct
	// with no bit fields has Packed equal to Fields.
	Packed []FieldIR
}

// FieldIR represents a single field in a struct.
type FieldIR struct {
	Name string
	Type Type
	// BitWidth is the width of a bit field, 0 for an ordinary field.
	BitWidth int
	// Unit is the storage field a bit field lives in, and Shift its bit offset
	// there. Both are empty and 0 for an ordinary field.
	Unit  string
	Shift int
}

/* -------------------------------------------------------------------------- */
/*  Functions                                                                 */
/* -------------------------------------------------------------------------- */

// FuncIR represents a function in IR.
type FuncIR struct {
	Name       string
	CName      string
	Params     []ParamIR
	ReturnType Type
	Body       []Instr // Rho 0.1: may be empty
	Extern     bool
}

// ParamIR represents a function parameter in IR.
type ParamIR struct {
	Name string
	Type Type
}

/* -------------------------------------------------------------------------- */
/*  Instructions                                                              */
/* -------------------------------------------------------------------------- */

// OpCode represents the operation code for an IR instruction.
type OpCode int

const (
	OpNop OpCode = iota
	OpReturn
	OpCall
	OpAssign
	OpLoad
	OpStore
	OpRaw
	OpLet
	OpExpr
	OpIf
	OpWhile
	OpBreak
	OpContinue
	// OpArenaMark declares Dest as a mark of the string arena, taken here.
	// OpArenaRelease gives back every string built since the mark Dest names.
	//
	// They are what the C backend frees strings with: a mark at the top of a
	// loop and a release at the end of each iteration, and the same around a
	// statement whose temporaries nothing keeps. Only the C backend acts on them;
	// a backend with a collector of its own, as Go has, leaves them out.
	OpArenaMark
	OpArenaRelease
)

// Instr represents a single IR instruction.
type Instr struct {
	Op        OpCode
	Dest      string   // destination symbol
	Type      string   // declared type for OpLet, in the C spelling
	TypeName  string   // the Rho type as written, when the C spelling is lossy
	Args      []string // argument symbols
	Condition string
	Then      []Instr
	Else      []Instr
	// Arena names the arena mark an instruction is released against, for the
	// backend that has an arena. On OpLet and OpAssign the stored string is kept
	// and everything else built since the mark is released; on OpIf the
	// condition's temporaries are released once it is evaluated; on OpWhile the
	// mark is taken afresh before each test of the condition and released after
	// it. Empty means none, and a backend without an arena ignores it.
	Arena string
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// NewModuleIR creates a new empty IR module.
func NewModuleIR(name string) ModuleIR {
	return ModuleIR{
		Name:    name,
		Funcs:   []FuncIR{},
		Structs: []StructIR{},
	}
}

/* -------------------------------------------------------------------------- */
/*  HTTP Routes                                                               */
/* -------------------------------------------------------------------------- */

// RouteIR is a compiled HTTP route: an HTTP method and ServeMux pattern bound
// to the Rho function that serves it.
type RouteIR struct {
	// Method is the HTTP method, upper case.
	Method string
	// Path is the ServeMux pattern, for example "/modules/{name}".
	Path string
	// Handler is the name of the serving Rho function.
	Handler string
}

// WithoutArena is a body with the string arena's instructions taken out, for a
// backend that has no arena: what is left is the body as it was lowered before
// the arena existed, so that backend's output does not change.
func WithoutArena(body []Instr) []Instr {
	out := make([]Instr, 0, len(body))
	for _, instr := range body {
		if instr.Op == OpArenaMark || instr.Op == OpArenaRelease {
			continue
		}
		instr.Arena = ""
		if len(instr.Then) > 0 {
			instr.Then = WithoutArena(instr.Then)
		}
		if len(instr.Else) > 0 {
			instr.Else = WithoutArena(instr.Else)
		}
		out = append(out, instr)
	}
	return out
}
