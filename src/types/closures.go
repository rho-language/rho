//
// @file closures.go
// @brief Function types, functions as values, and closures.
//
// A function type is written `fn(A, B) -> R` (`fn(A)` when it returns nothing).
// A value of one is a named function, written by its name where a value goes,
// or a function literal, written like a declaration without a name:
//
//	let add = fn(x: i32) -> i32 { return x + k }
//
// A literal captures the locals of the enclosing function it names, by value:
// each is copied when the literal is evaluated, and a later change to the
// variable is not seen. A captured name cannot be assigned or have its address
// taken inside the body, so a closure is a function of the values it was made
// with and of its arguments, and nothing else.
//
// Both backends represent a value the same way, through helpers the lowering
// names and each backend defines (ir.FuncSigIR, ir.ModuleIR.Envs):
//
//   - Every function value is a function that takes an environment first and
//     the environment it is called with. A literal is lifted to a function of
//     its own, `rho_lambda_N(void* rho_env, ...)`, whose captures are read out
//     of its environment (`rho_env_N`, a struct of the captured values); a named
//     function is reached through a thunk, `rho_thunk_<name>`, that ignores the
//     environment and calls it.
//   - `rho_fn_N_bind(fn, env)` makes the value and `rho_fn_N_call(f, ...)` calls
//     it, stopping the program with the same words on both backends when the
//     value holds no function (the zero of a function type).
//   - C holds every value as an `RhoFn`, `{ void (*fn)(void); void* env; }`; Go
//     as a `func(A, B) R`, the closure bind makes.
//
// Where the environment lives is the string arena's question (arena.go), and it
// is answered the way it is for a string: `rho_env_N_make` copies the captured
// values into the arena, and a function value is treated everywhere as a value
// that holds a string. So an environment lives exactly as long as a string
// built at the same place would — kept by a `let`, kept by a loop that stores
// it outside itself, returned to a caller — and is given back when the arena is
// released past it. A board's arena is fixed, and a loop that keeps what it
// builds is reported there as it is for strings.
//
// That is sound only because nothing else can keep an environment's contents
// alive: a closure may not capture a pointer, a `[T]` or a `Slice[T]` (or a
// struct holding one), because what it points at may be the frame that made the
// closure, which a returned closure outlives; and it may not capture a fixed
// array, which C cannot copy into its environment by assignment. Each is
// refused with what to write instead: pass the reference as a parameter, or
// wrap the array in a struct.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                     */
/* -------------------------------------------------------------------------- */

// funcTypeOf is the function type of a parameter list and a result.
func funcTypeOf(params []ir.Type, result ir.Type) ir.Type {
	if result.Kind == ir.TypeInvalid {
		result = ir.Type{Kind: ir.TypeVoid, Name: "void"}
	}
	names := make([]string, len(params))
	args := make([]ir.Type, 0, len(params)+1)
	for i, param := range params {
		names[i] = typeName(param)
		args = append(args, param)
	}
	args = append(args, result)
	resultName := "void"
	if result.Kind != ir.TypeVoid && !isUnitOrVoid(result) {
		resultName = typeName(result)
	}
	return ir.Type{Kind: ir.TypeFunc, Name: ast.FuncTypeName(names, resultName), Args: args}
}

// resolveFuncType resolves a function type as written. A component that names
// a type parameter of the generic being checked or specialised is that
// parameter, to be substituted later.
func (tc *TypeChecker) resolveFuncType(name string) (ir.Type, bool) {
	params, result, ok := ast.SplitFuncTypeName(name)
	if !ok {
		return ir.Type{}, false
	}
	component := func(written string) (ir.Type, bool) {
		for _, param := range tc.checkingTypeParams {
			if written == param {
				return ir.Type{Kind: ir.TypeStruct, Name: written}, true
			}
			if containsWord(written, param) {
				typ, found := tc.resolveType(written)
				return typ, found
			}
		}
		if _, isParam := tc.currentTypeArgs[written]; isParam {
			return ir.Type{Kind: ir.TypeStruct, Name: written}, true
		}
		// A generic application is the specialisation it names, recorded so a
		// backend emits it.
		if open := strings.IndexByte(written, '['); open > 0 && !ast.IsFuncTypeName(written) {
			if _, generic := tc.genericStructs[written[:open]]; generic {
				typ := tc.declaredTypeWith(written, nil)
				return typ, typ.Kind != ir.TypeInvalid
			}
		}
		return tc.resolveType(written)
	}
	resolved := make([]ir.Type, 0, len(params))
	for _, param := range params {
		typ, found := component(param)
		if !found {
			return ir.Type{}, false
		}
		resolved = append(resolved, typ)
	}
	resultType := ir.Type{Kind: ir.TypeVoid, Name: "void"}
	if result != "void" {
		typ, found := component(result)
		if !found {
			return ir.Type{}, false
		}
		resultType = typ
	}
	return funcTypeOf(resolved, resultType), true
}

// parenthesisedArray splits `(fn(i32) -> i32)[4]` into the function type and
// the length.
func parenthesisedArray(name string) (string, int, bool) {
	if !strings.HasPrefix(name, "(") || !strings.HasSuffix(name, "]") {
		return "", 0, false
	}
	depth := 0
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				rest := name[i+1:]
				if !strings.HasPrefix(rest, "[") {
					return "", 0, false
				}
				n, err := strconv.Atoi(rest[1 : len(rest)-1])
				if err != nil || n <= 0 {
					return "", 0, false
				}
				return name[1:i], n, true
			}
		}
	}
	return "", 0, false
}

// substituteFunc rebuilds a function type's spelling after its components were
// substituted.
func substituteFunc(typ ir.Type) ir.Type {
	if typ.Kind != ir.TypeFunc || len(typ.Args) == 0 {
		return typ
	}
	return funcTypeOf(typ.FuncParams(), typ.FuncResult())
}

// sameFuncType is whether two function types are one: the same parameters and
// the same result, each exactly — a function taking an i32 is not a function
// taking a u8, because the two are called differently.
func sameFuncType(a, b ir.Type) bool {
	if a.Kind != ir.TypeFunc || b.Kind != ir.TypeFunc || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !sameComponent(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

// sameComponent is whether two types in a function's signature are the same
// type, with a generic application and its mangled name read as one.
func sameComponent(a, b ir.Type) bool {
	if a.Kind == ir.TypeInvalid || b.Kind == ir.TypeInvalid {
		return true
	}
	if a.Kind == ir.TypeFunc || b.Kind == ir.TypeFunc {
		return sameFuncType(a, b)
	}
	if isUnitOrVoid(a) && isUnitOrVoid(b) {
		return true
	}
	if a.Kind != b.Kind || a.Enum != b.Enum || a.ArrayLen != b.ArrayLen {
		return false
	}
	switch a.Kind {
	case ir.TypeStruct, ir.TypePtr, ir.TypeSlice:
		return mangleTypeName(a.Name) == mangleTypeName(b.Name)
	}
	if a.Enum {
		return baseTypeName(a.Name) == baseTypeName(b.Name)
	}
	return true
}

// checkFuncTypeComponents refuses what a function type cannot carry: a fixed
// array, which C passes as a pointer and Go as a copy, so the two would mean
// different things by one signature.
func checkFuncTypeComponents(typ ir.Type) error {
	if typ.Kind != ir.TypeFunc {
		return nil
	}
	for _, component := range typ.Args {
		if component.ArrayLen > 0 || isArrayValue(component) && component.Kind != ir.TypeSlice && component.Kind != ir.TypeFunc {
			return fmt.Errorf(msgFuncTypeArray, typeName(typ), typeName(component), elementName(component))
		}
		if err := checkFuncTypeComponents(component); err != nil {
			return err
		}
	}
	return nil
}

// elementName is the element of a fixed array as written, for a message.
func elementName(typ ir.Type) string {
	if element, ok := arrayElementOf(typ); ok {
		return typeName(element)
	}
	return "T"
}

/* -------------------------------------------------------------------------- */
/*  Named functions as values                                                 */
/* -------------------------------------------------------------------------- */

// funcValue is a function named where a value goes.
type funcValue struct {
	typ ir.Type
	// callee is the name a call to it is emitted under.
	callee string
	// instantiation is the specialisation of a generic it names, if it does.
	instantiation *instantiation
}

// namedFuncValue resolves a name, not a local, that names a function used as a
// value. handled says the name is a function at all; err says why it cannot be
// a value. expected is the function type wanted, which a generic needs to know
// which specialisation is meant.
func (tc *TypeChecker) namedFuncValue(name string, at ast.Position, scope map[string]ir.Type, expected ir.Type) (funcValue, bool, error) {
	if _, local := scope[name]; local {
		return funcValue{}, false, nil
	}
	if decl, generic := tc.genericFuncs[name]; generic && tc.isGenericName(name) {
		if expected.Kind != ir.TypeFunc {
			// Without a function type to read the specialisation from there is
			// no value to make. A bare `None` is the usual way to get here.
			return funcValue{}, true, locate(fmt.Errorf(msgGenericNotAValue, name, name, name), at, "")
		}
		params := expected.FuncParams()
		if len(params) != len(decl.Params) {
			return funcValue{}, true, locate(fmt.Errorf(msgFuncValueMismatch, name, tc.genericSignatureText(decl), typeName(expected)), at, "")
		}
		subs := map[string]ir.Type{}
		for i, param := range decl.Params {
			tc.unifyTypeArgs(decl.TypeParams, tc.parameterType(param.TypeExpr, param.Type, decl.TypeParams), params[i], subs)
		}
		tc.unifyTypeArgs(decl.TypeParams, tc.typeFor(decl.ReturnType), expected.FuncResult(), subs)
		args := tc.orderedArgs(decl.TypeParams, decl.TypeParamDefaults, subs)
		if len(args) == 0 {
			return funcValue{}, true, locate(fmt.Errorf(msgGenericFuncValue, name, typeName(expected), name, strings.Join(decl.TypeParams, ", ")), at, "")
		}
		full := tc.buildSubstitutionFromTypes(decl.TypeParams, decl.TypeParamDefaults, args)
		paramTypes := make([]ir.Type, len(decl.Params))
		for i, param := range decl.Params {
			paramTypes[i] = tc.substituteType(tc.parameterType(param.TypeExpr, param.Type, decl.TypeParams), full)
		}
		result := tc.substituteType(tc.typeFor(decl.ReturnType), full)
		in := instantiation{Name: name, TypeArgs: args}
		return funcValue{
			typ:           funcTypeOf(paramTypes, result),
			callee:        tc.qualifiedName(decl.Owner, mangleGeneric(name, args)),
			instantiation: &in,
		}, true, nil
	}
	signature, found := tc.lookupFunc(name)
	if !found {
		if name == "print" || isCallableBuiltin(name, tc.types) || isExternalBuiltin(name) {
			return funcValue{}, true, locate(fmt.Errorf(msgBuiltinFuncValue, name), at, "")
		}
		if _, intrinsic := tc.intrinsicSignature(name); intrinsic {
			return funcValue{}, true, locate(fmt.Errorf(msgBuiltinFuncValue, name), at, "")
		}
		return funcValue{}, false, nil
	}
	if reason := tc.notExported(signature, name); reason != "" {
		return funcValue{}, true, locate(fmt.Errorf("%s", reason), at, "")
	}
	callee := signature.cName
	if callee == "" || runtimeForeign(signature, name) {
		callee = name
	}
	return funcValue{typ: funcTypeOf(signature.params, signature.result), callee: callee}, true, nil
}

// qualifiedFuncValue is `module.function` named where a value goes.
func (tc *TypeChecker) qualifiedFuncValue(member ast.MemberExpr, scope map[string]ir.Type) (funcValue, bool, error) {
	signature, named, err := tc.qualifiedCall(member, scope)
	if !named {
		return funcValue{}, false, nil
	}
	if err != nil {
		return funcValue{}, true, err
	}
	callee := signature.cName
	if callee == "" || runtimeForeign(signature, member.Name) {
		callee = member.Name
	}
	return funcValue{typ: funcTypeOf(signature.params, signature.result), callee: callee}, true, nil
}

// genericSignatureText is a generic function's signature, as written.
func (tc *TypeChecker) genericSignatureText(decl ast.FuncDecl) string {
	params := make([]string, len(decl.Params))
	for i, param := range decl.Params {
		params[i] = param.Type
	}
	return ast.FuncTypeName(params, decl.ReturnType)
}

/* -------------------------------------------------------------------------- */
/*  Calls through a value                                                     */
/* -------------------------------------------------------------------------- */

// funcValueCallee is the function type of a call's target when the call is
// through a value: a local, a field, an element, or any expression of a
// function type. ok is false for a call to a named function, which is checked
// as one; err is set when the target is a value that is not a function.
func (tc *TypeChecker) funcValueCallee(call ast.CallExpr, scope map[string]ir.Type) (ir.Type, bool, error) {
	switch callee := call.Callee.(type) {
	case ast.IdentifierExpr:
		typ, local := scope[callee.Name]
		if !local {
			return ir.Type{}, false, nil
		}
		if typ.Kind == ir.TypeFunc {
			return typ, true, nil
		}
		// A binding that failed has been reported already.
		if typ.Name == poisonName {
			return typ, true, nil
		}
		// A local that shares a function's name has always given way to the
		// function in a call; only a name that is nothing but a value is
		// refused as one.
		if tc.namesCallable(callee.Name) {
			return ir.Type{}, false, nil
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgNotAFunction, callee.Name, article(typeName(typ))), placed(callee.Pos, call.Pos), "")
	case ast.MemberExpr:
		if _, named, _ := tc.qualifiedCall(callee, scope); named {
			return ir.Type{}, false, nil
		}
		if isBuiltinMemberCall(callee) || tc.isEnumConstruction(call, scope) {
			return ir.Type{}, false, nil
		}
		if object, isName := callee.Object.(ast.IdentifierExpr); isName {
			if _, local := scope[object.Name]; !local {
				return ir.Type{}, false, nil
			}
		}
		object, err := tc.exprType(callee.Object, scope)
		if err != nil || object.Kind != ir.TypeStruct {
			return ir.Type{}, false, nil
		}
		typ, err := tc.memberType(object, callee.Name)
		if err != nil {
			return ir.Type{}, false, nil
		}
		if typ.Kind == ir.TypeFunc {
			return typ, true, nil
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgNotAFunction, writtenExpr(callee), article(typeName(typ))), placed(callee.Pos, call.Pos), "")
	case ast.CallExpr, ast.IndexExpr, ast.FuncLitExpr, ast.IfExpr:
		typ, err := tc.exprType(call.Callee, scope)
		if err != nil {
			return ir.Type{}, true, err
		}
		if typ.Kind == ir.TypeFunc {
			return typ, true, nil
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgNotAFunction, writtenExpr(call.Callee), article(typeName(typ))), placed(call.Callee.Position(), call.Pos), "")
	}
	return ir.Type{}, false, nil
}

// namesCallable is whether a name can be called as a function: one the program
// or a module declares, or one the language supplies.
func (tc *TypeChecker) namesCallable(name string) bool {
	if _, found := tc.lookupFunc(name); found {
		return true
	}
	if _, generic := tc.genericFuncs[name]; generic {
		return true
	}
	if _, intrinsic := tc.intrinsicSignature(name); intrinsic {
		return true
	}
	return name == "print" || isCallableBuiltin(name, tc.types) || isExternalBuiltin(name) || tc.isTypeName(name) || tc.isExternName(name)
}

// funcValueCallType checks a call through a value of a function type.
func (tc *TypeChecker) funcValueCallType(call ast.CallExpr, typ ir.Type, scope map[string]ir.Type) (ir.Type, error) {
	if typ.Kind != ir.TypeFunc {
		if tc.collect != nil {
			tc.collect.touchedPoison = true
		}
		return typ, nil
	}
	params := typ.FuncParams()
	if len(call.Args) != len(params) {
		return ir.Type{}, locate(fmt.Errorf(msgFuncValueArity, writtenExpr(call.Callee), typeName(typ), plural(len(params), "argument"), len(call.Args)), call.Pos, "")
	}
	for i, arg := range call.Args {
		want := params[i]
		actual, err := tc.exprTypeFor(arg, scope, want)
		if err != nil {
			return ir.Type{}, err
		}
		if !assignable(want, actual) {
			return ir.Type{}, locate(fmt.Errorf("argument %d of %s has type %s, want %s; %s", i+1, writtenExpr(call.Callee), typeName(actual), typeName(want), conversionHint(actual, want)), arg.Position(), "")
		}
		if err := checkConstantFits(arg, want); err != nil {
			return ir.Type{}, err
		}
	}
	return typ.FuncResult(), nil
}

// isFuncValueCall is whether a call is through a value, and the value's type.
func (tc *TypeChecker) isFuncValueCall(call ast.CallExpr, scope map[string]ir.Type) (ir.Type, bool) {
	typ, ok, err := tc.funcValueCallee(call, scope)
	if !ok || err != nil || typ.Kind != ir.TypeFunc {
		return ir.Type{}, false
	}
	return typ, true
}

/* -------------------------------------------------------------------------- */
/*  Literals                                                                  */
/* -------------------------------------------------------------------------- */

// capture is a local of the enclosing function that a literal names.
type capture struct {
	name string
	typ  ir.Type
	// at is where the body first names it.
	at ast.Position
}

// funcLitSignature is the function type a literal is written with.
func (tc *TypeChecker) funcLitSignature(lit ast.FuncLitExpr) (ir.Type, error) {
	params := make([]ir.Type, len(lit.Params))
	for i, param := range lit.Params {
		typ, ok := tc.resolveType(param.Type)
		if !ok {
			return ir.Type{}, locate(fmt.Errorf("unknown type %q in parameter %q of a function literal: no struct, alias or built-in type has that name; declare it, import the module that does, or check the spelling", param.Type, param.Name), placed(param.Pos, lit.Pos), "")
		}
		params[i] = tc.substituteType(tc.declaredType(param.Type), tc.currentTypeArgs)
		if params[i].Kind == ir.TypeInvalid {
			params[i] = typ
		}
	}
	result := ir.Type{Kind: ir.TypeVoid, Name: "void"}
	if lit.ReturnType != "" && lit.ReturnType != "void" {
		if _, ok := tc.resolveType(lit.ReturnType); !ok {
			return ir.Type{}, locate(fmt.Errorf("unknown return type %q of a function literal: no struct, alias or built-in type has that name; declare it, import the module that does, or check the spelling", lit.ReturnType), lit.Pos, "")
		}
		result = tc.substituteType(tc.declaredType(lit.ReturnType), tc.currentTypeArgs)
	}
	typ := funcTypeOf(params, result)
	if err := checkFuncTypeComponents(typ); err != nil {
		return ir.Type{}, locate(err, lit.Pos, "")
	}
	return typ, nil
}

// funcLitKey names one literal at one specialisation: a literal inside a
// generic is checked and lifted once for each.
func (tc *TypeChecker) funcLitKey(lit ast.FuncLitExpr) string {
	key := fmt.Sprintf("%s:%d:%d", lit.Pos.File, lit.Pos.Line, lit.Pos.Column)
	if len(tc.currentTypeArgs) > 0 {
		names := make([]string, 0, len(tc.currentTypeArgs))
		for name := range tc.currentTypeArgs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			key += "|" + name + "=" + typeName(tc.currentTypeArgs[name])
		}
	}
	return key
}

// funcLitType checks a literal and returns its type. The body is checked once
// for each key, and the answer kept, because an expression is typed from many
// places and a body reported twice is a report read twice.
func (tc *TypeChecker) funcLitType(lit ast.FuncLitExpr, scope map[string]ir.Type) (ir.Type, error) {
	typ, err := tc.funcLitSignature(lit)
	if err != nil {
		return ir.Type{}, err
	}
	if tc.closures.checked == nil {
		tc.closures.checked = map[string]error{}
	}
	key := tc.funcLitKey(lit)
	if previous, done := tc.closures.checked[key]; done {
		return typ, previous
	}
	tc.closures.checked[key] = nil
	err = tc.checkFuncLit(lit, typ, scope)
	tc.closures.checked[key] = err
	return typ, err
}

// checkFuncLit checks a literal's captures and body.
func (tc *TypeChecker) checkFuncLit(lit ast.FuncLitExpr, typ ir.Type, scope map[string]ir.Type) error {
	for _, param := range lit.Params {
		if backendReservedName(param.Name) {
			return locate(fmt.Errorf("parameter %q of a function literal: %s", param.Name, backendReservedReason(param.Name)), placed(param.Pos, lit.Pos), "")
		}
		if strings.HasPrefix(param.Name, "rho_") {
			return locate(fmt.Errorf("parameter %q of a function literal: names starting with rho_ are the compiler's own; choose another name", param.Name), placed(param.Pos, lit.Pos), "")
		}
	}
	captures := tc.funcLitCaptures(lit, scope)
	for _, c := range captures {
		if c.typ.Name == poisonName || c.typ.Kind == ir.TypeInvalid {
			continue
		}
		if c.typ.ArrayLen > 0 || (isArrayValue(c.typ) && c.typ.Kind != ir.TypeSlice && c.typ.Kind != ir.TypeFunc) {
			return locate(fmt.Errorf(msgCaptureArray, c.name, typeName(c.typ)), c.at, "")
		}
		if what, refers := tc.captureReference(c.typ, 0); refers {
			return locate(fmt.Errorf(msgCaptureReference, c.name, tc.writtenTypeName(c.typ), what, c.name, tc.writtenTypeName(c.typ)), c.at, "")
		}
	}
	if err := checkCapturesUnchanged(lit, captures); err != nil {
		return err
	}
	bodyScope := cloneScope(scope)
	for _, param := range lit.Params {
		bodyScope[param.Name] = tc.substituteType(tc.declaredType(param.Type), tc.currentTypeArgs)
	}
	previousBackings := tc.backings
	tc.backings = sliceBackings(lit.Body, lit.Params)
	defer func() { tc.backings = previousBackings }()
	result := typ.FuncResult()
	reported := 0
	if tc.collect != nil {
		reported = len(tc.collect.errors) + tc.collect.omitted
	}
	terminates, err := tc.checkStatements(lit.Body, bodyScope, result, false)
	if err != nil {
		return err
	}
	if tc.collect != nil && len(tc.collect.errors)+tc.collect.omitted > reported {
		return nil
	}
	if result.Kind != ir.TypeVoid && !isUnitOrVoid(result) && !terminates {
		end := lit.Pos
		if n := len(lit.Body); n > 0 && lit.Body[n-1].Position().Valid() {
			end = lit.Body[n-1].Position()
		}
		return locate(fmt.Errorf("function literal: missing return: it is declared to return %s, and the end of its body can be reached; end every path with `return <value>`", typeName(result)), end, "")
	}
	return nil
}

// writtenTypeName is a type as a program writes it: a specialisation by its
// generic and arguments, `Slice[u32]`, rather than the name it is emitted under.
func (tc *TypeChecker) writtenTypeName(typ ir.Type) string {
	if spec, ok := tc.specialisations[typ.Name]; ok && len(spec.Args) > 0 {
		args := make([]string, len(spec.Args))
		for i, arg := range spec.Args {
			args[i] = tc.writtenTypeName(arg)
		}
		return spec.Base + "[" + strings.Join(args, ", ") + "]"
	}
	return typeName(typ)
}

// captureReference is whether a captured type holds a reference — a pointer, a
// `[T]` or a `Slice[T]`, directly or in a field, an element or a variant — and
// which kind, for the message.
func (tc *TypeChecker) captureReference(typ ir.Type, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	switch typ.Kind {
	case ir.TypePtr:
		return "a pointer", true
	case ir.TypeSlice:
		return "a [T]", true
	case ir.TypeFunc:
		return "", false
	}
	if _, isSlice := tc.sliceElement(typ); isSlice {
		return "a Slice", true
	}
	if element, isArray := arrayElementOf(typ); isArray {
		return tc.captureReference(element, depth+1)
	}
	if typ.Kind != ir.TypeStruct {
		return "", false
	}
	if info, args := tc.enumOf(typ); info != nil {
		if info.Plain {
			return "", false
		}
		for i := range info.Variants {
			for _, payload := range tc.payloadTypes(info, args, &info.Variants[i], false) {
				if what, refers := tc.captureReference(payload, depth+1); refers {
					return what, true
				}
			}
		}
		return "", false
	}
	for _, field := range tc.structFields(typ.Name) {
		if what, refers := tc.captureReference(field, depth+1); refers {
			return what, true
		}
	}
	return "", false
}

// checkCapturesUnchanged refuses an assignment to a captured name, or its
// address, inside a literal's body: a capture is a copy made when the literal
// was evaluated, and writing to it would change neither the variable nor, in
// a way both backends agree on, the closure.
func checkCapturesUnchanged(lit ast.FuncLitExpr, captures []capture) error {
	if len(captures) == 0 {
		return nil
	}
	captured := map[string]bool{}
	for _, c := range captures {
		captured[c.name] = true
	}
	var found error
	walkLitBody(lit.Body, paramNames(lit.Params), func(statement ast.Stmt, declared map[string]bool) {
		if found != nil {
			return
		}
		isCapture := func(name string) bool { return name != "" && captured[name] && !declared[name] }
		if assign, ok := statement.(ast.AssignStmt); ok {
			if root := rootName(assign.Target); isCapture(root) {
				found = locate(fmt.Errorf(msgCaptureAssigned, root, root, root), assign.Pos, "")
				return
			}
		}
		visitExprs(statement, func(expr ast.Expr) {
			if unary, ok := expr.(ast.UnaryExpr); ok && unary.Op == "&" && found == nil {
				if root := rootName(unary.Value); isCapture(root) {
					found = locate(fmt.Errorf(msgCaptureAddressed, root, root), unary.Pos, "")
				}
			}
		})
	})
	return found
}

func paramNames(params []ast.Param) map[string]bool {
	names := map[string]bool{}
	for _, param := range params {
		names[param.Name] = true
	}
	return names
}

// walkLitBody visits every statement of a literal's body, at any depth, with
// the names declared in the body so far — its parameters, its lets, a `for`'s
// variable and the names a pattern binds. It does not go into a nested literal,
// whose body is its own.
func walkLitBody(body []ast.Stmt, declared map[string]bool, visit func(ast.Stmt, map[string]bool)) {
	for _, statement := range body {
		visit(statement, declared)
		switch stmt := statement.(type) {
		case ast.LetStmt:
			declared[stmt.Name] = true
		case ast.IfStmt:
			walkLitBody(stmt.Then, cloneNames(declared), visit)
			walkLitBody(stmt.Else, cloneNames(declared), visit)
		case ast.WhileStmt:
			walkLitBody(stmt.Body, cloneNames(declared), visit)
		case ast.ForStmt:
			inner := cloneNames(declared)
			inner[stmt.Name] = true
			walkLitBody(stmt.Body, inner, visit)
		case ast.MatchStmt:
			for _, arm := range stmt.Arms {
				inner := cloneNames(declared)
				if arm.Pattern != nil {
					for _, name := range patternNames(arm.Pattern) {
						inner[name] = true
					}
				}
				walkLitBody(arm.Body, inner, visit)
			}
		case ast.BlockStmt:
			walkLitBody(stmt.Stmts, cloneNames(declared), visit)
		}
	}
}

// patternNames are the names written in a pattern, any of which may be a name
// it binds.
func patternNames(pattern ast.Expr) []string {
	names := []string{}
	visitExpr(pattern, func(e ast.Expr) {
		if identifier, ok := e.(ast.IdentifierExpr); ok {
			names = append(names, identifier.Name)
		}
	})
	return names
}

// funcLitCaptures are the locals of the enclosing scope a literal's body names
// (in a nested literal's too), in the order they are first named.
func (tc *TypeChecker) funcLitCaptures(lit ast.FuncLitExpr, scope map[string]ir.Type) []capture {
	out := []capture{}
	seen := map[string]bool{}
	// A name inside a postfix expression carries no position of its own, so
	// the statement it is in stands for it.
	statementAt := lit.Pos
	var walkBody func(body []ast.Stmt, declared map[string]bool)
	var walkExpr func(expr ast.Expr, declared map[string]bool)
	walkExpr = func(expr ast.Expr, declared map[string]bool) {
		if expr == nil {
			return
		}
		switch e := expr.(type) {
		case ast.IdentifierExpr:
			if declared[e.Name] || seen[e.Name] {
				return
			}
			if typ, local := scope[e.Name]; local {
				seen[e.Name] = true
				out = append(out, capture{name: e.Name, typ: typ, at: placed(e.Pos, statementAt)})
			}
		case ast.FuncLitExpr:
			inner := cloneNames(declared)
			for _, param := range e.Params {
				inner[param.Name] = true
			}
			walkBody(e.Body, inner)
		case ast.UnaryExpr:
			walkExpr(e.Value, declared)
		case ast.BinaryExpr:
			walkExpr(e.Left, declared)
			walkExpr(e.Right, declared)
		case ast.CallExpr:
			walkExpr(e.Callee, declared)
			for _, arg := range e.Args {
				walkExpr(arg, declared)
			}
		case ast.MemberExpr:
			walkExpr(e.Object, declared)
		case ast.IndexExpr:
			walkExpr(e.Object, declared)
			walkExpr(e.Index, declared)
		case ast.SliceExpr:
			walkExpr(e.Object, declared)
			walkExpr(e.Low, declared)
			walkExpr(e.High, declared)
		case ast.CastExpr:
			walkExpr(e.Value, declared)
		case ast.StructLiteralExpr:
			for _, value := range e.Values {
				walkExpr(value, declared)
			}
		case ast.ArrayLiteralExpr:
			for _, value := range e.Values {
				walkExpr(value, declared)
			}
			if e.Repeat != nil {
				walkExpr(*e.Repeat, declared)
			}
		case ast.IfExpr:
			walkExpr(e.Condition, declared)
			walkExpr(e.Then, declared)
			walkExpr(e.Else, declared)
		}
	}
	walkBody = func(body []ast.Stmt, declared map[string]bool) {
		walkLitBody(body, declared, func(statement ast.Stmt, names map[string]bool) {
			outer := statementAt
			statementAt = placed(statement.Position(), outer)
			defer func() { statementAt = outer }()
			for _, expr := range statementExprs(statement) {
				walkExpr(expr, names)
			}
		})
	}
	walkBody(lit.Body, paramNames(lit.Params))
	return out
}

/* -------------------------------------------------------------------------- */
/*  Lowering                                                                  */
/* -------------------------------------------------------------------------- */

// closureState is what lowering has made for function values.
type closureState struct {
	// checked are the literals checked, by funcLitKey, with what checking
	// found.
	checked map[string]error
	// sigs are the signatures values are made and called at, by type name.
	sigs     map[string]string
	sigOrder []ir.FuncSigIR
	// lifted are the functions literals and thunks became, by key.
	lifted   map[string]liftedLit
	funcs    []ir.FuncIR
	envs     []ir.StructIR
	envNames []string
	// thunks are the thunks made for named functions, by the name a call to
	// the function is emitted under.
	thunks map[string]string
	seq    int
}

// liftedLit is the function a literal became, and its environment ("" when it
// captures nothing).
type liftedLit struct {
	name string
	env  string
}

// funcSig is the prefix of the helpers a function type is made and called
// with, recorded so each backend defines them.
func (tc *TypeChecker) funcSig(typ ir.Type) string {
	if tc.closures.sigs == nil {
		tc.closures.sigs = map[string]string{}
	}
	if name, ok := tc.closures.sigs[typ.Name]; ok {
		return name
	}
	name := "rho_fn_" + strconv.Itoa(len(tc.closures.sigOrder)+1)
	tc.closures.sigs[typ.Name] = name
	tc.closures.sigOrder = append(tc.closures.sigOrder, ir.FuncSigIR{Name: name, Type: typ})
	return name
}

// lowerFuncValueCall is a call through a value: the call helper, which stops
// the program when the value holds no function.
func (tc *TypeChecker) lowerFuncValueCall(call ast.CallExpr, typ ir.Type, scope map[string]ir.Type) string {
	sig := tc.funcSig(typ)
	params := typ.FuncParams()
	parts := []string{tc.lowerExpr(call.Callee, scope)}
	for i, arg := range call.Args {
		want := ir.Type{}
		if i < len(params) {
			want = params[i]
		}
		lowered := tc.lowerExprFor(arg, scope, want)
		if i < len(params) {
			lowered = tc.converted(lowered, arg, scope, params[i])
		}
		parts = append(parts, lowered)
	}
	return sig + "_call(" + strings.Join(parts, ", ") + ")"
}

// lowerNamedFuncValue is a named function as a value: its thunk, bound to no
// environment.
func (tc *TypeChecker) lowerNamedFuncValue(value funcValue) string {
	if value.instantiation != nil {
		tc.recordInstantiation(*value.instantiation)
	}
	if tc.closures.thunks == nil {
		tc.closures.thunks = map[string]string{}
	}
	sig := tc.funcSig(value.typ)
	key := value.callee + "|" + value.typ.Name
	thunk, made := tc.closures.thunks[key]
	if !made {
		thunk = "rho_thunk_" + strings.TrimPrefix(value.callee, "rho_")
		if len(tc.closures.thunks) > 0 {
			for _, existing := range tc.closures.thunks {
				if existing == thunk {
					thunk += "_" + strconv.Itoa(len(tc.closures.thunks))
					break
				}
			}
		}
		tc.closures.thunks[key] = thunk
		params := []ir.ParamIR{{Name: "rho_env", Type: unitPointer()}}
		args := make([]string, 0, len(value.typ.FuncParams()))
		for i, param := range value.typ.FuncParams() {
			name := "a" + strconv.Itoa(i)
			params = append(params, ir.ParamIR{Name: name, Type: param})
			args = append(args, name)
		}
		call := value.callee + "(" + strings.Join(args, ", ") + ")"
		result := value.typ.FuncResult()
		var body []ir.Instr
		if result.Kind == ir.TypeVoid || isUnitOrVoid(result) {
			body = append(body, ir.Instr{Op: ir.OpExpr, Args: []string{call}})
		} else {
			body = append(body, ir.Instr{Op: ir.OpReturn, Args: []string{call}})
		}
		tc.closures.funcs = append(tc.closures.funcs, ir.FuncIR{Name: thunk, CName: thunk, Params: params, ReturnType: result, Body: body})
		// What a value is called from is not known, so the function is
		// reached from anywhere as far as the arena is concerned.
		tc.arenaMarkEntry(value.callee)
	}
	return sig + "_bind(" + thunk + ", NULL)"
}

// unitPointer is `Ptr[Unit]`: C's void*, Go's unsafe.Pointer.
func unitPointer() ir.Type {
	return ir.Type{Kind: ir.TypePtr, Name: "Ptr[Unit]", Args: []ir.Type{{Kind: ir.TypeStruct, Name: "Unit"}}}
}

// arenaMarkEntry treats a function as one a program may start at: one called
// through a value, from wherever the value went.
func (tc *TypeChecker) arenaMarkEntry(name string) {
	if tc.arena.entries == nil {
		tc.arena.entries = map[string]bool{}
	}
	tc.arena.entries[name] = true
}

// lowerFuncLit lowers a literal: its body becomes a function of its own (once
// for each key), and the value binds it to a copy of what it captures.
func (tc *TypeChecker) lowerFuncLit(lit ast.FuncLitExpr, scope map[string]ir.Type) string {
	typ, err := tc.funcLitType(lit, scope)
	if err != nil {
		return "RHO_FN_NIL"
	}
	sig := tc.funcSig(typ)
	captures := tc.funcLitCaptures(lit, scope)
	if tc.closures.lifted == nil {
		tc.closures.lifted = map[string]liftedLit{}
	}
	key := tc.funcLitKey(lit)
	lifted, done := tc.closures.lifted[key]
	if !done {
		lifted = tc.liftFuncLit(lit, typ, captures)
		tc.closures.lifted[key] = lifted
	}
	env := "NULL"
	if lifted.env != "" {
		values := make([]string, len(captures))
		for i, c := range captures {
			values[i] = tc.lowerExpr(ast.IdentifierExpr{Node: ast.Node{Pos: c.at}, Name: c.name}, scope)
		}
		env = lifted.env + "_make((" + lifted.env + "){ " + strings.Join(values, ", ") + " })"
	}
	return sig + "_bind(" + lifted.name + ", " + env + ")"
}

// liftFuncLit makes the function a literal's body becomes, and the struct its
// captures are copied into.
func (tc *TypeChecker) liftFuncLit(lit ast.FuncLitExpr, typ ir.Type, captures []capture) liftedLit {
	tc.closures.seq++
	n := strconv.Itoa(tc.closures.seq)
	lifted := liftedLit{name: "rho_lambda_" + n}
	scope := map[string]ir.Type{}
	aliases := map[string]string{}
	var prelude []ir.Instr
	if len(captures) > 0 {
		lifted.env = "rho_env_" + n
		fields := make([]ir.FieldIR, len(captures))
		for i, c := range captures {
			fields[i] = ir.FieldIR{Name: c.name, Type: c.typ}
			scope[c.name] = c.typ
			aliases[c.name] = "rho_e." + c.name
		}
		tc.closures.envs = append(tc.closures.envs, ir.StructIR{Name: lifted.env, CName: lifted.env, Fields: fields, Packed: fields})
		tc.closures.envNames = append(tc.closures.envNames, lifted.env)
		prelude = []ir.Instr{{Op: ir.OpLet, Dest: "rho_e", Type: lifted.env, Args: []string{lifted.env + "_at(rho_env)"}}}
	}
	params := []ir.ParamIR{{Name: "rho_env", Type: unitPointer()}}
	for i, param := range lit.Params {
		paramType := typ.FuncParams()[i]
		params = append(params, ir.ParamIR{Name: param.Name, Type: paramType})
		scope[param.Name] = paramType
		delete(aliases, param.Name)
	}
	result := typ.FuncResult()

	savedAliases := tc.lowerAliases
	savedBackings := tc.backings
	savedHint := tc.currentReturnHint
	savedReleased := tc.arena.released
	savedArena := tc.arenaEnter(lifted.name, fmt.Sprintf("closure at line %d", lit.Pos.Line), tc.owner)
	tc.lowerAliases = aliases
	tc.backings = sliceBackings(lit.Body, lit.Params)
	tc.currentReturnHint = result
	tc.arena.released = 0
	tc.arenaMarkEntry(lifted.name)
	body := append(prelude, tc.lowerStatements(lit.Body, scope)...)
	tc.arena.released = savedReleased
	tc.arenaLeave(savedArena)
	tc.currentReturnHint = savedHint
	tc.backings = savedBackings
	tc.lowerAliases = savedAliases

	tc.closures.funcs = append(tc.closures.funcs, ir.FuncIR{Name: lifted.name, CName: lifted.name, Params: params, ReturnType: result, Body: body})
	return lifted
}

// closureIR adds what lowering made for function values to a module.
func (tc *TypeChecker) closureIR(out *ir.ModuleIR) {
	out.Structs = append(out.Structs, tc.closures.envs...)
	out.Funcs = append(out.Funcs, tc.closures.funcs...)
	out.FuncSigs = append(out.FuncSigs, tc.closures.sigOrder...)
	out.Envs = append(out.Envs, tc.closures.envNames...)
}

// funcLitAllocates is whether evaluating a literal builds something in the
// arena: its environment, when it captures anything.
func (tc *TypeChecker) funcLitAllocates(lit ast.FuncLitExpr, scope map[string]ir.Type) bool {
	return len(tc.funcLitCaptures(lit, scope)) > 0
}
