//
// @file letbind.go
// @brief A `let` binds a name that does not change.
//
// `syntax.md` has always said so, and the checker did not hold a program to it:
// `let i: u32 = 0` followed by `i = i + 1` compiled (FINDINGS #7). This walks a
// function's body for an assignment whose target is a name a `let` bound — `x =`
// or `x op=` — and reports it where it is written, with the fix: declare the
// name with `var`.
//
// What a `let` binds is the name. Writing an element or a field of the value
// it holds (`buf[i] = b`, `p.x = 1`) and taking its address are not assignments
// to the name and are left alone, as Go leaves a value's parts writable.
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

	"rho/src/frontend/ast"
)

// letAssignment is one assignment to a name a `let` bound.
type letAssignment struct {
	Name string
	// At is where the assignment's target is written; Bound is where the
	// `let` is.
	At    ast.Position
	Bound ast.Position
}

// warnLetAssignments warns at every assignment to a let-bound name in a
// function. It is a deprecation warning for one release, as docs/stability.md
// promises a change that breaks a program, and an error after it: programs in
// use reassign a `let` (the RoboHub sources, the standard library until this
// release), and the fix is one word at the binding.
func (tc *TypeChecker) warnLetAssignments(fn ast.FuncDecl) {
	for _, found := range letAssignments(fn.Params, fn.Body) {
		if !found.At.Valid() {
			continue
		}
		tc.warnDeprecated(found.At, letAssignmentMessage(found.Name),
			Note{Position: found.Bound, Message: found.Name + " is bound by `let` here"})
	}
}

// letAssignmentMessage is the warning, with what to write instead.
func letAssignmentMessage(name string) string {
	return fmt.Sprintf("assigning to %s, which is bound by `let`, is deprecated: a `let` binds a name that does not change, and the next release refuses this; declare %s with `var` to assign to it", name, name)
}

// letAssignments finds every assignment to a let-bound name in a function's
// body, in the order they are written.
func letAssignments(params []ast.Param, body []ast.Stmt) []letAssignment {
	walker := &letWalker{}
	scope := map[string]*ast.Position{}
	for _, param := range params {
		scope[param.Name] = nil
	}
	walker.statements(body, scope)
	return walker.found
}

// letWalker carries what has been found; a scope maps each name bound so far
// to where its `let` is, or nil for a name that may change (a `var`, a
// parameter, a loop or pattern variable).
type letWalker struct {
	found []letAssignment
}

func copyLetScope(scope map[string]*ast.Position) map[string]*ast.Position {
	out := make(map[string]*ast.Position, len(scope))
	for name, at := range scope {
		out[name] = at
	}
	return out
}

func (w *letWalker) statements(statements []ast.Stmt, scope map[string]*ast.Position) {
	for _, statement := range statements {
		w.statement(statement, scope)
	}
}

func (w *letWalker) statement(statement ast.Stmt, scope map[string]*ast.Position) {
	switch s := statement.(type) {
	case ast.LetStmt:
		w.expr(s.Value, scope)
		if s.Mutable {
			scope[s.Name] = nil
		} else {
			at := s.Pos
			scope[s.Name] = &at
		}
	case ast.AssignStmt:
		w.expr(s.Target, scope)
		w.expr(s.Value, scope)
		if target, ok := s.Target.(ast.IdentifierExpr); ok {
			if bound, known := scope[target.Name]; known && bound != nil {
				at := target.Pos
				if !at.Valid() {
					at = s.Pos
				}
				w.found = append(w.found, letAssignment{Name: target.Name, At: at, Bound: *bound})
			}
		}
	case ast.ExprStmt:
		w.expr(s.Expr, scope)
	case ast.ReturnStmt:
		w.expr(s.Value, scope)
	case ast.IfStmt:
		w.expr(s.Condition, scope)
		w.statements(s.Then, copyLetScope(scope))
		w.statements(s.Else, copyLetScope(scope))
	case ast.WhileStmt:
		w.expr(s.Condition, scope)
		w.statements(s.Body, copyLetScope(scope))
	case ast.ForStmt:
		w.expr(s.Collection, scope)
		w.expr(s.Step, scope)
		inner := copyLetScope(scope)
		inner[s.Name] = nil
		w.statements(s.Body, inner)
	case ast.MatchStmt:
		w.expr(s.Value, scope)
		for _, arm := range s.Arms {
			inner := copyLetScope(scope)
			// A pattern's names are bound by the arm; they shadow, and are
			// not the `let` they may share a name with.
			for _, name := range patternNames(arm.Pattern) {
				inner[name] = nil
			}
			w.statements(arm.Body, inner)
		}
	case ast.BlockStmt:
		w.statements(s.Stmts, copyLetScope(scope))
	}
}

// expr looks inside an expression for a function literal, whose body is a
// function of its own that sees the names around it.
func (w *letWalker) expr(expr ast.Expr, scope map[string]*ast.Position) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case ast.FuncLitExpr:
		inner := copyLetScope(scope)
		for _, param := range e.Params {
			inner[param.Name] = nil
		}
		w.statements(e.Body, inner)
	case ast.UnaryExpr:
		w.expr(e.Value, scope)
	case ast.BinaryExpr:
		w.expr(e.Left, scope)
		w.expr(e.Right, scope)
	case ast.CallExpr:
		w.expr(e.Callee, scope)
		for _, arg := range e.Args {
			w.expr(arg, scope)
		}
	case ast.MemberExpr:
		w.expr(e.Object, scope)
	case ast.IndexExpr:
		w.expr(e.Object, scope)
		w.expr(e.Index, scope)
	case ast.StructLiteralExpr:
		for _, value := range e.Values {
			w.expr(value, scope)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range e.Values {
			w.expr(value, scope)
		}
	case ast.IfExpr:
		w.expr(e.Condition, scope)
		w.expr(e.Then, scope)
		w.expr(e.Else, scope)
	case ast.CastExpr:
		w.expr(e.Value, scope)
	}
}
