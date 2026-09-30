//
// @file positioned_test.go
// @brief An error that knows where it happened, and survives being wrapped.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package types

import (
	"fmt"
	"testing"

	"rho/src/frontend/ast"
)

// A report is built from a chain rather than from one value: an error is wrapped
// as it is passed along, and a reader looking for the position in the outermost
// value finds a wrapper and concludes there is none.
func TestAPositionSurvivesWrapping(t *testing.T) {
	plain := fmt.Errorf("something is wrong")
	positioned := locate(plain, ast.Position{Offset: 40, Line: 4, Column: 4}, "function run")
	if got := positioned.Error(); got != "something is wrong" {
		t.Errorf("the message changed: %q", got)
	}

	// Wrapped twice, which is what a report sees on the way out.
	wrapped := fmt.Errorf("typechecking failed: %w", fmt.Errorf("module: %w", positioned))
	found := AsError(wrapped)
	if found == nil {
		t.Fatal("the position was lost through two wrappers")
	}
	if found.Position.String() != "4:4" {
		t.Errorf("the position is %s, want 4:4", found.Position)
	}
	if found.Scope != "function run" {
		t.Errorf("the scope is %q, want the declaration the error was in", found.Scope)
	}
}

// A check that already knows where its failure is keeps that place. The
// declaration is the fallback, not the answer.
func TestAKnownPositionIsNotReplaced(t *testing.T) {
	precise := &Error{
		Position: ast.Position{Offset: 60, Line: 6, Column: 18},
		Message:  "no such member",
	}
	wrapped := locate(fmt.Errorf("while checking: %w", precise), ast.Position{Offset: 40, Line: 4, Column: 4}, "function run")
	found := AsError(wrapped)
	if found == nil || found.Position.Line != 6 {
		t.Fatalf("a known position was replaced: %+v", found)
	}
}

// The first byte of a file is a real position, and a node that was never placed
// is not. Telling them apart is what stops a report pointing at the top of a
// file because nothing was known.
func TestAPositionThatIsUnknownIsNotTheFirstByte(t *testing.T) {
	first := ast.Position{Offset: 0, Line: 1, Column: 1}
	if !first.Valid() {
		t.Error("the first byte of a file is not a position")
	}
	var never ast.Position
	if never.Valid() {
		t.Error("a position that was never set reads as a real one")
	}
	if never.String() != "unknown position" {
		t.Errorf("an unknown position reads as %q", never.String())
	}
}
