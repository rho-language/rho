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
	types         map[string]ir.Type // known types
	funcs         map[string]functionType
	fields        map[string]map[string]ir.Type
	genericParams map[string][]string
}

type functionType struct {
	params []ir.Type
	result ir.Type
	cName  string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new type checker with built-in primitive types.
func New() *TypeChecker {
	tc := &TypeChecker{
		types:         map[string]ir.Type{},
		funcs:         map[string]functionType{},
		fields:        map[string]map[string]ir.Type{},
		genericParams: map[string][]string{},
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
	standaloneStdlib := strings.HasPrefix(m.Name, "std.")
	// First pass: register struct types
	for _, decl := range m.Decls {
		if s, ok := decl.(ast.StructDecl); ok {
			tc.types[s.Name] = ir.Type{Kind: ir.TypeStruct, Name: s.Name}
			fields := make(map[string]ir.Type, len(s.Fields))
			for _, field := range s.Fields {
				fields[field.Name] = tc.typeFor(field.Type)
			}
			tc.fields[s.Name] = fields
			tc.genericParams[s.Name] = append([]string{}, s.TypeParams...)
		}
	}
	for _, decl := range m.Decls {
		if fn, ok := decl.(ast.FuncDecl); ok {
			params := make([]ir.Type, len(fn.Params))
			for i, param := range fn.Params {
				params[i] = tc.typeFor(param.Type)
			}
			entry := functionType{params: params, result: tc.typeFor(fn.ReturnType), cName: qualifiedCName(fn.Owner, fn.Name)}
			if fn.Owner != "" {
				tc.funcs[fn.Owner+"."+fn.Name] = entry
			}
			if _, exists := tc.funcs[fn.Name]; !exists {
				tc.funcs[fn.Name] = entry
			}
		}
	}
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
			if standaloneStdlib || strings.HasPrefix(d.Owner, "std.") {
				continue
			}
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

	if fn.Extern {
		if len(fn.Body) != 0 {
			return fmt.Errorf("extern function %q cannot have a body", fn.Name)
		}
		return nil
	}
	scope := make(map[string]ir.Type, len(fn.Params))
	for _, param := range fn.Params {
		scope[param.Name] = tc.typeFor(param.Type)
	}
	terminates, err := tc.checkStatements(fn.Body, scope, tc.typeFor(fn.ReturnType), false)
	if err != nil {
		return fmt.Errorf("function %s: %w", fn.Name, err)
	}
	if tc.typeFor(fn.ReturnType).Kind != ir.TypeVoid && len(fn.Body) > 0 && !terminates {
		return fmt.Errorf("function %s: missing return", fn.Name)
	}
	return nil
}

func (tc *TypeChecker) checkStatements(statements []ast.Stmt, scope map[string]ir.Type, result ir.Type, inLoop bool) (bool, error) {
	terminates := false
	for _, statement := range statements {
		if terminates {
			return true, fmt.Errorf("unreachable statement")
		}
		var statementTerminates bool
		switch stmt := statement.(type) {
		case ast.LetStmt:
			if _, exists := scope[stmt.Name]; exists {
				return false, fmt.Errorf("duplicate local %q", stmt.Name)
			}
			declared := tc.typeFor(stmt.Type.String())
			if stmt.Value != nil {
				actual, err := tc.exprType(stmt.Value, scope)
				if err != nil {
					return false, err
				}
				if !assignable(declared, actual) {
					return false, fmt.Errorf("cannot assign %s to %s", typeName(actual), typeName(declared))
				}
			}
			scope[stmt.Name] = declared
		case ast.AssignStmt:
			actual, err := tc.exprType(stmt.Target, scope)
			if err != nil {
				return false, err
			}
			value, err := tc.exprType(stmt.Value, scope)
			if err != nil {
				return false, err
			}
			if !assignable(actual, value) {
				return false, fmt.Errorf("cannot assign %s to %s", typeName(value), typeName(actual))
			}
		case ast.ExprStmt:
			if _, err := tc.exprType(stmt.Expr, scope); err != nil {
				return false, err
			}
		case ast.ReturnStmt:
			if stmt.Value == nil {
				if result.Kind != ir.TypeVoid {
					return false, fmt.Errorf("missing return value")
				}
			} else {
				actual, err := tc.exprType(stmt.Value, scope)
				if err != nil {
					return false, err
				}
				if !assignable(result, actual) {
					return false, fmt.Errorf("returning %s from %s function", typeName(actual), typeName(result))
				}
			}
			statementTerminates = true
		case ast.IfStmt:
			condition, err := tc.exprType(stmt.Condition, scope)
			if err != nil {
				return false, err
			}
			if condition.Kind != ir.TypeBool {
				return false, fmt.Errorf("if condition must be bool")
			}
			thenScope := cloneScope(scope)
			thenEnds, err := tc.checkStatements(stmt.Then, thenScope, result, inLoop)
			if err != nil {
				return false, err
			}
			elseEnds := false
			if len(stmt.Else) > 0 {
				elseScope := cloneScope(scope)
				elseEnds, err = tc.checkStatements(stmt.Else, elseScope, result, inLoop)
				if err != nil {
					return false, err
				}
			}
			statementTerminates = thenEnds && elseEnds
		case ast.WhileStmt:
			condition, err := tc.exprType(stmt.Condition, scope)
			if err != nil {
				return false, err
			}
			if condition.Kind != ir.TypeBool {
				return false, fmt.Errorf("while condition must be bool")
			}
			if _, err := tc.checkStatements(stmt.Body, cloneScope(scope), result, true); err != nil {
				return false, err
			}
		case ast.BreakStmt:
			if !inLoop {
				return false, fmt.Errorf("break outside loop")
			}
			statementTerminates = true
		default:
			return false, fmt.Errorf("unsupported statement %T", statement)
		}
		terminates = statementTerminates
	}
	return terminates, nil
}

func cloneScope(scope map[string]ir.Type) map[string]ir.Type {
	copy := make(map[string]ir.Type, len(scope))
	for name, typ := range scope {
		copy[name] = typ
	}
	return copy
}

func (tc *TypeChecker) exprType(expr ast.Expr, scope map[string]ir.Type) (ir.Type, error) {
	switch value := expr.(type) {
	case ast.IdentifierExpr:
		if typ, ok := scope[value.Name]; ok {
			return typ, nil
		}
		if _, ok := tc.funcs[value.Name]; ok {
			return ir.Type{Kind: ir.TypeStruct, Name: "function"}, nil
		}
		return ir.Type{}, fmt.Errorf("undefined name %q", value.Name)
	case ast.NumberExpr:
		if strings.Contains(value.Value, ".") {
			return ir.Type{Kind: ir.TypeF64, Name: "f64"}, nil
		}
		return ir.Type{Kind: ir.TypeI32, Name: "i32"}, nil
	case ast.StringExpr:
		return ir.Type{Kind: ir.TypeString, Name: "string"}, nil
	case ast.BoolExpr:
		return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
	case ast.UnaryExpr:
		inner, err := tc.exprType(value.Value, scope)
		if err != nil {
			return ir.Type{}, err
		}
		if value.Op == "!" {
			if inner.Kind != ir.TypeBool {
				return ir.Type{}, fmt.Errorf("! requires bool")
			}
			return inner, nil
		}
		if value.Op == "-" && !numeric(inner) {
			return ir.Type{}, fmt.Errorf("unary - requires numeric value")
		}
		return inner, nil
	case ast.BinaryExpr:
		left, err := tc.exprType(value.Left, scope)
		if err != nil {
			return ir.Type{}, err
		}
		right, err := tc.exprType(value.Right, scope)
		if err != nil {
			return ir.Type{}, err
		}
		if value.Op == "==" || value.Op == "!=" || value.Op == "<" || value.Op == "<=" || value.Op == ">" || value.Op == ">=" {
			if !comparable(left, right) {
				return ir.Type{}, fmt.Errorf("incompatible comparison")
			}
			return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
		}
		if value.Op == "&&" || value.Op == "||" {
			if left.Kind != ir.TypeBool || right.Kind != ir.TypeBool {
				return ir.Type{}, fmt.Errorf("logical operator requires bool")
			}
			return left, nil
		}
		if value.Op == "&" || value.Op == "|" || value.Op == "^" || value.Op == "<<" || value.Op == ">>" {
			if (numeric(left) || left.Kind == ir.TypeInvalid) && (numeric(right) || right.Kind == ir.TypeInvalid) {
				return left, nil
			}
			return ir.Type{}, fmt.Errorf("bitwise operator %s requires integer values", value.Op)
		}
		if value.Op == "+" && (left.Kind == ir.TypeString || right.Kind == ir.TypeString) {
			if left.Kind != ir.TypeString || right.Kind != ir.TypeString {
				return ir.Type{}, fmt.Errorf("string concatenation requires strings")
			}
			return left, nil
		}
		if !numeric(left) || !numeric(right) {
			return ir.Type{}, fmt.Errorf("operator %s requires numeric values", value.Op)
		}
		if left.Kind == ir.TypeF64 || right.Kind == ir.TypeF64 {
			return ir.Type{Kind: ir.TypeF64, Name: "f64"}, nil
		}
		return left, nil
	case ast.MemberExpr:
		if object, ok := value.Object.(ast.IdentifierExpr); ok && ((object.Name == "yaml" && value.Name == "load") || (object.Name == "string" && value.Name == "from_i32")) {
			return ir.Type{Kind: ir.TypeStruct, Name: "function"}, nil
		}
		object, err := tc.exprType(value.Object, scope)
		if err != nil {
			return ir.Type{}, err
		}
		return tc.memberType(object, value.Name)
	case ast.IndexExpr:
		object, err := tc.exprType(value.Object, scope)
		if err != nil {
			return ir.Type{}, err
		}
		if _, err := tc.exprType(value.Index, scope); err != nil {
			return ir.Type{}, err
		}
		return elementType(object), nil
	case ast.CastExpr:
		if _, err := tc.exprType(value.Value, scope); err != nil {
			return ir.Type{}, err
		}
		converted, ok := tc.resolveType(value.Type.String())
		if !ok {
			return ir.Type{}, fmt.Errorf("unknown cast type %q", value.Type.String())
		}
		return converted, nil
	case ast.CallExpr:
		var signature functionType
		if identifier, ok := value.Callee.(ast.IdentifierExpr); ok {
			var exists bool
			signature, exists = tc.funcs[identifier.Name]
			if !exists && identifier.Name != "print" && !isCallableBuiltin(identifier.Name, tc.types) && !isExternalBuiltin(identifier.Name) {
				return ir.Type{}, fmt.Errorf("undefined function %q", identifier.Name)
			}
			if identifier.Name == "print" {
				if len(value.Args) != 1 {
					return ir.Type{}, fmt.Errorf("print expects one argument")
				}
				if _, err := tc.exprType(value.Args[0], scope); err != nil {
					return ir.Type{}, err
				}
				return ir.Type{Kind: ir.TypeVoid, Name: "void"}, nil
			}
			if identifier.Name == "str_eq" {
				signature = functionType{params: []ir.Type{{Kind: ir.TypeString, Name: "string"}, {Kind: ir.TypeString, Name: "string"}}, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}}
			}
			if !exists {
				if identifier.Name == "clamp_f64" || identifier.Name == "lerp" || identifier.Name == "sqrt" || identifier.Name == "sin" || identifier.Name == "cos" || identifier.Name == "atan2" || identifier.Name == "floor" {
					return ir.Type{Kind: ir.TypeF64, Name: "f64"}, nil
				}
				if identifier.Name == "str_eq" {
					return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
				}
				if strings.HasPrefix(identifier.Name, "c_") || identifier.Name == "sizeof" {
					return ir.Type{Kind: ir.TypeI32, Name: "i32"}, nil
				}
				return ir.Type{}, nil
			}
		} else if member, ok := value.Callee.(ast.MemberExpr); ok && isBuiltinMemberCall(member) {
			if member.Name == "load" {
				signature = functionType{params: []ir.Type{{Kind: ir.TypeString, Name: "string"}}, result: ir.Type{Kind: ir.TypeString, Name: "string"}}
			} else {
				signature = functionType{params: []ir.Type{{Kind: ir.TypeI32, Name: "i32"}}, result: ir.Type{Kind: ir.TypeString, Name: "string"}}
			}
		} else {
			return ir.Type{}, fmt.Errorf("unsupported call target")
		}
		if len(value.Args) != len(signature.params) {
			return ir.Type{}, fmt.Errorf("wrong argument count")
		}
		for i, arg := range value.Args {
			actual, err := tc.exprType(arg, scope)
			if err != nil {
				return ir.Type{}, err
			}
			if !assignable(signature.params[i], actual) {
				return ir.Type{}, fmt.Errorf("argument %d has type %s, want %s", i+1, typeName(actual), typeName(signature.params[i]))
			}
		}
		return signature.result, nil
	case ast.StructLiteralExpr:
		typ, ok := tc.resolveType(value.Type.String())
		if !ok {
			return ir.Type{}, fmt.Errorf("unknown struct %q", value.Type.String())
		}
		return typ, nil
	case ast.ArrayLiteralExpr:
		if len(value.Values) == 0 {
			return ir.Type{Kind: ir.TypeStruct, Name: "Array"}, nil
		}
		element, err := tc.exprType(value.Values[0], scope)
		if err != nil {
			return ir.Type{}, err
		}
		for _, item := range value.Values[1:] {
			itemType, itemErr := tc.exprType(item, scope)
			if itemErr != nil {
				return ir.Type{}, itemErr
			}
			if !assignable(element, itemType) {
				return ir.Type{}, fmt.Errorf("array literal elements have different types")
			}
		}
		return ir.Type{Kind: ir.TypeStruct, Name: "Array[" + typeName(element) + "]"}, nil
	case ast.IfExpr:
		condition, err := tc.exprType(value.Condition, scope)
		if err != nil {
			return ir.Type{}, err
		}
		if condition.Kind != ir.TypeBool {
			return ir.Type{}, fmt.Errorf("if expression condition must be bool")
		}
		thenType, err := tc.exprType(value.Then, scope)
		if err != nil {
			return ir.Type{}, err
		}
		elseType, err := tc.exprType(value.Else, scope)
		if err != nil {
			return ir.Type{}, err
		}
		if !assignable(thenType, elseType) {
			return ir.Type{}, fmt.Errorf("if expression branches have different types")
		}
		return thenType, nil
	default:
		return ir.Type{}, fmt.Errorf("unsupported expression %T", expr)
	}
}

func (tc *TypeChecker) memberType(object ir.Type, name string) (ir.Type, error) {
	if object.Kind != ir.TypeStruct {
		return ir.Type{}, fmt.Errorf("member access requires a struct")
	}
	fields, ok := tc.fields[baseTypeName(object.Name)]
	if !ok {
		if _, builtin := tc.types[baseTypeName(object.Name)]; builtin {
			return ir.Type{Kind: ir.TypeI32, Name: "i32"}, nil
		}
		return ir.Type{}, fmt.Errorf("unknown member %q", name)
	}
	field, ok := fields[name]
	if !ok {
		return ir.Type{}, fmt.Errorf("unknown member %q", name)
	}
	params := tc.genericParams[baseTypeName(object.Name)]
	args := typeArguments(object.Name)
	for index, param := range params {
		if index < len(args) && field.Name == param {
			return args[index], nil
		}
	}
	return substituteGeneric(field, params, args), nil
}

func baseTypeName(name string) string {
	if index := strings.IndexByte(name, '['); index >= 0 {
		return name[:index]
	}
	return name
}
func elementType(value ir.Type) ir.Type {
	if index := strings.IndexByte(value.Name, '['); index >= 0 {
		return ir.Type{Kind: ir.TypeStruct, Name: value.Name[:index]}
	}
	return value
}

func typeArguments(name string) []ir.Type {
	start := strings.IndexByte(name, '[')
	if start < 0 || !strings.HasSuffix(name, "]") {
		return nil
	}
	parts := strings.Split(name[start+1:len(name)-1], ",")
	args := make([]ir.Type, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		args = append(args, ir.Type{Kind: primitiveKind(part), Name: part})
	}
	return args
}

func primitiveKind(name string) ir.TypeKind {
	switch name {
	case "bool":
		return ir.TypeBool
	case "u8":
		return ir.TypeU8
	case "u16":
		return ir.TypeU16
	case "u32":
		return ir.TypeU32
	case "u64":
		return ir.TypeU64
	case "i32":
		return ir.TypeI32
	case "f64":
		return ir.TypeF64
	case "string":
		return ir.TypeString
	default:
		return ir.TypeStruct
	}
}

func substituteGeneric(value ir.Type, params []string, args []ir.Type) ir.Type {
	for index, param := range params {
		if index < len(args) && value.Name == param {
			return args[index]
		}
	}
	return value
}
func isCallableBuiltin(name string, types map[string]ir.Type) bool {
	_, ok := types[name]
	return ok || name == "Some" || name == "None" || name == "Ok" || name == "Err"
}

func isExternalBuiltin(name string) bool {
	if strings.HasPrefix(name, "c_") {
		return true
	}
	for _, builtin := range []string{"sqrt", "sin", "cos", "atan2", "floor", "clamp_f64", "lerp", "str_eq", "panic", "sizeof", "robohub_platform_init", "robohub_board_register"} {
		if name == builtin {
			return true
		}
	}
	return false
}
func isBuiltinMemberCall(member ast.MemberExpr) bool {
	object, ok := member.Object.(ast.IdentifierExpr)
	return ok && ((object.Name == "yaml" && member.Name == "load") || (object.Name == "string" && member.Name == "from_i32"))
}

func numeric(value ir.Type) bool {
	return value.Kind == ir.TypeI32 || value.Kind == ir.TypeU8 || value.Kind == ir.TypeU16 || value.Kind == ir.TypeU32 || value.Kind == ir.TypeU64 || value.Kind == ir.TypeF64
}
func comparable(left, right ir.Type) bool { return assignable(left, right) || assignable(right, left) }

func assignable(want, got ir.Type) bool {
	if want.Kind == ir.TypeInvalid || got.Kind == ir.TypeInvalid {
		return true
	}
	return want.Kind == got.Kind || (numeric(want) && numeric(got))
}
func typeName(value ir.Type) string {
	if value.Name != "" {
		return value.Name
	}
	return "invalid"
}

/* -------------------------------------------------------------------------- */
/*  IR Conversion (Optional for Rho 0.1)                                      */
/* -------------------------------------------------------------------------- */

// ToIR converts a validated AST module into IR.
func (tc *TypeChecker) ToIR(m ast.Module) ir.ModuleIR {
	out := ir.NewModuleIR(m.Name)
	out.Features = append(out.Features, "math")
	out.Features = append(out.Features, m.Features...)

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
		fieldType := tc.typeFromExpr(f.TypeExpr, s.TypeParams)
		if f.TypeExpr.Name == "" {
			fieldType = tc.typeFor(f.Type)
		}
		fields[i] = ir.FieldIR{
			Name: f.Name,
			Type: fieldType,
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
		paramType := tc.typeFromExpr(p.TypeExpr, fn.TypeParams)
		if p.TypeExpr.Name == "" {
			paramType = tc.typeFor(p.Type)
		}
		params[i] = ir.ParamIR{
			Name: p.Name,
			Type: paramType,
		}
	}

	// Lowering needs the local scope so operator lowering can pick the right
	// form for the operand types. Locals are added as the body is lowered.
	scope := make(map[string]ir.Type, len(params))
	for _, param := range params {
		scope[param.Name] = param.Type
	}

	return ir.FuncIR{
		Name:       fn.Name,
		CName:      qualifiedCName(fn.Owner, fn.Name),
		Params:     params,
		ReturnType: tc.typeFor(fn.ReturnType),
		Body:       tc.lowerStatements(fn.Body, scope),
		Extern:     fn.Extern,
	}
}

func qualifiedCName(owner, name string) string {
	if owner == "" {
		return name
	}
	return strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(owner) + "_" + name
}

func (tc *TypeChecker) typeFromExpr(expr ast.TypeExpr, params []string) ir.Type {
	if expr.Name != "" {
		for _, param := range params {
			if expr.Name == param {
				return ir.Type{Kind: ir.TypeStruct, Name: expr.Name}
			}
		}
	}
	typ := tc.typeFor(expr.Name)
	typ.Name = expr.String()
	typ.ArrayLen = 0
	if expr.ArrayLen != nil {
		typ.ArrayLen = *expr.ArrayLen
	}
	for _, argument := range expr.Args {
		typ.Args = append(typ.Args, tc.typeFromExpr(argument, params))
	}
	return typ
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

func (tc *TypeChecker) lowerStatements(statements []ast.Stmt, scope map[string]ir.Type) []ir.Instr {
	out := []ir.Instr{}
	for _, stmt := range statements {
		out = append(out, tc.lowerStatement(stmt, scope)...)
	}
	return out
}

func (tc *TypeChecker) lowerStatement(statement ast.Stmt, scope map[string]ir.Type) []ir.Instr {
	switch stmt := statement.(type) {
	case ast.LetStmt:
		value := ""
		if stmt.Value != nil {
			value = tc.lowerExpr(stmt.Value, scope)
		}
		// Record the binding so later expressions in the same body can see
		// its type when deciding how to lower operators.
		scope[stmt.Name] = tc.typeFor(stmt.Type.String())
		return []ir.Instr{{Op: ir.OpLet, Dest: stmt.Name, Type: cTypeForName(stmt.Type.String()), Args: []string{value}}}
	case ast.AssignStmt:
		return []ir.Instr{{Op: ir.OpAssign, Dest: tc.lowerExpr(stmt.Target, scope) + " " + stmt.Op, Args: []string{tc.lowerExpr(stmt.Value, scope)}}}
	case ast.ExprStmt:
		return []ir.Instr{{Op: ir.OpExpr, Args: []string{tc.lowerExpr(stmt.Expr, scope)}}}
	case ast.ReturnStmt:
		value := ""
		if stmt.Value != nil {
			value = tc.lowerExpr(stmt.Value, scope)
		}
		return []ir.Instr{{Op: ir.OpReturn, Args: []string{value}}}
	case ast.IfStmt:
		return []ir.Instr{{Op: ir.OpIf, Condition: tc.lowerExpr(stmt.Condition, scope), Then: tc.lowerStatements(stmt.Then, scope), Else: tc.lowerStatements(stmt.Else, scope)}}
	case ast.WhileStmt:
		return []ir.Instr{{Op: ir.OpWhile, Condition: tc.lowerExpr(stmt.Condition, scope), Then: tc.lowerStatements(stmt.Body, scope)}}
	case ast.BreakStmt:
		return []ir.Instr{{Op: ir.OpBreak}}
	default:
		return nil
	}
}

func (tc *TypeChecker) lowerExpr(expr ast.Expr, scope map[string]ir.Type) string {
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
		return expr.Op + tc.lowerExpr(expr.Value, scope)
	case ast.BinaryExpr:
		// `+` means different things per operand type. Strings need the
		// runtime helper because C has no string concatenation operator, while
		// numbers use the host operator directly.
		if expr.Op == "+" && tc.isStringExpr(expr, scope) {
			return "rho_concat(" + tc.lowerExpr(expr.Left, scope) + ", " + tc.lowerExpr(expr.Right, scope) + ")"
		}

		left, right := tc.lowerExpr(expr.Left, scope), tc.lowerExpr(expr.Right, scope)

		// C promotes mismatched numeric operands implicitly; Go does not. An
		// explicit cast is valid in both, so both operands are widened here
		// and the backends emit the same semantics.
		//
		// A numeric literal needs no handling: it is an untyped constant that
		// adapts to the other operand in C and in Go alike.
		leftType, leftErr := tc.exprType(expr.Left, scope)
		rightType, rightErr := tc.exprType(expr.Right, scope)
		promote := leftErr == nil && rightErr == nil &&
			numeric(leftType) && numeric(rightType) &&
			leftType.Kind != rightType.Kind &&
			!isNumericLiteral(expr.Left) && !isNumericLiteral(expr.Right)
		if promote {
			target := promotedKind(leftType.Kind, rightType.Kind)
			left = "(" + cTypeForKind(target) + ")" + left
			right = "(" + cTypeForKind(target) + ")" + right
		}

		return "(" + left + " " + expr.Op + " " + right + ")"
	case ast.MemberExpr:
		object, isIdentifier := expr.Object.(ast.IdentifierExpr)
		if isIdentifier && object.Name == "yaml" && expr.Name == "load" {
			return "rho_yaml_load"
		}
		if isIdentifier && object.Name == "string" && expr.Name == "from_i32" {
			return "rho_string_from_i32"
		}
		return tc.lowerExpr(expr.Object, scope) + "." + expr.Name
	case ast.IndexExpr:
		return tc.lowerExpr(expr.Object, scope) + "[" + tc.lowerExpr(expr.Index, scope) + "]"
	case ast.CallExpr:
		if callee, ok := expr.Callee.(ast.IdentifierExpr); ok && callee.Name == "print" {
			if len(expr.Args) == 1 {
				return "printf(\"%s\\n\", " + tc.lowerExpr(expr.Args[0], scope) + ")"
			}
			return "printf(\"%s\\n\", \"\")"
		}
		callee := tc.lowerExpr(expr.Callee, scope)
		if identifier, ok := expr.Callee.(ast.IdentifierExpr); ok {
			if identifier.Name == "str_eq" {
				callee = "rho_str_eq"
			} else if signature, found := tc.funcs[identifier.Name]; found && signature.cName != "" {
				// A function belonging to an imported module is emitted under
				// its qualified name, so the call site must use the same name
				// or the generated code will not link.
				callee = signature.cName
			}
		}
		args := []string{}
		for _, arg := range expr.Args {
			args = append(args, tc.lowerExpr(arg, scope))
		}
		return callee + "(" + strings.Join(args, ", ") + ")"
	case ast.StructLiteralExpr:
		values := []string{}
		for _, value := range expr.Values {
			values = append(values, tc.lowerExpr(value, scope))
		}
		return "(" + cTypeForName(expr.Type.String()) + "){ " + strings.Join(values, ", ") + " }"
	case ast.ArrayLiteralExpr:
		values := make([]string, len(expr.Values))
		for index, value := range expr.Values {
			values[index] = tc.lowerExpr(value, scope)
		}
		return "{ " + strings.Join(values, ", ") + " }"
	case ast.IfExpr:
		return "(" + tc.lowerExpr(expr.Condition, scope) + " ? " + tc.lowerExpr(expr.Then, scope) + " : " + tc.lowerExpr(expr.Else, scope) + ")"
	default:
		return "0"
	}
}

// isStringExpr reports whether an expression has string type. It is used to
// decide whether `+` needs the runtime concatenation helper.
//
// The module has already been type checked by this point, so a failure here
// means the type is not statically known. That case is treated as numeric so
// the generated host code stays valid; the checker has already rejected
// genuinely ill-typed expressions.
func (tc *TypeChecker) isStringExpr(expr ast.Expr, scope map[string]ir.Type) bool {
	typ, err := tc.exprType(expr, scope)
	if err != nil {
		return false
	}
	return typ.Kind == ir.TypeString
}

// isNumericLiteral reports whether an expression is a bare numeric literal,
// which is an untyped constant for promotion purposes.
func isNumericLiteral(expr ast.Expr) bool {
	_, ok := expr.(ast.NumberExpr)
	return ok
}

// promotedKind returns the numeric type both operands are promoted to,
// following the usual widening order.
func promotedKind(a, b ir.TypeKind) ir.TypeKind {
	if a > b {
		return a
	}
	return b
}

// cTypeForKind returns the C spelling of an IR numeric kind, used to emit
// explicit widening casts.
func cTypeForKind(kind ir.TypeKind) string {
	switch kind {
	case ir.TypeBool:
		return "bool"
	case ir.TypeU8:
		return "uint8_t"
	case ir.TypeU16:
		return "uint16_t"
	case ir.TypeU32:
		return "uint32_t"
	case ir.TypeU64:
		return "uint64_t"
	case ir.TypeI32:
		return "int32_t"
	case ir.TypeF64:
		return "double"
	default:
		return "double"
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
