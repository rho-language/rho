//
// @file positioned.go
// @brief An error that knows where in the source it happened.
//
// A type checker that cannot say where a mistake is makes a person go and look
// for it. Worse, the compiler had been working around that by scraping an
// integer out of the error's text — a thing that works until an error message is
// reworded, and fails silently when it is.
//
// This is the honest form: an error that carries a position, so the compiler
// reports where rather than guessing.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import "rho/src/frontend/ast"

// Error is a type error and where in the source it is.
//
// The message stays the one the checker wrote, and the position is added to it.
// A report that reads "module 'shop' has no member 'nam' at 12:9" is one a person
// can act on; the same report with no position is a search.
type Error struct {
	// Position is where the error is, which may be unknown — a check that
	// cannot place its own failure says so rather than pointing at the top of
	// the file.
	Position ast.Position
	// Message is what the checker said.
	Message string
	// Scope is the declaration being checked when this happened, and is what a
	// position falls back to when the exact expression is not known. It is
	// recorded because "in function run" is a great deal better than nothing
	// even when the line is an approximation.
	Scope string
	// Cause is the error as it was, when this wraps one.
	Cause error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return e.Message + ": " + e.Cause.Error()
}

// Unwrap gives the error underneath, so that errors.Is and errors.As still work
// through it.
func (e *Error) Unwrap() error { return e.Cause }

// locate attributes an error to a place in the source.
//
// A position that is already known is kept, because an expression inside a
// function knows better than the function does. One that is not falls back to
// where the declaration being checked begins, which is the best that can be said
// for a check that did not place its own failure.
func locate(err error, at ast.Position, scope string) error {
	if err == nil {
		return nil
	}
	var positioned *Error
	if asError(err, &positioned) {
		return err
	}
	// A position of zero is a thing whose place was never recorded, and it must
	// not be mistaken for the first byte of the file.
	if !at.Valid() {
		at = ast.Position{}
	}
	if scope == "" {
		return &Error{Position: at, Message: err.Error()}
	}
	return &Error{Position: at, Scope: scope, Message: err.Error()}
}

// asError unwraps to a positioned error, so that an error already carrying a
// position is not given a second one.
func asError(err error, into **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*into = e
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

// AsError is the positioned error inside an error, however many times it has
// been wrapped, or nil when there is none.
//
// A report is built from a chain rather than from one value: an error is wrapped
// as it is passed along, and a caller that asserted the outermost type would find
// a wrapper and conclude there was no position at all.
func AsError(err error) *Error {
	for err != nil {
		if positioned, ok := err.(*Error); ok {
			return positioned
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}
		err = unwrapped.Unwrap()
	}
	return nil
}
