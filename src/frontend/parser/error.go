package parser

import (
	"fmt"
	"strings"

	"rho/src/frontend/lexer"
)

// Error is a parser error tied to a specific token location.
type Error struct {
	Message string
	// File is the source the error is in, when the parser was told it.
	File   string
	Offset int
	Line   int
	Column int
}

func (e *Error) Error() string { return e.Message }

// NewError builds a parser error at a token location.
func NewError(token lexer.Token, message string) *Error {
	return &Error{Message: message, Offset: token.Pos, Line: token.Line, Column: token.Column}
}

// NewErrorf builds a formatted parser error at a token location.
func NewErrorf(token lexer.Token, format string, args ...any) *Error {
	return NewError(token, fmt.Sprintf(format, args...))
}

// ErrorList is every declaration of a file that did not parse, in order. The
// parser recovers at the next declaration, so one run reports them all.
type ErrorList []*Error

func (l ErrorList) Error() string {
	lines := make([]string, len(l))
	for i, err := range l {
		lines[i] = err.Message
	}
	return strings.Join(lines, "\n")
}

// Errors returns every parser error in an error chain: each of a list, or the
// one.
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

// AsError returns the parser Error in an error chain, if there is one.
func AsError(err error) *Error {
	for err != nil {
		if parseErr, ok := err.(*Error); ok {
			return parseErr
		}
		if list, ok := err.(ErrorList); ok && len(list) > 0 {
			return list[0]
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}
		err = unwrapped.Unwrap()
	}
	return nil
}
