package parser

import (
	"strings"
	"testing"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

// bodyOf parses one function and returns its statements.
func bodyOf(t *testing.T, body string) []ast.Stmt {
	t.Helper()
	tokens, err := lexer.Lex("module app\n\nfn main() void {\n" + body + "\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	module, err := Parse(tokens)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return module.Decls[0].(ast.FuncDecl).Body
}

// FINDINGS #9: a newline ends a statement where it can end, as in Go. A line
// that starts with `(` or `[` begins a new statement rather than calling or
// indexing the line above.
func TestALineThatStartsWithAParenthesisStartsAStatement(t *testing.T) {
	body := bodyOf(t, "    x = fields\n    (*s).f = v\n    y = list\n    [1, 2][0] = 3")
	if len(body) != 4 {
		t.Fatalf("%d statements, want 4: %#v", len(body), body)
	}
	if first, ok := body[0].(ast.AssignStmt); !ok {
		t.Fatalf("first statement is %T", body[0])
	} else if _, isName := first.Value.(ast.IdentifierExpr); !isName {
		t.Fatalf("the value of the first assignment is %T, want the name `fields`", first.Value)
	}
	if second, ok := body[1].(ast.AssignStmt); !ok {
		t.Fatalf("second statement is %T", body[1])
	} else if _, isMember := second.Target.(ast.MemberExpr); !isMember {
		t.Fatalf("the second assignment's target is %T", second.Target)
	}
}

// A binary operator continues a line only at the end of it; at the start of
// the next one it is an error that says so.
func TestABinaryOperatorContinuesOnlyAtTheEndOfALine(t *testing.T) {
	body := bodyOf(t, "    let a: u32 = 1 +\n        2\n    let b: bool = a > 0 &&\n        a < 9")
	if len(body) != 2 {
		t.Fatalf("%d statements, want 2", len(body))
	}
	tokens, _ := lexer.Lex("module app\n\nfn main() void {\n    let a: u32 = 1\n        + 2\n}\n")
	_, err := Parse(tokens)
	if err == nil || !strings.Contains(err.Error(), "end the line above with the operator") {
		t.Fatalf("a leading operator: %v", err)
	}
}

// A `return` at the end of its line returns nothing.
func TestAReturnAtTheEndOfALineReturnsNothing(t *testing.T) {
	body := bodyOf(t, "    return\n    f(x)")
	if len(body) != 2 {
		t.Fatalf("%d statements, want 2", len(body))
	}
	if ret, ok := body[0].(ast.ReturnStmt); !ok || ret.Value != nil {
		t.Fatalf("first statement is %#v, want a bare return", body[0])
	}
	// A method-style chain that starts a line with `.` still continues it.
	chain := bodyOf(t, "    let n: u32 = s\n        .len")
	if len(chain) != 1 {
		t.Fatalf("a line starting with . : %d statements, want 1", len(chain))
	}
}

// A struct literal is an expression wherever it is delimited, a condition's
// parentheses and call arguments included; the brace after a condition is
// still the body.
func TestAStructLiteralInsideAConditionsParentheses(t *testing.T) {
	body := bodyOf(t, "    if sum(P { 1, 1 }) == 2 {\n        x = 1\n    }\n    if (P { 1, 2 }).x == 1 {\n        x = 2\n    }\n    while n < MAX {\n        n = n + 1\n    }")
	if len(body) != 3 {
		t.Fatalf("%d statements, want 3", len(body))
	}
	first := body[0].(ast.IfStmt)
	call := first.Condition.(ast.BinaryExpr).Left.(ast.CallExpr)
	if _, ok := call.Args[0].(ast.StructLiteralExpr); !ok {
		t.Fatalf("the argument is %T, want a struct literal", call.Args[0])
	}
	if len(first.Then) != 1 {
		t.Fatalf("the body has %d statements", len(first.Then))
	}
}

// FINDINGS #8: `mod.f[T](...)` is an application of another module's generic
// function; `arr[u32(k)]` is an index by a conversion.
func TestAQualifiedApplicationAndAnIndexByAConversion(t *testing.T) {
	body := bodyOf(t, "    slices.sort_by[G](xs, less)\n    arr[u32(k % 3)] = 1\n    s.items[i] = 2")
	call, ok := body[0].(ast.ExprStmt).Expr.(ast.CallExpr)
	if !ok || len(call.TypeArgs) != 1 || call.TypeArgs[0].Name != "G" {
		t.Fatalf("first statement is %#v", body[0])
	}
	if _, ok := call.Callee.(ast.MemberExpr); !ok {
		t.Fatalf("the callee is %T", call.Callee)
	}
	index, ok := body[1].(ast.AssignStmt).Target.(ast.IndexExpr)
	if !ok || len(index.TypeArgs) != 0 {
		t.Fatalf("second target is %#v", body[1].(ast.AssignStmt).Target)
	}
	if _, ok := index.Index.(ast.CallExpr); !ok {
		t.Fatalf("the index is %T, want the conversion", index.Index)
	}
}
