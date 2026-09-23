//
// @file symbols.go
// @brief Rho LSP Symbol Tables - Enterprise v1.
//
// Defines:
//   - local symbol table
//   - global symbol table
//   - symbol locations
//   - symbol references
//   - extraction from AST
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import (
	"rho/cmd/rho-lsp/analysis"
	"rho/cmd/rho-lsp/protocol"
)

/* -------------------------------------------------------------------------- */
/*  SymbolTable (Local)                                                        */
/* -------------------------------------------------------------------------- */

type SymbolTable struct {
	Local          map[string]bool
	LocalLocations map[string]protocol.Location
	LocalRefs      map[string][]protocol.Location
	RoleRefs       map[string][]protocol.Location
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Local:          map[string]bool{},
		LocalLocations: map[string]protocol.Location{},
		LocalRefs:      map[string][]protocol.Location{},
		RoleRefs:       map[string][]protocol.Location{},
	}
}

/* -------------------------------------------------------------------------- */
/*  GlobalSymbolTable                                                          */
/* -------------------------------------------------------------------------- */

type GlobalSymbolTable struct {
	All        map[string]bool
	Locations  map[string]protocol.Location
	References map[string][]protocol.Location
}

func NewGlobalSymbolTable() *GlobalSymbolTable {
	return &GlobalSymbolTable{
		All:        map[string]bool{},
		Locations:  map[string]protocol.Location{},
		References: map[string][]protocol.Location{},
	}
}

/* -------------------------------------------------------------------------- */
/*  ExtractSymbols                                                              */
/* -------------------------------------------------------------------------- */

func ExtractSymbols(ast *analysis.Module) *SymbolTable {
	table := NewSymbolTable()

	if ast == nil {
		return table
	}

	for _, node := range ast.Nodes {
		name := node.Name()
		if name == "" {
			continue
		}

		// Declaration
		if node.IsDecl() {
			table.Local[name] = true
			table.LocalLocations[name] = protocol.Location{
				URI: ast.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: node.Line(), Character: node.Column()},
					End:   protocol.Position{Line: node.Line(), Character: node.Column() + len(name)},
				},
			}
			table.RoleRefs[node.Role()+":"+name] = append(table.RoleRefs[node.Role()+":"+name], table.LocalLocations[name])
		}

		// Reference
		if node.IsRef() {
			loc := protocol.Location{
				URI: ast.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: node.Line(), Character: node.Column()},
					End:   protocol.Position{Line: node.Line(), Character: node.Column() + len(name)},
				},
			}
			table.LocalRefs[name] = append(table.LocalRefs[name], loc)
			table.RoleRefs[node.Role()+":"+name] = append(table.RoleRefs[node.Role()+":"+name], loc)
		}
	}

	return table
}
