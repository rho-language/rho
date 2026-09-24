//
// @file indexer.go
// @brief Rho AST Indexer - Enterprise v1.
//
// Provides fast lookup of AST nodes by (line, column).
// Used by LSP for:
//   - hover
//   - definition
//   - references
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

/* -------------------------------------------------------------------------- */
/*  Indexer                                                                    */
/* -------------------------------------------------------------------------- */

type Indexer struct {
	nodes     []Node
	intervals *IntervalIndex
}

/* -------------------------------------------------------------------------- */
/*  Build Index                                                                */
/* -------------------------------------------------------------------------- */

func BuildIndexer(ast *Module) *Indexer {
	if ast == nil {
		return &Indexer{nodes: []Node{}}
	}

	return &Indexer{
		nodes:     ast.Nodes,
		intervals: BuildIntervalIndex(ast.Nodes),
	}
}

/* -------------------------------------------------------------------------- */
/*  NodeAt                                                                      */
/* -------------------------------------------------------------------------- */

func (idx *Indexer) NodeAt(line, col int) Node {
	if idx.intervals == nil {
		return nil
	}
	return idx.intervals.NodeAt(line, col)
}
