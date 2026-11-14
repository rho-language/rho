//
// @file arena.go
// @brief When the C backend may give a string back.
//
// Every string the C runtime builds comes from one arena, and nothing frees a
// string on its own. The compiler releases them in bulk, at two places a
// program's text makes safe:
//
//   - At the end of each iteration of a loop: a mark is taken before the loop,
//     and every iteration — and a `continue` — releases back to it. A `break`
//     leaves for the release after the loop; a `return` releases nothing, so the
//     string it returns survives for the caller.
//   - After a statement: what an expression statement or a condition built is
//     released once it has run, and a `let` or an assignment of one string keeps
//     that string and releases the rest.
//
// Both are sound because a Rho string can outlive the place it was built only by
// being returned, stored in something declared outside the loop, written through
// a pointer, or kept by a C function. The language has no mutable globals, no
// closures and no heap objects, so those are all of the ways. A loop whose body
// stores a string outside itself is not reset; a statement that passes a pointer
// is not released. A C function is given a string for the duration of the call
// and copies what it keeps.
//
// A loop that cannot be reset keeps every string its iterations build. On a host
// that is memory that grows; on a board it is a fixed arena that runs out. So the
// loop is reported — a warning on a host and an error on a board — when its
// garbage can live as long as the program: it is a `while` loop (a `for` is
// bounded by its array), its strings are stored in nothing an enclosing loop
// releases, and the function it is in is reached from `main` other than from
// inside a loop that is reset.
//
// The Go backend has a collector, and drops all of this.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"fmt"
	"strconv"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// ArenaEscape is a loop whose strings are never released, because a string an
// iteration builds is kept in a variable declared outside it.
type ArenaEscape struct {
	// Module and Function are where the loop is.
	Module   string
	Function string
	// Line and Column are where the loop begins.
	Line   int
	Column int
	// Variable is what the string is kept in, as the program names it.
	Variable string
}

// String is the escape as a sentence.
func (e ArenaEscape) String() string {
	where := e.Function
	if e.Module != "" {
		where = e.Module + "." + e.Function
	}
	return fmt.Sprintf("%s: the loop at line %d keeps a string it builds in %q, which is declared outside the loop, so none of the strings its iterations build is ever released",
		where, e.Line, e.Variable)
}

// arenaLoop is one loop enclosing the statement being lowered.
type arenaLoop struct {
	// mark is the arena mark the loop releases to, or "" when it is not reset.
	mark string
	// declared is every name declared anywhere in its body.
	declared map[string]bool
}

// arenaCall is one call from a function Rho emits.
type arenaCall struct {
	caller, callee string
	// reclaimed says what the callee leaves behind is given back once it
	// returns: the call is inside a loop released every iteration, or in a
	// statement whose temporaries are released.
	reclaimed bool
}

// arenaCandidate is a loop that is not reset and might keep its garbage for the
// life of the program, pending whether its function is reached from main.
type arenaCandidate struct {
	function string
	escape   ArenaEscape
	// endless says the loop cannot end: its condition does not change in its
	// body and nothing in it breaks out or returns. What called it never gets
	// to release anything.
	endless bool
}

// arenaState is what the lowering pass knows about the arena.
type arenaState struct {
	// function, name and owner are the function being lowered: its C name, its
	// name as written and its module.
	function, name, owner string
	// loops are the loops enclosing the statement being lowered, innermost last.
	loops []arenaLoop
	// released counts the statements being lowered whose temporaries are
	// released after them.
	released   int
	calls      []arenaCall
	candidates []arenaCandidate
	// decls are the functions Rho emits, by C name, and allocates whether each
	// may build a string, worked out once.
	decls     map[string]ast.FuncDecl
	allocates map[string]int
	// entries are the C names of the functions a program starts at.
	entries map[string]bool
	// root is the program's own module, which its functions name as "".
	root string
}

const (
	allocUnknown = iota
	allocPending
	allocNo
	allocYes
)

// ArenaEscapes are the loops a program cannot release that may keep their strings
// for as long as it runs. They are known once ToIR has run.
//
// A loop that cannot be released is reported when its strings are never given
// back: it never ends and the program reaches it, or the function it is in is
// reached from `main` only through calls whose leftovers nothing releases. A
// bounded loop in a function whose result a statement keeps is not: the
// statement gives back everything else once it has the result.
func (tc *TypeChecker) ArenaEscapes() []ArenaEscape {
	reachable := tc.arenaReach(func(arenaCall) bool { return true })
	exposed := tc.arenaReach(func(call arenaCall) bool { return !call.reclaimed })
	out := []ArenaEscape{}
	for _, candidate := range tc.arena.candidates {
		if exposed[candidate.function] || (candidate.endless && reachable[candidate.function]) {
			out = append(out, candidate.escape)
		}
	}
	return out
}

// arenaReach is every function reached from an entry through the calls follow
// accepts.
func (tc *TypeChecker) arenaReach(follow func(arenaCall) bool) map[string]bool {
	reached := map[string]bool{}
	for name := range tc.arena.entries {
		reached[name] = true
	}
	for changed := true; changed; {
		changed = false
		for _, call := range tc.arena.calls {
			if reached[call.caller] && !reached[call.callee] && follow(call) {
				reached[call.callee] = true
				changed = true
			}
		}
	}
	return reached
}

// arenaRegister records the functions a module declares, before any is lowered,
// so a call can be followed into the function it names.
func (tc *TypeChecker) arenaRegister(m ast.Module) {
	if tc.arena.decls == nil {
		tc.arena.decls = map[string]ast.FuncDecl{}
		tc.arena.allocates = map[string]int{}
		tc.arena.entries = map[string]bool{}
	}
	if tc.arena.root == "" {
		tc.arena.root = m.Name
	}
	for _, decl := range m.Decls {
		fn, ok := decl.(ast.FuncDecl)
		if !ok || fn.Extern {
			continue
		}
		name := qualifiedCName(fn.Owner, fn.Name)
		tc.arena.decls[name] = fn
		if fn.Name == "main" && len(fn.TypeParams) == 0 {
			tc.arena.entries[name] = true
		}
	}
}

// arenaModule is the module of the function being lowered, by its name.
func (tc *TypeChecker) arenaModule() string {
	if tc.arena.owner != "" {
		return tc.arena.owner
	}
	return tc.arena.root
}

// arenaEnter starts lowering a function, and returns what to restore after it.
func (tc *TypeChecker) arenaEnter(function, name, owner string) arenaState {
	saved := tc.arena
	tc.arena.function, tc.arena.name, tc.arena.owner = function, name, owner
	tc.arena.loops = nil
	return saved
}

// arenaLeave restores what arenaEnter saved, keeping what was learned.
func (tc *TypeChecker) arenaLeave(saved arenaState) {
	tc.arena.function, tc.arena.name, tc.arena.owner = saved.function, saved.name, saved.owner
	tc.arena.loops = saved.loops
}

// arenaRecordCall notes a call the function being lowered makes, by the name
// the call is emitted under.
func (tc *TypeChecker) arenaRecordCall(callee string) {
	if tc.arena.function == "" {
		return
	}
	reclaimed := tc.arena.released > 0
	for _, loop := range tc.arena.loops {
		if loop.mark != "" {
			reclaimed = true
		}
	}
	tc.arena.calls = append(tc.arena.calls, arenaCall{caller: tc.arena.function, callee: callee, reclaimed: reclaimed})
}

// arenaMarkName is a fresh name for an arena mark.
func (tc *TypeChecker) arenaMarkName() string {
	tc.lowerSeq++
	return "rho_mark_" + strconv.Itoa(tc.lowerSeq)
}

// innermostLoopMark is the mark of the loop a `continue` continues, or "".
func (tc *TypeChecker) innermostLoopMark() string {
	if n := len(tc.arena.loops); n > 0 {
		return tc.arena.loops[n-1].mark
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Loops                                                                     */
/* -------------------------------------------------------------------------- */

// lowerLoop lowers a loop's body with the arena around it. It returns the
// instructions that go before the loop, the body to put in it, the
// instructions after it, and the mark of a condition that is released on its
// own (for a loop that is not reset). lower lowers the body, and is called with
// the loop pushed, so a `continue` in it knows where to release to.
func (tc *TypeChecker) lowerLoop(at ast.Position, kind string, body []ast.Stmt, condition ast.Expr, loopVar string, scope map[string]ir.Type, lower func() []ir.Instr) (before []ir.Instr, lowered []ir.Instr, after []ir.Instr, conditionMark string) {
	escapes := tc.loopEscapes(body, scope, loopVar)
	allocates := tc.stmtsAllocate(body, scope) || (condition != nil && tc.exprAllocates(condition, scope))
	reset := len(escapes) == 0 && allocates

	if len(escapes) > 0 && kind == "while" {
		// Reported only for a string that no enclosing loop releases: one kept in
		// a variable an enclosing loop's body declares lives until that loop's
		// iteration ends, and that loop is reset.
		for _, escape := range escapes {
			if tc.declaredByEnclosingLoop(escape) {
				continue
			}
			tc.arena.candidates = append(tc.arena.candidates, arenaCandidate{
				function: tc.arena.function,
				endless:  loopEndless(condition, body),
				escape: ArenaEscape{
					Module:   tc.arenaModule(),
					Function: tc.arena.name,
					Line:     at.Line,
					Column:   at.Column,
					Variable: escape,
				},
			})
			break
		}
	}

	mark := ""
	if reset {
		mark = tc.arenaMarkName()
	}
	tc.arena.loops = append(tc.arena.loops, arenaLoop{mark: mark, declared: declaredNames(body, loopVar)})
	lowered = lower()
	tc.arena.loops = tc.arena.loops[:len(tc.arena.loops)-1]

	if reset {
		before = []ir.Instr{{Op: ir.OpArenaMark, Dest: mark}}
		lowered = append(lowered, ir.Instr{Op: ir.OpArenaRelease, Dest: mark})
		after = []ir.Instr{{Op: ir.OpArenaRelease, Dest: mark}}
		return before, lowered, after, ""
	}
	if condition != nil && tc.exprAllocates(condition, scope) && !tc.passesReference(condition, scope) {
		conditionMark = tc.arenaMarkName()
		before = []ir.Instr{{Op: ir.OpArenaMark, Dest: conditionMark}}
	}
	return before, lowered, nil, conditionMark
}

// loopEndless is whether a `while` loop can never end: its condition is `true`,
// or reads no variable its body assigns and calls nothing, and its body has no
// `break` of its own and no `return`.
func loopEndless(condition ast.Expr, body []ast.Stmt) bool {
	if condition == nil {
		return false
	}
	read := map[string]bool{}
	calls := false
	visitExpr(condition, func(e ast.Expr) {
		switch v := e.(type) {
		case ast.IdentifierExpr:
			read[v.Name] = true
		case ast.CallExpr:
			calls = true
		}
	})
	if calls {
		return false
	}
	leaves := false
	var walk func(statements []ast.Stmt, nested bool)
	walk = func(statements []ast.Stmt, nested bool) {
		for _, statement := range statements {
			// A variable the condition reads, given by address, may be changed
			// through it.
			visitExprs(statement, func(e ast.Expr) {
				if unary, ok := e.(ast.UnaryExpr); ok && unary.Op == "&" && read[rootName(unary.Value)] {
					leaves = true
				}
			})
			switch stmt := statement.(type) {
			case ast.AssignStmt:
				if read[rootName(stmt.Target)] {
					leaves = true
				}
			case ast.ReturnStmt:
				leaves = true
			case ast.BreakStmt:
				if !nested {
					leaves = true
				}
			case ast.IfStmt:
				walk(stmt.Then, nested)
				walk(stmt.Else, nested)
			case ast.MatchStmt:
				for _, arm := range stmt.Arms {
					walk(arm.Body, nested)
				}
			case ast.BlockStmt:
				walk(stmt.Stmts, nested)
			case ast.WhileStmt:
				walk(stmt.Body, true)
			case ast.ForStmt:
				walk(stmt.Body, true)
			}
		}
	}
	walk(body, false)
	return !leaves
}

// declaredByEnclosingLoop is whether a name is declared in the body of a loop
// that encloses the one being lowered.
func (tc *TypeChecker) declaredByEnclosingLoop(name string) bool {
	for _, loop := range tc.arena.loops {
		if loop.declared[name] {
			return true
		}
	}
	return false
}

// declaredNames is every name a body declares, at any depth, and the loop's own
// variable.
func declaredNames(body []ast.Stmt, loopVar string) map[string]bool {
	names := map[string]bool{}
	if loopVar != "" {
		names[loopVar] = true
	}
	var walk func([]ast.Stmt)
	walk = func(statements []ast.Stmt) {
		for _, statement := range statements {
			switch stmt := statement.(type) {
			case ast.LetStmt:
				names[stmt.Name] = true
			case ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case ast.WhileStmt:
				walk(stmt.Body)
			case ast.ForStmt:
				names[stmt.Name] = true
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
	return names
}

// loopEscapes are the variables declared outside a loop's body that a string
// built in an iteration can reach: assigned one, given by address, or passed
// as a reference to a function Rho emits. Each is named once, in the order
// found. An empty answer means the loop may be released every iteration.
func (tc *TypeChecker) loopEscapes(body []ast.Stmt, scope map[string]ir.Type, loopVar string) []string {
	found := []string{}
	seen := map[string]bool{}
	note := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	}
	inner := map[string]bool{}
	if loopVar != "" {
		inner[loopVar] = true
	}
	tc.walkStatements(body, cloneScope(scope), inner, func(statement ast.Stmt, local map[string]ir.Type, declared map[string]bool) {
		outer := func(name string) bool { return name != "" && !declared[name] }
		visitExprs(statement, func(expr ast.Expr) {
			switch e := expr.(type) {
			case ast.UnaryExpr:
				// `&x`: whatever is written through it lands in x.
				if e.Op != "&" {
					return
				}
				root := rootName(e.Value)
				if !outer(root) {
					return
				}
				if typ, err := tc.exprType(e.Value, local); err != nil || tc.containsString(typ, 0) {
					if !tc.isExternArgument(statement, e, local) {
						note(root)
					}
				}
			case ast.CallExpr:
				// A reference passed to a function Rho emits may be written
				// through. A C function borrows what it is given.
				if tc.isExternCall(e, local) {
					return
				}
				for _, arg := range e.Args {
					if unary, isAddress := arg.(ast.UnaryExpr); isAddress && unary.Op == "&" {
						// The address of a value: the `&` case above decides.
						continue
					}
					typ, err := tc.exprType(arg, local)
					if err != nil || !tc.refersToStrings(typ, 0) {
						continue
					}
					// Only an array of the loop's own is storage of the loop's own: a
					// pointer, a slice or a struct carrying one may lead outside it.
					root := rootName(arg)
					if outer(root) || root == "" || !isArrayValue(local[root]) {
						if root == "" {
							root = "an argument"
						}
						note(root)
					}
				}
			}
		})
		assign, ok := statement.(ast.AssignStmt)
		if !ok {
			return
		}
		root := rootName(assign.Target)
		if root == "" {
			return
		}
		if !outer(root) && !tc.throughReference(assign.Target, local) {
			// Storage of the loop's own, not reached through a pointer or a
			// slice that may lead out of it.
			return
		}
		// A pointer deref, `*p = ...`, is a write into what p points at, which is
		// outside the loop whatever it is.
		target, err := tc.exprType(assign.Target, local)
		if err == nil && !tc.containsString(target, 0) {
			return
		}
		if tc.olderValue(assign.Value, declared) {
			return
		}
		note(root)
	})
	return found
}

// olderValue is whether a value is one that was there before the loop began, or
// a literal: a string the iteration did not build, which storing outside the loop
// does not keep alive.
func (tc *TypeChecker) olderValue(value ast.Expr, declared map[string]bool) bool {
	switch v := value.(type) {
	case ast.StringExpr, ast.NumberExpr, ast.BoolExpr:
		return true
	case ast.IdentifierExpr, ast.MemberExpr, ast.IndexExpr:
		root := rootName(v)
		if root == "" || declared[root] {
			return false
		}
		// An index is computed in the iteration, but what it selects is not.
		return true
	case ast.IfExpr:
		return tc.olderValue(v.Then, declared) && tc.olderValue(v.Else, declared)
	case ast.StructLiteralExpr:
		for _, field := range v.Values {
			if !tc.olderValue(field, declared) {
				return false
			}
		}
		return true
	case ast.CastExpr:
		return tc.olderValue(v.Value, declared)
	}
	return false
}

// isReference is whether a variable of a type is a pointer or a slice: storage
// somewhere else, which a write through it reaches.
func (tc *TypeChecker) isReference(typ ir.Type) bool {
	if typ.Kind == ir.TypePtr || typ.Kind == ir.TypeSlice {
		return true
	}
	_, isPointer := stripPointerSpelling(typ.Name)
	return isPointer
}

// isArrayValue is whether a type is a fixed array: storage where it is declared.
func isArrayValue(typ ir.Type) bool {
	if typ.ArrayLen > 0 {
		return true
	}
	_, _, named := nameArrayDims(typ.Name)
	return named && typ.Kind != ir.TypeSlice
}

// throughReference is whether an lvalue is reached through a pointer or a
// slice anywhere along it: `p.name`, `holder.ptr.name`, `items[i]` of a slice.
// What it writes is somewhere else, which may be outside the loop.
func (tc *TypeChecker) throughReference(target ast.Expr, scope map[string]ir.Type) bool {
	for {
		var object ast.Expr
		switch e := target.(type) {
		case ast.IdentifierExpr:
			return tc.isReference(scope[e.Name])
		case ast.MemberExpr:
			object = e.Object
		case ast.IndexExpr:
			object = e.Object
		case ast.UnaryExpr:
			if e.Op == "*" {
				return true
			}
			object = e.Value
		default:
			return true
		}
		if typ, err := tc.exprType(object, scope); err != nil || tc.isReference(typ) {
			return true
		}
		target = object
	}
}

// rootName is the variable an lvalue or a reference starts from: `a` in
// `a.b[i].c`, `p` in `*p`. "" when it starts from something that is not a
// variable, such as a call.
func rootName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case ast.IdentifierExpr:
			return e.Name
		case ast.MemberExpr:
			expr = e.Object
		case ast.IndexExpr:
			expr = e.Object
		case ast.UnaryExpr:
			if e.Op != "*" && e.Op != "&" {
				return ""
			}
			expr = e.Value
		case ast.CastExpr:
			expr = e.Value
		default:
			return ""
		}
	}
}

// isExternCall is whether a call names a C function: one a port or the
// platform supplies, which by convention copies what it keeps.
func (tc *TypeChecker) isExternCall(call ast.CallExpr, scope map[string]ir.Type) bool {
	switch callee := call.Callee.(type) {
	case ast.IdentifierExpr:
		if signature, found := tc.lookupFunc(callee.Name); found {
			return signature.extern
		}
		return tc.externNames[callee.Name]
	case ast.MemberExpr:
		if signature, named, err := tc.qualifiedCall(callee, scope); named && err == nil {
			return signature.extern
		}
	}
	return false
}

// isExternArgument is whether `&x` is written directly as an argument of a call
// to a C function.
func (tc *TypeChecker) isExternArgument(statement ast.Stmt, address ast.UnaryExpr, scope map[string]ir.Type) bool {
	extern := false
	visitExprs(statement, func(expr ast.Expr) {
		call, ok := expr.(ast.CallExpr)
		if !ok || !tc.isExternCall(call, scope) {
			return
		}
		for _, arg := range call.Args {
			if unary, isUnary := arg.(ast.UnaryExpr); isUnary && unary.Op == "&" && unary.Pos == address.Pos && rootName(unary.Value) == rootName(address.Value) {
				extern = true
			}
		}
	})
	return extern
}

/* -------------------------------------------------------------------------- */
/*  Statements                                                                */
/* -------------------------------------------------------------------------- */

// arenaWrap is how a statement's temporaries are released.
type arenaWrap int

const (
	// wrapNone leaves the statement as it is.
	wrapNone arenaWrap = iota
	// wrapRelease releases everything the statement built once it has run.
	wrapRelease
	// wrapKeep keeps the one string the statement stores and releases the rest.
	wrapKeep
)

// arenaWrapFor decides how the temporaries of a value are released, given the
// type it is stored as. An expression statement stores nothing, and passes the
// zero type.
func (tc *TypeChecker) arenaWrapFor(value ast.Expr, stored ir.Type, scope map[string]ir.Type) arenaWrap {
	if value == nil || !tc.exprAllocates(value, scope) || tc.passesReference(value, scope) {
		return wrapNone
	}
	if stored.Kind == ir.TypeInvalid || stored.Kind == ir.TypeVoid {
		return wrapRelease
	}
	valueType, err := tc.exprType(value, scope)
	if err != nil {
		return wrapNone
	}
	switch {
	case stored.Kind == ir.TypeString && valueType.Kind == ir.TypeString:
		return wrapKeep
	case !tc.containsString(valueType, 0) && !tc.containsString(stored, 0):
		// A number or a bool out of strings, nothing of which is kept. A string
		// stored as something else — a `Ptr[u8]` for a C call — is still that
		// string, and is not released.
		return wrapRelease
	}
	// A struct that holds strings could hold several the statement built, and
	// keeping one string cannot keep them all.
	return wrapNone
}

// arenaReleasing lowers a statement's value with the calls it makes noted as
// released, when the statement is.
func (tc *TypeChecker) arenaReleasing(wrap arenaWrap, lower func() string) string {
	if wrap == wrapNone {
		return lower()
	}
	tc.arena.released++
	defer func() { tc.arena.released-- }()
	return lower()
}

// arenaWrapped puts a statement's lowered instruction between a mark and what
// releases it.
func (tc *TypeChecker) arenaWrapped(wrap arenaWrap, instr ir.Instr) []ir.Instr {
	switch wrap {
	case wrapRelease:
		mark := tc.arenaMarkName()
		return []ir.Instr{{Op: ir.OpArenaMark, Dest: mark}, instr, {Op: ir.OpArenaRelease, Dest: mark}}
	case wrapKeep:
		mark := tc.arenaMarkName()
		instr.Arena = mark
		return []ir.Instr{{Op: ir.OpArenaMark, Dest: mark}, instr}
	}
	return []ir.Instr{instr}
}

// passesReference is whether an expression hands something a string could be
// written into to a function: an address of a value that holds strings, or a
// pointer, array or slice whose elements do. Such a statement may store a string
// it built where it outlives the statement, so its temporaries are left alone.
func (tc *TypeChecker) passesReference(expr ast.Expr, scope map[string]ir.Type) bool {
	passes := false
	visitExpr(expr, func(e ast.Expr) {
		switch v := e.(type) {
		case ast.UnaryExpr:
			if v.Op == "&" {
				if typ, err := tc.exprType(v.Value, scope); err != nil || tc.containsString(typ, 0) {
					passes = true
				}
			}
		case ast.CallExpr:
			for _, arg := range v.Args {
				if typ, err := tc.exprType(arg, scope); err == nil && tc.refersToStrings(typ, 0) {
					passes = true
				}
			}
		}
	})
	return passes
}

/* -------------------------------------------------------------------------- */
/*  What builds a string                                                      */
/* -------------------------------------------------------------------------- */

// exprAllocates is whether evaluating an expression may build a string in the
// arena: a concatenation, a call whose result holds a string, or a call to a
// function Rho emits that builds one and may leave it behind.
//
// It is what keeps the arena out of a program that never builds a string: a
// loop that toggles a pin is lowered exactly as it was, and a firmware that
// builds no string carries no arena.
func (tc *TypeChecker) exprAllocates(expr ast.Expr, scope map[string]ir.Type) bool {
	allocates := false
	var visit func(ast.Expr)
	visit = func(e ast.Expr) {
		if allocates || e == nil {
			return
		}
		switch v := e.(type) {
		case ast.BinaryExpr:
			if v.Op == "+" && tc.isStringExpr(v, scope) {
				allocates = true
				return
			}
			visit(v.Left)
			visit(v.Right)
		case ast.CallExpr:
			// `print(int_to_str(n))` prints the number without building it.
			if callee, ok := v.Callee.(ast.IdentifierExpr); ok && callee.Name == "print" && len(v.Args) == 1 {
				if inner, printsNumber := numberPrinted(v.Args[0]); printsNumber {
					visit(inner)
					return
				}
			}
			if callee, ok := v.Callee.(ast.IdentifierExpr); !ok || callee.Name != "print" {
				if typ, err := tc.exprType(v, scope); err == nil && tc.containsString(typ, 0) {
					allocates = true
					return
				}
			}
			if decl, found := tc.calledDecl(v, scope); found && tc.funcAllocates(decl) {
				allocates = true
				return
			}
			if member, ok := v.Callee.(ast.MemberExpr); ok {
				visit(member.Object)
			}
			for _, arg := range v.Args {
				visit(arg)
			}
		case ast.UnaryExpr:
			visit(v.Value)
		case ast.MemberExpr:
			visit(v.Object)
		case ast.IndexExpr:
			visit(v.Object)
			visit(v.Index)
		case ast.CastExpr:
			visit(v.Value)
		case ast.IfExpr:
			visit(v.Condition)
			visit(v.Then)
			visit(v.Else)
		case ast.StructLiteralExpr:
			for _, value := range v.Values {
				visit(value)
			}
		case ast.ArrayLiteralExpr:
			for _, value := range v.Values {
				visit(value)
			}
		}
	}
	visit(expr)
	return allocates
}

// numberPrinted is the number in `int_to_str(n)`, `i64_to_str(n)` and
// `string.from_i32(n)`, which `print` writes without building the string.
func numberPrinted(expr ast.Expr) (ast.Expr, bool) {
	call, ok := expr.(ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	switch callee := call.Callee.(type) {
	case ast.IdentifierExpr:
		if callee.Name == "int_to_str" || callee.Name == "i64_to_str" {
			return call.Args[0], true
		}
	case ast.MemberExpr:
		if object, isIdentifier := callee.Object.(ast.IdentifierExpr); isIdentifier && object.Name == "string" &&
			(callee.Name == "from_i32" || callee.Name == "from_u32") {
			return call.Args[0], true
		}
	}
	return nil, false
}

// stmtsAllocate is whether any statement in a body, at any depth, may build a
// string.
func (tc *TypeChecker) stmtsAllocate(body []ast.Stmt, scope map[string]ir.Type) bool {
	allocates := false
	tc.walkStatements(body, cloneScope(scope), map[string]bool{}, func(statement ast.Stmt, local map[string]ir.Type, _ map[string]bool) {
		if allocates {
			return
		}
		for _, expr := range statementExprs(statement) {
			if tc.exprAllocates(expr, local) {
				allocates = true
				return
			}
		}
	})
	return allocates
}

// calledDecl is the function Rho emits that a call names, when it names one.
func (tc *TypeChecker) calledDecl(call ast.CallExpr, scope map[string]ir.Type) (ast.FuncDecl, bool) {
	switch callee := call.Callee.(type) {
	case ast.IdentifierExpr:
		if _, intrinsic := intrinsicSignatures[callee.Name]; intrinsic {
			return ast.FuncDecl{}, false
		}
		if decl, generic := tc.genericFuncs[callee.Name]; generic {
			return decl, true
		}
		if signature, found := tc.lookupFunc(callee.Name); found && !signature.extern {
			decl, known := tc.arena.decls[signature.cName]
			return decl, known
		}
	case ast.MemberExpr:
		if signature, named, err := tc.qualifiedCall(callee, scope); named && err == nil && !signature.extern {
			decl, known := tc.arena.decls[signature.cName]
			return decl, known
		}
	}
	return ast.FuncDecl{}, false
}

// funcAllocates is whether a function may build a string, directly or through
// a function it calls. Recursion reads as "no" while it is being worked out,
// which the rest of the function decides.
func (tc *TypeChecker) funcAllocates(decl ast.FuncDecl) bool {
	if tc.arena.allocates == nil {
		tc.arena.allocates = map[string]int{}
	}
	key := qualifiedCName(decl.Owner, decl.Name)
	switch tc.arena.allocates[key] {
	case allocYes:
		return true
	case allocNo, allocPending:
		return false
	}
	tc.arena.allocates[key] = allocPending
	previousOwner := tc.owner
	tc.owner = decl.Owner
	scope := map[string]ir.Type{}
	for _, param := range decl.Params {
		scope[param.Name] = tc.typeFor(param.Type)
	}
	allocates := tc.stmtsAllocate(decl.Body, scope)
	tc.owner = previousOwner
	if allocates {
		tc.arena.allocates[key] = allocYes
	} else {
		tc.arena.allocates[key] = allocNo
	}
	return allocates
}

/* -------------------------------------------------------------------------- */
/*  Types                                                                     */
/* -------------------------------------------------------------------------- */

// containsString is whether a value of a type holds a string: is one, or is a
// struct or an array with one inside.
func (tc *TypeChecker) containsString(typ ir.Type, depth int) bool {
	if depth > 8 {
		return true
	}
	switch typ.Kind {
	case ir.TypeString:
		return true
	case ir.TypePtr:
		return false
	}
	if element, isArray := arrayElementOf(typ); isArray {
		return tc.containsString(element, depth+1)
	}
	if typ.Kind != ir.TypeStruct {
		return false
	}
	if _, isPointer := stripPointerSpelling(typ.Name); isPointer {
		return false
	}
	for _, field := range tc.structFields(typ.Name) {
		if tc.containsString(field, depth+1) {
			return true
		}
	}
	for _, arg := range typ.Args {
		if tc.containsString(arg, depth+1) {
			return true
		}
	}
	return false
}

// refersToStrings is whether a value of a type lets a function write a string
// somewhere the caller sees: a pointer, array or slice whose elements hold
// strings, or a struct carrying one.
func (tc *TypeChecker) refersToStrings(typ ir.Type, depth int) bool {
	if depth > 8 {
		return true
	}
	if typ.Kind == ir.TypePtr {
		if len(typ.Args) > 0 {
			return tc.containsString(typ.Args[0], depth+1)
		}
		if element, ok := stripPointerSpelling(typ.Name); ok {
			return tc.containsString(tc.typeFor(element), depth+1)
		}
		return false
	}
	if element, isArray := arrayElementOf(typ); isArray {
		return tc.containsString(element, depth+1) || tc.refersToStrings(element, depth+1)
	}
	if typ.Kind != ir.TypeStruct {
		return false
	}
	if element, isPointer := stripPointerSpelling(typ.Name); isPointer {
		return tc.containsString(tc.typeFor(element), depth+1)
	}
	for _, field := range tc.structFields(typ.Name) {
		if tc.refersToStrings(field, depth+1) {
			return true
		}
	}
	return false
}

// structFields are the field types of a struct, by any name it is known by.
func (tc *TypeChecker) structFields(name string) []ir.Type {
	fields, ok := tc.fields[name]
	if !ok {
		fields, ok = tc.fields[mangleTypeName(name)]
	}
	if !ok {
		if layout, laid := tc.monomorphicStructs[mangleTypeName(name)]; laid {
			out := make([]ir.Type, 0, len(layout.Fields))
			for _, field := range layout.Fields {
				out = append(out, field.Type)
			}
			return out
		}
		return nil
	}
	out := make([]ir.Type, 0, len(fields))
	for _, field := range fields {
		out = append(out, field)
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Walking                                                                   */
/* -------------------------------------------------------------------------- */

// walkStatements visits every statement of a body, at any depth, with the scope
// the statement sees and the names declared inside the body so far.
func (tc *TypeChecker) walkStatements(body []ast.Stmt, scope map[string]ir.Type, declared map[string]bool, visit func(ast.Stmt, map[string]ir.Type, map[string]bool)) {
	for _, statement := range body {
		visit(statement, scope, declared)
		switch stmt := statement.(type) {
		case ast.LetStmt:
			typ := ir.Type{}
			if written := stmt.Type.String(); written != "" {
				typ = tc.typeFor(written)
			} else if stmt.Value != nil {
				typ, _ = tc.exprType(stmt.Value, scope)
			}
			scope[stmt.Name] = typ
			declared[stmt.Name] = true
		case ast.IfStmt:
			tc.walkStatements(stmt.Then, cloneScope(scope), cloneNames(declared), visit)
			tc.walkStatements(stmt.Else, cloneScope(scope), cloneNames(declared), visit)
		case ast.WhileStmt:
			tc.walkStatements(stmt.Body, cloneScope(scope), cloneNames(declared), visit)
		case ast.ForStmt:
			inner := cloneScope(scope)
			if collection, err := tc.exprType(stmt.Collection, scope); err == nil {
				inner[stmt.Name], _ = arrayElementOf(collection)
			}
			names := cloneNames(declared)
			names[stmt.Name] = true
			tc.walkStatements(stmt.Body, inner, names, visit)
		case ast.MatchStmt:
			for _, arm := range stmt.Arms {
				tc.walkStatements(arm.Body, cloneScope(scope), cloneNames(declared), visit)
			}
		case ast.BlockStmt:
			tc.walkStatements(stmt.Stmts, cloneScope(scope), cloneNames(declared), visit)
		}
	}
}

func cloneNames(names map[string]bool) map[string]bool {
	out := make(map[string]bool, len(names))
	for name := range names {
		out[name] = true
	}
	return out
}

// statementExprs are the expressions a statement evaluates itself, not those of
// the statements nested in it.
func statementExprs(statement ast.Stmt) []ast.Expr {
	var out []ast.Expr
	add := func(e ast.Expr) {
		if e != nil {
			out = append(out, e)
		}
	}
	switch stmt := statement.(type) {
	case ast.LetStmt:
		add(stmt.Value)
	case ast.AssignStmt:
		add(stmt.Target)
		add(stmt.Value)
	case ast.ExprStmt:
		add(stmt.Expr)
	case ast.ReturnStmt:
		add(stmt.Value)
	case ast.IfStmt:
		add(stmt.Condition)
	case ast.WhileStmt:
		add(stmt.Condition)
	case ast.ForStmt:
		add(stmt.Collection)
		add(stmt.Step)
	case ast.MatchStmt:
		add(stmt.Value)
		for _, arm := range stmt.Arms {
			add(arm.Pattern)
		}
	}
	return out
}

// visitExprs visits every expression a statement evaluates itself.
func visitExprs(statement ast.Stmt, visit func(ast.Expr)) {
	for _, expr := range statementExprs(statement) {
		visitExpr(expr, visit)
	}
}

// visitExpr visits an expression and every expression inside it.
func visitExpr(expr ast.Expr, visit func(ast.Expr)) {
	if expr == nil {
		return
	}
	visit(expr)
	switch e := expr.(type) {
	case ast.UnaryExpr:
		visitExpr(e.Value, visit)
	case ast.BinaryExpr:
		visitExpr(e.Left, visit)
		visitExpr(e.Right, visit)
	case ast.CallExpr:
		visitExpr(e.Callee, visit)
		for _, arg := range e.Args {
			visitExpr(arg, visit)
		}
	case ast.MemberExpr:
		visitExpr(e.Object, visit)
	case ast.IndexExpr:
		visitExpr(e.Object, visit)
		visitExpr(e.Index, visit)
	case ast.CastExpr:
		visitExpr(e.Value, visit)
	case ast.StructLiteralExpr:
		for _, value := range e.Values {
			visitExpr(value, visit)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range e.Values {
			visitExpr(value, visit)
		}
		if e.Repeat != nil {
			visitExpr(*e.Repeat, visit)
		}
	case ast.IfExpr:
		visitExpr(e.Condition, visit)
		visitExpr(e.Then, visit)
		visitExpr(e.Else, visit)
	}
}
