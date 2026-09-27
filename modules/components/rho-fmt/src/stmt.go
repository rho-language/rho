//
// @file stmt.go
// @brief Statement printing for Rho.
//
// Bodies are part of the program, so a formatter has to print them. Dropping a
// body is not a formatting choice: it is data loss, and it is why the previous
// formatter could not be run over real sources.
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

// indent is one level of Rho source indentation.
const indent = "    "

// printStmts writes a statement list, each on its own line.
func printStmts(b *strings.Builder, list []ast.Stmt, depth int) {
	for _, stmt := range list {
		printStmt(b, stmt, depth)
	}
}

// printStmt writes one statement, indented to the given depth.
func printStmt(b *strings.Builder, stmt ast.Stmt, depth int) {
	pad := strings.Repeat(indent, depth)

	switch x := stmt.(type) {
	case ast.BlockStmt:
		// A nested block keeps its braces, since dropping them would change the
		// shape of the body.
		b.WriteString(pad + "{\n")
		printStmts(b, x.Stmts, depth+1)
		b.WriteString(pad + "}\n")

	case ast.LetStmt:
		b.WriteString(pad + "let " + x.Name)
		// The annotation is optional: a type-less binding takes its type from
		// the value, so it is only written when the source declared one.
		if x.Type.Name != "" || len(x.Type.Args) > 0 {
			b.WriteString(": " + typeWithArgs(x.Type))
		}
		if x.Value != nil {
			b.WriteString(" = ")
			printExpr(b, x.Value, 0)
		}
		b.WriteString("\n")

	case ast.AssignStmt:
		b.WriteString(pad)
		printExpr(b, x.Target, 0)
		op := x.Op
		if op == "" {
			op = "="
		}
		b.WriteString(" " + op + " ")
		printExpr(b, x.Value, 0)
		b.WriteString("\n")

	case ast.ExprStmt:
		b.WriteString(pad)
		printExpr(b, x.Expr, 0)
		b.WriteString("\n")

	case ast.ReturnStmt:
		b.WriteString(pad + "return")
		if x.Value != nil {
			b.WriteString(" ")
			printExpr(b, x.Value, 0)
		}
		b.WriteString("\n")

	case ast.BreakStmt:
		b.WriteString(pad + "break\n")

	case ast.IfStmt:
		b.WriteString(pad + "if ")
		printExpr(b, x.Condition, 0)
		b.WriteString(" {\n")
		printStmts(b, x.Then, depth+1)
		if len(x.Else) > 0 {
			b.WriteString(pad + "} else {\n")
			printStmts(b, x.Else, depth+1)
		}
		b.WriteString(pad + "}\n")

	case ast.WhileStmt:
		b.WriteString(pad + "while ")
		printExpr(b, x.Condition, 0)
		b.WriteString(" {\n")
		printStmts(b, x.Body, depth+1)
		b.WriteString(pad + "}\n")

	default:
		// An unrecognised statement is reported rather than dropped.
		b.WriteString(pad + "/* unprinted statement */\n")
	}
}
