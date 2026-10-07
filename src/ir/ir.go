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
	TypeI32
	// TypeI64 is a signed 64-bit integer. A database column is one, and so is
	// anything read from a file or a clock: i32 is a width, and a value that
	// came from outside is not one the program chose.
	TypeI64
	TypeF64
	TypeString
	TypeStruct
	// TypePtr is a pointer to another type. It is a type of its own rather than
	// a name, because a pointer is how a value crosses a foreign boundary: C
	// takes `const char*` where Rho takes `Ptr[u8]`, and a backend that spelled a
	// pointer as a name would generate something that does not compile.
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
}

// AliasIR is one type alias a module declares.
type AliasIR struct {
	// Name is the alias as the program wrote it.
	Name string
	// Target is the type it names, already resolved to the spelling the backend
	// renders — a C type name for the C backend, a Go one for the Go backend.
	Target string
}

/* -------------------------------------------------------------------------- */
/*  Structs                                                                   */
/* -------------------------------------------------------------------------- */

// StructIR represents a struct in IR.
type StructIR struct {
	Name   string
	CName  string
	Fields []FieldIR
}

// FieldIR represents a single field in a struct.
type FieldIR struct {
	Name string
	Type Type
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
