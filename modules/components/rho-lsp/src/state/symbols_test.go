//
// @file symbols_test.go
// @brief Tests for Rho LSP Symbol Tables.
//
// Validates:
// - local symbol extraction
// - local reference extraction
// - symbol locations
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import (
	"testing"

	"github.com/rho/rho-lsp/src/analysis"
)

func TestExtractSymbolsLocal(t *testing.T) {
	ast := &analysis.Module{
		URI: "file:///test.rho",
		Nodes: []analysis.Node{
			analysis.NewDeclNode("test", 1, 0),
		},
	}

	table := ExtractSymbols(ast)

	if !table.Local["test"] {
		t.Fatalf("expected local symbol 'test'")
	}
}

func TestExtractSymbolsLocation(t *testing.T) {
	ast := &analysis.Module{
		URI: "file:///test.rho",
		Nodes: []analysis.Node{
			analysis.NewDeclNode("test", 2, 5),
		},
	}

	table := ExtractSymbols(ast)
	loc := table.LocalLocations["test"]

	if loc.URI != "file:///test.rho" {
		t.Fatalf("unexpected URI: %s", loc.URI)
	}
	if loc.Range.Start.Line != 2 {
		t.Fatalf("unexpected line: %d", loc.Range.Start.Line)
	}
	if loc.Range.Start.Character != 5 {
		t.Fatalf("unexpected column: %d", loc.Range.Start.Character)
	}
}

func TestExtractSymbolsReferences(t *testing.T) {
	ast := &analysis.Module{
		URI: "file:///test.rho",
		Nodes: []analysis.Node{
			analysis.NewRefNode("test", 3, 10),
		},
	}

	table := ExtractSymbols(ast)

	if len(table.LocalRefs["test"]) == 0 {
		t.Fatalf("expected reference for 'test'")
	}
}
