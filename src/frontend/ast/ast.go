//
// @file ast.go
// @brief Rho Compiler - Abstract Syntax Tree (AST).
//
// This file defines the Abstract Syntax Tree structures used by the Rho
// compiler. The AST is produced by the parser and consumed by the type
// checker and IR generator. All nodes are deterministic and designed to
// map directly into the Rho → IR → C pipeline.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package ast

import "fmt"

/* -------------------------------------------------------------------------- */
/*  Base Types                                                                */
/* -------------------------------------------------------------------------- */

// Position is where something is in a source file.
//
// It carries all three ways of saying so because all three are asked for: a byte
// offset to cut at, a line and column to show a person, and nothing at all for
// something whose place was never known. Valid is what tells those apart — the
// first byte of a file is at 0:1, and a node that was never placed is at 0:0.
type Position struct {
	// Offset is the byte offset into the source.
	Offset int
	// Line and Column are where the thing begins, counting from one. A column
	// counts characters and not bytes, so it lines up with what an editor shows.
	Line   int
	Column int
}

// Valid reports whether a position is known.
func (p Position) Valid() bool { return p.Line > 0 }

// String is a position the way a compiler writes one.
func (p Position) String() string {
	if !p.Valid() {
		return "unknown position"
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Node is what every AST node carries in common.
//
// It is embedded rather than referenced so that every node is positioned without
// anything having to ask. A position that has to be threaded in is one that is
// missing from the nodes nobody remembered, and a report that cannot point at a
// node is a report a person cannot act on.
type Node struct {
	Pos Position
}

/* -------------------------------------------------------------------------- */
/*  Module                                                                    */
/* -------------------------------------------------------------------------- */

// Module is the root AST node for a Rho source file.
type Module struct {
	Node
	Name     string
	Imports  []Import
	Includes []Include
	Features []string
	Decls    []Decl
}

// TypeExpr represents a named type with optional generic arguments and array
// dimensions, for example Slice[u8] or u8[8].
type TypeExpr struct {
	Name     string
	Args     []TypeExpr
	ArrayLen *int
}

func (t TypeExpr) String() string {
	name := t.Name
	if len(t.Args) > 0 {
		name += "["
		for i, arg := range t.Args {
			if i > 0 {
				name += ", "
			}
			name += arg.String()
		}
		name += "]"
	}
	if t.ArrayLen != nil {
		name += fmt.Sprintf("[%d]", *t.ArrayLen)
	}
	return name
}

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

// Import represents a single `import xyz` declaration.
type Import struct {
	Node
	Name string
}

// Include represents a source file included relative to the current file.
type Include struct {
	Node
	Path string
}

/* -------------------------------------------------------------------------- */
/*  Declarations (Top-Level)                                                  */
/* -------------------------------------------------------------------------- */

// Decl is the interface implemented by all top-level declarations.
type Decl interface {
	declNode()
	// Position is where it was written, so a report about it can say where it is
	// and not only what it is.
	Position() Position
}

type TypeDecl struct {
	Node
	Name string
	Type TypeExpr
}

func (TypeDecl) declNode() {}

func (n TypeDecl) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  HTTP Routes                                                               */
/* -------------------------------------------------------------------------- */

// RouteDecl exposes a Rho function over HTTP, as in
//
//	route GET "/api/v1/modules/{name}" => get_module
//
// The path syntax is Go's http.ServeMux pattern syntax, so the Go backend passes
// it through unchanged. Routes are meaningful only for a server target.
type RouteDecl struct {
	Node
	// Method is the HTTP method: GET, POST, PUT, DELETE or PATCH.
	Method string
	// Path is the ServeMux pattern, for example "/modules/{name}".
	Path string
	// Handler names the Rho function serving the route.
	Handler string
}

func (RouteDecl) declNode() {}

func (n RouteDecl) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  Struct Declaration                                                         */
/* -------------------------------------------------------------------------- */

// StructDecl represents:
//
//	struct Name {
//	    field: Type
//	    ...
//	}
type StructDecl struct {
	Node
	Name       string
	Owner      string
	Fields     []Field
	TypeParams []string
	// TypeParamDefaults holds the default type written for a parameter, as in
	// `struct Result[T, E = Unit]`. A parameter with no entry has no default.
	TypeParamDefaults map[string]string
}

func (StructDecl) declNode() {}

func (n StructDecl) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  Struct Fields                                                             */
/* -------------------------------------------------------------------------- */

type Field struct {
	Node
	Name     string
	Type     string
	TypeExpr TypeExpr
}

/* -------------------------------------------------------------------------- */
/*  Function Declaration                                                       */
/* -------------------------------------------------------------------------- */

// FuncDecl represents:
//
//	fn name(param: Type, ...) -> ReturnType {
//	    ...
//	}
type FuncDecl struct {
	Node
	Name       string
	Owner      string
	Params     []Param
	ReturnType string
	Body       []Stmt // Rho 0.1: empty, parser skips body
	TypeParams []string
	// TypeParamDefaults holds the default type written for a parameter, as in
	// `fn Ok[T, E = Unit](v: T)`. A parameter with no entry has no default.
	TypeParamDefaults map[string]string
	Extern            bool
}

func (FuncDecl) declNode() {}

func (n FuncDecl) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  Function Parameters                                                        */
/* -------------------------------------------------------------------------- */

type Param struct {
	Node
	Name     string
	Type     string
	TypeExpr TypeExpr
}

/* -------------------------------------------------------------------------- */
/*  Statements (Rho 0.2+)                                                     */
/* -------------------------------------------------------------------------- */

// Stmt is the interface for all statements.
// Rho 0.1 does not parse statements yet.
type Stmt interface {
	stmtNode()
	// Position is where it was written, so a report about it can say where it is
	// and not only what it is.
	Position() Position
}

type BlockStmt struct {
	Node
	Stmts []Stmt
}

func (BlockStmt) stmtNode() {}

func (n BlockStmt) Position() Position { return n.Pos }

type LetStmt struct {
	Node
	Name  string
	Type  TypeExpr
	Value Expr
}

func (LetStmt) stmtNode() {}

func (n LetStmt) Position() Position { return n.Pos }

type AssignStmt struct {
	Node
	Target Expr
	Value  Expr
	Op     string
}

func (AssignStmt) stmtNode() {}

func (n AssignStmt) Position() Position { return n.Pos }

type ExprStmt struct {
	Node
	Expr Expr
}

func (ExprStmt) stmtNode() {}

func (n ExprStmt) Position() Position { return n.Pos }

type ReturnStmt struct {
	Node
	Value Expr
}

func (ReturnStmt) stmtNode() {}

func (n ReturnStmt) Position() Position { return n.Pos }

type IfStmt struct {
	Node
	Condition Expr
	Then      []Stmt
	Else      []Stmt
}

func (IfStmt) stmtNode() {}

func (n IfStmt) Position() Position { return n.Pos }

type WhileStmt struct {
	Node
	Condition Expr
	Body      []Stmt
}

func (WhileStmt) stmtNode() {}

func (n WhileStmt) Position() Position { return n.Pos }

type BreakStmt struct {
	Node
}

func (BreakStmt) stmtNode() {}

func (n BreakStmt) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  Expressions (Rho 0.2+)                                                    */
/* -------------------------------------------------------------------------- */

// Expr is the interface for all expressions.
// Rho 0.1 does not parse expressions yet.
type Expr interface {
	exprNode()
	// Position is where it was written, so a report about it can say where it is
	// and not only what it is.
	Position() Position
}

type IdentifierExpr struct {
	Node
	Name string
}

func (IdentifierExpr) exprNode() {}

func (n IdentifierExpr) Position() Position { return n.Pos }

type NumberExpr struct {
	Node
	Value string
}

func (NumberExpr) exprNode() {}

func (n NumberExpr) Position() Position { return n.Pos }

type StringExpr struct {
	Node
	Value string
}

func (StringExpr) exprNode() {}

func (n StringExpr) Position() Position { return n.Pos }

type BoolExpr struct {
	Node
	Value bool
}

func (BoolExpr) exprNode() {}

func (n BoolExpr) Position() Position { return n.Pos }

type UnaryExpr struct {
	Node
	Op    string
	Value Expr
}

func (UnaryExpr) exprNode() {}

func (n UnaryExpr) Position() Position { return n.Pos }

type BinaryExpr struct {
	Node
	Op          string
	Left, Right Expr
}

func (BinaryExpr) exprNode() {}

func (n BinaryExpr) Position() Position { return n.Pos }

type CallExpr struct {
	Node
	Callee   Expr
	TypeArgs []TypeExpr
	Args     []Expr
}

func (CallExpr) exprNode() {}

func (n CallExpr) Position() Position { return n.Pos }

type MemberExpr struct {
	Node
	Object Expr
	Name   string
}

func (MemberExpr) exprNode() {}

func (n MemberExpr) Position() Position { return n.Pos }

type IndexExpr struct {
	Node
	Object, Index Expr
}

func (IndexExpr) exprNode() {}

func (n IndexExpr) Position() Position { return n.Pos }

type CastExpr struct {
	Node
	Type  TypeExpr
	Value Expr
}

func (CastExpr) exprNode() {}

func (n CastExpr) Position() Position { return n.Pos }

type StructLiteralExpr struct {
	Node
	Type   TypeExpr
	Values []Expr
}

func (StructLiteralExpr) exprNode() {}

func (n StructLiteralExpr) Position() Position { return n.Pos }

type ArrayLiteralExpr struct {
	Node
	Values []Expr
}

func (ArrayLiteralExpr) exprNode() {}

func (n ArrayLiteralExpr) Position() Position { return n.Pos }

type IfExpr struct {
	Node
	Condition Expr
	Then      Expr
	Else      Expr
}

func (IfExpr) exprNode() {}

func (n IfExpr) Position() Position { return n.Pos }
