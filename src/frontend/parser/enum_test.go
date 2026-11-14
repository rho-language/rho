//
// @file enum_test.go
// @brief How an enum, a pattern and `if let` are read.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package parser

import (
	"testing"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

func parseSource(t *testing.T, source string) *ast.Module {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	module, err := ParseFile(tokens, "main.rho")
	if err != nil {
		t.Fatal(err)
	}
	return module
}

// Every variant form is read: plain, numbered (negative too), counted on from
// the one before, tuple-like and struct-like, and a generic enum's parameters
// with their defaults.
func TestAnEnumReadsEveryVariantForm(t *testing.T) {
	module := parseSource(t, `module app

enum Level { Low = -2, Mid, High = 0x10, Top }

enum Result[T, E = Unit] {
    Err(E)
    Ok(T)
    Pair(u8, string),
    Point { x: i32, y: i32 }; Empty
}
`)
	level := module.Decls[0].(ast.EnumDecl)
	values := []int{-2, -1, 16, 17}
	for i, variant := range level.Variants {
		if variant.Value != values[i] {
			t.Errorf("%s is %d, want %d", variant.Name, variant.Value, values[i])
		}
	}
	if !level.Variants[0].Explicit || level.Variants[1].Explicit {
		t.Error("only a written number is explicit")
	}
	if level.CarriesData() || !level.Pos.Valid() {
		t.Errorf("a plain enum carries no data and is placed: %+v", level)
	}

	result := module.Decls[1].(ast.EnumDecl)
	if len(result.TypeParams) != 2 || result.TypeParamDefaults["E"] != "Unit" {
		t.Fatalf("type parameters: %v %v", result.TypeParams, result.TypeParamDefaults)
	}
	if !result.CarriesData() || len(result.Variants) != 5 {
		t.Fatalf("variants: %+v", result.Variants)
	}
	pair, point, empty := result.Variants[2], result.Variants[3], result.Variants[4]
	if len(pair.Fields) != 2 || pair.Named || pair.Fields[1].Type != "string" {
		t.Errorf("a tuple-like variant: %+v", pair)
	}
	if len(point.Fields) != 2 || !point.Named || point.Fields[1].Name != "y" {
		t.Errorf("a struct-like variant: %+v", point)
	}
	if len(empty.Fields) != 0 || empty.Name != "Empty" {
		t.Errorf("a variant after a semicolon: %+v", empty)
	}
}

// A variant of a generic enum may be named with its arguments, and `if let`
// is a match of one arm whose else is the wildcard.
func TestIfLetAndAVariantWithTypeArguments(t *testing.T) {
	module := parseSource(t, `module app

fn main() void {
    let none = Option[u32].None
    if let Shape.Rect { w, h } = shape() {
        print("rect")
    } else if let Some(x) = find() {
        print("found")
    }
}
`)
	body := module.Decls[0].(ast.FuncDecl).Body
	member := body[0].(ast.LetStmt).Value.(ast.MemberExpr)
	if member.Name != "None" || len(member.TypeArgs) != 1 || member.TypeArgs[0].Name != "u32" {
		t.Errorf("Option[u32].None: %+v", member)
	}
	match := body[1].(ast.MatchStmt)
	if !match.IfLet || len(match.Arms) != 2 || !match.Arms[1].Wildcard {
		t.Fatalf("if let: %+v", match)
	}
	pattern := match.Arms[0].Pattern.(ast.StructLiteralExpr)
	if pattern.Type.Name != "Shape.Rect" || len(pattern.Values) != 2 {
		t.Errorf("a struct-like pattern: %+v", pattern)
	}
	inner := match.Arms[1].Body[0].(ast.MatchStmt)
	if !inner.IfLet {
		t.Errorf("else if let: %+v", inner)
	}
}
