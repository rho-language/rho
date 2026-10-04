//
// @file index.go
// @brief What a file declares, read from the compiler's own syntax tree.
//
// The server used to build this from a parser of its own. A second front end is
// a second opinion about what a file means, and it goes stale the first time the
// language moves: a new keyword, a new declaration, a new rule. Reading the
// tree the compiler builds means a name a completion offers is a name the
// compiler knows, and a place it reports is a place in the same coordinates
// everything else is in.
//
// What this answers is the three questions navigation asks: what a file
// declares, where a name is used, and what is under the cursor.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package state

import (
	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	"strconv"

	"github.com/rho/rho-lsp/src/protocol"
)

// Index is what one file declares and where.
type Index struct {
	// uri is the file this describes, which a place without it cannot be sent
	// with.
	uri string
	// Symbols is what the file declares and where each name is used.
	Symbols *SymbolTable
	// Names is every name with a place, which is what a cursor is looked up in.
	Names []NamedPosition
	// Fields are the struct fields the file declares, which is what a completion
	// after a dot offers.
	Fields []Field
}

// NamedPosition is a name and where it was written.
type NamedPosition struct {
	Name string
	Role Role
	Line int
	// Column counts characters, as the compiler's positions do and as an editor
	// shows them.
	Column int
}

// Role is what kind of thing a name is, which is what a hover says and what
// separates one use of a name from another.
type Role string

const (
	// RoleModule is a module's own name.
	RoleModule Role = "module"
	// RoleFunction is a function's name.
	RoleFunction Role = "function"
	// RoleStruct is a struct's name.
	RoleStruct Role = "struct"
	// RoleType is a type alias's name.
	RoleType Role = "type"
	// RoleField is a field or a member access.
	RoleField Role = "field"
	// RoleVariable is a local binding's name.
	RoleVariable Role = "variable"
	// RoleParameter is a parameter's name.
	RoleParameter Role = "parameter"
	// RoleSymbol is any other name being used.
	RoleSymbol Role = "symbol"
)

// Field is a struct field, and the struct it belongs to.
type Field struct {
	Struct string
	Name   string
	Line   int
	Column int
}

// locationAt is a name at a place, in the shape a language server sends.
//
// The compiler counts lines and columns from one; the protocol counts from zero.
// The conversion happens here, once, so that nothing downstream has to remember
// which of two conventions it is holding: a handler that printed a position to
// a person would otherwise be off by one, and off by one everywhere at once.
func locationAt(uri string, line, column int) protocol.Location {
	return protocol.Location{
		URI: uri,
		Range: protocol.Range{
			Start: protocol.Position{Line: line - 1, Character: column - 1},
			End:   protocol.Position{Line: line - 1, Character: column},
		},
	}
}

// BuildIndex reads a file and indexes what it declares.
//
// A file that cannot be parsed is indexed as far as it goes: the declarations
// before the mistake are still real, and a completion that offers them is more
// useful than one that offers nothing because a later line is wrong.
func BuildIndex(uri, text string) *Index {
	out := &Index{uri: uri, Symbols: NewSymbolTable()}

	tokens, lexErr := lexer.Lex(text)
	if lexErr != nil {
		return out
	}
	module, parseErr := parser.Parse(tokens)
	if parseErr != nil {
		// A parser reports a place inside a message rather than as a value, so
		// the declarations that were read before the mistake are the ones worth
		// keeping. The tree is not handed back, so what is indexed is what the
		// tokens can be read as: every name that looks like a declaration.
		out.indexFromTokens(tokens)
		return out
	}
	out.indexModule(module)
	// The parser does not build statement trees yet — `FuncDecl.Body` is empty
	// by design at this version — so the uses *inside* a function are not in the
	// tree to be read. They are read from the tokens instead, which are the
	// compiler's own: a name the server offers a reference for is a name the
	// compiler lexed, in the compiler's coordinates.
	//
	// This is the one place tokens rather than a tree are used, and it is a gap
	// in the language rather than a second way of reading it.
	out.indexUses(tokens)
	return out
}

// Tokens is the file's tokens, for the handler that highlights with them.
func Tokens(text string) []lexer.Token {
	tokens, err := lexer.Lex(text)
	if err != nil {
		return nil
	}
	return tokens
}

// indexModule reads a whole module.
func (ix *Index) indexModule(module *ast.Module) {
	ix.declare(string(RoleModule), module.Name, modulePosition(module))
	for _, decl := range module.Decls {
		ix.indexDecl(decl)
	}
}

// modulePosition is where a module's own name is, which the tree records.
func modulePosition(module *ast.Module) ast.Position {
	return module.Pos
}

// indexDecl reads one declaration and everything inside it.
func (ix *Index) indexDecl(decl ast.Decl) {
	switch d := decl.(type) {
	case ast.FuncDecl:
		ix.declare(string(RoleFunction), d.Name, d.Pos)
		ix.indexBody(d)
	case ast.StructDecl:
		ix.declare(string(RoleStruct), d.Name, d.Pos)
		for _, field := range d.Fields {
			ix.field(d.Name, field)
		}
	case ast.TypeDecl:
		ix.declare(string(RoleType), d.Name, d.Pos)
		ix.reference(string(RoleType), d.Type.Name, typePosition(d.Type))
	case ast.RouteDecl:
		ix.declare(string(RoleFunction), d.Handler, d.Pos)
	}
}

// indexBody reads what a function declares in its parameters and uses in its
// body.
//
// Parameters are declarations and everything else is a use, which is the
// difference between "this function exists" and "this name is mentioned here"
// that navigation turns on.
func (ix *Index) indexBody(fn ast.FuncDecl) {
	for _, param := range fn.Params {
		ix.declare(string(RoleParameter), param.Name, param.Pos)
	}
	for _, stmt := range fn.Body {
		ix.indexStmt(stmt)
	}
}

func (ix *Index) indexStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.LetStmt:
		ix.declare(string(RoleVariable), s.Name, s.Pos)
		ix.indexExpr(s.Value)
	case ast.ReturnStmt:
		ix.indexExpr(s.Value)
	case ast.AssignStmt:
		ix.indexExpr(s.Target)
		ix.indexExpr(s.Value)
	case ast.ExprStmt:
		ix.indexExpr(s.Expr)
	case ast.IfStmt:
		ix.indexExpr(s.Condition)
		for _, inner := range s.Then {
			ix.indexStmt(inner)
		}
		for _, inner := range s.Else {
			ix.indexStmt(inner)
		}
	case ast.WhileStmt:
		ix.indexExpr(s.Condition)
		for _, inner := range s.Body {
			ix.indexStmt(inner)
		}
	}
}

// indexExpr reads a name out of an expression.
//
// An expression is a tree rather than a list, so it is walked rather than
// ranged. A missing kind is not a mistake: a form the language adds and this has
// not been taught is a form whose names are not indexed, which loses a
// completion rather than reporting one wrongly.
func (ix *Index) indexExpr(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case ast.IdentifierExpr:
		ix.reference(string(RoleSymbol), e.Name, e.Pos)
	case ast.CallExpr:
		ix.indexExpr(e.Callee)
		for _, argument := range e.Args {
			ix.indexExpr(argument)
		}
	case ast.BinaryExpr:
		ix.indexExpr(e.Left)
		ix.indexExpr(e.Right)
	case ast.UnaryExpr:
		ix.indexExpr(e.Value)
	case ast.MemberExpr:
		ix.reference(string(RoleField), e.Name, e.Pos)
		ix.indexExpr(e.Object)
	case ast.IndexExpr:
		ix.indexExpr(e.Object)
		ix.indexExpr(e.Index)
	case ast.CastExpr:
		ix.indexExpr(e.Value)
	case ast.StructLiteralExpr:
		for _, value := range e.Values {
			ix.indexExpr(value)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range e.Values {
			ix.indexExpr(value)
		}
	}
}

// typePosition is where a type name is written.
//
// A type is written rather than pointed at, and the tree does not keep a place
// for it, so nothing is claimed: a name indexed at no place is a name a cursor
// can never be inside, which is the same as not indexing it.
func typePosition(ast.TypeExpr) ast.Position { return ast.Position{} }

// field records a struct field, and the struct it belongs to.
func (ix *Index) field(owner string, field ast.Field) {
	ix.Fields = append(ix.Fields, Field{Struct: owner, Name: field.Name, Line: field.Pos.Line - 1, Column: field.Pos.Column - 1})
	ix.declare(string(RoleField), field.Name, field.Pos)
}

// declare records a name the file introduces.
func (ix *Index) declare(role, name string, at ast.Position) {
	if name == "" {
		return
	}
	ix.Names = append(ix.Names, NamedPosition{Name: name, Role: Role(role), Line: at.Line - 1, Column: at.Column - 1})
	ix.Symbols.Local[name] = true
	if !at.Valid() {
		return
	}
	location := locationAt(ix.uri, at.Line, at.Column)
	ix.Symbols.LocalLocations[name] = location
	ix.Symbols.LocalRefs[name] = append(ix.Symbols.LocalRefs[name], location)
	ix.Symbols.RoleRefs[role+":"+name] = append(ix.Symbols.RoleRefs[role+":"+name], location)
}

// reference records a name the file uses.
func (ix *Index) reference(role, name string, at ast.Position) {
	if name == "" || !at.Valid() {
		return
	}
	location := locationAt(ix.uri, at.Line, at.Column)
	ix.Names = append(ix.Names, NamedPosition{Name: name, Role: Role(role), Line: at.Line - 1, Column: at.Column - 1})
	ix.Symbols.LocalRefs[name] = append(ix.Symbols.LocalRefs[name], location)
	ix.Symbols.RoleRefs[role+":"+name] = append(ix.Symbols.RoleRefs[role+":"+name], location)
}

// indexUses records a name used in a body, from the tokens.
//
// A name is a use unless the tree says it is a declaration, so this skips what
// was already indexed rather than reporting a declaration as a use of itself.
func (ix *Index) indexUses(tokens []lexer.Token) {
	// Only the declaration *sites* are skipped, not every use of a name that is
	// declared somewhere. Skipping by name would drop the very uses that
	// references are made of, and "find references" would answer with the
	// declaration alone.
	declaredAt := map[string]bool{}
	for _, name := range ix.Names {
		if isDeclaration(name.Role) {
			declaredAt[keyAt(name.Line, name.Column)] = true
		}
	}
	for _, token := range tokens {
		if token.Kind != lexer.TokenIdent {
			continue
		}
		if declaredAt[keyAt(token.Line, token.Column)] {
			continue
		}
		// A name after a dot is a member of something, not a name in its own
		// right, and offering a definition for `a` inside `a.b` would send a
		// reader to the wrong thing.
		if previous, ok := previousToken(tokens, token); ok && previous.Lex == "." {
			continue
		}
		ix.reference(string(RoleSymbol), token.Lex,
			ast.Position{Offset: token.Pos, Line: token.Line, Column: token.Column})
	}
}

// isDeclaration reports whether a role is one that introduces a name rather
// than uses one.
func isDeclaration(role Role) bool {
	switch role {
	case RoleModule, RoleFunction, RoleStruct, RoleType, RoleField, RoleVariable, RoleParameter:
		return true
	}
	return false
}

// keyAt identifies a place, so a declaration can be told from a use of the same
// name.
func keyAt(line, column int) string {
	return strconv.Itoa(line) + ":" + strconv.Itoa(column)
}

// previousToken is the token before this one, which is how a name is told apart
// from a member of one.
func previousToken(tokens []lexer.Token, at lexer.Token) (lexer.Token, bool) {
	for index, token := range tokens {
		if token.Pos == at.Pos && index > 0 {
			return tokens[index-1], true
		}
	}
	return lexer.Token{}, false
}

// indexFromTokens records the declarations in a file that could not be parsed.
//
// This is the reading the server had before it had a compiler to read with, and
// it is deliberately worse: names are taken from the shape of the tokens rather
// than from a tree, so a file with a mistake in it still offers what it declares
// before the mistake and no more than that.
func (ix *Index) indexFromTokens(tokens []lexer.Token) {
	for index, token := range tokens {
		switch token.Lex {
		case "module", "fn", "struct", "type":
		default:
			continue
		}
		if index+1 >= len(tokens) || tokens[index+1].Kind != lexer.TokenIdent {
			continue
		}
		name := tokens[index+1]
		ix.declare(string(Role(token.Lex)), name.Lex, ast.Position{Offset: name.Pos, Line: name.Line, Column: name.Column})
	}
}

// At is what is under the cursor, if anything is.
func (ix *Index) At(line, col int) (NamedPosition, bool) {
	for _, name := range ix.Names {
		if name.Line != line {
			continue
		}
		if col >= name.Column && col < name.Column+len(name.Name) {
			return name, true
		}
	}
	return NamedPosition{}, false
}
