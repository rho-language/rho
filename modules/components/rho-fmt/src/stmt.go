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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

// indent is one level of Rho source indentation.
const indent = "    "

// statementDepth is the depth of the statement being printed, which a function
// literal inside one indents its body from.
var statementDepth int

// printStmts writes a statement list, each on its own line, then the comments
// written before the brace that closes it, which is the token at close.
func printStmts(b *strings.Builder, list []ast.Stmt, depth, close int) {
	starts := stmtStarts(list)
	ends := unitEnds(starts, close)
	for i, stmt := range list {
		printStmt(b, stmt, depth, starts[i], ends[i])
	}
	flushBefore(b, close, depth, false)
}

// printStmt writes one statement, indented to the given depth. It begins at
// the token start and ends at the token end in the source, -1 when not known.
func printStmt(b *strings.Builder, stmt ast.Stmt, depth, start, end int) {
	pad := strings.Repeat(indent, depth)
	// A function literal in this statement indents its body from here.
	outer := statementDepth
	statementDepth = depth
	defer func() { statementDepth = outer }()

	flushBefore(b, start, depth, true)
	// A statement printed on one line has the comments written inside it
	// printed above it, unless a closure in it has a body for them.
	single := func() {
		if !hasClosure(start, end) {
			flushBefore(b, end, depth, false)
		}
	}
	// header prints the comments inside a statement's header above it.
	header := func(open int) {
		if !hasClosure(start, open) {
			flushBefore(b, open, depth, false)
		}
	}

	switch x := stmt.(type) {
	case ast.BlockStmt:
		// A nested block keeps its braces, since dropping them would change the
		// shape of the body.
		open := -1
		if kindAt(start) == lexer.TokenLBrace {
			open = start
		}
		b.WriteString(pad + "{")
		endLine(b, open, depth+1)
		printStmts(b, x.Stmts, depth+1, pairOf(open))
		b.WriteString(pad + "}")
		endLine(b, end, depth)

	case ast.LetStmt:
		single()
		keyword := "let"
		if x.Mutable {
			keyword = "var"
		}
		b.WriteString(pad + keyword + " " + x.Name)
		// The annotation is optional: a type-less binding takes its type from
		// the value, so it is only written when the source declared one.
		if x.Type.Name != "" || len(x.Type.Args) > 0 {
			b.WriteString(": " + typeWithArgs(x.Type))
		}
		if x.Value != nil {
			b.WriteString(" = ")
			printExpr(b, x.Value, 0)
		}
		endLine(b, end, depth)

	case ast.AssignStmt:
		single()
		b.WriteString(pad)
		printExpr(b, x.Target, 0)
		op := x.Op
		if op == "" {
			op = "="
		}
		b.WriteString(" " + op + " ")
		printExpr(b, x.Value, 0)
		endLine(b, end, depth)

	case ast.ExprStmt:
		single()
		b.WriteString(pad)
		printExpr(b, x.Expr, 0)
		endLine(b, end, depth)

	case ast.ReturnStmt:
		single()
		b.WriteString(pad + "return")
		if x.Value != nil {
			b.WriteString(" ")
			printExpr(b, x.Value, 0)
		}
		endLine(b, end, depth)

	case ast.BreakStmt:
		single()
		b.WriteString(pad + "break")
		endLine(b, end, depth)

	case ast.ContinueStmt:
		single()
		b.WriteString(pad + "continue")
		endLine(b, end, depth)

	case ast.ForStmt:
		open := braceFrom(start)
		header(open)
		b.WriteString(pad + "for " + x.Name + " in ")
		printExpr(b, x.Collection, 0)
		if x.Reverse {
			b.WriteString(" reverse")
		}
		if x.Step != nil {
			b.WriteString(" step ")
			printExpr(b, x.Step, 0)
		}
		b.WriteString(" {")
		endLine(b, open, depth+1)
		printStmts(b, x.Body, depth+1, pairOf(open))
		b.WriteString(pad + "}")
		endLine(b, end, depth)

	case ast.MatchStmt:
		// `if let` is a match of one arm and a wildcard, and is printed as it
		// was written.
		if x.IfLet && len(x.Arms) == 2 && !x.Arms[0].Wildcard && x.Arms[1].Wildcard {
			open := braceFrom(findFrom(start, end, lexer.TokenAssign, "="))
			header(open)
			b.WriteString(pad + "if let ")
			printExpr(b, x.Arms[0].Pattern, 0)
			b.WriteString(" = ")
			printExpr(b, x.Value, 0)
			b.WriteString(" {")
			endLine(b, open, depth+1)
			printStmts(b, x.Arms[0].Body, depth+1, pairOf(open))
			printElse(b, x.Arms[1].Body, depth, pairOf(open), end)
			return
		}
		open := braceFrom(start)
		close := pairOf(open)
		header(open)
		b.WriteString(pad + "match ")
		printExpr(b, x.Value, 0)
		b.WriteString(" {")
		endLine(b, open, depth+1)
		starts := make([]int, len(x.Arms))
		for i, arm := range x.Arms {
			starts[i] = tokOf(arm.Pos)
		}
		ends := unitEnds(starts, close)
		for i, arm := range x.Arms {
			printArm(b, arm, depth, starts[i], ends[i])
		}
		flushBefore(b, close, depth+1, false)
		b.WriteString(pad + "}")
		endLine(b, end, depth)

	case ast.IfStmt:
		open := braceFrom(start)
		header(open)
		b.WriteString(pad + "if ")
		printExpr(b, x.Condition, 0)
		b.WriteString(" {")
		endLine(b, open, depth+1)
		printStmts(b, x.Then, depth+1, pairOf(open))
		printElse(b, x.Else, depth, pairOf(open), end)

	case ast.WhileStmt:
		open := braceFrom(start)
		header(open)
		b.WriteString(pad + "while ")
		printExpr(b, x.Condition, 0)
		b.WriteString(" {")
		endLine(b, open, depth+1)
		printStmts(b, x.Body, depth+1, pairOf(open))
		b.WriteString(pad + "}")
		endLine(b, end, depth)

	default:
		// An unrecognised statement is reported rather than dropped.
		b.WriteString(pad + "/* unprinted statement */\n")
	}
}

// printElse closes an if's first block, whose closing brace is the token at
// thenClose, and prints its else: a block, or an `else if`, which is printed
// as an if inside the else block. end is where the whole statement ends.
func printElse(b *strings.Builder, body []ast.Stmt, depth, thenClose, end int) {
	pad := strings.Repeat(indent, depth)
	if len(body) == 0 {
		b.WriteString(pad + "}")
		endLine(b, end, depth)
		return
	}
	elseTok := -1
	if thenClose >= 0 && kindAt(thenClose+1) == lexer.TokenElse {
		elseTok = thenClose + 1
	}
	if elseTok >= 0 && kindAt(elseTok+1) == lexer.TokenIf && len(body) == 1 {
		// `else if`: the inner if is the rest of the statement.
		b.WriteString(pad + "} else {\n")
		printStmt(b, body[0], depth+1, elseTok+1, end)
		b.WriteString(pad + "}\n")
		return
	}
	open := -1
	if elseTok >= 0 && kindAt(elseTok+1) == lexer.TokenLBrace {
		open = elseTok + 1
	}
	b.WriteString(pad + "} else {")
	endLine(b, open, depth+1)
	printStmts(b, body, depth+1, pairOf(open))
	b.WriteString(pad + "}")
	endLine(b, end, depth)
}

// printArm prints one arm of a match, which begins at the token start and ends
// at the token end. An arm written as one expression is printed as a block.
func printArm(b *strings.Builder, arm ast.MatchArm, depth, start, end int) {
	pad := strings.Repeat(indent, depth+1)
	flushBefore(b, start, depth+1, true)
	arrow := findFrom(start, end, lexer.TokenFatArrow, "")
	open := -1
	if arrow >= 0 && kindAt(arrow+1) == lexer.TokenLBrace {
		open = arrow + 1
	}
	// The comments inside the pattern go above the arm.
	if open >= 0 {
		flushBefore(b, open, depth+1, false)
	} else if arrow >= 0 {
		flushBefore(b, arrow+1, depth+1, false)
	}
	b.WriteString(pad)
	if arm.Wildcard || arm.Pattern == nil {
		b.WriteString("_")
	} else {
		printExpr(b, arm.Pattern, 0)
	}
	b.WriteString(" => {")
	if open >= 0 || arrow < 0 || len(arm.Body) != 1 {
		endLine(b, open, depth+2)
		printStmts(b, arm.Body, depth+2, pairOf(open))
		b.WriteString(pad + "}")
		endLine(b, end, depth+1)
		return
	}
	// `pattern => expression`: the expression is the arm's one statement, and
	// the rest of the arm.
	b.WriteString("\n")
	printStmt(b, arm.Body[0], depth+2, arrow+1, end)
	b.WriteString(pad + "}\n")
}
