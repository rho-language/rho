//
// @file types.go
// @brief Rho Type System - Enterprise v1.
//
// Defines minimal type system used by:
//   - semantic analysis
//   - hover information
//   - future type checker (Enterprise v2/v3)
//
// Supported types (v1):
//   - primitive types (int, float, bool, void)
//   - function types
//   - named types
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

/* -------------------------------------------------------------------------- */
/*  Type Interface                                                             */
/* -------------------------------------------------------------------------- */

type Type interface {
	Name() string
}

/* -------------------------------------------------------------------------- */
/*  Primitive Types                                                             */
/* -------------------------------------------------------------------------- */

type PrimitiveType struct {
	name string
}

func NewPrimitiveType(name string) *PrimitiveType {
	return &PrimitiveType{name}
}

func (t *PrimitiveType) Name() string { return t.name }

/* -------------------------------------------------------------------------- */
/*  Named Types (structs, modules, aliases)                                    */
/* -------------------------------------------------------------------------- */

type NamedType struct {
	name string
}

func NewNamedType(name string) *NamedType {
	return &NamedType{name}
}

func (t *NamedType) Name() string { return t.name }

/* -------------------------------------------------------------------------- */
/*  Function Types                                                              */
/* -------------------------------------------------------------------------- */

type FunctionType struct {
	Params []Type
	Return Type
}

func NewFunctionType(params []Type, ret Type) *FunctionType {
	return &FunctionType{params, ret}
}

func (t *FunctionType) Name() string {
	out := "fn("
	for i, p := range t.Params {
		out += p.Name()
		if i < len(t.Params)-1 {
			out += ", "
		}
	}
	out += ") -> " + t.Return.Name()
	return out
}

/* -------------------------------------------------------------------------- */
/*  Built-in Types                                                              */
/* -------------------------------------------------------------------------- */

var (
	TypeInt   = NewPrimitiveType("int")
	TypeFloat = NewPrimitiveType("float")
	TypeBool  = NewPrimitiveType("bool")
	TypeVoid  = NewPrimitiveType("void")
)
