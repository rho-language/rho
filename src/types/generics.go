//
// @file generics.go
// @brief Generic instantiation for Rho.
//
// Generic declarations are templates. This file monomorphises them: every
// concrete application seen in a program is expanded into its own specialised
// struct and function, with type parameters substituted and the name mangled
// so the C and Go backends can emit one definition per instantiation.
//
// For example, with `struct Result[T, E]` and `fn Ok[T, E](v: T) Result[T, E]`:
//
//	Ok[u32, u32](7)  ->  fn Ok_u32_u32(v u32) Result_u32_u32
//	Result[T, E]      ->  struct Result_u32_u32 { bool is_ok; u32 ok; u32 err }
//
// A type parameter nested inside another application is substituted too, so
// Result[Option[u32], E] expands correctly.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"errors"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Instantiations                                                            */
/* -------------------------------------------------------------------------- */

// instantiation is one concrete application of a generic declaration.
type instantiation struct {
	// Name is the generic declaration's name, for example "Ok".
	Name string
	// TypeArgs are the concrete type arguments, in declaration order.
	TypeArgs []ir.Type
	// IsStruct distinguishes a struct application from a function call.
	IsStruct bool
}

// key renders an instantiation as a map key, so duplicates collapse.
func (in instantiation) key() string {
	parts := make([]string, len(in.TypeArgs))
	for i, arg := range in.TypeArgs {
		parts[i] = arg.Name
	}
	kind := "fn"
	if in.IsStruct {
		kind = "struct"
	}
	return kind + ":" + in.Name + "[" + strings.Join(parts, ",") + "]"
}

/* -------------------------------------------------------------------------- */
/*  Mangling                                                                  */
/* -------------------------------------------------------------------------- */

// declaredType resolves a type name written in a declaration. A generic
// application is recorded for expansion and resolved to the mangled name its
// specialisation is emitted under, so parameter, return and field types all
// name the same concrete struct.
func (tc *TypeChecker) declaredType(name string) ir.Type {
	return tc.declaredTypeWith(name, tc.currentTypeArgs)
}

// declaredTypeWith is declaredType with an explicit substitution, used while
// emitting a specialisation where the parameters are known.
func (tc *TypeChecker) declaredTypeWith(name string, subs map[string]ir.Type) ir.Type {
	open := strings.IndexByte(name, '[')
	if open < 0 {
		return tc.substituteType(tc.typeFor(name), subs)
	}
	base := name[:open]
	// A pointer is not a generic struct that happens to be called Ptr. It is the
	// pointer type, and it has to be resolved as one here as well as on the path
	// that reads a type used in an expression, or the same `Ptr[u8]` becomes two
	// different things depending on where it was written.
	//
	// Mangling it produced `Ptr_u8` in a parameter and return position, which a
	// backend then emitted as a struct — so an extern declared to take a C string
	// pointer declared a function taking a struct, and no call of it could
	// compile however the argument was written.
	if base == "Ptr" {
		return tc.substituteType(tc.typeFor(name), subs)
	}
	if _, ok := tc.genericStructs[base]; !ok {
		return tc.substituteType(tc.typeFor(name), subs)
	}

	decl := tc.genericStructs[base]
	args := splitTypeArgs(strings.TrimSuffix(name[open+1:], "]"))
	resolved := make([]ir.Type, 0, len(args))
	for _, arg := range args {
		typ, ok := tc.resolveType(arg)
		if !ok {
			typ = ir.Type{Kind: ir.TypeStruct, Name: arg}
		}
		resolved = append(resolved, tc.substituteType(typ, subs))
	}
	// A partial application is completed with the declared defaults, so it
	// mangles to the same name as the deduced one.
	resolved = tc.expandWithDefaults(decl.TypeParams, decl.TypeParamDefaults, resolved)
	if resolved == nil {
		return tc.substituteType(tc.typeFor(name), subs)
	}
	tc.recordInstantiation(instantiation{Name: base, TypeArgs: resolved, IsStruct: true})
	mangled := mangleGeneric(base, resolved)
	tc.registerSpecialisationFields(base, resolved)
	return ir.Type{Kind: ir.TypeStruct, Name: mangled}
}

// registerSpecialisationFields makes a specialisation's fields reachable under
// its mangled name, so a local declared as `let b: Box[u32] = ...` supports
// member access while being checked.
//
// The substitution is built from the struct's own type parameters and the
// concrete arguments, because the caller's substitution is empty for an
// application written directly.
func (tc *TypeChecker) registerSpecialisationFields(base string, args []ir.Type) {
	decl, ok := tc.genericStructs[base]
	if !ok {
		return
	}
	mangled := mangleGeneric(base, args)
	if _, done := tc.fields[mangled]; done {
		return
	}
	subs := tc.buildSubstitutionFromTypes(decl.TypeParams, decl.TypeParamDefaults, args)
	fields := make(map[string]ir.Type, len(decl.Fields))
	for _, field := range decl.Fields {
		fields[field.Name] = tc.declaredTypeWith(field.Type, subs)
	}
	tc.fields[mangled] = fields
	tc.types[mangled] = ir.Type{Kind: ir.TypeStruct, Name: mangled}
}

// cTypeForTypeName returns the C spelling of a written type name, mangling a
// generic application to the name its specialisation is emitted under.
func (tc *TypeChecker) cTypeForTypeName(name string) string {
	open := strings.IndexByte(name, '[')
	if open < 0 {
		return cTypeForName(name)
	}
	base := name[:open]
	// A pointer is spelled as a C pointer, by `cTypeForName` below, which knows
	// to do it. Mangling it first wrote `Ptr_u8`, which is a name a C compiler
	// has never heard of, so a local declared to hold what a foreign function
	// returned could not be declared at all.
	if base == "Ptr" {
		return cTypeForName(name)
	}
	if _, ok := tc.genericStructs[base]; !ok {
		return cTypeForName(name)
	}
	decl := tc.genericStructs[base]
	args := splitTypeArgs(strings.TrimSuffix(name[open+1:], "]"))
	resolved := make([]ir.Type, 0, len(args))
	for _, arg := range args {
		typ, ok := tc.resolveType(arg)
		if !ok {
			typ = ir.Type{Kind: ir.TypeStruct, Name: arg}
		}
		resolved = append(resolved, tc.substituteType(typ, tc.currentTypeArgs))
	}
	resolved = tc.expandWithDefaults(decl.TypeParams, decl.TypeParamDefaults, resolved)
	if resolved == nil {
		return cTypeForName(name)
	}
	tc.recordInstantiation(instantiation{Name: base, TypeArgs: resolved, IsStruct: true})
	return mangleGeneric(base, resolved)
}

/* -------------------------------------------------------------------------- */
/*  Inference                                                                  */
/* -------------------------------------------------------------------------- */

// typePair is a parameter type paired with the type supplied for it.
type typePair struct {
	param  ir.Type
	actual ir.Type
}

// unify binds type parameters by matching a declared type against a supplied
// one. It returns false when the two cannot be reconciled.
//
// A declared type that names a parameter binds it directly. Two applications of
// the same generic are matched argument by argument, so a parameter nested
// inside another generic is still found.
func unify(param, actual ir.Type, params []string, subs map[string]ir.Type) bool {
	if param.Kind == ir.TypeInvalid || actual.Kind == ir.TypeInvalid {
		return true
	}

	for _, candidate := range params {
		if param.Kind == ir.TypeStruct && param.Name == candidate {
			if bound, ok := subs[candidate]; ok {
				// Already bound: the two uses must agree.
				return bound.Kind == actual.Kind
			}
			subs[candidate] = actual
			return true
		}
	}

	// Match nested applications of the same generic pairwise.
	paramBase, paramArgs, paramGeneric := splitApplication(param.Name)
	actualBase, actualArgs, actualGeneric := splitApplication(actual.Name)
	if paramGeneric != actualGeneric {
		// One side is applied and the other is not, so they cannot agree.
		return false
	}
	if paramGeneric {
		if paramBase != actualBase {
			return false
		}
		// The supplied type may be a partial application that omits defaulted
		// parameters, so match the prefix both sides share.
		limit := len(paramArgs)
		if len(actualArgs) < limit {
			limit = len(actualArgs)
		}
		for i := 0; i < limit; i++ {
			if !unify(paramArgs[i], actualArgs[i], params, subs) {
				return false
			}
		}
		return true
	}

	return assignable(param, actual)
}

// splitApplication separates "Result[u32, E]" into its base name and argument
// types. The third result reports whether the name was an application at all.
func splitApplication(name string) (string, []ir.Type, bool) {
	open := strings.IndexByte(name, '[')
	if open < 0 || !strings.HasSuffix(name, "]") {
		return name, nil, false
	}
	base := name[:open]
	parts := splitTypeArgs(name[open+1 : len(name)-1])
	args := make([]ir.Type, 0, len(parts))
	for _, part := range parts {
		typ, err := resolveTypeName(part)
		if err != nil {
			args = append(args, ir.Type{Kind: ir.TypeStruct, Name: part})
			continue
		}
		args = append(args, typ)
	}
	return base, args, true
}

// resolveTypeName resolves a written type name using the package-level table of
// primitives plus the single-letter parameter convention the typechecker uses.
func resolveTypeName(name string) (ir.Type, error) {
	switch name {
	case "void":
		return ir.Type{Kind: ir.TypeVoid, Name: "void"}, nil
	case "bool":
		return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
	case "u8":
		return ir.Type{Kind: ir.TypeU8, Name: "u8"}, nil
	case "u16":
		return ir.Type{Kind: ir.TypeU16, Name: "u16"}, nil
	case "u32":
		return ir.Type{Kind: ir.TypeU32, Name: "u32"}, nil
	case "u64":
		return ir.Type{Kind: ir.TypeU64, Name: "u64"}, nil
	case "i32":
		return ir.Type{Kind: ir.TypeI32, Name: "i32"}, nil
	case "i64":
		return ir.Type{Kind: ir.TypeI64, Name: "i64"}, nil
	case "f64":
		return ir.Type{Kind: ir.TypeF64, Name: "f64"}, nil
	case "f32":
		return ir.Type{Kind: ir.TypeF32, Name: "f32"}, nil
	case "string":
		return ir.Type{Kind: ir.TypeString, Name: "string"}, nil
	}
	return ir.Type{}, errNotAType
}

// applicationFor returns the concrete type arguments of a generic application
// written without them, deduced from the surrounding context.
//
// Both the checking and the lowering pass call this, so the deduced arguments
// and therefore the emitted specialisation name are guaranteed to agree.
func (tc *TypeChecker) applicationFor(expr ast.Expr, scope map[string]ir.Type, expected ir.Type) []ir.Type {
	switch node := expr.(type) {
	case ast.CallExpr:
		identifier, ok := node.Callee.(ast.IdentifierExpr)
		if !ok {
			return nil
		}
		decl, generic := tc.genericFuncs[identifier.Name]
		if !generic || len(node.TypeArgs) > 0 {
			return nil
		}
		return tc.deduceCallArgs(decl, node.Args, scope, expected)

	case ast.StructLiteralExpr:
		if len(node.Type.Args) > 0 {
			return nil
		}
		decl, generic := tc.genericStructs[node.Type.Name]
		if !generic {
			return nil
		}
		return tc.deduceStructArgs(decl, node.Values, scope, expected)
	}
	return nil
}

// deduceCallArgs infers a generic function's type arguments from its arguments.
// An expected result type is consulted first, which is what lets
// `let x: string = make(1)` pin the result even when an argument is ambiguous.
func (tc *TypeChecker) deduceCallArgs(decl ast.FuncDecl, args []ast.Expr, scope map[string]ir.Type, expected ir.Type) []ir.Type {
	subs := map[string]ir.Type{}

	// The expected type seeds the result, and any parameter it mentions.
	if expected.Kind != ir.TypeInvalid {
		tc.unifyTypeArgs(decl.TypeParams, tc.typeFor(decl.ReturnType), expected, subs)
	}

	limit := len(args)
	if len(decl.Params) < limit {
		limit = len(decl.Params)
	}
	for i := 0; i < limit; i++ {
		actual, err := tc.exprType(args[i], scope)
		if err != nil {
			return nil
		}
		tc.unifyTypeArgs(decl.TypeParams, tc.typeFor(decl.Params[i].Type), actual, subs)
	}

	return tc.orderedArgs(decl.TypeParams, decl.TypeParamDefaults, subs)
}

// deduceStructArgs infers a generic struct's type arguments from its field
// values, preferring an expected type when one is known.
func (tc *TypeChecker) deduceStructArgs(decl ast.StructDecl, values []ast.Expr, scope map[string]ir.Type, expected ir.Type) []ir.Type {
	subs := map[string]ir.Type{}

	if expected.Kind != ir.TypeInvalid {
		tc.unifyTypeArgs(decl.TypeParams, ir.Type{Kind: ir.TypeStruct, Name: decl.Name}, expected, subs)
	}

	limit := len(values)
	if len(decl.Fields) < limit {
		limit = len(decl.Fields)
	}
	for i := 0; i < limit; i++ {
		actual, err := tc.exprType(values[i], scope)
		if err != nil {
			return nil
		}
		tc.unifyTypeArgs(decl.TypeParams, tc.typeFor(decl.Fields[i].Type), actual, subs)
	}

	return tc.orderedArgs(decl.TypeParams, decl.TypeParamDefaults, subs)
}

// unifyTypeArgs matches a declared type against a supplied one, binding the
// supplied type's own parameters where the declared type names a parameter.
func (tc *TypeChecker) unifyTypeArgs(typeParams []string, declared, actual ir.Type, subs map[string]ir.Type) {
	if declared.Kind == ir.TypeInvalid || actual.Kind == ir.TypeInvalid {
		return
	}
	// The declared type may itself be an application of this generic, as in
	// `Result[T, E]`, so the supplied type's arguments are matched by position.
	declBase, declArgs, declGeneric := splitApplication(declared.Name)
	actualBase, actualArgs, actualGeneric := splitApplication(actual.Name)

	// An optional pointer is matched by what it is rather than by what it is
	// called. `Option[Ptr[T]]` and the mangled `Option_Ptr_X` are the same type,
	// and a generic function taking `Option[T]` has to be able to deduce its
	// argument from one — otherwise `is_some(handle)` needs the caller to spell
	// out the type the value already has.
	if declBase == "Option" && IsOptionalPointer(actual.Name) && len(declArgs) == 1 {
		tc.unifyTypeArgs(typeParams, declArgs[0],
			ir.Type{Kind: ir.TypePtr, Name: actual.Name[len("Option_Ptr_"):],
				Args: []ir.Type{{Kind: ir.TypeStruct, Name: actual.Name[len("Option_Ptr_"):]}}},
			subs)
		return
	}

	if declGeneric && actualGeneric && declBase == actualBase {
		// A partial application omits defaulted parameters, so match the
		// prefix both sides share.
		limit := len(declArgs)
		if len(actualArgs) < limit {
			limit = len(actualArgs)
		}
		for i := 0; i < limit; i++ {
			tc.unifyTypeArgs(typeParams, declArgs[i], actualArgs[i], subs)
		}
		return
	}
	if declared.Kind == ir.TypeStruct {
		unify(declared, actual, typeParams, subs)
	}
}

// mangleTypeName turns a possibly generic type name into an identifier that is
// valid in both C and Go. "Result[u32, u32]" becomes "Result_u32_u32".
func mangleTypeName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			// '[', ']', ',', ' ', '*' and friends all collapse to one
			// underscore so "A[u32, u32]" does not become "A_u32__u32_".
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "anon"
	}
	return out
}

// mangleGeneric builds the specialised name of a generic declaration.
func mangleGeneric(name string, args []ir.Type) string {
	if len(args) == 0 {
		return name
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, mangleTypeName(arg.Name))
	}
	return name + "_" + strings.Join(parts, "_")
}

/* -------------------------------------------------------------------------- */
/*  Substitution                                                              */
/* -------------------------------------------------------------------------- */

// substituteType replaces type parameters inside a type. A type parameter is
// represented as a struct type whose name is the parameter, so a direct hit is
// a map lookup. A nested application is rebuilt from its parts so a parameter
// buried inside it is still replaced.
func (tc *TypeChecker) substituteType(typ ir.Type, subs map[string]ir.Type) ir.Type {
	if len(subs) == 0 || typ.Kind != ir.TypeStruct {
		return typ
	}
	if replacement, ok := subs[typ.Name]; ok {
		// The replacement may itself be an application that was carried as a
		// name rather than as a type. A pointer is the case that matters: a
		// generic function returning its argument, specialised at a pointer,
		// would otherwise return a name the backend has never heard of, and every
		// call of it would be declared against a type that does not exist.
		return tc.resolveApplied(replacement)
	}
	open := strings.IndexByte(typ.Name, '[')
	if open < 0 {
		return typ
	}

	base := typ.Name[:open]
	inner := strings.TrimSuffix(typ.Name[open+1:], "]")
	parts := splitTypeArgs(inner)

	rebuilt := make([]string, 0, len(parts))
	for _, part := range parts {
		resolved, ok := tc.resolveType(part)
		if !ok {
			rebuilt = append(rebuilt, part)
			continue
		}
		rebuilt = append(rebuilt, tc.substituteType(resolved, subs).Name)
	}
	typ.Name = base + "[" + strings.Join(rebuilt, ", ") + "]"
	return typ
}

// splitTypeArgs splits "u32, Option[u32]" on top-level commas.
func splitTypeArgs(text string) []string {
	out := []string{}
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// buildSubstitution pairs a declaration's type parameters with the concrete
// arguments supplied at a use site.
func (tc *TypeChecker) buildSubstitution(typeParams []string, typeDefaults map[string]string, args []ast.TypeExpr) map[string]ir.Type {
	if len(typeParams) == 0 {
		return nil
	}
	subs := map[string]ir.Type{}
	for i, param := range typeParams {
		if i >= len(args) {
			break
		}
		resolved, ok := tc.resolveType(args[i].String())
		if !ok {
			continue
		}
		subs[param] = resolved
	}
	// A partial application such as `Ok[u32](7)` leaves the rest to defaults.
	tc.applyDefaults(typeParams, typeDefaults, subs)
	if len(subs) == 0 {
		return nil
	}
	return subs
}

/* -------------------------------------------------------------------------- */
/*  Zero Values                                                               */
/* -------------------------------------------------------------------------- */

// zeroValueFor returns the zero value of a type, backing the `default(T)`
// expression that Result and Option need.
//
// The spelling is the one both backends read, so it has to be valid in each.
// `{0}` is how C zeroes a struct's first member and means nothing in Go, where a
// struct literal needs every field written out; `{}` is valid in both and is
// what "no value" actually is. A struct with fields is therefore zeroed with
// braces rather than with a single zero.
func (tc *TypeChecker) zeroValueFor(name string) string {
	base := name
	if i := strings.IndexByte(base, '['); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "bool":
		return "false"
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64":
		return "0"
	case "string":
		// An empty string rather than a null pointer: it is the zero value in
		// C and Go alike, and Go has no null string.
		return `""`
	default:
		// A struct, possibly a mangled generic specialisation, uses the same
		// spelling as its declaration. An empty braces is the zero value in
		// both languages: C guarantees the members are zeroed, and Go fills
		// them with their own zero values.
		return "(" + mangleTypeName(name) + "){}"
	}
}

// structHasFields reports whether a struct type has any fields. The name may
// be a base name, a mangled specialisation, or a written application.
func (tc *TypeChecker) structHasFields(name string) bool {
	if fields, ok := tc.fields[name]; ok {
		return len(fields) > 0
	}
	if fields, ok := tc.fields[mangleTypeName(name)]; ok {
		return len(fields) > 0
	}
	if open := strings.IndexByte(name, '['); open >= 0 {
		if fields, ok := tc.fields[name[:open]]; ok {
			return len(fields) > 0
		}
	}
	return false
}

// isDefaultCall reports whether an expression is `default(T)`.
func isDefaultCall(expr ast.Expr) (string, bool) {
	call, ok := expr.(ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return "", false
	}
	callee, ok := call.Callee.(ast.IdentifierExpr)
	if !ok || callee.Name != "default" {
		return "", false
	}
	arg, ok := call.Args[0].(ast.IdentifierExpr)
	if !ok {
		return "", false
	}
	return arg.Name, true
}

/* -------------------------------------------------------------------------- */
/*  Specialisation                                                            */
/* -------------------------------------------------------------------------- */

// structToSpecialisation emits a generic struct with its type parameters
// substituted and its name mangled per instantiation.
func (tc *TypeChecker) structToSpecialisation(s ast.StructDecl, subs map[string]ir.Type, args []ir.Type) ir.StructIR {
	fields := make([]ir.FieldIR, len(s.Fields))
	for i, f := range s.Fields {
		fieldType := tc.typeFromExpr(f.TypeExpr, s.TypeParams)
		if f.TypeExpr.Name == "" {
			fieldType = tc.typeFor(f.Type)
		} else if fieldType.Kind == ir.TypePtr {
			// A pointer is a kind with an element, not a name, so substituting
			// it means substituting the element. Resolving the name instead —
			// which is what a struct field goes through normally — loses both the
			// kind and the element, so `ptr: Ptr[T]` became a field of a type
			// called `Ptr` that no backend can name.
			fieldType = ir.Type{Kind: ir.TypePtr, Name: fieldType.Name}
			for _, argument := range fieldType.Args {
				fieldType.Args = append(fieldType.Args, tc.substituteType(argument, subs))
			}
			fields[i] = ir.FieldIR{Name: f.Name, Type: fieldType}
			continue
		}
		fieldType = tc.declaredTypeWith(fieldType.Name, subs)
		fields[i] = ir.FieldIR{Name: f.Name, Type: fieldType}
	}
	name := mangleGeneric(s.Name, args)
	return ir.StructIR{
		Name:   name,
		CName:  tc.qualifiedName(s.Owner, name),
		Fields: fields,
	}
}

// funcToSpecialisation emits a generic function with its type parameters
// substituted and its name mangled per instantiation.
//
// currentTypeArgs is set while lowering the body so that nested expressions
// such as `default(T)` and `Option[T]{...}` resolve against the concrete
// arguments rather than the parameters.
func (tc *TypeChecker) funcToSpecialisation(fn ast.FuncDecl, subs map[string]ir.Type, args []ir.Type) ir.FuncIR {
	params := make([]ir.ParamIR, len(fn.Params))
	scope := make(map[string]ir.Type, len(params))
	for i, p := range fn.Params {
		paramType := tc.typeFromExpr(p.TypeExpr, fn.TypeParams)
		if p.TypeExpr.Name == "" {
			paramType = tc.typeFor(p.Type)
		}
		paramType = tc.declaredTypeWith(paramType.Name, subs)
		params[i] = ir.ParamIR{Name: p.Name, Type: paramType}
		scope[p.Name] = paramType
	}

	returnType := tc.declaredTypeWith(fn.ReturnType, subs)

	// The body is lowered with the substitution in place.
	previous := tc.currentTypeArgs
	tc.currentTypeArgs = subs
	tc.currentReturnHint = tc.typeFor(fn.ReturnType)
	body := tc.lowerStatements(fn.Body, scope)
	tc.currentTypeArgs = previous

	name := mangleGeneric(fn.Name, args)
	return ir.FuncIR{
		Name:       name,
		CName:      tc.qualifiedName(fn.Owner, name),
		Params:     params,
		ReturnType: returnType,
		Body:       body,
		Extern:     fn.Extern,
	}
}

// qualifiedName mirrors the C name qualification used for non-generic
// declarations, so a specialisation from an imported module keeps its prefix.
func (tc *TypeChecker) qualifiedName(owner, name string) string {
	if owner == "" {
		return name
	}
	return strings.NewReplacer(".", "_", "/", "_", "-", "_").Replace(owner) + "_" + name
}

/* -------------------------------------------------------------------------- */
/*  Monomorphisation Driver                                                   */
/* -------------------------------------------------------------------------- */

// monomorphise expands every generic application reachable from a module.
//
// It is driven by the worklist the lowering pass fills in, so an application
// discovered while lowering a specialisation is itself expanded.
func (tc *TypeChecker) monomorphise(m ast.Module, out *ir.ModuleIR) {
	if len(tc.genericStructs) == 0 && len(tc.genericFuncs) == 0 {
		return
	}

	// Collect the applications the non-generic code asked for.
	tc.seedInstantiations(m)

	for len(tc.instantiationQueue) > 0 {
		next := tc.instantiationQueue[0]
		tc.instantiationQueue = tc.instantiationQueue[1:]
		if tc.emitted[next.key()] {
			continue
		}
		tc.emitted[next.key()] = true

		if next.IsStruct {
			decl, ok := tc.genericStructs[next.Name]
			if !ok {
				continue
			}
			subs := tc.buildSubstitutionFromTypes(decl.TypeParams, decl.TypeParamDefaults, next.TypeArgs)
			tc.monomorphicStructs[next.key()] = tc.structToSpecialisation(decl, subs, next.TypeArgs)
			continue
		}

		decl, ok := tc.genericFuncs[next.Name]
		if !ok {
			continue
		}
		subs := tc.buildSubstitutionFromTypes(decl.TypeParams, decl.TypeParamDefaults, next.TypeArgs)
		tc.monomorphicFuncs[next.key()] = tc.funcToSpecialisation(decl, subs, next.TypeArgs)
	}

	// Specialisations are appended after the ordinary declarations so the
	// backends see a stable order.
	out.Structs = append(out.Structs, tc.monomorphicStructsInOrder()...)
	out.Funcs = append(out.Funcs, tc.monomorphicFuncsInOrder()...)
}

// seedInstantiations records every generic application reachable from the
// non-generic part of the module.
func (tc *TypeChecker) seedInstantiations(m ast.Module) {
	for _, decl := range m.Decls {
		switch decl := decl.(type) {
		case ast.FuncDecl:
			if len(decl.TypeParams) > 0 {
				// A template is expanded through the applications that name it,
				// so its own signature is not an application of itself.
				continue
			}
			// A function's signature is where a generated function keeps its
			// types: a query takes a `Slice[CustomerRow]` and returns a
			// `Result[u32, SqlError]`, and both applications appear only here and
			// never in a body. Seeding from bodies alone left the one shape the
			// language is most often written in unexpanded.
			tc.scanTypeApplication(decl.ReturnType)
			for _, param := range decl.Params {
				tc.scanTypeApplication(param.Type)
			}
			tc.scanExprs(decl.Body)
		case ast.StructDecl:
			if len(decl.TypeParams) > 0 {
				continue
			}
			for _, field := range decl.Fields {
				tc.scanTypeApplication(field.Type)
			}
		}
	}
}

// buildSubstitutionFromTypes pairs type parameters with already-resolved types.
// buildSubstitutionFromTypes pairs type parameters with already-resolved type
// arguments, filling anything the application left out from the defaults.
func (tc *TypeChecker) buildSubstitutionFromTypes(typeParams []string, defaults map[string]string, args []ir.Type) map[string]ir.Type {
	if len(typeParams) == 0 {
		return nil
	}
	subs := map[string]ir.Type{}
	for i, param := range typeParams {
		if i >= len(args) {
			break
		}
		subs[param] = args[i]
	}
	tc.applyDefaults(typeParams, defaults, subs)
	if len(subs) == 0 {
		return nil
	}
	return subs
}

// recordInstantiation queues a generic application for expansion, collapsing
// duplicates so a type used a hundred times is emitted once.
func (tc *TypeChecker) recordInstantiation(in instantiation) {
	key := in.key()
	if tc.seenInstantiation[key] {
		return
	}
	tc.seenInstantiation[key] = true
	tc.instantiationQueue = append(tc.instantiationQueue, in)
}

// monomorphicStructsInOrder returns collected struct specialisations in a
// deterministic order.
func (tc *TypeChecker) monomorphicStructsInOrder() []ir.StructIR {
	keys := make([]string, 0, len(tc.monomorphicStructs))
	for key := range tc.monomorphicStructs {
		keys = append(keys, key)
	}
	sortStrings(keys)
	out := make([]ir.StructIR, 0, len(keys))
	for _, key := range keys {
		out = append(out, tc.monomorphicStructs[key])
	}
	return out
}

func (tc *TypeChecker) monomorphicFuncsInOrder() []ir.FuncIR {
	keys := make([]string, 0, len(tc.monomorphicFuncs))
	for key := range tc.monomorphicFuncs {
		keys = append(keys, key)
	}
	sortStrings(keys)
	out := make([]ir.FuncIR, 0, len(keys))
	for _, key := range keys {
		out = append(out, tc.monomorphicFuncs[key])
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// scanExprs walks statements recording generic applications.
func (tc *TypeChecker) scanExprs(statements []ast.Stmt) {
	for _, statement := range statements {
		tc.scanStmt(statement)
	}
}

func (tc *TypeChecker) scanStmt(statement ast.Stmt) {
	switch statement := statement.(type) {
	case ast.LetStmt:
		tc.scanTypeApplication(statement.Type.String())
		if statement.Value != nil {
			tc.scanExpr(statement.Value)
		}
	case ast.AssignStmt:
		tc.scanExpr(statement.Target)
		tc.scanExpr(statement.Value)
	case ast.ExprStmt:
		tc.scanExpr(statement.Expr)
	case ast.ReturnStmt:
		if statement.Value != nil {
			tc.scanExpr(statement.Value)
		}
	case ast.IfStmt:
		tc.scanExpr(statement.Condition)
		tc.scanExprs(statement.Then)
		tc.scanExprs(statement.Else)
	case ast.WhileStmt:
		tc.scanExpr(statement.Condition)
		tc.scanExprs(statement.Body)
	}
}

func (tc *TypeChecker) scanExpr(expr ast.Expr) {
	switch expr := expr.(type) {
	case ast.CallExpr:
		if len(expr.TypeArgs) > 0 {
			tc.recordInstantiation(instantiation{
				Name:     calleeName(expr.Callee),
				TypeArgs: tc.resolveTypeArgs(expr.TypeArgs),
				IsStruct: false,
			})
		}
		tc.scanExpr(expr.Callee)
		for _, arg := range expr.Args {
			tc.scanExpr(arg)
		}
	case ast.StructLiteralExpr:
		if len(expr.Type.Args) > 0 {
			tc.recordInstantiation(instantiation{
				Name:     expr.Type.Name,
				TypeArgs: tc.resolveTypeArgs(expr.Type.Args),
				IsStruct: true,
			})
		}
		for _, value := range expr.Values {
			tc.scanExpr(value)
		}
	case ast.UnaryExpr:
		tc.scanExpr(expr.Value)
	case ast.BinaryExpr:
		tc.scanExpr(expr.Left)
		tc.scanExpr(expr.Right)
	case ast.MemberExpr:
		tc.scanExpr(expr.Object)
	case ast.IndexExpr:
		tc.scanExpr(expr.Object)
		tc.scanExpr(expr.Index)
	case ast.CastExpr:
		tc.scanExpr(expr.Value)
	case ast.ArrayLiteralExpr:
		for _, value := range expr.Values {
			tc.scanExpr(value)
		}
	case ast.IfExpr:
		tc.scanExpr(expr.Condition)
		tc.scanExpr(expr.Then)
		tc.scanExpr(expr.Else)
	}
}

// scanTypeApplication records `Result[u32, u32]` appearing in a type position.
func (tc *TypeChecker) scanTypeApplication(typeName string) {
	open := strings.IndexByte(typeName, '[')
	if open < 0 {
		return
	}
	base := typeName[:open]
	if _, ok := tc.genericStructs[base]; !ok {
		return
	}
	decl := tc.genericStructs[base]
	args := splitTypeArgs(strings.TrimSuffix(typeName[open+1:], "]"))
	resolved := make([]ir.Type, 0, len(args))
	for _, arg := range args {
		typ, ok := tc.resolveType(arg)
		if !ok {
			return
		}
		resolved = append(resolved, tc.substituteType(typ, tc.currentTypeArgs))
	}
	resolved = tc.expandWithDefaults(decl.TypeParams, decl.TypeParamDefaults, resolved)
	if resolved == nil {
		return
	}
	tc.recordInstantiation(instantiation{Name: base, TypeArgs: resolved, IsStruct: true})
}

func calleeName(expr ast.Expr) string {
	if identifier, ok := expr.(ast.IdentifierExpr); ok {
		return identifier.Name
	}
	return ""
}

func (tc *TypeChecker) resolveTypeArgs(args []ast.TypeExpr) []ir.Type {
	out := make([]ir.Type, 0, len(args))
	for _, arg := range args {
		typ, ok := tc.resolveType(arg.String())
		if !ok {
			typ = ir.Type{Kind: ir.TypeStruct, Name: arg.String()}
		}
		out = append(out, tc.substituteType(typ, tc.currentTypeArgs))
	}
	return out
}

// errNotAType marks a written name that is not a resolvable type.
var errNotAType = errors.New("not a type")

// expectedHint returns the type to use as a deduction hint. A written generic
// application keeps its written name so deduction can match it against the
// declaration's parameters; anything else uses the resolved type.
//
// The resolved type of `let r: Result[u32, u32]` is the mangled
// Result_u32_u32, which no longer reveals its arguments, so the hint has to
// carry the written form.
func (tc *TypeChecker) expectedHint(written string, resolved ir.Type) ir.Type {
	open := strings.IndexByte(written, '[')
	if open < 0 {
		return resolved
	}
	if _, ok := tc.genericStructs[written[:open]]; !ok {
		return resolved
	}
	return ir.Type{Kind: ir.TypeStruct, Name: written}
}

// defaultTypeName is the conventional name for the zero-sized type used as a
// generic parameter default. It plays the role Rust's `()` plays in
// `Result<T, E = ()>`.
const defaultTypeName = "Unit"

// synthesizeDefaultTypes emits a zero-sized struct for any name used as a
// type parameter default that the module does not define itself, so a default
// always has a definition the backends can emit.
func (tc *TypeChecker) synthesizeDefaultTypes(m ast.Module, out *ir.ModuleIR) {
	needed := map[string]bool{}
	for _, decl := range m.Decls {
		var defaults map[string]string
		switch decl := decl.(type) {
		case ast.StructDecl:
			defaults = decl.TypeParamDefaults
		case ast.FuncDecl:
			defaults = decl.TypeParamDefaults
		default:
			continue
		}
		for _, written := range defaults {
			if written == "" {
				continue
			}
			// Only a bare, undefined name needs synthesising; a concrete type
			// the module already declares is used as written.
			if strings.ContainsAny(written, "[],") {
				continue
			}
			if _, declared := tc.types[written]; declared {
				continue
			}
			needed[written] = true
		}
	}

	names := make([]string, 0, len(needed))
	for name := range needed {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		tc.types[name] = ir.Type{Kind: ir.TypeStruct, Name: name}
		tc.fields[name] = map[string]ir.Type{}
		out.Structs = append(out.Structs, ir.StructIR{Name: name, CName: name})
	}
}

// expandWithDefaults completes a partial application by appending the
// defaults for the parameters it omitted, so `Pair[u32]` and the deduced
// `Pair[u32, Unit]` mangle to the same name.
func (tc *TypeChecker) expandWithDefaults(typeParams []string, defaults map[string]string, args []ir.Type) []ir.Type {
	if len(args) >= len(typeParams) {
		return args
	}
	out := make([]ir.Type, len(typeParams))
	copy(out, args)
	for i := len(args); i < len(typeParams); i++ {
		param := typeParams[i]
		written, ok := defaults[param]
		if !ok || written == "" {
			return nil
		}
		resolved, found := tc.resolveType(written)
		if !found {
			resolved = ir.Type{Kind: ir.TypeStruct, Name: written}
		}
		out[i] = resolved
	}
	return out
}

// applyDefaults fills any type parameter left unbound by deduction with the
// default written in the declaration, as in `fn Ok[T, E = Unit]`.
func (tc *TypeChecker) applyDefaults(typeParams []string, defaults map[string]string, subs map[string]ir.Type) {
	for _, param := range typeParams {
		if _, bound := subs[param]; bound {
			continue
		}
		written, ok := defaults[param]
		if !ok || written == "" {
			continue
		}
		resolved, ok := tc.resolveType(written)
		if !ok {
			resolved = ir.Type{Kind: ir.TypeStruct, Name: written}
		}
		subs[param] = resolved
	}
}

// orderedArgs renders a substitution as the argument list in declaration
// order, so a mangled name is stable. Parameters with no binding and no default
// make the deduction fail, returning nil.
func (tc *TypeChecker) orderedArgs(typeParams []string, defaults map[string]string, subs map[string]ir.Type) []ir.Type {
	tc.applyDefaults(typeParams, defaults, subs)
	if len(subs) == 0 {
		return nil
	}
	out := make([]ir.Type, 0, len(typeParams))
	for _, param := range typeParams {
		bound, ok := subs[param]
		if !ok {
			return nil
		}
		out = append(out, bound)
	}
	return out
}

// undeducedParams lists the type parameters a deduction could not resolve,
// ignoring any that carry a default.
func (tc *TypeChecker) undeducedParams(typeParams []string, defaults map[string]string, subs map[string]ir.Type) []string {
	missing := []string{}
	for _, param := range typeParams {
		if _, bound := subs[param]; bound {
			continue
		}
		if written, ok := defaults[param]; ok && written != "" {
			continue
		}
		missing = append(missing, param)
	}
	return missing
}

// isGenericCallee reports whether a name refers to a generic function, so
// lowering knows to emit a specialisation name even without type arguments.
// isGenericCallee reports whether a call names a generic function.
//
// A name declared both ways — a generic and a concrete declaration of the same
// name — is called as the concrete one. A logger's `error(subject, message)`
// beside the standard library's `error[T, E](result)` is the case this is about:
// an unqualified call with no type arguments is a call to the declaration that
// takes none, and treating it as a generic one asks a call that named no types
// to deduce them, and lowers to a specialisation of the other function.
func isGenericCallee(tc *TypeChecker, name string) bool {
	if _, ok := tc.genericFuncs[name]; !ok {
		return false
	}
	return tc.isGenericName(name)
}

// resolveApplied turns a carried-as-a-name application back into a type, when it
// is one.
//
// A generic application reaches the backends mangled, because that is what it
// has to be called. A pointer does not have to be: it is a primitive with an
// element, and rendering it as a name produced a declaration no compiler knows.
func (tc *TypeChecker) resolveApplied(typ ir.Type) ir.Type {
	if typ.Kind != ir.TypeStruct || !strings.Contains(typ.Name, "[") {
		return typ
	}
	if resolved, ok := tc.resolveType(typ.Name); ok {
		return resolved
	}
	return typ
}
