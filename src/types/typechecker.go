//
// @file typechecker.go
// @brief Rho Compiler - Type Checker.
//
// This file implements the deterministic type checker for the Rho compiler.
// Rho 0.1 has a minimal type system: no inference, no generics, no polymorphism.
// The type checker validates struct declarations, function signatures,
// parameter types, return types, and ensures that all referenced types exist.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package types

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"fmt"
	"rho/src/frontend/ast"
	"rho/src/ir"
	"strconv"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  TypeChecker                                                               */
/* -------------------------------------------------------------------------- */

// TypeChecker performs deterministic type validation on AST modules.
type TypeChecker struct {
	types map[string]ir.Type // known types
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new type checker with built-in primitive types.
func New() *TypeChecker {
	tc := &TypeChecker{
		types: map[string]ir.Type{},
	}

	// Built-in types
	tc.types["void"] = ir.Type{Kind: ir.TypeVoid, Name: "void"}
	tc.types["bool"] = ir.Type{Kind: ir.TypeBool, Name: "bool"}
	tc.types["u8"] = ir.Type{Kind: ir.TypeU8, Name: "u8"}
	tc.types["u16"] = ir.Type{Kind: ir.TypeU16, Name: "u16"}
	tc.types["u32"] = ir.Type{Kind: ir.TypeU32, Name: "u32"}
	tc.types["u64"] = ir.Type{Kind: ir.TypeU64, Name: "u64"}
	tc.types["i32"] = ir.Type{Kind: ir.TypeI32, Name: "i32"}
	tc.types["f64"] = ir.Type{Kind: ir.TypeF64, Name: "f64"}
	tc.types["string"] = ir.Type{Kind: ir.TypeString, Name: "string"}
	for _, name := range []string{"String", "Slice", "Ptr", "Option", "Result", "Array", "Map", "Range", "RS485", "UART", "I2C", "SPI", "GPIO", "Timer", "Modbus", "TimeSpec"} {
		tc.types[name] = ir.Type{Kind: ir.TypeStruct, Name: name}
	}

	return tc
}

// CheckModule validates all declarations inside a module.
func (tc *TypeChecker) CheckModule(m ast.Module) error {
	// First pass: register struct types
	for _, decl := range m.Decls {
		if s, ok := decl.(ast.StructDecl); ok {
			tc.types[s.Name] = ir.Type{Kind: ir.TypeStruct, Name: s.Name}
		}
	}

	// Second pass: validate declarations
	for _, decl := range m.Decls {
		switch d := decl.(type) {
		case ast.StructDecl:
			if err := tc.checkStruct(d); err != nil {
				return err
			}
		case ast.FuncDecl:
			if err := tc.checkFunc(d); err != nil {
				return err
			}
		case ast.TypeDecl:
			tc.types[d.Name] = ir.Type{Kind: ir.TypeStruct, Name: d.Type.String()}
		default:
			return fmt.Errorf("unknown top-level declaration")
		}
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Struct Validation                                                          */
/* -------------------------------------------------------------------------- */

func (tc *TypeChecker) checkStruct(s ast.StructDecl) error {
	for _, f := range s.Fields {
		if _, ok := tc.resolveType(f.Type); !ok {
			return fmt.Errorf("unknown type %q in struct %q", f.Type, s.Name)
		}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Function Validation                                                        */
/* -------------------------------------------------------------------------- */

func (tc *TypeChecker) checkFunc(fn ast.FuncDecl) error {
	// Check parameters
	for _, p := range fn.Params {
		if _, ok := tc.resolveType(p.Type); !ok {
			return fmt.Errorf("unknown type %q in parameter %q of function %q",
				p.Type, p.Name, fn.Name)
		}
	}

	// Check return type
	if _, ok := tc.resolveType(fn.ReturnType); !ok {
		return fmt.Errorf("unknown return type %q in function %q", fn.ReturnType, fn.Name)
	}

	// Rho 0.1: body is ignored (no statements)
	return nil
}

/* -------------------------------------------------------------------------- */
/*  IR Conversion (Optional for Rho 0.1)                                      */
/* -------------------------------------------------------------------------- */

// ToIR converts a validated AST module into IR.
func (tc *TypeChecker) ToIR(m ast.Module) ir.ModuleIR {
	out := ir.NewModuleIR(m.Name)

	// Structs
	for _, decl := range m.Decls {
		if s, ok := decl.(ast.StructDecl); ok {
			out.Structs = append(out.Structs, tc.structToIR(s))
		}
	}

	// Functions
	for _, decl := range m.Decls {
		if fn, ok := decl.(ast.FuncDecl); ok {
			out.Funcs = append(out.Funcs, tc.funcToIR(fn))
		}
	}

	return out
}

/* -------------------------------------------------------------------------- */
/*  IR Conversion Helpers                                                     */
/* -------------------------------------------------------------------------- */

func (tc *TypeChecker) structToIR(s ast.StructDecl) ir.StructIR {
	fields := make([]ir.FieldIR, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = ir.FieldIR{
			Name: f.Name,
			Type: tc.typeFor(f.Type),
		}
	}
	return ir.StructIR{
		Name:   s.Name,
		Fields: fields,
	}
}

func (tc *TypeChecker) funcToIR(fn ast.FuncDecl) ir.FuncIR {
	params := make([]ir.ParamIR, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = ir.ParamIR{
			Name: p.Name,
			Type: tc.typeFor(p.Type),
		}
	}

	return ir.FuncIR{
		Name:       fn.Name,
		Params:     params,
		ReturnType: tc.typeFor(fn.ReturnType),
		Body:       lowerStatements(fn.Body),
		Extern:     fn.Extern,
	}
}

func (tc *TypeChecker) resolveType(name string) (ir.Type, bool) {
	if typ, ok := tc.types[name]; ok {
		return typ, true
	}
	base := name
	if i := strings.IndexByte(base, '['); i >= 0 {
		base = base[:i]
	}
	if typ, ok := tc.types[base]; ok {
		typ.Name = name
		return typ, true
	}
	if (strings.Contains(name, "[") || (len(base) == 1 && base != "")) && unicodeUpper(base[0]) {
		return ir.Type{Kind: ir.TypeStruct, Name: name}, true
	}
	return ir.Type{}, false
}

func unicodeUpper(ch byte) bool { return ch >= 'A' && ch <= 'Z' }

func (tc *TypeChecker) typeFor(name string) ir.Type { typ, _ := tc.resolveType(name); return typ }

func lowerStatements(statements []ast.Stmt) []ir.Instr {
	out := []ir.Instr{}
	for _, stmt := range statements {
		out = append(out, lowerStatement(stmt)...)
	}
	return out
}

func lowerStatement(statement ast.Stmt) []ir.Instr {
	switch stmt := statement.(type) {
	case ast.LetStmt:
		line := cTypeForName(stmt.Type.String()) + " " + stmt.Name
		if stmt.Value != nil {
			line += " = " + lowerExpr(stmt.Value)
		}
		return []ir.Instr{{Op: ir.OpRaw, Dest: "    " + line + ";"}}
	case ast.AssignStmt:
		return []ir.Instr{{Op: ir.OpRaw, Dest: "    " + lowerExpr(stmt.Target) + " " + stmt.Op + " " + lowerExpr(stmt.Value) + ";"}}
	case ast.ExprStmt:
		return []ir.Instr{{Op: ir.OpRaw, Dest: "    " + lowerExpr(stmt.Expr) + ";"}}
	case ast.ReturnStmt:
		line := "    return"
		if stmt.Value != nil {
			line += " " + lowerExpr(stmt.Value)
		}
		return []ir.Instr{{Op: ir.OpRaw, Dest: line + ";"}}
	case ast.IfStmt:
		line := "    if (" + lowerExpr(stmt.Condition) + ") {\n" + lowerStatementsText(stmt.Then) + "    }"
		if len(stmt.Else) > 0 {
			line += " else {\n" + lowerStatementsText(stmt.Else) + "    }"
		}
		return []ir.Instr{{Op: ir.OpRaw, Dest: line}}
	case ast.WhileStmt:
		return []ir.Instr{{Op: ir.OpRaw, Dest: "    while (" + lowerExpr(stmt.Condition) + ") {\n" + lowerStatementsText(stmt.Body) + "    }"}}
	case ast.BreakStmt:
		return []ir.Instr{{Op: ir.OpRaw, Dest: "    break;"}}
	default:
		return nil
	}
}

func lowerStatementsText(statements []ast.Stmt) string {
	text := ""
	for _, stmt := range statements {
		for _, instr := range lowerStatement(stmt) {
			text += instr.Dest + "\n"
		}
	}
	return text
}

func lowerExpr(expr ast.Expr) string {
	switch expr := expr.(type) {
	case ast.IdentifierExpr:
		return expr.Name
	case ast.NumberExpr:
		return expr.Value
	case ast.StringExpr:
		return strconv.Quote(expr.Value)
	case ast.BoolExpr:
		if expr.Value {
			return "true"
		}
		return "false"
	case ast.UnaryExpr:
		return expr.Op + lowerExpr(expr.Value)
	case ast.BinaryExpr:
		if expr.Op == "+" {
			return "rho_concat(" + lowerExpr(expr.Left) + ", " + lowerExpr(expr.Right) + ")"
		}
		return "(" + lowerExpr(expr.Left) + " " + expr.Op + " " + lowerExpr(expr.Right) + ")"
	case ast.MemberExpr:
		if expr.Object.(ast.IdentifierExpr).Name == "yaml" && expr.Name == "load" {
			return "rho_yaml_load"
		}
		return lowerExpr(expr.Object) + "." + expr.Name
	case ast.IndexExpr:
		return lowerExpr(expr.Object) + "[" + lowerExpr(expr.Index) + "]"
	case ast.CallExpr:
		if callee, ok := expr.Callee.(ast.IdentifierExpr); ok && callee.Name == "print" {
			if len(expr.Args) == 1 {
				return "printf(\"%s\\n\", " + lowerExpr(expr.Args[0]) + ")"
			}
			return "printf(\"%s\\n\", \"\")"
		}
		callee := lowerExpr(expr.Callee)
		args := []string{}
		for _, arg := range expr.Args {
			args = append(args, lowerExpr(arg))
		}
		return callee + "(" + strings.Join(args, ", ") + ")"
	case ast.StructLiteralExpr:
		values := []string{}
		for _, value := range expr.Values {
			values = append(values, lowerExpr(value))
		}
		return "(" + cTypeForName(expr.Type.String()) + "){ " + strings.Join(values, ", ") + " }"
	case ast.IfExpr:
		return "(" + lowerExpr(expr.Condition) + " ? " + lowerExpr(expr.Then) + " : " + lowerExpr(expr.Else) + ")"
	default:
		return "0"
	}
}

func cTypeForName(name string) string {
	base := name
	if i := strings.IndexByte(base, '['); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "u8":
		return "uint8_t"
	case "u16":
		return "uint16_t"
	case "u32":
		return "uint32_t"
	case "u64":
		return "uint64_t"
	case "i32":
		return "int32_t"
	case "f64":
		return "double"
	case "bool":
		return "bool"
	case "string":
		return "const char*"
	case "void":
		return "void"
	}
	return strings.NewReplacer("[", "_", "]", "", ",", "_").Replace(name)
}
