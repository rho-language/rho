//
// @file expr.go
// @brief Expression and statement printing for Rho.
//
// The AST carries every expression and statement the parser understands, so a
// formatter that prints all of them round-trips: re-parsing formatted source
// yields the same tree. A formatter that prints only declarations silently
// drops code, which is worse than not formatting at all.
//
// Parentheses are added where an expression's shape would otherwise change on a
// second parse, so precedence survives the round trip.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"strings"

	"rho/src/frontend/ast"
)

// precedence returns the binding strength of an expression, mirroring the
// parser's own table.
//
// It has to match exactly, or a formatted file parses to a different tree than
// the one it was printed from. Two details matter: the shifts share a level with
// the comparisons, because the lexer emits ">>" as the greater-than token, and
// the right-hand operand of a binary is parsed one level tighter, which is why a
// right operand of equal strength still needs parentheses.
func precedence(e ast.Expr) int {
	switch x := e.(type) {
	case ast.BinaryExpr:
		switch x.Op {
		case "||":
			return 1
		case "&&":
			return 2
		case "==", "!=":
			return 3
		case "<", "<=", ">", ">=", "<<", ">>":
			return 4
		case "+", "-", "|", "^":
			return 5
		case "&":
			return 6
		case "*", "/", "%":
			return 7
		default:
			return 4
		}
	case ast.UnaryExpr:
		// A unary binds tighter than any binary but looser than a call.
		return 6
	case ast.CallExpr, ast.IndexExpr, ast.MemberExpr, ast.CastExpr, ast.StructLiteralExpr:
		return 7
	case ast.IfExpr:
		return 0
	default:
		// Names, literals and grouping parentheses never need their own.
		return 8
	}
}

// printExpr writes an expression, adding parentheses when the surrounding
// context binds more loosely than the expression itself.
func printExpr(b *strings.Builder, e ast.Expr, context int) {
	p := precedence(e)
	needs := p < context
	if needs {
		b.WriteString("(")
	}
	printExprBare(b, e)
	if needs {
		b.WriteString(")")
	}
}

// printExprBare writes an expression without adding parentheses.
func printExprBare(b *strings.Builder, e ast.Expr) {
	switch x := e.(type) {
	case nil:
		return

	case ast.IdentifierExpr:
		b.WriteString(x.Name)

	case ast.NumberExpr:
		b.WriteString(x.Value)

	case ast.StringExpr:
		// A string is re-quoted rather than copied, so an escape in the source
		// survives a round trip instead of being read literally.
		b.WriteString(quoteRhoString(x.Value))

	case ast.BoolExpr:
		if x.Value {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}

	case ast.UnaryExpr:
		b.WriteString(x.Op)
		// A unary operand must not absorb the operator itself.
		printExpr(b, x.Value, 6)

	case ast.BinaryExpr:
		own := precedence(x)
		// Both sides are printed against the operator's own strength, so a chain
		// such as a + b + c keeps its grouping.
		printExpr(b, x.Left, own)
		b.WriteString(" " + x.Op + " ")
		// The right side is printed one step tighter, so `a - (b + c)` keeps
		// its parentheses and `a + b * c` does not gain any.
		printExpr(b, x.Right, own+1)

	case ast.CallExpr:
		printExpr(b, x.Callee, 7)
		b.WriteString("(")
		printArguments(b, x.TypeArgs, x.Args)
		b.WriteString(")")

	case ast.MemberExpr:
		printExpr(b, x.Object, 7)
		b.WriteString(".")
		b.WriteString(x.Name)

	case ast.IndexExpr:
		printExpr(b, x.Object, 7)
		b.WriteString("[")
		printExpr(b, x.Index, 0)
		b.WriteString("]")

	case ast.CastExpr:
		// The cast is written `value as T`, which is what the parser reads. A
		// C-style `(T) value` would come back as a call to T, so it is not
		// interchangeable here.
		printExpr(b, x.Value, 6)
		b.WriteString(" as " + x.Type.String())

	case ast.StructLiteralExpr:
		b.WriteString(typeWithArgs(x.Type))
		b.WriteString("{ ")
		printExprList(b, x.Values)
		b.WriteString(" }")

	case ast.ArrayLiteralExpr:
		b.WriteString("[ ")
		printExprList(b, x.Values)
		b.WriteString(" ]")

	case ast.IfExpr:
		b.WriteString("if ")
		printExpr(b, x.Condition, 0)
		b.WriteString(" { ")
		printExpr(b, x.Then, 0)
		b.WriteString(" } else { ")
		printExpr(b, x.Else, 0)
		b.WriteString(" }")

	default:
		// An expression kind this formatter does not know yet must not be
		// dropped silently.
		b.WriteString("/* unprinted expression */")
	}
}

// printArguments writes a call's type arguments followed by its arguments.
func printArguments(b *strings.Builder, typeArgs []ast.TypeExpr, args []ast.Expr) {
	first := true
	for _, ta := range typeArgs {
		if !first {
			b.WriteString(", ")
		}
		b.WriteString(typeWithArgs(ta))
		first = false
	}
	for _, arg := range args {
		if !first {
			b.WriteString(", ")
		}
		printExpr(b, arg, 0)
		first = false
	}
}

// printExprList writes a comma-separated expression list.
func printExprList(b *strings.Builder, list []ast.Expr) {
	for i, e := range list {
		if i > 0 {
			b.WriteString(", ")
		}
		printExpr(b, e, 0)
	}
}

// typeWithArgs renders a type together with any type arguments, as in
// Result[u32, u32].
func typeWithArgs(t ast.TypeExpr) string {
	if len(t.Args) == 0 {
		return t.Name
	}
	parts := make([]string, 0, len(t.Args))
	for _, arg := range t.Args {
		parts = append(parts, arg.String())
	}
	return t.Name + "[" + strings.Join(parts, ", ") + "]"
}

// quoteRhoString re-quotes a string literal's contents.
//
// The AST holds the decoded text, so a quote or a backslash inside it has to be
// escaped again. Without this a source containing "\\" would be re-emitted as
// a single backslash and the next parse would read it as an escape.
func quoteRhoString(value string) string {
	var b strings.Builder
	b.WriteString("\"")
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		default:
			b.WriteByte(value[i])
		}
	}
	b.WriteString("\"")
	return b.String()
}
