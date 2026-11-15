//
// @file autoderef.go
// @brief A field reached through a pointer: `p.f` for a `Ptr` to a struct.
//
// Go's selector reaches through a pointer to a struct, and Rho's does the same:
// `p.f`, with `p` a `Ptr[S]`, is `(*p).f`, read or written (FINDINGS #10). It is
// done by rewriting the expression into the dereference it means, once, before a
// statement is checked, so every pass after — the checker, the arena's view of a
// write through a pointer, both backends' lowering — sees the `(*p).f` it would
// have seen had the program written it, and none of them has a second spelling
// to know about.
//
// One level only, as in Go: a `Ptr[Ptr[S]]` is dereferenced by the program.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"rho/src/frontend/ast"
	"rho/src/ir"
)

// pointsAtStruct reports whether a type is a pointer whose element has fields.
func (tc *TypeChecker) pointsAtStruct(typ ir.Type) bool {
	if typ.Kind != ir.TypePtr || len(typ.Args) != 1 || typ.ArrayLen > 0 {
		return false
	}
	element := typ.Args[0]
	return element.Kind == ir.TypeStruct && element.ArrayLen == 0 && !isUnitOrVoid(element)
}

// derefStatement rewrites the expressions a statement holds directly. The
// statements nested in it are rewritten when they are checked, with the scope
// they are checked in.
func (tc *TypeChecker) derefStatement(statement ast.Stmt, scope map[string]ir.Type) ast.Stmt {
	switch s := statement.(type) {
	case ast.LetStmt:
		s.Value = tc.derefExpr(s.Value, scope)
		return s
	case ast.AssignStmt:
		s.Target = tc.derefExpr(s.Target, scope)
		s.Value = tc.derefExpr(s.Value, scope)
		return s
	case ast.ExprStmt:
		s.Expr = tc.derefExpr(s.Expr, scope)
		return s
	case ast.ReturnStmt:
		s.Value = tc.derefExpr(s.Value, scope)
		return s
	case ast.IfStmt:
		s.Condition = tc.derefExpr(s.Condition, scope)
		return s
	case ast.WhileStmt:
		s.Condition = tc.derefExpr(s.Condition, scope)
		return s
	case ast.ForStmt:
		s.Collection = tc.derefExpr(s.Collection, scope)
		s.Step = tc.derefExpr(s.Step, scope)
		return s
	case ast.MatchStmt:
		s.Value = tc.derefExpr(s.Value, scope)
		return s
	}
	return statement
}

// derefExpr rewrites `p.f` into `(*p).f` wherever p is a pointer to a struct.
// A function literal's body is its own scope and is rewritten when it is
// checked.
func (tc *TypeChecker) derefExpr(expr ast.Expr, scope map[string]ir.Type) ast.Expr {
	switch e := expr.(type) {
	case nil:
		return nil
	case ast.MemberExpr:
		e.Object = tc.derefExpr(e.Object, scope)
		if len(e.TypeArgs) > 0 {
			return e
		}
		// A name that is not a value — a module, a type, an enum — is not a
		// pointer, and is left for the checker to read as what it is.
		if identifier, ok := e.Object.(ast.IdentifierExpr); ok {
			if _, local := scope[identifier.Name]; !local {
				return e
			}
		}
		typ, err := tc.exprType(e.Object, scope)
		if err != nil || !tc.pointsAtStruct(typ) {
			return e
		}
		deref := ast.UnaryExpr{Op: "*", Value: e.Object}
		deref.Pos = e.Object.Position()
		e.Object = deref
		return e
	case ast.UnaryExpr:
		e.Value = tc.derefExpr(e.Value, scope)
		return e
	case ast.BinaryExpr:
		e.Left = tc.derefExpr(e.Left, scope)
		e.Right = tc.derefExpr(e.Right, scope)
		return e
	case ast.CallExpr:
		e.Callee = tc.derefExpr(e.Callee, scope)
		e.Callee = tc.unqualifiedGeneric(e.Callee, scope)
		if len(e.Args) > 0 {
			args := make([]ast.Expr, len(e.Args))
			for i, arg := range e.Args {
				args[i] = tc.derefExpr(arg, scope)
			}
			e.Args = args
		}
		return e
	case ast.IndexExpr:
		e.Object = tc.derefExpr(e.Object, scope)
		e.Index = tc.derefExpr(e.Index, scope)
		return e
	case ast.SliceExpr:
		e.Object = tc.derefExpr(e.Object, scope)
		e.Low = tc.derefExpr(e.Low, scope)
		e.High = tc.derefExpr(e.High, scope)
		return e
	case ast.CastExpr:
		e.Value = tc.derefExpr(e.Value, scope)
		return e
	case ast.StructLiteralExpr:
		if len(e.Values) > 0 {
			values := make([]ast.Expr, len(e.Values))
			for i, value := range e.Values {
				values[i] = tc.derefExpr(value, scope)
			}
			e.Values = values
		}
		return e
	case ast.ArrayLiteralExpr:
		if len(e.Values) > 0 {
			values := make([]ast.Expr, len(e.Values))
			for i, value := range e.Values {
				values[i] = tc.derefExpr(value, scope)
			}
			e.Values = values
		}
		if e.Repeat != nil {
			count := tc.derefExpr(*e.Repeat, scope)
			e.Repeat = &count
		}
		return e
	case ast.IfExpr:
		e.Condition = tc.derefExpr(e.Condition, scope)
		e.Then = tc.derefExpr(e.Then, scope)
		e.Else = tc.derefExpr(e.Else, scope)
		return e
	}
	return expr
}

// unqualifiedGeneric reads a qualified call of a generic function,
// `slices.sort_by(xs, less)` or `slices.sort_by[G](xs, less)`, as the call by
// its own name it is: a generic function is deduced and specialised under that
// name, and the qualified one had no deduction of its own, so its type
// parameters were compared unsubstituted (FINDINGS #8). It is left qualified
// when the bare name means another function where the call is written.
func (tc *TypeChecker) unqualifiedGeneric(callee ast.Expr, scope map[string]ir.Type) ast.Expr {
	member, ok := callee.(ast.MemberExpr)
	if !ok || len(member.TypeArgs) > 0 {
		return callee
	}
	object, ok := member.Object.(ast.IdentifierExpr)
	if !ok {
		return callee
	}
	if _, isValue := scope[object.Name]; isValue {
		return callee
	}
	entry, named, err := tc.moduleFunc(object.Name, member.Name)
	if !named || err != nil || len(entry.genericParams) == 0 {
		return callee
	}
	decl, generic := tc.genericFuncs[member.Name]
	if !generic || decl.Pos != entry.pos {
		return callee
	}
	if seen, declared := tc.lookupFunc(member.Name); declared && seen.pos != entry.pos {
		return callee
	}
	bare := ast.IdentifierExpr{Name: member.Name}
	bare.Pos = member.Pos
	return bare
}
