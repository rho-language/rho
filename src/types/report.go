//
// @file report.go
// @brief Every independent mistake in a program, in one run.
//
// A checker that stops at the first mistake makes a person fix one thing, build,
// and be told about the next: a loop of builds for a file that had three typos.
// This is how the checker keeps going instead. Each declaration is checked on its
// own, and inside a function each statement is: a statement that is wrong is
// reported and the next one is checked.
//
// Keeping going has a cost, which is cascades: a `let` whose value is wrong
// leaves a name nothing can type, and every later use of it would be reported as
// a mistake of its own. Two rules keep that out. A binding whose value failed is
// bound to a poison type, and a later statement that fails while reading a poison
// name is not reported, because its mistake is the one already said. And a name
// reported as undefined is reported once per declaration, not at every use.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"errors"
	"fmt"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// MaxErrors is how many errors one check records. A program with more than
// that has one mistake repeated, and the rest are counted rather than listed.
const MaxErrors = 100

// Note is something a diagnostic adds about another place: where a function
// that was called wrongly is declared, say.
type Note struct {
	Position ast.Position
	Message  string
}

// ErrorList is every error one check found, in the order it found them.
type ErrorList []*Error

func (l ErrorList) Error() string {
	lines := make([]string, len(l))
	for i, err := range l {
		lines[i] = err.Error()
	}
	return strings.Join(lines, "\n")
}

// Errors is every positioned error in an error chain: each of a list, or the one.
func Errors(err error) []*Error {
	for err != nil {
		switch e := err.(type) {
		case ErrorList:
			return e
		case *Error:
			return []*Error{e}
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}
		err = unwrapped.Unwrap()
	}
	return nil
}

// undefinedName is the error for a name nothing declares. It is a type of its
// own so the collector can tell which name it was, and say it once.
type undefinedName struct {
	name    string
	message string
}

func (e *undefinedName) Error() string { return e.message }

// poisonName marks a binding whose value could not be typed.
const poisonName = "\x00invalid"

var poisonType = ir.Type{Kind: ir.TypeInvalid, Name: poisonName}

// collector gathers the errors of one CheckModule.
type collector struct {
	errors  ErrorList
	omitted int
	// seen are the messages reported in the declaration being checked, at
	// their place, so the same mistake reached twice is said once.
	seen map[string]bool
	// undefined are the names already reported as undefined in this
	// declaration; a later use of one is the same mistake.
	undefined map[string]bool
	// touchedPoison is set when the statement being checked read a binding
	// whose value had failed: whatever is wrong with it was already said.
	touchedPoison bool
}

// beginDeclaration starts a declaration: a name is undefined once per
// declaration, since each is a place a person reads on its own.
func (c *collector) beginDeclaration() {
	c.seen = map[string]bool{}
	c.undefined = map[string]bool{}
}

// add records an error, unless it is one already said.
func (c *collector) add(err error) {
	if err == nil {
		return
	}
	for _, positioned := range Errors(err) {
		c.addOne(positioned)
	}
	if Errors(err) == nil {
		c.addOne(&Error{Message: err.Error()})
	}
}

func (c *collector) addOne(err *Error) {
	if name := undefinedOf(err); name != "" {
		if c.undefined[name] {
			return
		}
		c.undefined[name] = true
	}
	key := err.Position.String() + "\x00" + err.Message
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	if len(c.errors) >= MaxErrors {
		c.omitted++
		return
	}
	c.errors = append(c.errors, err)
}

// result is what CheckModule returns: nothing, the one error, or the list.
func (c *collector) result() error {
	switch len(c.errors) {
	case 0:
		return nil
	case 1:
		return c.errors[0]
	}
	return c.errors
}

// report records an error found while checking a statement, unless it follows
// from one already reported. It is called only while collecting.
func (tc *TypeChecker) report(err error) {
	if tc.collect.touchedPoison {
		return
	}
	if name := undefinedOf(err); name != "" && tc.collect.undefined[name] {
		return
	}
	tc.collect.add(err)
}

// undefinedOf is the undefined name an error is about, or "".
func undefinedOf(err error) string {
	if positioned := AsError(err); positioned != nil && positioned.undefined != "" {
		return positioned.undefined
	}
	var undefined *undefinedName
	if errors.As(err, &undefined) {
		return undefined.name
	}
	return ""
}

// undefinedNameError is the error for a name nothing declares.
func undefinedNameError(name, message string) error {
	return &undefinedName{name: name, message: message}
}

// withNote adds a note to a positioned error, placing the error first when it
// has no place yet.
func withNote(err error, at ast.Position, note Note) error {
	positioned := AsError(err)
	if positioned == nil {
		positioned = &Error{Position: at, Message: err.Error(), Cause: nil}
		if !at.Valid() {
			positioned.Position = ast.Position{}
		}
	}
	positioned.Notes = append(positioned.Notes, note)
	return positioned
}

// declaredAt is a note saying where a declaration is.
func declaredAt(what string, at ast.Position) Note {
	return Note{Position: at, Message: fmt.Sprintf("%s is declared here", what)}
}
