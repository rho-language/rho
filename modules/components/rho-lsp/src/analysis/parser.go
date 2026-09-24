//
// @file parser.go
// @brief Rho Parser - Enterprise v1.
//
// Minimal parser for Rho language, used by LSP for:
//   - AST generation
//   - symbol extraction
//   - diagnostics
//   - node lookup
//
// Grammar supported (v1):
//   module_decl := "module" IDENT
//   fn_decl     := "fn" IDENT "(" ... ")"
//   ident_ref   := IDENT
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

/* -------------------------------------------------------------------------- */
/*  Token                                                                      */
/* -------------------------------------------------------------------------- */

type Token struct {
	Kind   string
	Text   string
	Line   int
	Column int
}

/* -------------------------------------------------------------------------- */
/*  Node                                                                       */
/* -------------------------------------------------------------------------- */

type Node interface {
	Name() string
	Line() int
	Column() int
	IsDecl() bool
	IsRef() bool
	Role() string
}

/* -------------------------------------------------------------------------- */
/*  DeclNode                                                                    */
/* -------------------------------------------------------------------------- */

type DeclNode struct {
	name   string
	line   int
	column int
	role   string
}

func NewDeclNode(name string, line, col int) *DeclNode {
	return &DeclNode{name: name, line: line, column: col, role: "symbol"}
}

func (n *DeclNode) Name() string { return n.name }
func (n *DeclNode) Line() int    { return n.line }
func (n *DeclNode) Column() int  { return n.column }
func (n *DeclNode) IsDecl() bool { return true }
func (n *DeclNode) IsRef() bool  { return false }
func (n *DeclNode) Role() string { return n.role }

/* -------------------------------------------------------------------------- */
/*  RefNode                                                                     */
/* -------------------------------------------------------------------------- */

type RefNode struct {
	name   string
	line   int
	column int
	role   string
}

func NewRefNode(name string, line, col int) *RefNode {
	return &RefNode{name: name, line: line, column: col, role: "symbol"}
}

func (n *RefNode) Name() string { return n.name }
func (n *RefNode) Line() int    { return n.line }
func (n *RefNode) Column() int  { return n.column }
func (n *RefNode) IsDecl() bool { return false }
func (n *RefNode) IsRef() bool  { return true }
func (n *RefNode) Role() string { return n.role }

/* -------------------------------------------------------------------------- */
/*  Module                                                                      */
/* -------------------------------------------------------------------------- */

type Module struct {
	URI   string
	Nodes []Node
}

func (m *Module) NodeAt(line, col int) Node {
	for _, n := range m.Nodes {
		if n.Line() == line && col >= n.Column() && col < n.Column()+len(n.Name()) {
			return n
		}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Parse                                                                       */
/* -------------------------------------------------------------------------- */

func Parse(tokens []Token) (*Module, []Diagnostic) {
	mod := &Module{Nodes: []Node{}}
	diags := []Diagnostic{}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		// module x
		if t.Kind == "keyword" && t.Text == "module" {
			if i+1 < len(tokens) && tokens[i+1].Kind == "identifier" {
				node := NewDeclNode(tokens[i+1].Text, tokens[i+1].Line, tokens[i+1].Column)
				node.role = "module"
				mod.Nodes = append(mod.Nodes, node)
			} else {
				diags = append(diags, Diagnostic{
					Message: "expected module name",
					Line:    t.Line,
					Column:  t.Column,
				})
			}
		}

		// fn test
		if t.Kind == "keyword" && t.Text == "fn" {
			if i+1 < len(tokens) && tokens[i+1].Kind == "identifier" {
				node := NewDeclNode(tokens[i+1].Text, tokens[i+1].Line, tokens[i+1].Column)
				node.role = "function"
				mod.Nodes = append(mod.Nodes, node)
			} else {
				diags = append(diags, Diagnostic{
					Message: "expected function name",
					Line:    t.Line,
					Column:  t.Column,
				})
			}
		}

		// identifier reference
		if t.Kind == "identifier" {
			node := NewRefNode(t.Text, t.Line, t.Column)
			if i > 0 && tokens[i-1].Text == "." {
				node.role = "field"
			}
			if i > 0 && (tokens[i-1].Text == "import" || tokens[i-1].Text == "module") {
				node.role = "module"
			}
			mod.Nodes = append(mod.Nodes, node)
		}
	}

	return mod, diags
}
