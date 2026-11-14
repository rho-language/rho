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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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
	// File is the source file the node was read from, as the loader named it.
	// A program is one module made of several files — an include is merged
	// into the module that includes it, an import into the program — so a
	// node carries its file rather than inheriting the root's: a report about a
	// line in helper.rho points at helper.rho. It is empty for a node read from
	// text with no file behind it.
	File string
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
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
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
	Name      string
	Namespace string
	Imports   []Import
	Includes  []Include
	Features  []string
	Decls     []Decl
	// LoadedFiles are every source file the graph was loaded from, set on the
	// root module only: what the program is made of, so the modules it draws on
	// (and their assets) are known without loading it twice.
	LoadedFiles []string
	// Exports are the names each imported module lets its importers use, by the
	// module's import path. A module with no entry exports everything, which is
	// what a module written before export lists does.
	Exports map[string][]string
}

// TypeExpr represents a named type with optional generic arguments and array
// dimensions, for example Slice[u8] or u8[8].
type TypeExpr struct {
	Name     string
	Args     []TypeExpr
	ArrayLen *int
	// Dims are the further dimensions of a multi-dimensional array, after
	// ArrayLen: `u32[2][2]` has ArrayLen 2 and Dims [2].
	Dims []int
}

func (t TypeExpr) String() string {
	name := t.Name
	// An array of functions is written with the function type in brackets,
	// `(fn(i32) -> i32)[4]`, so its length is not read as the result's.
	if IsFuncTypeName(name) && (t.ArrayLen != nil || len(t.Dims) > 0) {
		name = "(" + name + ")"
	}
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
	for _, dim := range t.Dims {
		name += fmt.Sprintf("[%d]", dim)
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
	Name  string
	Owner string
	Type  TypeExpr
}

func (TypeDecl) declNode() {}

func (n TypeDecl) Position() Position { return n.Pos }

// ConstDecl is a module-level constant:
//
//	const NAME: T = value
//
// The keyword may be `const`, `let` or `var`; a top-level binding is a constant
// whichever word introduced it, because a module has no run-time initialisation
// order to assign one in. Its value is inlined at every use.
type ConstDecl struct {
	Node
	Name string
	// Owner is the imported module that declared it, as for a function.
	Owner string
	Type  TypeExpr
	Value Expr
}

func (ConstDecl) declNode() {}

func (n ConstDecl) Position() Position { return n.Pos }

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
	// Enum says the struct is the layout the checker made for an enum that
	// carries data: its tag, then each variant's values (Field.Variant). It is
	// never written; the enum it stands for is checked in its place.
	Enum bool
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
	// BitWidth is the width of a bit field, as in `flag: u8 : 1`. It is nil for
	// an ordinary field.
	BitWidth *int
	// Variant is the enum variant a field of an enum's layout belongs to
	// (StructDecl.Enum); empty for a field of a struct.
	Variant string
	// Shared marks a field of an enum's layout stored by its own name in the
	// storage the variants share (ir.FieldIR.Shared).
	Shared bool
}

/* -------------------------------------------------------------------------- */
/*  Enum Declaration                                                          */
/* -------------------------------------------------------------------------- */

// EnumDecl is an enum: a type whose value is one of a fixed set of variants.
//
//	enum Severity {            // a plain enum: each variant is a number
//	    INFO = 0
//	    WARN                   // counted on from the one before: 1
//	}
//
//	enum Shape {               // an enum whose variants carry data
//	    Circle(i32)            // tuple-like: values by position
//	    Rect { w: i32, h: i32 } // struct-like: values by name
//	    Empty                  // no data
//	}
//
//	enum Option[T] { None, Some(T) }   // generic, specialised per use
//
// A variant of a plain enum may give its number (`A = 1`), which is what a C
// header or a wire protocol agrees on; a variant without one is the one before
// it plus one, starting at 0. An enum where any variant carries data has no
// numbers of its own to give, and writing one is refused by the checker.
//
// A protocol schema reads the same declaration, and only the plain form.
type EnumDecl struct {
	Node
	Name     string
	Owner    string
	Variants []EnumVariant
	// TypeParams are the type parameters of a generic enum, as in
	// `enum Result[T, E = Unit]`, and TypeParamDefaults the defaults written
	// for them.
	TypeParams        []string
	TypeParamDefaults map[string]string
}

func (EnumDecl) declNode() {}

func (n EnumDecl) Position() Position { return n.Pos }

// EnumVariant is one variant of an enum.
type EnumVariant struct {
	Node
	Name string
	// Value is the variant's number: the one written, or the one before it plus
	// one. It is what a plain enum is on the wire and in C.
	Value int
	// Explicit says the number was written, `A = 1`.
	Explicit bool
	// Fields are the values the variant carries: none for a plain variant,
	// unnamed ones for `B(u32, string)`, named ones for `C { x: i32 }`.
	Fields []Field
	// Named says the fields were written with names, in braces.
	Named bool
}

// CarriesData reports whether any variant of an enum carries a value.
func (n EnumDecl) CarriesData() bool {
	for _, variant := range n.Variants {
		if len(variant.Fields) > 0 {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Message Declaration                                                       */
/* -------------------------------------------------------------------------- */

// MessageDecl is a schema message: a named, ordered set of fields that crosses a
// wire. It is a struct that makes a promise a struct does not — a byte layout
// and a version — which is why it is written with a keyword of its own.
type MessageDecl struct {
	Node
	Name   string
	Fields []Field
}

func (MessageDecl) declNode() {}

func (n MessageDecl) Position() Position { return n.Pos }

/* -------------------------------------------------------------------------- */
/*  RPC Declaration                                                           */
/* -------------------------------------------------------------------------- */

// RpcDecl is a request/response contract between two sides:
//
//	rpc SetMotorSpeed {
//	    request  { ... }
//	    response { ... }
//	}
type RpcDecl struct {
	Node
	Name     string
	Request  []Field
	Response []Field
}

func (RpcDecl) declNode() {}

func (n RpcDecl) Position() Position { return n.Pos }

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
	// TypeParamBounds holds the constraints written on a parameter, as in
	// `fn sum[T: Number](xs: Slice[T])` or `[T: Ordered + Equal]`. A parameter
	// with no entry is unconstrained.
	TypeParamBounds map[string][]TypeBound
	Extern          bool
}

// TypeBound is one constraint written on a type parameter: `Number` in
// `[T: Number]`, with where it is written, so a constraint nothing defines is
// reported at its name.
type TypeBound struct {
	Name string
	Pos  Position
}

// BoundsText renders a parameter's constraints as written: `Ordered + Equal`.
func BoundsText(bounds []TypeBound) string {
	text := ""
	for i, bound := range bounds {
		if i > 0 {
			text += " + "
		}
		text += bound.Name
	}
	return text
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
	Name    string
	Type    TypeExpr
	Value   Expr
	Mutable bool
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

// ContinueStmt leaves the innermost loop and goes on with the next thing it
// has to do.
//
// A loop that skips what it has already done is ordinary to write and
// impossible without it: a guard inside the body says what to skip, and the
// rest of the body is the rest.
type ContinueStmt struct {
	Node
}

func (ContinueStmt) stmtNode() {}

func (n ContinueStmt) Position() Position { return n.Pos }

// ForStmt iterates an array, binding each element to Name. `reverse` walks it
// from the end, and `step` visits every Nth element.
type ForStmt struct {
	Node
	Name       string
	Collection Expr
	Reverse    bool
	Step       Expr // nil means one
	Body       []Stmt
}

func (ForStmt) stmtNode() {}

func (n ForStmt) Position() Position { return n.Pos }

// MatchArm is one alternative of a match: a pattern, or a wildcard, and the
// statements to run.
//
// A pattern is an expression read as a shape: a literal compares, a variant of
// an enum (`Shape.Circle`, or `Circle` against a value of that enum) tests the
// variant, and the values written inside it — `Shape.Circle(r)`,
// `Shape.Rect { w, h }` — bind names, compare literals, or nest further
// patterns. `_` stands for anything, as an arm or as one value inside one.
type MatchArm struct {
	Node
	Pattern  Expr // nil when Wildcard
	Wildcard bool
	Body     []Stmt
}

// MatchStmt runs the first arm whose pattern matches Value, or the wildcard.
type MatchStmt struct {
	Node
	Value Expr
	Arms  []MatchArm
	// IfLet says the match was written `if let Pattern = value { } else { }`:
	// one arm and the else as its wildcard. It is not checked for being
	// exhaustive, because the else is what it does otherwise.
	IfLet bool
}

func (MatchStmt) stmtNode() {}

func (n MatchStmt) Position() Position { return n.Pos }

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
	// TypeArgs are type arguments written on the object before the dot, as in
	// `Option[u32].None`: the variant of a generic enum named with its
	// specialisation.
	TypeArgs []TypeExpr
}

func (MemberExpr) exprNode() {}

func (n MemberExpr) Position() Position { return n.Pos }

type IndexExpr struct {
	Node
	Object, Index Expr
	// TypeArgs are set when the brackets held types rather than a value:
	// `identity[u32]` or `pair[u32, string]` naming a specialisation of a
	// generic function as a value. Index is then the first of them, as a name.
	TypeArgs []TypeExpr
}

func (IndexExpr) exprNode() {}

func (n IndexExpr) Position() Position { return n.Pos }

// SliceExpr is a sub-slice, `s[low..high]`: the elements from low up to, and
// not including, high. Either bound may be left out — `s[..high]`, `s[low..]`,
// `s[..]` — and is then the start or the length. It is a `Slice[T]` over the
// same storage, on a `Slice[T]` or a fixed array `T[N]`.
type SliceExpr struct {
	Node
	Object    Expr
	Low, High Expr // nil when not written
}

func (SliceExpr) exprNode() {}

func (n SliceExpr) Position() Position { return n.Pos }

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
	// Repeat carries the count of a `[value; count]` literal, which fills a
	// fixed-size array with one value. It is nil for an ordinary list.
	Repeat *Expr
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

// FuncLitExpr is a function written where a value goes, a closure:
//
//	fn(x: i32) -> i32 { return x + k }
//
// It is written like a declaration without a name. The locals of the enclosing
// function it names are captured by value: each is copied into the closure when
// the literal is evaluated, and a later change to the variable is not seen by
// it. A captured name cannot be assigned inside the body.
type FuncLitExpr struct {
	Node
	Params []Param
	// ReturnType is the result as written, "void" when none was.
	ReturnType string
	Body       []Stmt
}

func (FuncLitExpr) exprNode() {}

func (n FuncLitExpr) Position() Position { return n.Pos }

// IsFuncTypeName reports whether a written type name is a function type,
// `fn(i32, string) -> bool`.
func IsFuncTypeName(name string) bool {
	return len(name) >= 3 && name[:3] == "fn("
}

// FuncTypeName is the one spelling of a function type: `fn(A, B) -> R`, or
// `fn(A, B)` when it returns nothing. Every function type is spelled this way,
// whoever builds it, so two spellings of one type are the same text.
func FuncTypeName(params []string, result string) string {
	name := "fn("
	for i, param := range params {
		if i > 0 {
			name += ", "
		}
		name += param
	}
	name += ")"
	if result != "" && result != "void" {
		name += " -> " + result
	}
	return name
}

// SplitFuncTypeName takes a function type's spelling apart into its parameter
// types and its result ("void" when it returns nothing). The third result says
// whether the name was a function type at all.
func SplitFuncTypeName(name string) ([]string, string, bool) {
	if !IsFuncTypeName(name) {
		return nil, "", false
	}
	depth := 0
	closing := -1
	for i := 2; i < len(name); i++ {
		switch name[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
			if depth == 0 && name[i] == ')' {
				closing = i
			}
		}
		if closing >= 0 {
			break
		}
	}
	if closing < 0 {
		return nil, "", false
	}
	params := []string{}
	inner := name[3:closing]
	start := 0
	depth = 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				params = append(params, trimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	if tail := trimSpace(inner[start:]); tail != "" {
		params = append(params, tail)
	}
	rest := trimSpace(name[closing+1:])
	if rest == "" {
		return params, "void", true
	}
	if len(rest) < 2 || rest[:2] != "->" {
		return nil, "", false
	}
	return params, trimSpace(rest[2:]), true
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
