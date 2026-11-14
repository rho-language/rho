//
// @file slices.go
// @brief Slice[T], fixed arrays and bare pointers: what each one can do.
//
// Three sequence types, each with one job:
//
//   - `T[N]` is a fixed array, a value whose length is part of its type. It
//     converts to a `Slice[T]` of all N elements (pointing at the array) where
//     one is wanted, and decays to a `[T]` where a bare pointer is wanted.
//   - `Slice[T]` is a pointer and a length, `{ T* ptr; uint32_t len; }` in C and
//     the same two fields in Go. It is the idiomatic sequence: `s.len`,
//     `len(s)`, `s[i]` checked against the length on every backend, `s[a..b]`
//     checked as `0 <= a <= b <= len`, `for x in s`, and `s = append(s, v)`.
//   - `[T]` is a bare pointer for C interfaces, DMA and MMIO buffers and
//     pointers handed to a callback. It has no length, so nothing that needs one
//     is offered on it; `Slice.from_raw(p, n)` gives it one.
//
// Every operation is lowered to text both backends read: the C the C backend
// compiles, which the Go backend translates (codegen/goexpr.go). The checks are
// helpers named for what they check (`rho_index`, `rho_subslice`,
// `rho_append_at`), defined `static inline` beside a C program and as runtime
// functions in Go, and they stop the program with the same message on both.
//
// append has to put an element somewhere, and the C backend has no heap: the
// only memory it manages is the string arena, released in bulk where the
// program's text makes it safe. A slice grows into the fixed array it was cut
// from. `var s = buf[..0]` makes an empty slice whose capacity is the N of
// `buf: T[N]`, `s = append(s, v)` writes v at `buf[s.len]` and moves the length
// on, and appending to a full slice stops the program. The compiler knows the
// capacity because it knows the array: append is accepted only on a local slice
// declared from a fixed array (starting at its first element) and changed
// afterwards only by append or by cutting it back from its start. Anything else
// would need a capacity the slice does not carry, and is refused with what to
// write instead.
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
	"math/big"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                     */
/* -------------------------------------------------------------------------- */

// sliceElement is the element type of a `Slice[T]` (or of an alias of one,
// `String`), and whether the type is one.
func (tc *TypeChecker) sliceElement(typ ir.Type) (ir.Type, bool) {
	if !isSliceStruct(typ) || typ.ArrayLen > 0 {
		return ir.Type{}, false
	}
	if _, _, dims := nameArrayDims(typ.Name); dims && !strings.HasPrefix(typ.Name, "Slice[") {
		return ir.Type{}, false
	}
	if ptr, err := tc.memberType(typ, "ptr"); err == nil && ptr.Kind == ir.TypePtr && len(ptr.Args) == 1 {
		return ptr.Args[0], true
	}
	if strings.HasPrefix(typ.Name, "Slice[") && strings.HasSuffix(typ.Name, "]") {
		return tc.typeFor(typ.Name[len("Slice[") : len(typ.Name)-1]), true
	}
	if typ.Name == "String" {
		return ir.Type{Kind: ir.TypeU8, Name: "u8"}, true
	}
	return ir.Type{}, false
}

// sliceOf is the `Slice[T]` of an element, with its specialisation recorded so
// the backends emit it.
func (tc *TypeChecker) sliceOf(element ir.Type) ir.Type {
	written := "Slice[" + typeName(element) + "]"
	// While a generic body is checked its parameters are names, not types:
	// there is nothing to specialise yet, and a specialisation over `T` would
	// be a struct no backend can spell.
	for _, param := range tc.checkingTypeParams {
		if strings.Contains(written, param) && containsWord(written, param) {
			return ir.Type{Kind: ir.TypeStruct, Name: written}
		}
	}
	return tc.declaredType(written)
}

// containsWord is whether a type name mentions a name as a whole word.
func containsWord(text, word string) bool {
	for i := 0; i+len(word) <= len(text); i++ {
		if text[i:i+len(word)] != word {
			continue
		}
		before := i == 0 || !isNameByte(text[i-1])
		after := i+len(word) == len(text) || !isNameByte(text[i+len(word)])
		if before && after {
			return true
		}
	}
	return false
}

func isNameByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// sliceElementName is the element of a Slice type as its name spells it, for
// assignable, which has no checker to ask.
func sliceElementName(typ ir.Type) (string, bool) {
	switch {
	case strings.HasPrefix(typ.Name, "Slice[") && strings.HasSuffix(typ.Name, "]"):
		return typ.Name[len("Slice[") : len(typ.Name)-1], true
	case strings.HasPrefix(typ.Name, "Slice_"):
		return strings.TrimPrefix(typ.Name, "Slice_"), true
	case typ.Name == "String":
		return "u8", true
	}
	return "", false
}

// fixedArrayToSlice is whether a fixed array converts to a Slice type: the
// array's element is the slice's.
func fixedArrayToSlice(want, got ir.Type) bool {
	if want.Kind != ir.TypeStruct || !isSliceStruct(want) {
		return false
	}
	if _, fixed := fixedArrayLength(got); !fixed || got.Kind == ir.TypeSlice {
		return false
	}
	element, ok := sliceElementName(want)
	if !ok {
		return false
	}
	return mangleTypeName(element) == mangleTypeName(typeName(elementType(got)))
}

// convertsToSlice is fixedArrayToSlice with aliases followed: a `String[4]`
// converts to a `Slice[String]`, which is a `Slice[Slice[u8]]`.
func (tc *TypeChecker) convertsToSlice(want, got ir.Type) bool {
	element, isSlice := tc.sliceElement(want)
	if !isSlice {
		return false
	}
	if _, fixed := fixedArrayLength(got); !fixed || got.Kind == ir.TypeSlice {
		return false
	}
	return tc.sameElement(element, elementType(got))
}

// sameElement is whether two types are one, aliases followed.
func (tc *TypeChecker) sameElement(a, b ir.Type) bool {
	canon := func(t ir.Type) string {
		if resolved, ok := tc.resolveType(typeName(t)); ok && resolved.Name != "" {
			return mangleTypeName(typeName(resolved))
		}
		return mangleTypeName(typeName(t))
	}
	return canon(a) == canon(b)
}

/* -------------------------------------------------------------------------- */
/*  What may be evaluated twice                                               */
/* -------------------------------------------------------------------------- */

// repeatable is whether an expression can be evaluated more than once with the
// same result and nothing else happening: names, fields, constants, arithmetic
// over them, indexes by them, conversions and `len`. A slice operation reads its
// slice twice — its pointer and its length — and a sub-slice its bounds twice,
// so those are what it accepts.
func repeatable(expr ast.Expr) bool {
	switch e := expr.(type) {
	case ast.IdentifierExpr, ast.NumberExpr, ast.BoolExpr:
		return true
	case ast.MemberExpr:
		return repeatable(e.Object)
	case ast.IndexExpr:
		return repeatable(e.Object) && repeatable(e.Index)
	case ast.UnaryExpr:
		return repeatable(e.Value)
	case ast.BinaryExpr:
		return repeatable(e.Left) && repeatable(e.Right)
	case ast.CastExpr:
		return repeatable(e.Value)
	case ast.SliceExpr:
		return repeatable(e.Object) && (e.Low == nil || repeatable(e.Low)) && (e.High == nil || repeatable(e.High))
	case ast.CallExpr:
		callee, ok := e.Callee.(ast.IdentifierExpr)
		if !ok || len(e.Args) != 1 {
			return false
		}
		if callee.Name == "len" || isScalarConversion(callee.Name) {
			return repeatable(e.Args[0])
		}
	}
	return false
}

// isScalarConversion is whether a name is a scalar type, so a call to it is a
// conversion.
func isScalarConversion(name string) bool {
	switch name {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bool":
		return true
	}
	return false
}

// addressable is whether an expression names storage: a variable, a field of
// one, or an element of one.
func addressable(expr ast.Expr) bool {
	switch e := expr.(type) {
	case ast.IdentifierExpr:
		return true
	case ast.MemberExpr:
		return addressable(e.Object)
	case ast.IndexExpr:
		return addressable(e.Object) && repeatable(e.Index)
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Checking                                                                  */
/* -------------------------------------------------------------------------- */

// checkIndexBound checks the index of `s[i]` on a Slice: an integer, and a
// constant one not negative.
func (tc *TypeChecker) checkIndexBound(index ast.Expr, scope map[string]ir.Type, what string) error {
	typ, err := tc.exprType(index, scope)
	if err != nil {
		return err
	}
	if !isIntegerKind(typ.Kind) || typ.Kind == ir.TypeBool || typ.Enum {
		return locate(fmt.Errorf("%s must be an integer, got %s", what, typeName(typ)), index.Position(), "")
	}
	if value, ok := constIntValue(index); ok && value.Sign() < 0 {
		return locate(fmt.Errorf("%s %s is negative", what, value.String()), index.Position(), "")
	}
	return nil
}

// sliceIndexType checks `s[i]` on a Slice and is the element it reads.
func (tc *TypeChecker) sliceIndexType(expr ast.IndexExpr, element ir.Type, scope map[string]ir.Type) (ir.Type, error) {
	if !repeatable(expr.Object) {
		return ir.Type{}, locate(fmt.Errorf("index a Slice through a name: %s is evaluated for its pointer and again for its length, so bind it first (`let s = ...`) and index s",
			writtenExpr(expr.Object)), placed(expr.Object.Position(), expr.Position()), "")
	}
	if err := tc.checkIndexBound(expr.Index, scope, "an index"); err != nil {
		return ir.Type{}, err
	}
	return element, nil
}

// subsliceType checks `x[a..b]` and is the Slice it makes.
func (tc *TypeChecker) subsliceType(expr ast.SliceExpr, scope map[string]ir.Type) (ir.Type, error) {
	object, err := tc.exprType(expr.Object, scope)
	if err != nil {
		return ir.Type{}, err
	}
	var element ir.Type
	length := -1
	switch {
	case object.Kind == ir.TypeSlice:
		return ir.Type{}, locate(fmt.Errorf("%s is %s, a bare pointer with no length, so it cannot be cut into a sub-slice; give it its length first with `Slice.from_raw(%s, n)`",
			writtenExpr(expr.Object), typeName(object), writtenExpr(expr.Object)), expr.Position(), "")
	case object.Kind == ir.TypeString && !isFixedArray(object):
		return ir.Type{}, locate(fmt.Errorf("a string is not cut with `[a..b]`; use `str_slice(s, start, length)`, which counts bytes"), expr.Position(), "")
	default:
		if e, ok := tc.sliceElement(object); ok {
			element = e
			if !repeatable(expr.Object) {
				return ir.Type{}, locate(fmt.Errorf("cut a Slice through a name: %s is evaluated for its pointer and again for its length, so bind it first (`let s = ...`) and cut s",
					writtenExpr(expr.Object)), placed(expr.Object.Position(), expr.Position()), "")
			}
		} else if n, fixed := fixedArrayLength(object); fixed {
			element = elementType(object)
			length = n
			if !addressable(expr.Object) {
				return ir.Type{}, locate(fmt.Errorf("a sub-slice of a fixed array points at the array, so the array has to be storage: bind %s to a name first",
					writtenExpr(expr.Object)), placed(expr.Object.Position(), expr.Position()), "")
			}
		} else {
			return ir.Type{}, locate(fmt.Errorf("`[a..b]` cuts a Slice[T] or a fixed array T[N], and %s is %s", writtenExpr(expr.Object), article(typeName(object))), expr.Position(), "")
		}
	}
	var low, high *big.Int
	for _, bound := range []struct {
		expr  ast.Expr
		value **big.Int
	}{{expr.Low, &low}, {expr.High, &high}} {
		if bound.expr == nil {
			continue
		}
		if err := tc.checkIndexBound(bound.expr, scope, "a slice bound"); err != nil {
			return ir.Type{}, err
		}
		if !repeatable(bound.expr) {
			return ir.Type{}, locate(fmt.Errorf("a slice bound is evaluated twice — once to check it and once for the length — so it cannot call a function; bind %s to a name first",
				writtenExpr(bound.expr)), bound.expr.Position(), "")
		}
		if value, ok := constIntValue(bound.expr); ok {
			*bound.value = value
		}
	}
	if low != nil && high != nil && low.Cmp(high) > 0 {
		return ir.Type{}, locate(fmt.Errorf("slice [%s..%s] starts after it ends", low.String(), high.String()), expr.Position(), "")
	}
	if length >= 0 {
		limit := big.NewInt(int64(length))
		for _, value := range []*big.Int{low, high} {
			if value != nil && value.Cmp(limit) > 0 {
				return ir.Type{}, locate(fmt.Errorf("slice bound %s out of range [0,%d]", value.String(), length), expr.Position(), "")
			}
		}
	}
	if !element.Enum && element.Kind == ir.TypeInvalid {
		return ir.Type{}, locate(fmt.Errorf("cannot tell what %s holds", writtenExpr(expr.Object)), expr.Position(), "")
	}
	return tc.sliceOf(element), nil
}

// isFromRaw is whether a call is `Slice.from_raw(p, n)` or
// `Slice[T].from_raw(p, n)`, and the element written, if any.
func (tc *TypeChecker) isFromRaw(call ast.CallExpr, scope map[string]ir.Type) (*ast.TypeExpr, bool) {
	member, ok := call.Callee.(ast.MemberExpr)
	if !ok || member.Name != "from_raw" {
		return nil, false
	}
	object, ok := member.Object.(ast.IdentifierExpr)
	if !ok || object.Name != "Slice" {
		return nil, false
	}
	if _, shadowed := scope["Slice"]; shadowed {
		return nil, false
	}
	if len(member.TypeArgs) == 1 {
		return &member.TypeArgs[0], true
	}
	return nil, true
}

// fromRawType checks `Slice.from_raw(p, n)`: p is a `Ptr[T]` or a `[T]`, n a
// count, and the result a `Slice[T]` of n elements at p.
func (tc *TypeChecker) fromRawType(call ast.CallExpr, written *ast.TypeExpr, scope map[string]ir.Type) (ir.Type, error) {
	if len(call.Args) != 2 {
		return ir.Type{}, locate(fmt.Errorf("Slice.from_raw takes a pointer and a length, as in `Slice.from_raw(p, n)`, and this call passes %d", len(call.Args)), call.Pos, "")
	}
	pointer, err := tc.exprType(call.Args[0], scope)
	if err != nil {
		return ir.Type{}, err
	}
	if (pointer.Kind != ir.TypePtr && pointer.Kind != ir.TypeSlice) || len(pointer.Args) != 1 || isUnitOrVoid(pointer.Args[0]) {
		return ir.Type{}, locate(fmt.Errorf("Slice.from_raw takes the pointer to the first element, a Ptr[T] or a [T], and this is %s", article(typeName(pointer))), call.Args[0].Position(), "")
	}
	element := pointer.Args[0]
	if written != nil {
		want := tc.substituteType(tc.typeFor(written.String()), tc.currentTypeArgs)
		if mangleTypeName(typeName(want)) != mangleTypeName(typeName(element)) {
			return ir.Type{}, locate(fmt.Errorf("Slice[%s].from_raw is given a pointer to %s", written.String(), typeName(element)), call.Args[0].Position(), "")
		}
		element = want
	}
	length, err := tc.exprType(call.Args[1], scope)
	if err != nil {
		return ir.Type{}, err
	}
	if !isIntegerKind(length.Kind) || length.Kind == ir.TypeBool || length.Enum {
		return ir.Type{}, locate(fmt.Errorf("the length given to Slice.from_raw must be an integer, got %s", typeName(length)), call.Args[1].Position(), "")
	}
	if err := checkConstantFits(call.Args[1], ir.Type{Kind: ir.TypeU32, Name: "u32"}); err != nil {
		return ir.Type{}, err
	}
	return tc.sliceOf(element), nil
}

// isSliceAppend is whether a call is the built-in append on a Slice, and the
// slice's type.
func (tc *TypeChecker) isSliceAppend(call ast.CallExpr, scope map[string]ir.Type) (ir.Type, bool) {
	callee, ok := call.Callee.(ast.IdentifierExpr)
	if !ok || callee.Name != "append" || len(call.Args) == 0 || tc.rhoFuncNames["append"] {
		return ir.Type{}, false
	}
	target, err := tc.exprType(call.Args[0], scope)
	if err != nil {
		return ir.Type{}, false
	}
	if _, isSlice := tc.sliceElement(target); !isSlice {
		return ir.Type{}, false
	}
	return target, true
}

// appendType checks `append(s, v...)` on a Slice. It is accepted only as the
// value of `s = append(s, ...)`, where checkStatement has said so, because that
// is the one place the slice it grows is named.
func (tc *TypeChecker) appendType(call ast.CallExpr, target ir.Type, scope map[string]ir.Type) (ir.Type, error) {
	allowed := tc.appendAllowed
	tc.appendAllowed = false
	if !allowed {
		return ir.Type{}, locate(fmt.Errorf("append on a Slice is written `s = append(s, v)`: it writes v into the fixed array s was cut from and lengthens s, so it is a statement that assigns s"), call.Pos, "")
	}
	element, _ := tc.sliceElement(target)
	if len(call.Args) < 2 {
		return ir.Type{}, locate(fmt.Errorf("append needs a value to add, as in `s = append(s, v)`"), call.Pos, "")
	}
	for _, value := range call.Args[1:] {
		got, err := tc.exprTypeExpected(value, scope, element)
		if err != nil {
			return ir.Type{}, err
		}
		if !assignable(element, got) || (element.Kind == ir.TypeStruct && got.Kind == ir.TypeStruct && !tc.sameElement(element, got)) {
			return ir.Type{}, locate(fmt.Errorf("append adds %s to a Slice of %s; %s", article(typeName(got)), typeName(element), conversionHint(got, element)), value.Position(), "")
		}
		if err := checkConstantFits(value, element); err != nil {
			return ir.Type{}, err
		}
	}
	return target, nil
}

// checkAppendStatement checks `s = append(s, v...)`: s is a local slice that
// grows into a fixed array it was declared from.
func (tc *TypeChecker) checkAppendStatement(stmt ast.AssignStmt, call ast.CallExpr, scope map[string]ir.Type) error {
	target, isName := stmt.Target.(ast.IdentifierExpr)
	first, firstIsName := call.Args[0].(ast.IdentifierExpr)
	if stmt.Op != "=" || !isName || !firstIsName || target.Name != first.Name {
		return locate(fmt.Errorf("append on a Slice is written `s = append(s, v)`, assigning the slice it grows: it writes into the fixed array s was cut from and lengthens s"), call.Pos, "")
	}
	if _, _, err := tc.appendCapacity(target.Name, scope); err != nil {
		return locate(err, call.Pos, "")
	}
	return nil
}

// checkSliceReturn refuses a returned Slice that points into the function's own
// frame: a fixed array it declares or was given (a parameter is a copy in Go and
// the caller's array in C, so neither answer could be the other's), or an array
// literal. In C that memory is gone once the function returns.
func (tc *TypeChecker) checkSliceReturn(value ast.Expr, result ir.Type, scope map[string]ir.Type) error {
	if _, isSlice := tc.sliceElement(result); !isSlice {
		return nil
	}
	refuse := func(what string) error {
		return locate(fmt.Errorf("cannot return a Slice that points at %s: that memory belongs to this function and is gone when it returns on C; "+
			"take the storage from the caller as a Slice[T] parameter and return a cut of it, or return the elements in a struct", what), value.Position(), "")
	}
	local := func(name string) bool {
		typ, ok := scope[name]
		if !ok {
			return false
		}
		return typ.Kind != ir.TypePtr && typ.Kind != ir.TypeSlice && !isSliceStruct(typ)
	}
	if _, isConstant := tc.moduleArrayLiteral(value, scope); isConstant {
		return refuse("the module-level array " + writtenExpr(value) + ", which is a literal written where it is named")
	}
	if cut, isCut := value.(ast.SliceExpr); isCut {
		if _, isConstant := tc.moduleArrayLiteral(cut.Object, scope); isConstant {
			return refuse("the module-level array " + writtenExpr(cut.Object) + ", which is a literal written where it is named")
		}
	}
	switch v := value.(type) {
	case ast.ArrayLiteralExpr:
		return refuse("an array literal")
	case ast.SliceExpr:
		object, err := tc.exprType(v.Object, scope)
		if err != nil {
			return nil
		}
		if _, isSlice := tc.sliceElement(object); isSlice {
			// A cut of a Slice points where the Slice does.
			if name, ok := v.Object.(ast.IdentifierExpr); ok {
				return tc.checkSliceReturn(name, result, scope)
			}
			return nil
		}
		if root := rootName(v.Object); root != "" && local(root) {
			return refuse("the fixed array " + writtenExpr(v.Object))
		}
	case ast.IdentifierExpr:
		if typ, ok := scope[v.Name]; ok && tc.convertsToSlice(result, typ) {
			return refuse("the fixed array " + v.Name)
		}
		if backing, ok := tc.backings[v.Name]; ok {
			if typ, inScope := scope[backing.array]; inScope && local(backing.array) {
				if _, fixed := fixedArrayLength(typ); fixed {
					return refuse("the fixed array " + backing.array + " (through " + v.Name + ")")
				}
			}
		}
	case ast.MemberExpr, ast.IndexExpr:
		if typ, err := tc.exprType(value, scope); err == nil && tc.convertsToSlice(result, typ) {
			if root := rootName(value); root != "" && local(root) {
				return refuse("the fixed array " + writtenExpr(value))
			}
		}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Where a slice grows                                                       */
/* -------------------------------------------------------------------------- */

// sliceBacking is the fixed array a local slice was cut from, which is where
// append writes.
type sliceBacking struct {
	array string
	// broken says why the slice cannot be appended to after all, "" when it can.
	broken string
}

// sliceBackings are the local slices of a function body that append may grow,
// keyed by name. A slice qualifies when it is declared once, from a fixed array
// named by a variable — `buf`, `buf[..n]`, `buf[0..n]` — so that it starts at
// the array's first element, and is afterwards assigned only `append(s, ...)`,
// a cut of itself from its start, or a fresh cut of the same array. Its address
// is not taken. The array is checked to be a fixed array where append is.
func sliceBackings(body []ast.Stmt, params []ast.Param) map[string]*sliceBacking {
	backings := map[string]*sliceBacking{}
	declared := map[string]int{}
	for _, param := range params {
		declared[param.Name]++
	}
	fromStart := func(value ast.Expr, self string) (string, bool) {
		switch v := value.(type) {
		case ast.IdentifierExpr:
			return v.Name, v.Name != self
		case ast.SliceExpr:
			array, ok := v.Object.(ast.IdentifierExpr)
			if !ok {
				return "", false
			}
			if v.Low != nil {
				if n, constant := constIntValue(v.Low); !constant || n.Sign() != 0 {
					return "", false
				}
			}
			return array.Name, true
		}
		return "", false
	}
	var lets []ast.LetStmt
	var assigns []ast.AssignStmt
	var addresses []string
	var walk func([]ast.Stmt)
	walk = func(statements []ast.Stmt) {
		for _, statement := range statements {
			visitExprs(statement, func(e ast.Expr) {
				if unary, ok := e.(ast.UnaryExpr); ok && unary.Op == "&" {
					addresses = append(addresses, rootName(unary.Value))
				}
			})
			switch stmt := statement.(type) {
			case ast.LetStmt:
				declared[stmt.Name]++
				lets = append(lets, stmt)
			case ast.AssignStmt:
				assigns = append(assigns, stmt)
			case ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case ast.WhileStmt:
				walk(stmt.Body)
			case ast.ForStmt:
				declared[stmt.Name]++
				walk(stmt.Body)
			case ast.MatchStmt:
				for _, arm := range stmt.Arms {
					walk(arm.Body)
				}
			case ast.BlockStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(body)
	for _, let := range lets {
		if let.Value == nil {
			continue
		}
		if array, ok := fromStart(let.Value, let.Name); ok {
			backings[let.Name] = &sliceBacking{array: array}
		}
	}
	for name, backing := range backings {
		switch {
		case declared[name] > 1:
			backing.broken = fmt.Sprintf("%s is declared more than once in this function", name)
		case declared[backing.array] > 1:
			backing.broken = fmt.Sprintf("%s, the array %s was cut from, is declared more than once in this function", backing.array, name)
		}
	}
	for _, address := range addresses {
		if backing, ok := backings[address]; ok && backing.broken == "" {
			backing.broken = fmt.Sprintf("the address of %s is taken, so it may change where append cannot see", address)
		}
	}
	for _, assign := range assigns {
		target, ok := assign.Target.(ast.IdentifierExpr)
		if !ok {
			continue
		}
		if backing, ok := backings[target.Name]; ok && backing.broken == "" {
			if call, isCall := assign.Value.(ast.CallExpr); isCall {
				if callee, named := call.Callee.(ast.IdentifierExpr); named && callee.Name == "append" && len(call.Args) > 0 {
					if first, firstIsName := call.Args[0].(ast.IdentifierExpr); firstIsName && first.Name == target.Name && assign.Op == "=" {
						continue
					}
				}
			}
			if array, cut := fromStart(assign.Value, ""); cut && assign.Op == "=" && (array == target.Name || array == backing.array) {
				continue
			}
			backing.broken = fmt.Sprintf("%s is assigned something other than `append(%s, ...)` or a cut from the start of %s at line %d",
				target.Name, target.Name, backing.array, assign.Pos.Line)
		}
	}
	return backings
}

// appendCapacity is the fixed array a local slice grows into and its length, or
// why append cannot grow it.
func (tc *TypeChecker) appendCapacity(name string, scope map[string]ir.Type) (string, int, error) {
	how := fmt.Sprintf("declare it from a fixed array, as in `var buf: T[N]` and `var %s = buf[..0]`, and change it only with `%s = append(%s, v)`", name, name, name)
	backing, ok := tc.backings[name]
	if !ok {
		return "", 0, fmt.Errorf("append cannot grow %s: a Slice carries no capacity, so append writes into the fixed array the slice was cut from, and %s was not cut from one in this function; %s",
			name, name, how)
	}
	if backing.broken != "" {
		return "", 0, fmt.Errorf("append cannot grow %s: %s; %s", name, backing.broken, how)
	}
	array, inScope := scope[backing.array]
	n, fixed := fixedArrayLength(array)
	if !inScope || !fixed || array.Kind == ir.TypeSlice {
		return "", 0, fmt.Errorf("append cannot grow %s: it was made from %s, which is not a fixed array; %s", name, backing.array, how)
	}
	return backing.array, n, nil
}

/* -------------------------------------------------------------------------- */
/*  Lowering                                                                  */
/* -------------------------------------------------------------------------- */

// sliceSpelling is the C name of a Slice specialisation, which is also how a
// compound literal of it is written for the Go backend to read.
func (tc *TypeChecker) sliceSpelling(element ir.Type) string {
	return mangleTypeName(typeName(tc.sliceOf(element)))
}

// lowerSliceIndex is `s[i]` on a Slice: the element at the checked index, an
// lvalue on both backends.
func (tc *TypeChecker) lowerSliceIndex(expr ast.IndexExpr, element ir.Type, scope map[string]ir.Type) string {
	object := tc.lowerExpr(expr.Object, scope)
	index := "rho_index(" + tc.lowerExpr(expr.Index, scope) + ", (" + object + ").len)"
	pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + typeName(element) + "]", Args: []ir.Type{element}}
	return tc.pointerElementAt(pointer, "("+object+").ptr", index)
}

// fixedArrayStart is the address of a fixed array's first element, typed as the
// pointer to its element.
func (tc *TypeChecker) fixedArrayStart(lowered string, element ir.Type) string {
	pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + typeName(element) + "]", Args: []ir.Type{element}}
	return "((" + tc.cTypeFor(pointer) + ")(&((" + lowered + ")[0])))"
}

// lowerSubslice is `x[a..b]`: a Slice over the same storage. The bounds are
// checked by rho_subslice, which answers the address of element a.
func (tc *TypeChecker) lowerSubslice(expr ast.SliceExpr, scope map[string]ir.Type) string {
	object, _ := tc.exprType(expr.Object, scope)
	lowered := tc.lowerExpr(expr.Object, scope)
	var element ir.Type
	var pointer, length string
	if e, ok := tc.sliceElement(object); ok {
		element = e
		pointer = "(" + lowered + ").ptr"
		length = "(" + lowered + ").len"
	} else {
		n, _ := fixedArrayLength(object)
		element = elementType(object)
		pointer = tc.fixedArrayStart(lowered, element)
		if literal, isConstant := tc.moduleArrayLiteral(expr.Object, scope); isConstant {
			// A module-level array is inlined where it is named, so its cut is
			// a cut of storage for the block it is written in.
			if slice, ok := tc.lowerToSlice("", literal, scope, tc.sliceOf(element)); ok {
				pointer = "(" + slice + ").ptr"
			}
		}
		length = strconv.Itoa(n)
	}
	low := "0"
	if expr.Low != nil {
		low = tc.lowerExpr(expr.Low, scope)
	}
	high := length
	if expr.High != nil {
		high = tc.lowerExpr(expr.High, scope)
	}
	return "(" + tc.sliceSpelling(element) + "){ rho_subslice(" + tc.cTypeFor(element) + ", " + pointer + ", " + low + ", " + high + ", " + length +
		"), rho_slice_len(" + low + ", " + high + ") }"
}

// lowerFromRaw is `Slice.from_raw(p, n)`.
func (tc *TypeChecker) lowerFromRaw(call ast.CallExpr, scope map[string]ir.Type) string {
	typ, _ := tc.exprType(call, scope)
	element, _ := tc.sliceElement(typ)
	source, _ := tc.exprType(call.Args[0], scope)
	pointer := tc.lowerExpr(call.Args[0], scope)
	if source.Kind == ir.TypeSlice {
		// A `[T]` is a pointer in C and a Go slice in Go; rho_raw_ptr is its
		// first element's address on both.
		pointer = "rho_raw_ptr(" + pointer + ")"
	}
	length := tc.converted(tc.lowerExpr(call.Args[1], scope), call.Args[1], scope, ir.Type{Kind: ir.TypeU32, Name: "u32"})
	return "(" + tc.sliceSpelling(element) + "){ " + pointer + ", " + length + " }"
}

// lowerAppend is `s = append(s, v...)`: each value written at the slice's
// length, checked against the capacity of the array it grows into, and the
// length moved on.
func (tc *TypeChecker) lowerAppend(target string, call ast.CallExpr, scope map[string]ir.Type) []ir.Instr {
	sliceType, _ := tc.exprType(call.Args[0], scope)
	element, _ := tc.sliceElement(sliceType)
	_, capacity, err := tc.appendCapacity(target, scope)
	if err != nil {
		return nil
	}
	pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + typeName(element) + "]", Args: []ir.Type{element}}
	out := []ir.Instr{}
	for _, value := range call.Args[1:] {
		lowered := tc.converted(tc.lowerExprExpected(value, scope, element), value, scope, element)
		slot := tc.pointerElementAt(pointer, target+".ptr", "rho_append_at("+target+".len, "+strconv.Itoa(capacity)+")")
		out = append(out,
			ir.Instr{Op: ir.OpAssign, Dest: slot, Args: []string{lowered}},
			ir.Instr{Op: ir.OpAssign, Dest: target + ".len", Args: []string{target + ".len + 1"}},
		)
	}
	return out
}

// lowerToSlice is a value converted to a Slice where one is wanted: a fixed
// array becomes a Slice of all its elements, pointing at it, and an array
// literal becomes one over storage of the block it is written in.
func (tc *TypeChecker) lowerToSlice(lowered string, expr ast.Expr, scope map[string]ir.Type, target ir.Type) (string, bool) {
	element, ok := tc.sliceElement(target)
	if !ok {
		return "", false
	}
	if literal, isLiteral := expr.(ast.ArrayLiteralExpr); isLiteral {
		values := []string{}
		if literal.Repeat != nil {
			count, isCount := constIntValue(*literal.Repeat)
			if !isCount || len(literal.Values) != 1 {
				return "", false
			}
			value := tc.converted(tc.lowerExprFor(literal.Values[0], scope, element), literal.Values[0], scope, element)
			for i := int64(0); i < count.Int64(); i++ {
				values = append(values, value)
			}
		} else {
			for _, value := range literal.Values {
				values = append(values, tc.converted(tc.lowerExprFor(value, scope, element), value, scope, element))
			}
		}
		spelling := tc.sliceSpelling(element)
		if len(values) == 0 {
			pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + typeName(element) + "]", Args: []ir.Type{element}}
			return "(" + spelling + "){ ((" + tc.cTypeFor(pointer) + ")NULL), 0 }", true
		}
		return "(" + spelling + "){ rho_array_ptr(" + tc.cTypeFor(element) + ", " + strings.Join(values, ", ") + "), " + strconv.Itoa(len(values)) + " }", true
	}
	if literal, isConstant := tc.moduleArrayLiteral(expr, scope); isConstant {
		return tc.lowerToSlice(lowered, literal, scope, target)
	}
	source, err := tc.exprType(expr, scope)
	if err != nil || !tc.convertsToSlice(target, source) {
		return "", false
	}
	n, _ := fixedArrayLength(source)
	return "(" + tc.sliceSpelling(element) + "){ " + tc.fixedArrayStart(lowered, element) + ", " + strconv.Itoa(n) + " }", true
}

// literalForSlice is whether an expression is an array literal written where a
// Slice is wanted, which takes its element type from it.
func (tc *TypeChecker) literalForSlice(expr ast.Expr, want ir.Type) bool {
	if _, isLiteral := expr.(ast.ArrayLiteralExpr); !isLiteral {
		return false
	}
	_, isSlice := tc.sliceElement(want)
	return isSlice
}

// refuseBarePointer records that what needs a length — len, for, append —
// cannot be done on a `[T]`, which is a bare pointer and carries none. It is
// refused on every backend, so a program means the same on each; a Slice[T] is
// what carries a length. written is the operation as the program wrote it, and
// what the operand it applies to.
func (tc *TypeChecker) refuseBarePointer(at ast.Position, written, what string, operand ast.Expr, typ ir.Type) {
	where := ""
	if tc.owner != "" {
		where = " (in " + tc.owner + ")"
	}
	tc.cRefusals = append(tc.cRefusals, ir.Refusal{
		File: at.File, Line: at.Line, Column: at.Column,
		Message: fmt.Sprintf("%s%s: %s is %s, a bare pointer that carries no length, so %s cannot be done on it; "+
			"use a Slice[T], which carries its length — a fixed-size array T[N] converts to one, and Slice.from_raw(p, n) gives a pointer one",
			written, where, writtenExpr(operand), typeName(typ), what),
		Everywhere: true,
	})
}

// firstArgBarePointer is whether a call is the built-in append on a `[T]`, and
// the `[T]`.
func (tc *TypeChecker) firstArgBarePointer(name string, call ast.CallExpr, scope map[string]ir.Type) (ir.Type, bool) {
	if name != "append" || len(call.Args) == 0 || tc.rhoFuncNames["append"] {
		return ir.Type{}, false
	}
	typ, err := tc.exprType(call.Args[0], scope)
	if err != nil || typ.Kind != ir.TypeSlice {
		return ir.Type{}, false
	}
	return typ, true
}

// placed is the first of two positions that says where it is: an expression
// inside a postfix chain carries none of its own, and the chain does.
func placed(at, fallback ast.Position) ast.Position {
	if at.Valid() {
		return at
	}
	return fallback
}

// moduleArrayLiteral is the literal a module-level array constant names, when
// an expression names one: a module's bindings are inlined at every use, so
// such an array is storage of the block that names it.
func (tc *TypeChecker) moduleArrayLiteral(expr ast.Expr, scope map[string]ir.Type) (ast.ArrayLiteralExpr, bool) {
	name, ok := expr.(ast.IdentifierExpr)
	if !ok {
		return ast.ArrayLiteralExpr{}, false
	}
	if _, local := scope[name.Name]; local {
		return ast.ArrayLiteralExpr{}, false
	}
	decl, constant := tc.moduleConsts[name.Name]
	if !constant || decl.Value == nil {
		return ast.ArrayLiteralExpr{}, false
	}
	literal, isLiteral := decl.Value.(ast.ArrayLiteralExpr)
	return literal, isLiteral
}

// structFieldTypes are the field types of the struct a literal builds, in the
// order the literal gives them, or nil when they are not known here.
func (tc *TypeChecker) structFieldTypes(name string, typeArgs []ir.Type) []ir.Type {
	if decl, generic := tc.genericStructs[name]; generic {
		if len(typeArgs) == 0 {
			return nil
		}
		subs := tc.buildSubstitutionFromTypes(decl.TypeParams, decl.TypeParamDefaults, typeArgs)
		out := make([]ir.Type, 0, len(decl.Fields))
		for _, field := range decl.Fields {
			out = append(out, tc.declaredTypeWith(field.Type, subs))
		}
		return out
	}
	layout, ok := tc.structIR[name]
	if !ok {
		return nil
	}
	out := make([]ir.Type, 0, len(layout.Fields))
	for _, field := range layout.Fields {
		out = append(out, field.Type)
	}
	return out
}

// letCType is the C spelling of a local's declared type, with a type parameter
// of the specialisation being lowered replaced by its argument.
func (tc *TypeChecker) letCType(declared string) string {
	if len(tc.currentTypeArgs) > 0 {
		if resolved, isParam := tc.currentTypeArgs[declared]; isParam {
			return tc.cTypeFor(resolved)
		}
	}
	return tc.cTypeForTypeName(declared)
}

// letTypeName is the Rho spelling of a local's declared type for a backend that
// declares it by name (Go). Inside a specialisation a type parameter is the type
// it was specialised with: `var out: T` in `last[u32]` is a u32.
func (tc *TypeChecker) letTypeName(declared string) string {
	if len(tc.currentTypeArgs) > 0 {
		if resolved, isParam := tc.currentTypeArgs[declared]; isParam {
			return typeName(resolved)
		}
	}
	return tc.writtenWithoutAliases(declared)
}

// writtenExpr is a short spelling of an expression for a message.
func writtenExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case ast.IdentifierExpr:
		return e.Name
	case ast.NumberExpr:
		return e.Value
	case ast.MemberExpr:
		return writtenExpr(e.Object) + "." + e.Name
	case ast.IndexExpr:
		return writtenExpr(e.Object) + "[" + writtenExpr(e.Index) + "]"
	case ast.CallExpr:
		return writtenCallee(e.Callee) + "(...)"
	case ast.BinaryExpr:
		return writtenExpr(e.Left) + " " + e.Op + " " + writtenExpr(e.Right)
	}
	return "the value"
}
