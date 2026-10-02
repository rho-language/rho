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
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
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

	// Structs the module declares itself, as opposed to synthesised ones.
	userStructs map[string]bool

	// externNames are the functions this module declared as externs, which a
	// port supplies. They are recorded so a call to one is checked by its
	// arguments even when the declaring module was not imported.
	externNames map[string]bool

	// aliases are names for other types, as in `type SqlDb = Ptr[Unit]`.
	aliases map[string]string
	// Generic declarations, kept as templates until monomorphised.
	genericStructs map[string]ast.StructDecl
	genericFuncs   map[string]ast.FuncDecl

	// Monomorphisation state.
	instantiationQueue []instantiation
	seenInstantiation  map[string]bool
	emitted            map[string]bool
	monomorphicStructs map[string]ir.StructIR
	monomorphicFuncs   map[string]ir.FuncIR

	// currentTypeArgs holds the type-parameter substitution of the generic
	// function currently being lowered, so nested applications and
	// `default(T)` resolve against concrete types.
	currentTypeArgs map[string]ir.Type

	// currentReturnHint is the enclosing function's declared return type, used
	// as a deduction hint while lowering a return expression.
	currentReturnHint ir.Type
}

type functionType struct {
	params []ir.Type
	result ir.Type
	cName  string
	// genericParams are the type parameters a declaration was written with, or
	// none. They are carried so a name declared twice can be resolved: a
	// concrete declaration is what an unqualified call means, and a generic one
	// only wins where nothing concrete was declared.
	genericParams []string
	// extern says the declaration is a foreign function rather than one Rho is
	// emitting. It decides the name the call site uses: a foreign function is
	// called by the name its header gave it, and a function Rho emits is called
	// by the name Rho chose for it.
	extern bool
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new type checker with built-in primitive types.
func New() *TypeChecker {
	tc := &TypeChecker{
		types:              map[string]ir.Type{},
		funcs:              map[string]functionType{},
		fields:             map[string]map[string]ir.Type{},
		genericParams:      map[string][]string{},
		userStructs:        map[string]bool{},
		externNames:        map[string]bool{},
		aliases:            map[string]string{},
		genericStructs:     map[string]ast.StructDecl{},
		genericFuncs:       map[string]ast.FuncDecl{},
		seenInstantiation:  map[string]bool{},
		emitted:            map[string]bool{},
		monomorphicStructs: map[string]ir.StructIR{},
		monomorphicFuncs:   map[string]ir.FuncIR{},
	}

	// Built-in types
	tc.types["void"] = ir.Type{Kind: ir.TypeVoid, Name: "void"}
	tc.types["bool"] = ir.Type{Kind: ir.TypeBool, Name: "bool"}
	tc.types["u8"] = ir.Type{Kind: ir.TypeU8, Name: "u8"}
	tc.types["u16"] = ir.Type{Kind: ir.TypeU16, Name: "u16"}
	tc.types["u32"] = ir.Type{Kind: ir.TypeU32, Name: "u32"}
	tc.types["u64"] = ir.Type{Kind: ir.TypeU64, Name: "u64"}
	tc.types["i32"] = ir.Type{Kind: ir.TypeI32, Name: "i32"}
	tc.types["i64"] = ir.Type{Kind: ir.TypeI64, Name: "i64"}
	tc.types["f64"] = ir.Type{Kind: ir.TypeF64, Name: "f64"}
	tc.types["string"] = ir.Type{Kind: ir.TypeString, Name: "string"}
	for _, name := range []string{"String", "Slice", "Ptr", "Option", "Result", "Array", "Map", "Range", "RS485", "UART", "I2C", "SPI", "GPIO", "Timer", "Modbus", "TimeSpec"} {
		tc.types[name] = ir.Type{Kind: ir.TypeStruct, Name: name}
	}
	// `Unit` is deliberately not registered here. It is the type of nothing,
	// used as a generic's default and as the element of a pointer a foreign
	// library owns, and it is synthesised — declared as an empty struct — by
	// synthesizeDefaultTypes when a program names it. Registering it as a
	// known type would make that synthesis believe it had already been declared
	// and leave the backend with a type nothing defines.

	return tc
}

// CheckModule validates all declarations inside a module.
func (tc *TypeChecker) CheckModule(m ast.Module) error {
	standaloneStdlib := strings.HasPrefix(m.Name, "std.")

	// Handler signature types are registered first so a handler's signature
	// resolves when the function table is built below.
	tc.registerProtocolTypes()

	// First pass: register struct types
	for _, decl := range m.Decls {
		if s, ok := decl.(ast.StructDecl); ok {
			// A generic struct is also a template. Its field types are recorded
			// with the parameters unresolved, because memberType substitutes
			// them at each use site.
			if len(s.TypeParams) > 0 {
				tc.genericStructs[s.Name] = s
			}
			tc.userStructs[s.Name] = true
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
			// A generic function keeps its declared signature so a use site
			// supplying type arguments can be checked after substitution.
			if len(fn.TypeParams) > 0 {
				tc.genericFuncs[fn.Name] = fn
			}
			// A written type in a signature is resolved the way one anywhere else
			// is, so a generic application mangles to the specialisation it is
			// built from. Going straight to typeFor left a signature naming
			// `Result[u8, i32]` while the struct emitted for it is
			// `Result_u8_i32`, and the backend was handed a type it had never
			// heard of.
			//
			// A template is the exception. Its own signature still names its
			// parameters — `Box[T]` is `Box_T` until something is substituted for
			// T — and mangling it here would fix the name before the
			// specialisation that needs it.
			signatureType := tc.declaredType
			if len(fn.TypeParams) > 0 {
				signatureType = tc.typeFor
			}
			params := make([]ir.Type, len(fn.Params))
			for i, param := range fn.Params {
				params[i] = signatureType(param.Type)
			}
			entry := functionType{
				params: params,
				result: signatureType(fn.ReturnType),
				cName:  qualifiedCName(fn.Owner, fn.Name),
				extern: fn.Extern,
			}
			if fn.Owner != "" {
				tc.funcs[fn.Owner+"."+fn.Name] = entry
			}
			// Recorded here, where declarations are read, rather than where the
			// module is lowered. A call is checked before the module is lowered,
			// so a rule that asks whether a call crosses into C has to know by
			// then — and the declarations it asks about are the ones sitting in
			// this module, which is where they have just been read.
			if fn.Extern {
				tc.externNames[fn.Name] = true
				if fn.Owner != "" {
					tc.externNames[fn.Owner+"."+fn.Name] = true
				}
			}
			// A name declared once is that declaration, and a second one does
			// not replace it. The exception is a generic: a call written
			// unqualified is a call to something the caller can see, and a
			// concrete declaration is what an unqualified call means when the
			// generic beside it would have to be deduced from the arguments.
			//
			// Two modules can each declare a name as ordinary as `error` — the
			// standard library's result accessor and a logger's own — and a
			// program that imported both would otherwise have the generic win
			// and be told it cannot deduce type arguments for a call that names
			// no types at all.
			existing, exists := tc.funcs[fn.Name]
			if !exists || (len(existing.genericParams) > 0 && len(fn.TypeParams) == 0) {
				entry.genericParams = fn.TypeParams
				tc.funcs[fn.Name] = entry
			}
		}
	}
	for _, decl := range m.Decls {
		if s, ok := decl.(ast.StructDecl); ok {
			tc.types[s.Name] = ir.Type{Kind: ir.TypeStruct, Name: s.Name}
		}
	}
	// An alias is registered before anything is validated, because a function
	// body may name the alias as a type and as a constructor — `SqlDb(0)` —
	// and a body validated before the alias exists would be told it calls a
	// function nobody declared. Registration is a first pass because it is a
	// declaration rather than a check: nothing here can fail on a well-formed
	// alias, and a malformed one is a manifest a resolver reports.
	for _, decl := range m.Decls {
		if d, ok := decl.(ast.TypeDecl); ok {
			// An alias is a name for another type, not a new type of its own.
			// Registering it as a struct would make the backend define a struct
			// whose name is the written spelling — `SqlDb` becoming a type called
			// `Ptr[Unit]` — so an alias a program uses for a handle is a type
			// nothing can name. The alias is recorded and resolution follows it.
			tc.aliases[d.Name] = d.Type.String()
			tc.types[d.Name] = ir.Type{Kind: ir.TypeStruct, Name: d.Name}
		}
	}

	// Second pass: validate declarations
	for _, decl := range m.Decls {
		switch d := decl.(type) {
		case ast.StructDecl:
			if err := tc.checkStruct(d); err != nil {
				return locate(err, d.Pos, "struct "+d.Name)
			}
		case ast.FuncDecl:
			if standaloneStdlib || strings.HasPrefix(d.Owner, "std.") {
				continue
			}
			if err := tc.checkFunc(d); err != nil {
				return locate(err, d.Pos, "function "+d.Name)
			}
		case ast.RouteDecl:
			if err := tc.checkRoute(d); err != nil {
				return locate(err, d.Pos, "route "+d.Method+" "+d.Path)
			}
		case ast.TypeDecl:
			// An alias was registered in the pass above, before anything was
			// validated. There is nothing to check: the target is resolved
			// wherever it is named, and a name that resolves to nothing becomes
			// an error at the use rather than at the declaration.
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
		statementTerminates, err := tc.checkStatement(statement, scope, result, inLoop)
		if err != nil {
			// The statement's own place, which is what a report points at. A
			// check that knows better — one about an expression, or a name that
			// resolved — carries its own position, and locate keeps that one.
			return false, locate(err, statement.Position(), "")
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

// exprType types an expression.
//
// Every expression is typed through here, so it is where an error about one is
// attributed to the expression rather than to the statement containing it: a
// name that does not resolve points at the name, not at the line the statement
// began on. A check that reports about a sub-expression overrides this, and this
// is the fallback for the ones that do not.
func (tc *TypeChecker) exprType(expr ast.Expr, scope map[string]ir.Type) (ir.Type, error) {
	typ, err := tc.exprTypeExpected(expr, scope, ir.Type{})
	if err != nil {
		return typ, locate(err, expr.Position(), "")
	}
	return typ, nil
}

// exprTypeExpected types an expression with a known expected type. The
// expected type is only a hint: it seeds deduction of a generic application
// that was written without type arguments, and is otherwise ignored.
func (tc *TypeChecker) exprTypeExpected(expr ast.Expr, scope map[string]ir.Type, expected ir.Type) (ir.Type, error) {
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
		// A generic call is checked against the signature with its type
		// arguments substituted, which is what makes Ok[u32, u32](7) work.
		// When the arguments are omitted they are deduced from the call site.
		//
		// This is only the case when the name is generic. A name declared twice
		// — a logger's `error(subject, message)` beside the standard library's
		// `error[T, E](result)` — is called as the concrete declaration, and
		// taking the generic one here would ask a call that names no types to
		// deduce them.
		var subs map[string]ir.Type
		if identifier, ok := value.Callee.(ast.IdentifierExpr); ok {
			if decl, generic := tc.genericFuncs[identifier.Name]; generic && tc.isGenericName(identifier.Name) {
				args := value.TypeArgs
				if len(args) == 0 {
					if deduced := tc.applicationFor(value, scope, expected); len(deduced) > 0 {
						// Rebuild the deduction as a substitution directly.
						subs = map[string]ir.Type{}
						for i, param := range decl.TypeParams {
							if i < len(deduced) {
								subs[param] = deduced[i]
							}
						}
					} else {
						return ir.Type{}, fmt.Errorf("cannot deduce type arguments for %s; %s unresolved, supply them explicitly",
							identifier.Name, strings.Join(tc.undeducedParams(decl.TypeParams, decl.TypeParamDefaults, subs), ", "))
					}
				} else {
					subs = tc.buildSubstitution(decl.TypeParams, decl.TypeParamDefaults, args)
				}
			}
		}
		// default(T) is the zero value of T.
		if typeName, ok := isDefaultCall(value); ok {
			if resolved, found := tc.currentTypeArgs[typeName]; found {
				return resolved, nil
			}
			if typ, resolved := tc.resolveType(typeName); resolved {
				return typ, nil
			}
			return ir.Type{Kind: ir.TypeStruct, Name: typeName}, nil
		}
		// Whether this call crosses into C decides whether a Rho string may
		// stand in for a C pointer, which is a question only a foreign boundary
		// can answer.
		foreign := false
		if identifier, ok := value.Callee.(ast.IdentifierExpr); ok {
			var exists bool
			if tc.isExternName(identifier.Name) {
				foreign = true
			}
			signature, exists = tc.funcs[identifier.Name]
			if !exists && identifier.Name != "print" && !isCallableBuiltin(identifier.Name, tc.types) && !isExternalBuiltin(identifier.Name) && !tc.isTypeName(identifier.Name) {
				// A call to a type is a construction, not a call to something
				// missing: `SqlDb(0)` builds a value of that type, and a program
				// that writes one is spelling a type rather than calling a
				// function that was never declared.
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
			// A server backend provides this signature, so the generic
			// fallback below must not claim it.
			_, hasProtocolSignature := protocolFuncs[identifier.Name]
			if !exists && hasProtocolSignature {
				signature = protocolFuncs[identifier.Name]
			}
			if !exists && !hasProtocolSignature {
				// An intrinsic a backend supplies has a known signature, so a
				// call to it is checked like any other.
				if intrinsic, isIntrinsic := intrinsicSignatures[identifier.Name]; isIntrinsic {
					signature = intrinsic
				} else {
					if identifier.Name == "clamp_f64" || identifier.Name == "lerp" || identifier.Name == "sqrt" || identifier.Name == "sin" || identifier.Name == "cos" || identifier.Name == "atan2" || identifier.Name == "floor" {
						return ir.Type{Kind: ir.TypeF64, Name: "f64"}, nil
					}
					if identifier.Name == "str_eq" {
						return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
					}
					if strings.HasPrefix(identifier.Name, "c_") || identifier.Name == "sizeof" {
						return ir.Type{Kind: ir.TypeI32, Name: "i32"}, nil
					}
					// An extern declared by an imported module is a promise a
					// port fulfils, and its signature is the declaration's. When
					// the declaration is not in scope — a module that calls one
					// without importing the module that declared it — the call
					// is checked by its arguments alone rather than refused: the
					// generated program carries the declaration, and refusing
					// here would fail a program that the port can build.
					if tc.isExternName(identifier.Name) {
						for _, argument := range value.Args {
							if _, err := tc.exprType(argument, scope); err != nil {
								return ir.Type{}, err
							}
						}
						return ir.Type{Kind: ir.TypeU32, Name: "u32"}, nil
					}
					return ir.Type{}, nil
				}
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
			want := signature.params[i]
			if subs != nil {
				want = tc.substituteType(want, subs)
			}
			if !assignable(want, actual) && !(foreign && stringToC(actual, want)) {
				return ir.Type{}, fmt.Errorf("argument %d has type %s, want %s", i+1, typeName(actual), typeName(want))
			}
		}
		result := signature.result
		if subs != nil {
			result = tc.substituteType(result, subs)
		}
		return result, nil
	case ast.StructLiteralExpr:
		// A generic struct literal written without type arguments is deduced
		// from the field values, preferring the expected type.
		if len(value.Type.Args) == 0 {
			if decl, generic := tc.genericStructs[value.Type.Name]; generic {
				args := tc.applicationFor(value, scope, expected)
				if len(args) == 0 {
					return ir.Type{}, fmt.Errorf("cannot deduce type arguments for %s; supply them explicitly", value.Type.Name)
				}
				return ir.Type{Kind: ir.TypeStruct, Name: mangleGeneric(decl.Name, args)}, nil
			}
		}
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
	// An optional pointer has no fields, because it is a pointer. It answers the
	// two fields the Option interface is written in terms of anyway, so that code
	// written against `Option` works unchanged against the cheaper
	// representation: `is_some` is the null test and `value` is the pointer.
	if IsOptionalPointer(object.Name) {
		switch name {
		case "is_some":
			return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
		case "value":
			return ir.Type{Kind: ir.TypePtr, Name: "Ptr[u8]", Args: []ir.Type{{Kind: ir.TypeU8, Name: "u8"}}}, nil
		}
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
	case "i64":
		return ir.TypeI64
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
	if ok || name == "Some" || name == "None" || name == "Ok" || name == "Err" {
		return true
	}
	_, protocol := protocolFuncs[name]
	return protocol
}

// intrinsicLowering maps an intrinsic to the name both backends emit. An empty
// value means the name is used unchanged.
var intrinsicLowering = map[string]string{
	"strlen":          "rho_strlen",
	"str_starts_with": "rho_str_starts_with",
	"str_trim_prefix": "rho_str_trim_prefix",
	"str_contains":    "rho_str_contains",
	"str_at":          "rho_str_at",
	"str_index_of":    "rho_str_index_of",
	"str_slice":       "rho_str_slice",
	"int_to_str":      "rho_int_to_str",
	"int_from_str":    "rho_int_from_str",
	"template_get":    "template_get",
	"template_has":    "template_has",
	"template_fill":   "template_fill",
	"auth_bearer":     "auth_bearer",
	"auth_subject":    "auth_subject",
	"str_from_byte":   "rho_str_from_byte",
	"env_or":          "rho_env_or",
}

// isExternalBuiltinNames are the intrinsics a module may call without
// declaring them, because a backend supplies the implementation.
var isExternalBuiltinNames = map[string]bool{}

func isExternalBuiltin(name string) bool {
	if isExternalBuiltinNames[name] {
		return true
	}
	if strings.HasPrefix(name, "c_") {
		return true
	}
	for _, builtin := range []string{"sqrt", "sin", "cos", "atan2", "floor", "clamp_f64", "lerp", "str_eq", "panic", "abort", "sizeof", "robohub_platform_init", "robohub_board_register"} {
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
	return value.Kind == ir.TypeI32 || value.Kind == ir.TypeI64 ||
		value.Kind == ir.TypeU8 || value.Kind == ir.TypeU16 ||
		value.Kind == ir.TypeU32 || value.Kind == ir.TypeU64 ||
		value.Kind == ir.TypeF64
}
func comparable(left, right ir.Type) bool { return assignable(left, right) || assignable(right, left) }

// stringToC reports whether a Rho string may be passed where a C pointer is
// wanted at a foreign call.
//
// It is the whole of how a Rho program passes text to C, and it costs nothing:
// the string is already the pointer. A literal is written into the program's
// read-only data with a terminating byte after it, and a string that was built
// at run time is the same thing — a `const char*` that C strings are. So
// `robohub_get_gpio("user_led")` passes the address of the literal, not a copy
// and not a buffer.
//
// The alternative was refusing, and refusing is what produced
// `robohub_get_gpio(name.something())` in the first place: a program with a
// string to give had nothing to give it with. Every header written afterwards
// would have carried the same workaround, which is the shape of a missing
// language rule rather than of a missing function.
//
// Only at a foreign boundary, and only outward. Inside Rho a string is not a
// pointer, because a Rho string knows its length and a C one does not, and
// pretending otherwise in Rho's own code would hide the difference that C
// callers rely on.
func stringToC(got, want ir.Type) bool {
	if got.Kind != ir.TypeString || !pointsAtBytes(want) {
		return false
	}
	// Only where the pointer is to a byte, which is what a C string is. A
	// pointer to a struct is not text, and a string is not a struct.
	return len(want.Args) == 0 || want.Args[0].Kind == ir.TypeU8
}

// pointsAtBytes reports whether a type is a pointer to a byte.
//
// A pointer arrives here in one of two spellings, because it is resolved on two
// paths: as the type of a thing, which is a `TypePtr`, and as a type written in
// a declaration, which is the concrete specialisation a generic application
// mangles to. `Ptr[u8]` and `Ptr_u8` are the same pointer; accepting only one of
// them would make the rule depend on which of the two paths a signature was read
// by, which is not a distinction a program can see and so should not be one the
// language makes.
func pointsAtBytes(want ir.Type) bool {
	if want.Kind == ir.TypePtr {
		return true
	}
	return strings.HasPrefix(want.Name, "Ptr_")
}

func assignable(want, got ir.Type) bool {
	if want.Kind == ir.TypeInvalid || got.Kind == ir.TypeInvalid {
		return true
	}
	// A pointer is usable where an optional pointer is wanted. They are the same
	// representation, so the conversion is the identity; what it means is that a
	// value returned by a C library can be held in an `Option[Ptr[T]]` without
	// the program writing a conversion at every call.
	//
	// A null still means absent, because that is what a null is in the
	// representation. The conversion loses nothing: `robohub_get_gpio` answering
	// NULL still arrives as `None`.
	if IsOptionalPointer(want.Name) && got.Kind == ir.TypePtr {
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

	// Generic declarations are templates: they are registered for expansion
	// rather than emitted directly.
	for _, decl := range m.Decls {
		switch decl := decl.(type) {
		case ast.StructDecl:
			if len(decl.TypeParams) > 0 {
				tc.genericStructs[decl.Name] = decl
				continue
			}
			out.Structs = append(out.Structs, tc.structToIR(decl))
		}
	}

	for _, decl := range m.Decls {
		if fn, ok := decl.(ast.FuncDecl); ok {
			if len(fn.TypeParams) > 0 {
				tc.genericFuncs[fn.Name] = fn
				continue
			}
			if fn.Extern {
				tc.externNames[fn.Name] = true
			}
			out.Funcs = append(out.Funcs, tc.funcToIR(fn))
		}
	}

	// Aliases are carried so a backend can define them: a program may declare a
	// value under the name it gave a type, and a file that emitted only the type
	// being named would leave that declaration referring to nothing.
	for _, decl := range m.Decls {
		if d, ok := decl.(ast.TypeDecl); ok {
			out.Aliases = append(out.Aliases, ir.AliasIR{
				Name:   d.Name,
				Target: tc.cTypeFor(tc.typeFor(d.Type.String())),
			})
		}
	}

	// Routes are collected for a server target; the C backend has no server, so
	// it only needs the types to compile.
	for _, decl := range m.Decls {
		if route, ok := decl.(ast.RouteDecl); ok {
			out.Routes = append(out.Routes, tc.routeToIR(route))
		}
	}
	if len(out.Routes) > 0 || irModuleUsesProtocolTypes(out) {
		tc.synthesiseProtocolTypes(&out)
	}

	// A type parameter default may name a type the module does not define; give
	// it a zero-sized definition so the backends can emit it.
	tc.synthesizeDefaultTypes(m, &out)

	// Expand every generic application the program asks for.
	tc.monomorphise(m, &out)

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
	// A written type in a signature is resolved the way one anywhere else is, so
	// a generic application mangles to the specialisation that was emitted for
	// it. Resolving a parameter from its expression left the name as written —
	// `Result[u8, i32]` — while the struct emitted for it is `Result_u8_i32`, and
	// the backend was handed a type it had never heard of.
	//
	// A template is the exception: its own signature still names its parameters,
	// and substituting into them needs the name as written.
	resolve := tc.declaredType
	if len(fn.TypeParams) > 0 {
		resolve = func(written string) ir.Type {
			return tc.typeFromExpr(fn.Params[0].TypeExpr, fn.TypeParams)
		}
	}
	params := make([]ir.ParamIR, len(fn.Params))
	for i, p := range fn.Params {
		paramType := resolve(p.Type)
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
		ReturnType: tc.declaredType(fn.ReturnType),
		Body:       tc.lowerFunctionBody(fn, scope),
		Extern:     fn.Extern,
	}
}

// lowerFunctionBody lowers a function body with the deduction hint that the
// checking pass used, so both passes pick the same specialisation.
func (tc *TypeChecker) lowerFunctionBody(fn ast.FuncDecl, scope map[string]ir.Type) []ir.Instr {
	previous := tc.currentReturnHint
	tc.currentReturnHint = tc.typeFor(fn.ReturnType)
	body := tc.lowerStatements(fn.Body, scope)
	tc.currentReturnHint = previous
	return body
}

// qualifiedCName is the C symbol a declaration is emitted as.
//
// A name is qualified because Rho is free to choose it. Two modules may each
// declare an `init`, and the generated code has to tell them apart, so a
// declaration carries its module: `logger_init`, `store_init`.
//
// Whether that qualification is the right name in the target language is a
// separate question, and the two backends answer it differently. Go compiles
// every module of a program into one package, where two unprefixed `init`s would
// collide, so the qualified name is what Go wants. C does not: an extern is a
// function a header already declared, and its symbol is whatever that header's
// compiler agreed on — the header does not know this language exists and could
// not have avoided the name. The C backend reads `Extern` and emits the plain
// name for exactly that reason.
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
	// A pointer is a kind and not a name. The parser writes `Ptr[u8]` as the
	// name Ptr with u8 as an argument, so the kind is decided from the name
	// before the written spelling replaces it: a backend handed a struct named
	// `Ptr[u8]` mangles it into a name no C compiler knows, and every foreign
	// call that passes a pointer goes through here.
	if expr.Name == "Ptr" {
		// A pointer to nothing is a handle, and a handle is a struct carrying an
		// address rather than a raw pointer. That is what lets a program hold
		// one, pass it, and get one back: a pointer to an empty struct has none
		// of those, and a cast of 0 could not build it.
		//
		// Every other `Ptr[T]` stays a real pointer, because a pointer into the
		// middle of a buffer has no name of its own and nothing to hold.
		element := ""
		if len(expr.Args) > 0 {
			element = expr.Args[0].String()
		}
		if element == "Unit" || element == "void" || element == "" {
			typ.Kind = ir.TypeStruct
			typ.Name = "PtrUnit"
			typ.Args = nil
			return typ
		}
		typ.Kind = ir.TypePtr
	}
	// An alias is a name for another type, so the written spelling is not the
	// type: replacing the name with it would make `SqlDb` a type of its own
	// that nothing defines, which is what a handle would otherwise be.
	_, aliased := tc.aliases[expr.Name]
	if typ.Kind != ir.TypePtr && !aliased {
		typ.Name = expr.String()
	}
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
	// An alias resolves to what it names, so a handle written as `SqlDb` is the
	// pointer it is and a backend spells it as one.
	//
	// The follow is depth-bounded because an alias is a claim a manifest can
	// make wrongly — `type A = B` and `type B = A` is a document nobody has to
	// compile to write — and a resolution that loops takes the compiler with it.
	if resolved, found := tc.followAlias(name, 0); found {
		return resolved, true
	}
	if typ, ok := tc.types[name]; ok {
		return typ, true
	}
	base := name
	element := ""
	if i := strings.IndexByte(base, '['); i >= 0 {
		element = strings.TrimSuffix(base[i+1:], "]")
		base = base[:i]
	}
	// A pointer keeps the type it points at. Losing it would make every backend
	// spell `Ptr[u8]` as a name, which is what a C compiler rejects.
	if base == "Ptr" {
		pointer := ir.Type{Kind: ir.TypePtr, Name: name}
		if element != "" {
			pointer.Args = []ir.Type{tc.typeFor(element)}
		}
		return pointer, true
	}
	if typ, ok := tc.types[base]; ok {
		typ.Name = name
		return typ, true
	}
	if (strings.Contains(name, "[") || (len(base) == 1 && base != "")) && unicodeUpper(base[0]) {
		return ir.Type{Kind: ir.TypeStruct, Name: name}, true
	}
	// `Unit` is the type of nothing, and it is what a handle points at. It is
	// synthesised when a generic names it as a default, but a pointer may name it
	// before anything has declared one, and a name that resolves to nothing is
	// how a handle becomes a pointer to a pointer.
	if name == "Unit" || name == "RhoUnit" {
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
		declaredName := stmt.Type.String()
		if stmt.Value != nil {
			if declaredName == "" {
				// A type-less binding lowers with the type of its value, which
				// may itself be a mangled generic specialisation.
				inferred, err := tc.exprType(stmt.Value, scope)
				if err == nil {
					declaredName = typeName(inferred)
				}
			} else {
				// The declared type is the deduction hint, matching the checking
				// pass so both agree on the emitted specialisation.
				declared := tc.declaredType(declaredName)
				value = tc.lowerExprExpected(stmt.Value, scope, tc.expectedHint(declaredName, declared))
			}
			if value == "" {
				value = tc.lowerExpr(stmt.Value, scope)
			}
		}
		// Record the binding so later expressions see its type.
		scope[stmt.Name] = tc.typeFor(declaredName)
		return []ir.Instr{{Op: ir.OpLet, Dest: stmt.Name, Type: tc.cTypeForTypeName(declaredName), Args: []string{value}}}
	case ast.AssignStmt:
		return []ir.Instr{{Op: ir.OpAssign, Dest: tc.lowerExpr(stmt.Target, scope) + " " + stmt.Op, Args: []string{tc.lowerExpr(stmt.Value, scope)}}}
	case ast.ExprStmt:
		return []ir.Instr{{Op: ir.OpExpr, Args: []string{tc.lowerExpr(stmt.Expr, scope)}}}
	case ast.ReturnStmt:
		value := ""
		if stmt.Value != nil {
			value = tc.lowerExprExpected(stmt.Value, scope, tc.currentReturnHint)
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
	return tc.lowerExprExpected(expr, scope, ir.Type{})
}

// lowerExprExpected lowers an expression with a known expected type, so a
// generic application written without type arguments deduces the same way the
// checking pass did.
func (tc *TypeChecker) lowerExprExpected(expr ast.Expr, scope map[string]ir.Type, expected ir.Type) string {
	switch expr := expr.(type) {
	case ast.IdentifierExpr:
		return expr.Name
	case ast.NumberExpr:
		// A Rho number may be written with separators — 1_000_000 — and the
		// backends are handed the digits, not the way the author spaced them.
		// Carrying the underscores through would write `1_000_000` into C or Go,
		// which is not a number either language knows.
		return strings.ReplaceAll(expr.Value, "_", "")
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
		// An optional pointer is a pointer, so the two fields the Option
		// interface is written in terms of are the null test and the pointer
		// itself. Answering them here rather than emitting a field access is what
		// lets code written against `Option` work against the cheaper
		// representation.
		if receiverType, typeErr := tc.exprType(expr.Object, scope); typeErr == nil && IsOptionalPointer(receiverType.Name) {
			receiver := tc.lowerExpr(expr.Object, scope)
			switch expr.Name {
			case "is_some":
				return "((" + receiver + ") != NULL)"
			case "value":
				return "(" + receiver + ")"
			}
		}
		return tc.lowerExpr(expr.Object, scope) + "." + expr.Name
	case ast.IndexExpr:
		return tc.lowerExpr(expr.Object, scope) + "[" + tc.lowerExpr(expr.Index, scope) + "]"
	case ast.CallExpr:
		// default(T) is the zero value of T, backing Result and Option.
		if typeName, ok := isDefaultCall(expr); ok {
			if resolved, found := tc.currentTypeArgs[typeName]; found {
				return tc.zeroValueFor(resolved.Name)
			}
			if typ, resolved := tc.resolveType(typeName); resolved {
				return tc.zeroValueFor(typ.Name)
			}
			return tc.zeroValueFor(typeName)
		}
		if callee, ok := expr.Callee.(ast.IdentifierExpr); ok && callee.Name == "print" {
			// Exactly what was given, with nothing added.
			//
			// A newline appended here would appear in a program's output whether
			// it was asked for or not: a string that already ends in one gets two,
			// and a number printed on a line of its own splits into three. Worse,
			// the two backends disagreed — this appended one and Go's own `print`
			// did not, so the same program printed differently depending on what it
			// was built for.
			//
			// A program that wants a line ending writes one.
			if len(expr.Args) == 1 {
				return "printf(\"%s\", " + tc.lowerExpr(expr.Args[0], scope) + ")"
			}
			return "printf(\"%s\", \"\")"
		}
		callee := tc.lowerExpr(expr.Callee, scope)
		if identifier, ok := expr.Callee.(ast.IdentifierExpr); ok {
			switch {
			case identifier.Name == "str_eq":
				callee = "rho_str_eq"
			case identifier.Name == "panic":
				// The intrinsic the backends supply for a message abort.
				callee = "panicf"
			case intrinsicLowering[identifier.Name] != "":
				// A string or environment intrinsic, emitted by both backends
				// under a fixed name.
				callee = intrinsicLowering[identifier.Name]
			case tc.isTypeName(identifier.Name):
				// A call to a type is a conversion, not a call. `Ptr[Unit](0)` and
				// `SqlDb(0)` build a value of that type; without this they are
				// mangled as if they named a generic function, and the generated
				// code calls a function that was never written.
				//
				// The target is spelled with its arguments, which is where the
				// element of a pointer lives: the parser splits `Ptr[Unit]` into
				// the name `Ptr` and one type argument, and a cast to the name
				// alone would be a cast to a pointer with no element.
				//
				// The spelling is the backend's own rather than a C type name,
				// because this string is the Go and the C expression alike: a
				// conversion written as a C type name is not a conversion in Go
				// at all, and the value it built was a nil.
				written := identifier.Name
				if len(expr.TypeArgs) > 0 {
					args := make([]string, 0, len(expr.TypeArgs))
					for _, argument := range expr.TypeArgs {
						args = append(args, tc.cTypeFor(tc.typeFor(argument.String())))
					}
					written = identifier.Name + "<" + strings.Join(args, ", ") + ">"
				}
				callee = tc.cTypeFor(tc.typeFor(written))
				if len(expr.Args) > 0 {
					// An alias resolves to what it names, so `SqlDb(0)` is a
					// conversion to the pointer it is rather than to a type
					// called SqlDb that nothing defines.
					return "(" + callee + ")" + tc.lowerExpr(expr.Args[0], scope)
				}
			case len(expr.TypeArgs) > 0 || isGenericCallee(tc, identifier.Name):
				// A generic call names a specialisation, so emit the mangled
				// name the monomorphised function is emitted under, including
				// the owner prefix when it came from an imported module.
				typeArgs := tc.resolveTypeArgs(expr.TypeArgs)
				if len(typeArgs) == 0 {
					typeArgs = tc.applicationFor(expr, scope, expected)
				}
				mangled := mangleGeneric(identifier.Name, typeArgs)
				if len(typeArgs) > 0 {
					tc.recordInstantiation(instantiation{
						Name:     identifier.Name,
						TypeArgs: typeArgs,
						IsStruct: false,
					})
				}
				if decl, ok := tc.genericFuncs[identifier.Name]; ok {
					callee = tc.qualifiedName(decl.Owner, mangled)
				} else {
					callee = mangled
				}
			default:
				// A function belonging to an imported module is emitted under
				// its qualified name, so the call site must use the same name
				// or the generated code will not link. An intrinsic is exempt:
				// the backend supplies it unprefixed, so a module that declares
				// one is documenting it rather than defining it.
				_, isIntrinsic := intrinsicSignatures[identifier.Name]
				if signature, found := tc.funcs[identifier.Name]; found && signature.cName != "" && !isIntrinsic {
					// A foreign function is called by the name its header gave
					// it. The qualified name is what the language uses to tell two
					// of its own declarations apart, and applying it here would
					// call a symbol no C compiler ever heard of — the header does
					// not know this language exists and could not have avoided
					// the name. A function Rho emits is still emitted under the
					// qualified name, and must be called the same way.
					callee = signature.cName
				}
			}
		}
		args := []string{}
		for _, arg := range expr.Args {
			args = append(args, tc.lowerExpr(arg, scope))
		}
		return callee + "(" + strings.Join(args, ", ") + ")"
	case ast.StructLiteralExpr:
		values := []string{}
		// A generic struct literal names a specialisation, so record the
		// application for monomorphisation and emit the mangled name. The type
		// arguments are taken from the source when written, and deduced
		// otherwise using the same rules as the checking pass.
		name := expr.Type.Name
		if decl, generic := tc.genericStructs[expr.Type.Name]; generic {
			typeArgs := tc.resolveTypeArgs(expr.Type.Args)
			if len(typeArgs) == 0 {
				typeArgs = tc.applicationFor(expr, scope, expected)
			}
			if len(typeArgs) > 0 {
				tc.recordInstantiation(instantiation{Name: decl.Name, TypeArgs: typeArgs, IsStruct: true})
				name = mangleGeneric(decl.Name, typeArgs)
			} else {
				name = decl.Name
			}
		}
		// An optional pointer is written as the pointer: `Some(p)` is `p` and
		// `None` is `NULL`. Building the struct that a general Option uses would
		// mean constructing a tagged pair that the representation does not have,
		// and the C emitted here would not match the C emitted for the type.
		if IsOptionalPointer(mangleTypeName(name)) {
			return tc.lowerOptionalPointerLiteral(expr, name, scope)
		}
		for _, value := range expr.Values {
			values = append(values, tc.lowerExpr(value, scope))
		}
		return "(" + mangleTypeName(name) + "){ " + strings.Join(values, ", ") + " }"
	case ast.ArrayLiteralExpr:
		values := make([]string, len(expr.Values))
		for index, value := range expr.Values {
			values[index] = tc.lowerExpr(value, scope)
		}
		return "{ " + strings.Join(values, ", ") + " }"
	case ast.IfExpr:
		return "(" + tc.lowerExpr(expr.Condition, scope) + " ? " + tc.lowerExpr(expr.Then, scope) + " : " + tc.lowerExpr(expr.Else, scope) + ")"
	case ast.CastExpr:
		// A C cast. The Go backend recognises the same spelling and emits a
		// real conversion, so both backends read one expression.
		//
		// The type is resolved rather than spelled from the written name, because
		// a cast to a pointer or to an alias is not a type with that name: `Ptr[T]`
		// cast to by name is a C expression naming a function, not a cast.
		castTo := tc.typeFor(expr.Type.String())
		return "(" + tc.cTypeFor(castTo) + ")" + tc.lowerExpr(expr.Value, scope)
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
	case ir.TypeI64:
		return "int64_t"
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
	case "void", "Unit":
		return "void"
	case "Ptr":
		// A pointer is spelled as a C pointer. Mangling the name would write
		// `Ptr_u8`, which is not a type a C compiler knows: every foreign call
		// that passes a pointer crosses this path, so a binding generated for
		// robohub-common would not compile at all.
		element := strings.TrimSuffix(strings.TrimPrefix(name[len(base):], "["), "]")
		// A pointer to nothing is a pointer to nothing: `Ptr[Unit]` is how a
		// foreign handle is declared, and C spells that void*.
		//
		// Go spells it as a nameable struct rather than a raw pointer, because a
		// program has to be able to hold a handle, pass it, and get one back. A
		// bare `*Unit` would be a pointer to an empty struct — something no
		// address can meaningfully be, and something a cast of `0` could not
		// build.
		if element == "" || element == "Unit" || element == "void" {
			return mangleTypeName("Ptr" + element)
		}
		return cTypeForName(element) + "*"
	}
	return strings.NewReplacer("[", "_", "]", "", ",", "_").Replace(name)
}

// intrinsicSignatures are the calls a backend supplies directly. They are
// declared `extern fn` in the stdlib, so the typechecker has to know their
// shape even though no function is registered for them.
var intrinsicSignatures = map[string]functionType{
	// abort ends the program with a non-zero status.
	"abort": {
		params: []ir.Type{},
		result: ir.Type{Kind: ir.TypeVoid, Name: "void"},
	},
	// panic ends the program with a message.
	"panic": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeVoid, Name: "void"},
	},
	// emit writes one log record. The rho-log library declares it and a backend
	// supplies it, so the library is the only surface a caller needs.
	"emit": {
		params: []ir.Type{
			{Kind: ir.TypeU32, Name: "u32"},
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeVoid, Name: "void"},
	},
}

func init() {
	// String and environment intrinsics. Rho's string library is still thin, so
	// these are language-level rather than tied to a particular target, which
	// keeps the code that uses them testable on every backend.
	stringArg := []ir.Type{{Kind: ir.TypeString, Name: "string"}}
	twoStrings := []ir.Type{
		{Kind: ir.TypeString, Name: "string"},
		{Kind: ir.TypeString, Name: "string"},
	}
	intrinsicSignatures["strlen"] = functionType{
		params: stringArg,
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	}
	intrinsicSignatures["str_starts_with"] = functionType{
		params: twoStrings,
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	}
	intrinsicSignatures["str_trim_prefix"] = functionType{
		params: twoStrings,
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	}
	intrinsicSignatures["str_contains"] = functionType{
		params: twoStrings,
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	}
	// str_at reads one byte, which is what lets Rho walk a string itself.
	intrinsicSignatures["str_at"] = functionType{
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeU32, Name: "u32"},
		},
		result: ir.Type{Kind: ir.TypeU8, Name: "u8"},
	}
	// str_index_of finds a substring, or returns -1 when it is absent.
	intrinsicSignatures["str_index_of"] = functionType{
		params: twoStrings,
		result: ir.Type{Kind: ir.TypeI32, Name: "i32"},
	}
	// str_slice takes a run of bytes, which is what turns a document into
	// something Rho can read field by field.
	// int_to_str renders a number, which text output needs.
	intrinsicSignatures["int_to_str"] = functionType{
		params: []ir.Type{{Kind: ir.TypeU32, Name: "u32"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	}
	// int_from_str reads a run of digits, returning 0 for anything else.
	intrinsicSignatures["int_from_str"] = functionType{
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	}
	// str_from_byte renders one byte as a one-character string.
	intrinsicSignatures["str_from_byte"] = functionType{
		params: []ir.Type{{Kind: ir.TypeU8, Name: "u8"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	}
	intrinsicSignatures["str_slice"] = functionType{
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeU32, Name: "u32"},
			{Kind: ir.TypeU32, Name: "u32"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	}
	intrinsicSignatures["env_or"] = functionType{
		params: twoStrings,
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	}
	for _, name := range []string{
		"strlen", "str_starts_with", "str_trim_prefix", "str_contains", "str_at",
		"str_index_of", "str_slice", "int_to_str", "str_from_byte",
		"template_get", "template_has", "template_fill", "env_or",
		"auth_bearer", "auth_subject", "auth_is_authenticated", "auth_expires",
		"auth_required", "auth_open_tokens", "auth_issue", "int_from_str", "emit",
		"store_list", "store_meta", "store_detail", "store_latest", "store_data_dir",
		"registry_data_dir", "registry_access", "config_get", "clock_year",
		"registry_title", "registry_version", "registry_site_url", "registry_url", "registry_token_days",
		"registry_token_max_days", "registry_open_tokens", "registry_max_upload_mb", "escape_html",
	} {
		isExternalBuiltinNames[name] = true
	}
}

// irModuleUsesProtocolTypes reports whether a module refers to Request or
// Response anywhere. Such a module needs the definitions even without routes,
// which is the case for a test that exercises handler-shaped code.
func irModuleUsesProtocolTypes(m ir.ModuleIR) bool {
	mentions := func(name string) bool {
		for _, structure := range m.Structs {
			for _, field := range structure.Fields {
				if field.Type.Name == name {
					return true
				}
			}
		}
		for _, fn := range m.Funcs {
			if fn.ReturnType.Name == name {
				return true
			}
			for _, param := range fn.Params {
				if param.Type.Name == name {
					return true
				}
			}
		}
		return false
	}
	return mentions(RequestTypeName) || mentions(ResponseTypeName)
}

// isUnitOrVoid reports whether a type is the one that means "nothing", which a
// pointer to it renders as the opaque handle C calls void*.
func isUnitOrVoid(t ir.Type) bool {
	if t.Kind == ir.TypeVoid {
		return true
	}
	// A type that names Unit means Unit whatever kind it carries: `Unit` is
	// synthesised on use rather than registered, so a pointer to it can arrive
	// before anything has declared it, and a handle would then be a pointer to
	// a pointer.
	return t.Name == "Unit" || t.Name == "RhoUnit"
}

// isGenericName reports whether the declaration an unqualified call resolves to
// is a generic one.
//
// A name may be declared twice: a generic one that needs its type arguments
// deduced and a concrete one that takes none. The concrete declaration is what an
// unqualified call means, so this is asked before the generic path is taken — a
// call that names no types is not a call that wants them deduced, it is a call to
// the declaration that has none.
func (tc *TypeChecker) isGenericName(name string) bool {
	entry, declared := tc.funcs[name]
	if !declared {
		// Nothing concrete was declared, so the generic declaration is the one
		// the call names.
		return true
	}
	return len(entry.genericParams) > 0
}

// isExternName reports whether a name is one this project declared as an extern.
//
// The set is filled as declarations are seen, and read when a call cannot be
// matched to one. A name that is a promise a port fulfils is checked against its
// arguments rather than refused, so a module that calls a port's function
// without importing the module that declared it still typechecks — the
// declaration travels with the generated program.
func (tc *TypeChecker) isExternName(name string) bool {
	return tc.externNames[name]
}

// cTypeFor spells a resolved type the way C spells it, for the casts the
// lowering writes.
//
// It resolves rather than spell from a written name because a cast target is
// often a pointer or an alias, neither of which is a name C knows. A cast to
// `Ptr[T]` written from the name would be a C expression naming a function that
// does not exist; spelled from the type it is a cast.
func (tc *TypeChecker) cTypeFor(t ir.Type) string {
	// The alias is followed once: `resolveType` has already depth-bounded the
	// chain, so a name that resolves to itself cannot send this into a loop
	// here. That matters because an alias is a claim a manifest can make
	// wrongly, and `type A = A` is a document nobody has to compile to write.
	if name, ok := tc.aliases[t.Name]; ok {
		if resolved, found := tc.resolveType(name); found && resolved.Name != t.Name {
			return tc.cTypeFor(resolved)
		}
	}
	switch t.Kind {
	case ir.TypeVoid:
		return "void"
	case ir.TypeBool:
		return "bool"
	case ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeI64:
		return cTypeForName(t.Name)
	case ir.TypeF64:
		return "double"
	case ir.TypeString:
		return "const char*"
	case ir.TypePtr:
		if len(t.Args) == 0 || isUnitOrVoid(t.Args[0]) {
			return "void*"
		}
		return tc.cTypeFor(t.Args[0]) + "*"
	case ir.TypeStruct:
		if t.Name != "" {
			return mangleTypeName(t.Name)
		}
	}
	return "void*"
}

// isTypeName reports whether a name is a type rather than a function, which is
// what tells a conversion from a call.
//
// The two are written the same way in Rho — `Name(value)` — and only the tables
// tell them apart. A call to a function mangles to a function, and a call to a
// type is a conversion, so mistaking one for the other produces a program that
// calls a function which was never written.
func (tc *TypeChecker) isTypeName(name string) bool {
	if _, alias := tc.aliases[name]; alias {
		return true
	}
	if _, generic := tc.genericStructs[name]; generic {
		return true
	}
	// A pointer is spelled `Ptr[T]` and is never a function, so a name that
	// begins like one and carries a type argument is a type.
	if strings.HasPrefix(name, "Ptr[") {
		return true
	}
	if _, known := tc.types[name]; known {
		return true
	}
	// A bare primitive or a single-letter type parameter.
	if isKnownTypeName(name) {
		return true
	}
	if len(name) == 1 && unicodeUpper(name[0]) {
		return true
	}
	return false
}

// isKnownTypeName reports whether a name is one of the language's own types.
func isKnownTypeName(name string) bool {
	switch name {
	case "void", "bool", "u8", "u16", "u32", "u64", "i32", "i64", "f64", "string",
		"Unit", "RhoUnit", "Result", "Option", "Slice", "Ptr", "Array", "Map", "String":
		return true
	}
	return false
}

// followAlias resolves a chain of aliases to the type at the end of it, or
// reports that the name is not one.
//
// The depth is bounded because an alias is a claim rather than a compiled
// declaration, and a cycle is a claim a manifest can make by accident. A bound
// turns a document that loops into a name nothing resolves, which is a message
// about the manifest rather than a compiler that never returns.
func (tc *TypeChecker) followAlias(name string, depth int) (ir.Type, bool) {
	if depth > 16 {
		return ir.Type{}, false
	}
	target, ok := tc.aliases[name]
	if !ok {
		return ir.Type{}, false
	}
	// `type A = A` names itself, and resolving it would be resolving it for
	// ever.
	if target == name {
		return ir.Type{}, false
	}
	return tc.resolveTypeBounded(target, depth+1)
}

// resolveTypeBounded is resolveType with the alias chain's depth carried, so a
// cycle is seen rather than followed.
func (tc *TypeChecker) resolveTypeBounded(name string, depth int) (ir.Type, bool) {
	if resolved, found := tc.followAlias(name, depth); found {
		return resolved, true
	}
	if typ, ok := tc.types[name]; ok {
		return typ, true
	}
	base := name
	element := ""
	if i := strings.IndexByte(base, '['); i >= 0 {
		element = strings.TrimSuffix(base[i+1:], "]")
		base = base[:i]
	}
	if base == "Ptr" {
		pointer := ir.Type{Kind: ir.TypePtr, Name: name}
		if element != "" {
			pointer.Args = []ir.Type{tc.typeFor(element)}
		}
		return pointer, true
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

// checkStatement reads one statement and reports whether it always returns.
//
// It is a function of its own so that checkStatements can attribute whatever it
// reports to the statement it came from, in one place rather than at each of
// the many places a statement can go wrong.
func (tc *TypeChecker) checkStatement(statement ast.Stmt, scope map[string]ir.Type, result ir.Type, inLoop bool) (bool, error) {
	var statementTerminates bool
	switch stmt := statement.(type) {
	case ast.LetStmt:
		if _, exists := scope[stmt.Name]; exists {
			return false, fmt.Errorf("duplicate local %q", stmt.Name)
		}
		if stmt.Type.Name == "" && stmt.Type.String() == "" {
			// A type-less binding takes its type from the value.
			if stmt.Value == nil {
				return false, fmt.Errorf("cannot infer a type for %q without a value", stmt.Name)
			}
			inferred, err := tc.exprType(stmt.Value, scope)
			if err != nil {
				return false, err
			}
			scope[stmt.Name] = inferred
			break
		}
		declared := tc.declaredType(stmt.Type.String())
		if stmt.Value != nil {
			// The declared type is passed down as the expected type, so a
			// generic literal can be deduced from it rather than guessed.
			actual, err := tc.exprTypeExpected(stmt.Value, scope, tc.expectedHint(stmt.Type.String(), declared))
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
			// The declared return type is the deduction hint, so
			// `return Ok(7)` can supply a type the argument cannot.
			actual, err := tc.exprTypeExpected(stmt.Value, scope, result)
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

	return statementTerminates, nil
}
