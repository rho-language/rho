//
// @file document.go
// @brief The documentation document model.
//
// A doc-as-code source is MyST-style Markdown: Markdown for prose, with the
// Sphinx constructs that a reference needs — explicit targets, admonitions,
// directives and cross-reference roles. Parsing it produces a document, a tree
// of blocks with inline spans, which the renderer turns into HTML. Nothing in
// this file knows about HTML, and nothing in the renderer knows about
// Markdown, so either half can be replaced without touching the other.
//
// The dialect is deliberately small, and a construct that is not understood
// is kept as text rather than dropped. Documentation that silently loses a
// paragraph is worse than documentation that shows the paragraph raw.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import "strings"

/* -------------------------------------------------------------------------- */
/*  Document model                                                             */
/* -------------------------------------------------------------------------- */

// Kind is the kind of a block or an inline span.
type Kind string

const (
	// Blocks.
	BlockDocument   Kind = "document"
	BlockHeading    Kind = "heading"
	BlockParagraph  Kind = "paragraph"
	BlockList       Kind = "list"
	BlockListItem   Kind = "list_item"
	BlockCode       Kind = "code_block"
	BlockQuote      Kind = "quote"
	BlockAdmonition Kind = "admonition"
	BlockTable      Kind = "table"
	BlockRow        Kind = "row"
	BlockCell       Kind = "cell"
	BlockDirective  Kind = "directive"
	BlockHTML       Kind = "html"

	// Inline spans.
	InlineText     Kind = "text"
	InlineCode     Kind = "code"
	InlineEmphasis Kind = "emphasis"
	InlineStrong   Kind = "strong"
	InlineLink     Kind = "link"
	InlineRole     Kind = "role"
	InlineBreak    Kind = "break"
)

// Node is one block or inline span.
type Node struct {
	Kind     Kind
	Level    int    // heading level, 1 to 6
	Text     string // the raw text a leaf carries
	Lang     string // the language of a code block
	Target   string // the explicit target of a heading, {#id}
	ID       string // the identifier a node is addressed by
	Href     string // the destination of a link or role
	Role     string // the role name of a cross-reference
	Ordered  bool   // a list is numbered rather than bulleted
	Anchor   string // the admonition or directive name
	Children []*Node
	// Line is the source line a block starts on, so a diagnostic can point at
	// the source rather than at a byte offset in a rendered page.
	Line int
}

// Document is a parsed doc-as-code source.
type Document struct {
	// Path is the file the document was read from, used in diagnostics.
	Path  string
	Title string
	// Target is the document's own identifier, from its file name. A
	// cross-reference resolves against it.
	Target string
	// Blocks is the body, with the leading heading kept: a page that opens with
	// a heading is titled by it, and a page that does not is titled by the
	// document metadata.
	Blocks []*Node
	// Sections are the headings, in document order, which is the page table of
	// contents.
	Sections []Section
	// Diagnostics are the problems found while parsing. A source that cannot be
	// understood is reported, not silently shortened.
	Diagnostics []Diagnostic
}

// Section is one heading in a document.
type Section struct {
	ID    string
	Title string
	Level int
	Line  int
	// Explicit records that the author wrote the identifier as {#id}. Only an
	// explicit identifier is addressable from another page: a derived one is a
	// convenience within a page, and treating it as a name site-wide would let
	// two pages' slugs collide and a reference to it be ambiguous.
	Explicit bool
}

// Diagnostic is one problem found while reading a source.
type Diagnostic struct {
	Path    string
	Line    int
	Message string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Path
	}
	return d.Path + ":" + itoa(d.Line) + ": " + d.Message
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	negative := value < 0
	if negative {
		value = -value
	}
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	if negative {
		return "-" + digits
	}
	return digits
}

// headingSlug derives the identifier of a heading from its text, the way a
// documentation tool does when the author gives no explicit target.
func headingSlug(text string) string {
	slug := strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	previousDash := false
	for _, r := range slug {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			previousDash = false
		default:
			// A run of anything else becomes a single separator, so a heading
			// reads as one slug rather than one word per punctuation mark.
			if !previousDash && b.Len() > 0 {
				b.WriteByte('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
