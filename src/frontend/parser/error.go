package parser

import (
	"fmt"

	"rho/src/frontend/lexer"
)

// Error is a parser error tied to a specific token location.
type Error struct {
	Message string
	Offset  int
	Line    int
	Column  int
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

// AsError returns the parser Error in an error chain, if there is one.
func AsError(err error) *Error {
	for err != nil {
		if parseErr, ok := err.(*Error); ok {
			return parseErr
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}
		err = unwrapped.Unwrap()
	}
	return nil
}
