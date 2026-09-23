//
// @file indexer_test.go
// @brief Tests for Rho AST Indexer - Enterprise v1.
//
// Validates:
//   - index building
//   - node lookup
//   - empty index behavior
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

import (
	"testing"
)

func TestIndexerBuild(t *testing.T) {
	ast := &Module{
		Nodes: []Node{
			NewDeclNode("x", 0, 0),
		},
	}

	idx := BuildIndexer(ast)

	if len(idx.nodes) != 1 {
		t.Fatalf("expected 1 node in index")
	}
}

func TestIndexerNodeAt(t *testing.T) {
	ast := &Module{
		Nodes: []Node{
			NewDeclNode("test", 1, 5),
		},
	}

	idx := BuildIndexer(ast)

	n := idx.NodeAt(1, 6)
	if n == nil {
		t.Fatalf("expected node at position")
	}

	if n.Name() != "test" {
		t.Fatalf("unexpected node name: %s", n.Name())
	}
}

func TestIndexerEmpty(t *testing.T) {
	idx := BuildIndexer(nil)

	n := idx.NodeAt(0, 0)
	if n != nil {
		t.Fatalf("expected nil node for empty index")
	}
}
