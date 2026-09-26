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
// SPDX-License-Identifier: MIT
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
	TypeF64
	TypeString
	TypeStruct
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
	Name     string
	Features []string
	Funcs    []FuncIR
	Structs  []StructIR
	// Routes are the HTTP routes the module exposes, meaningful for a server
	// target.
	Routes []RouteIR
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
	Type      string   // declared type for OpLet
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
