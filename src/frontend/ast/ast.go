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

// Node represents a generic AST node.
// All AST nodes embed this type for future metadata (pos, file, etc).
type Node struct {
	Pos int // byte offset in source (future: line/col)
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
}

type TypeDecl struct {
	Name string
	Type TypeExpr
}

func (TypeDecl) declNode() {}

/* -------------------------------------------------------------------------- */
/*  Struct Declaration                                                         */
/* -------------------------------------------------------------------------- */

// StructDecl represents:
//
//     struct Name {
//         field: Type
//         ...
//     }
//
type StructDecl struct {
	Node
	Name       string
	Fields     []Field
	TypeParams []string
}

func (StructDecl) declNode() {}

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
//     fn name(param: Type, ...) -> ReturnType {
//         ...
//     }
//
type FuncDecl struct {
	Node
	Name       string
	Params     []Param
	ReturnType string
	Body       []Stmt // Rho 0.1: empty, parser skips body
	TypeParams []string
	Extern     bool
}

func (FuncDecl) declNode() {}

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
}

type BlockStmt struct{ Stmts []Stmt }

func (BlockStmt) stmtNode() {}

type LetStmt struct {
	Name  string
	Type  TypeExpr
	Value Expr
}

func (LetStmt) stmtNode() {}

type AssignStmt struct {
	Target Expr
	Value  Expr
	Op     string
}

func (AssignStmt) stmtNode() {}

type ExprStmt struct{ Expr Expr }

func (ExprStmt) stmtNode() {}

type ReturnStmt struct{ Value Expr }

func (ReturnStmt) stmtNode() {}

type IfStmt struct {
	Condition Expr
	Then      []Stmt
	Else      []Stmt
}

func (IfStmt) stmtNode() {}

type WhileStmt struct {
	Condition Expr
	Body      []Stmt
}

func (WhileStmt) stmtNode() {}

type BreakStmt struct{}

func (BreakStmt) stmtNode() {}

/* -------------------------------------------------------------------------- */
/*  Expressions (Rho 0.2+)                                                    */
/* -------------------------------------------------------------------------- */

// Expr is the interface for all expressions.
// Rho 0.1 does not parse expressions yet.
type Expr interface {
	exprNode()
}

type IdentifierExpr struct{ Name string }

func (IdentifierExpr) exprNode() {}

type NumberExpr struct{ Value string }

func (NumberExpr) exprNode() {}

type StringExpr struct{ Value string }

func (StringExpr) exprNode() {}

type BoolExpr struct{ Value bool }

func (BoolExpr) exprNode() {}

type UnaryExpr struct {
	Op    string
	Value Expr
}

func (UnaryExpr) exprNode() {}

type BinaryExpr struct {
	Op          string
	Left, Right Expr
}

func (BinaryExpr) exprNode() {}

type CallExpr struct {
	Callee   Expr
	TypeArgs []TypeExpr
	Args     []Expr
}

func (CallExpr) exprNode() {}

type MemberExpr struct {
	Object Expr
	Name   string
}

func (MemberExpr) exprNode() {}

type IndexExpr struct{ Object, Index Expr }

func (IndexExpr) exprNode() {}

type CastExpr struct {
	Type  TypeExpr
	Value Expr
}

func (CastExpr) exprNode() {}

type StructLiteralExpr struct {
	Type   TypeExpr
	Values []Expr
}

func (StructLiteralExpr) exprNode() {}

type ArrayLiteralExpr struct{ Values []Expr }

func (ArrayLiteralExpr) exprNode() {}

type IfExpr struct {
	Condition Expr
	Then      Expr
	Else      Expr
}

func (IfExpr) exprNode() {}
