//
// @file emitter.go
// @brief The dynamic half: a code emitter that knows what is inside what.
//
// The static half of the code generator is C in the repository (`csrc.go`). This
// is the other half: the program itself, emitted from the IR by a program rather
// than by a template.
//
// A template is the wrong tool for this half and the failure is structural rather
// than stylistic. Lowering an expression tree means knowing what each node is, and
// a template expresses that as nesting inside the text:
//
//	{{if .IsPointer}}*{{end}}{{.Name}}{{if .HasArgs}}({{range .Args}}…{{end}}){{end}}
//
// Every construct in the language adds a clause, the clauses nest inside one
// another, and the result is unreadable at exactly the moment something is wrong.
// A recursive emitter is the same logic as ordinary Go control flow, which is what
// makes it debuggable: a wrong node is a wrong branch, not a wrong string.
//
// What the emitter knows that a `strings.Builder` does not:
//
//   - **Where it is.** Indentation is a counter, not something every call site has
//     to remember. A body emitted one level too shallow is the single most common
//     way generated C becomes hard to read, and it is the one a builder cannot
//     catch.
//   - **What is open.** `EmitLine` and `Emitf` open nothing; `Open`/`Close` and
//     `Block` keep braces and their indentation together, so a construct cannot be
//     emitted with its closing brace at the wrong depth.
//   - **Whether anything was written.** A generated program that emits nothing for
//     an empty body is correct; one that emits a stray blank line is noise.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"bytes"
	"fmt"
	"strings"
)

// IndentUnit is one level of indentation.
//
// Four spaces because that is what every C tool in this repository expects:
// clang-format's default, the vendor style files' own, and what a reader's editor
// will jump by.
const IndentUnit = "    "

// Emitter writes C, and knows what is inside what.
type Emitter struct {
	buf    bytes.Buffer
	indent int
	// wrote reports whether anything has been written at all, which is how a
	// caller tells "nothing to say" from "said nothing in particular".
	wrote bool
}

// NewEmitter is an emitter at the outermost level.
func NewEmitter() *Emitter { return &Emitter{} }

// Reset empties the emitter and returns it to the outermost level, for a
// generator that emits more than one module.
func (e *Emitter) Reset() {
	e.buf.Reset()
	e.indent = 0
	e.wrote = false
}

// String is everything emitted.
func (e *Emitter) String() string { return e.buf.String() }

// Wrote reports whether anything was emitted.
func (e *Emitter) Wrote() bool { return e.wrote }

// Indent is one level deeper.
func (e *Emitter) Indent() { e.indent++ }

// Dedent is one level back, and stops at the outermost rather than going negative:
// a construct that closes more blocks than it opened is a bug in the emitter, and
// rendering it as a thousand leading spaces hides that behind a wall of
// whitespace.
func (e *Emitter) Dedent() {
	if e.indent > 0 {
		e.indent--
	}
}

// Level is the current depth, for a diagnostic about where something was emitted.
func (e *Emitter) Level() int { return e.indent }

// prefix is the current indentation.
func (e *Emitter) prefix() string { return strings.Repeat(IndentUnit, e.indent) }

// EmitLine is one line, at the current indentation.
//
// A newline is added, so a caller writes a line and not a line ending — which is
// the difference between a generator that produces readable output and one that
// produces output whose shape depends on how many `\n` each call site remembered.
func (e *Emitter) EmitLine(line string) {
	e.wrote = true
	e.buf.WriteString(e.prefix())
	e.buf.WriteString(strings.TrimRight(line, "\n"))
	e.buf.WriteByte('\n')
}

// Emitf is one formatted line, at the current indentation.
func (e *Emitter) Emitf(format string, args ...any) {
	e.EmitLine(fmt.Sprintf(format, args...))
}

// WriteString appends text verbatim, with no indentation and no newline.
//
// Here so that the emitter can stand in for the `strings.Builder` the generator
// used to hold: a line assembled from fragments — a type, a space, a name — writes
// those fragments and then one line ending, and that is a shape worth keeping. What
// it must not be used for is the *start* of a line, because the whole point of the
// emitter is that only it knows where a line starts.
func (e *Emitter) WriteString(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	e.wrote = true
	return e.buf.WriteString(text)
}

// Emit is text written exactly as given, with no indentation and no newline.
//
// For the parts of a file that are not a line — a block of embedded C, a
// preprocessor run, a comment that is already formatted. Using it for a line is
// almost always a mistake, because it is the one call that does not know where it
// is.
func (e *Emitter) Emit(text string) {
	if text == "" {
		return
	}
	e.wrote = true
	e.buf.WriteString(text)
}

// EmitBlock is embedded C, indented as a unit.
//
// The lines are shifted by the current depth rather than written raw, so a C file
// that lives in the repository and is emitted into a generated one keeps its own
// shape. That is what makes `go:embed` and a builder compose: the static half is
// real C, and it still arrives at the right depth.
func (e *Emitter) EmitBlock(source string) {
	if strings.TrimSpace(source) == "" {
		return
	}
	e.wrote = true
	for _, line := range strings.Split(strings.TrimRight(source, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			e.buf.WriteByte('\n')
			continue
		}
		e.buf.WriteString(e.prefix())
		e.buf.WriteString(strings.TrimRight(line, " \t\r"))
		e.buf.WriteByte('\n')
	}
}

// Block is a braced construct: the opening line, the body emitted by `body`, and
// the closing brace at the outer level.
//
// The pair is one call because the two halves are one thing. An emitter that
// offers `Open` and `Close` separately will eventually be used with the wrong
// number of them, and the result is a brace at the wrong depth in generated code,
// which reads as a logic error rather than as a formatting one.
func (e *Emitter) Block(open string, body func()) {
	e.EmitLine(open)
	e.Indent()
	if body != nil {
		body()
	}
	e.Dedent()
	e.EmitLine("}")
}

// Braced is a construct whose body is one line, as in `if (x) { return; }`.
func (e *Emitter) Braced(open, statement string) {
	e.EmitLine(open + " { " + statement + " }")
}
