//
// @file semantic.go
// @brief Rho Semantic Analyzer - Enterprise v1.
//
// Performs minimal semantic analysis:
//   - validates declarations
//   - validates references
//   - generates semantic diagnostics
//
// Used by LSP for:
//   - hover
//   - definition
//   - references
//   - diagnostics
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

/* -------------------------------------------------------------------------- */
/*  Diagnostic                                                                 */
/* -------------------------------------------------------------------------- */

type Diagnostic struct {
	Message string
	Line    int
	Column  int
}

/* -------------------------------------------------------------------------- */
/*  Semantic Analysis                                                           */
/* -------------------------------------------------------------------------- */

func Analyze(ast *Module) []Diagnostic {
	diags := []Diagnostic{}

	if ast == nil {
		return diags
	}

	decls := map[string]Node{}

	// First pass: collect declarations
	for _, n := range ast.Nodes {
		if n.IsDecl() {
			name := n.Name()
			if _, exists := decls[name]; exists {
				diags = append(diags, Diagnostic{
					Message: "duplicate declaration: " + name,
					Line:    n.Line(),
					Column:  n.Column(),
				})
			} else {
				decls[name] = n
			}
		}
	}

	// Second pass: validate references
	for _, n := range ast.Nodes {
		if n.IsRef() {
			name := n.Name()
			if _, exists := decls[name]; !exists {
				diags = append(diags, Diagnostic{
					Message: "undefined symbol: " + name,
					Line:    n.Line(),
					Column:  n.Column(),
				})
			}
		}
	}

	return diags
}
