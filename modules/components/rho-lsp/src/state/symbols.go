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
// SPDX-License-Identifier: Apache-2.0
//

package state

import (
	"github.com/rho/rho-lsp/src/protocol"
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
