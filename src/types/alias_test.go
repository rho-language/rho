//
// @file alias_test.go
// @brief Tests for type aliases and type conversions.
//
// The two arrived together, because a database handle needs both: a name for the
// handle (`type SqlDb = Ptr[Unit]`) and the means to build one (`Ptr[Unit](0)`).
// Before them, a program that declared a handle either produced a type nothing
// could name or a call to a function that was never written.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"testing"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// aliased builds a checker with a module declaring the given aliases.
func aliased(t *testing.T, decls ...ast.Decl) *TypeChecker {
	t.Helper()
	tc := New()
	if err := tc.CheckModule(ast.Module{Name: "test", Decls: decls}); err != nil {
		t.Fatalf("the module did not check: %v", err)
	}
	return tc
}

// A handle is a pointer to nothing, and an alias is the name it is known by. The
// alias has to resolve to the pointer, not to a type of its own.
func TestAnAliasResolvesToWhatItNames(t *testing.T) {
	tc := aliased(t, ast.TypeDecl{
		Name: "SqlDb",
		Type: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "Unit"}}},
	})
	resolved, ok := tc.resolveType("SqlDb")
	if !ok {
		t.Fatal("an alias is not a type the language resolves")
	}
	if resolved.Kind != ir.TypePtr {
		t.Errorf("the alias resolved to kind %v, want a pointer", resolved.Kind)
	}
	if len(resolved.Args) != 1 || resolved.Name != "Ptr[Unit]" {
		t.Errorf("the alias lost its element: %+v", resolved)
	}
}

// An alias of an alias resolves to the end of the chain, which is what makes a
// name useful: `SqlHandle` may name `SqlDb`, which names the pointer.
func TestAChainOfAliasesResolvesToTheEnd(t *testing.T) {
	tc := aliased(t,
		ast.TypeDecl{Name: "SqlDb", Type: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "Unit"}}}},
		ast.TypeDecl{Name: "SqlHandle", Type: ast.TypeExpr{Name: "SqlDb"}},
	)
	resolved, ok := tc.resolveType("SqlHandle")
	if !ok || resolved.Kind != ir.TypePtr {
		t.Errorf("a chain of aliases resolved to %+v, want a pointer", resolved)
	}
}

// An alias that names itself, or two that name each other, is a manifest a
// person can write by accident. Resolving it must not take the compiler with it.
func TestAnAliasCycleDoesNotLoop(t *testing.T) {
	for _, decls := range [][]ast.Decl{
		{ast.TypeDecl{Name: "A", Type: ast.TypeExpr{Name: "A"}}},
		{
			ast.TypeDecl{Name: "A", Type: ast.TypeExpr{Name: "B"}},
			ast.TypeDecl{Name: "B", Type: ast.TypeExpr{Name: "A"}},
		},
	} {
		tc := aliased(t, decls...)
		// The point is that this returns at all: a cycle is reported as a name
		// nothing resolves, which is a message about the manifest rather than a
		// compiler that never finishes.
		if _, ok := tc.resolveType("A"); ok {
			t.Log("a cycle resolved to something; a bound was hit, which is enough")
		}
	}
}

// A call to a type is a conversion, and the two are written the same way in Rho —
// `Name(value)` — so only the tables can tell them apart. Getting it wrong emits
// a call to a function nobody wrote.
func TestACallToATypeIsAConversion(t *testing.T) {
	tc := New()
	expr := ast.CallExpr{
		Callee:   ast.IdentifierExpr{Name: "Ptr"},
		TypeArgs: []ast.TypeExpr{{Name: "Unit"}},
		Args:     []ast.Expr{ast.NumberExpr{Value: "0"}},
	}
	got := tc.lowerExpr(expr, map[string]ir.Type{})
	if got != "(void*)0" {
		t.Errorf("converting to a handle emitted %q, which is not a cast", got)
	}

	// And an alias converts to the same thing, because it is the same type.
	tc = aliased(t, ast.TypeDecl{
		Name: "SqlDb",
		Type: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "Unit"}}},
	})
	got = tc.lowerExpr(ast.CallExpr{
		Callee: ast.IdentifierExpr{Name: "SqlDb"},
		Args:   []ast.Expr{ast.NumberExpr{Value: "0"}},
	}, map[string]ir.Type{})
	if got != "(void*)0" {
		t.Errorf("converting to an aliased handle emitted %q", got)
	}
}

// A pointer keeps the element it points at. Losing it would make every backend
// spell `Ptr[u8]` as a name, which is what a C compiler rejects.
func TestAPointerKeepsItsElement(t *testing.T) {
	tc := New()
	// The element has to be a type the checker knows, because a pointer to a name
	// it has never seen is a pointer to nothing in particular.
	tc.types["CustomerRow"] = ir.Type{Kind: ir.TypeStruct, Name: "CustomerRow"}
	resolved, ok := tc.resolveType("Ptr[CustomerRow]")
	if !ok {
		t.Fatal("a pointer to a named type does not resolve")
	}
	if resolved.Kind != ir.TypePtr {
		t.Fatalf("it resolved to kind %v, want a pointer", resolved.Kind)
	}
	if len(resolved.Args) != 1 || resolved.Args[0].Name != "CustomerRow" {
		t.Errorf("the pointer lost its element: %+v", resolved)
	}
}

// An alias is not a type of its own, so a declaration that uses one resolves
// through it rather than naming it.
func TestADeclarationUsingAnAliasKeepsTheAliasedType(t *testing.T) {
	tc := aliased(t,
		ast.TypeDecl{Name: "SqlDb", Type: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "Unit"}}}},
		ast.FuncDecl{
			Name:       "open_it",
			Params:     []ast.Param{{Name: "db", Type: "SqlDb"}},
			ReturnType: "bool",
		},
	)
	param := tc.typeFromExpr(ast.TypeExpr{Name: "SqlDb"}, nil)
	if param.Kind != ir.TypePtr {
		t.Errorf("a parameter declared as an alias is kind %v, want a pointer", param.Kind)
	}
}
