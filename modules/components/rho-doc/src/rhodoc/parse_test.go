//
// @file parse_test.go
// @brief Tests for the MyST-style documentation parser.
//
// The parser is where doc-as-code is either understood or lost, so each
// construct of the dialect is pinned by a test: a construct that silently stops
// parsing is a page that silently loses its content.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodoc

import (
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Headings                                                                  */
/* -------------------------------------------------------------------------- */

func TestParseHeadingTakesTheTitleAndSections(t *testing.T) {
	document := Parse("docs/index.md", "# Registry\n\n## Publishing\n\n### Tokens\n")

	if document.Title != "Registry" {
		t.Fatalf("expected the title Registry, got %q", document.Title)
	}
	if len(document.Sections) != 3 {
		t.Fatalf("expected three sections, got %d", len(document.Sections))
	}
	if document.Sections[1].Title != "Publishing" || document.Sections[1].Level != 2 {
		t.Fatalf("unexpected section: %+v", document.Sections[1])
	}
	if document.Sections[1].ID != "publishing" {
		t.Fatalf("expected a derived target, got %q", document.Sections[1].ID)
	}
}

func TestParseHeadingKeepsAnExplicitTarget(t *testing.T) {
	document := Parse("docs/index.md", "## Publishing {#publish-a-version}\n")

	if document.Sections[0].ID != "publish-a-version" {
		t.Fatalf("expected the explicit target, got %q", document.Sections[0].ID)
	}
	if document.Sections[0].Title != "Publishing" {
		t.Fatalf("expected the target stripped from the title, got %q", document.Sections[0].Title)
	}
}

func TestParseKeepsDuplicateTargetsApart(t *testing.T) {
	document := Parse("docs/index.md", "# Notes\n\n## Item\n\n## Item\n")

	first, second := document.Sections[1].ID, document.Sections[2].ID
	if first == second {
		t.Fatalf("expected distinct targets, both were %q", first)
	}
}

func TestParseWarnsAboutARepeatedExplicitTarget(t *testing.T) {
	document := Parse("docs/index.md", "# Notes\n\n## One {#same}\n\n## Two {#same}\n")

	if !hasDiagnostic(document, "used more than once") {
		t.Fatalf("expected a diagnostic about the repeated target, got %+v", document.Diagnostics)
	}
}

func TestParseReportsAPageWithoutATitle(t *testing.T) {
	document := Parse("docs/notes.md", "Just a paragraph, with no heading.\n")

	if !hasDiagnostic(document, "no heading") {
		t.Fatalf("expected a diagnostic about the missing heading, got %+v", document.Diagnostics)
	}
}

func TestParseDerivesTheDocumentTargetFromTheFileName(t *testing.T) {
	if target := Parse("docs/deployment-guide.md", "# Guide\n").Target; target != "deployment-guide" {
		t.Fatalf("expected the file name as the target, got %q", target)
	}
}

/* -------------------------------------------------------------------------- */
/*  Prose and inline spans                                                    */
/* -------------------------------------------------------------------------- */

func TestParseParagraphKeepsItsText(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nA module documents itself\nin the sources it ships.\n")

	paragraph := document.Blocks[1]
	if paragraph.Kind != BlockParagraph {
		t.Fatalf("expected a paragraph, got %s", paragraph.Kind)
	}
	if !strings.Contains(renderInlineText(paragraph), "documents itself") {
		t.Fatalf("expected the paragraph text, got %q", renderInlineText(paragraph))
	}
}

func TestParseInlineSpans(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nCall `rho doc` to see *emphasis*, **strong** and [the site](https://rho-lang.org).\n")

	page := RenderPage(document)
	for _, want := range []string{
		"<code>rho doc</code>",
		"<em>emphasis</em>",
		"<strong>strong</strong>",
		"<a href=\"https://rho-lang.org\">the site</a>",
	} {
		if !strings.Contains(page.Body, want) {
			t.Fatalf("expected %s in the page, got:\n%s", want, page.Body)
		}
	}
}

func TestParseCodeBlockKeepsItsLanguageAndBody(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n```rho\nfn main() void {\n    print(\"hi\")\n}\n```\n")

	page := RenderPage(document)
	if !strings.Contains(page.Body, "class=\"language-rho\"") {
		t.Fatalf("expected the language on the code block, got:\n%s", page.Body)
	}
	if !strings.Contains(page.Body, "print(&#34;hi&#34;)") {
		t.Fatalf("expected the escaped body, got:\n%s", page.Body)
	}
}

func TestParseEscapesMarkupInProse(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nA <script> tag is text, not markup.\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "<script>") {
		t.Fatalf("expected the tag to be escaped, got:\n%s", page.Body)
	}
}

/* -------------------------------------------------------------------------- */
/*  Lists, quotes, tables                                                     */
/* -------------------------------------------------------------------------- */

func TestParseBulletList(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n- one\n- two\n  - nested\n")

	list := document.Blocks[1]
	if list.Kind != BlockList || list.Ordered {
		t.Fatalf("expected a bullet list, got %s", list.Kind)
	}
	if len(list.Children) != 2 {
		t.Fatalf("expected two items, got %d", len(list.Children))
	}
	if !strings.Contains(RenderPage(document).Body, "<ul>") {
		t.Fatal("expected a bullet list in the page")
	}
}

func TestParseOrderedList(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n1. first\n2. second\n")

	list := document.Blocks[1]
	if list.Kind != BlockList || !list.Ordered {
		t.Fatalf("expected a numbered list, got %s", list.Kind)
	}
	if !strings.Contains(RenderPage(document).Body, "<ol>") {
		t.Fatal("expected a numbered list in the page")
	}
}

func TestParseQuote(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n> Rho has no heap.\n")

	quote := document.Blocks[1]
	if quote.Kind != BlockQuote {
		t.Fatalf("expected a quote, got %s", quote.Kind)
	}
	if !strings.Contains(RenderPage(document).Body, "<blockquote>") {
		t.Fatal("expected a quote in the page")
	}
}

func TestParseTable(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n| Field | Type |\n|-------|------|\n| x | u8 |\n")

	page := RenderPage(document)
	for _, want := range []string{"<table>", "<thead>", "<th>Field</th>", "<td>u8</td>"} {
		if !strings.Contains(page.Body, want) {
			t.Fatalf("expected %s in the page, got:\n%s", want, page.Body)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Admonitions and directives                                                */
/* -------------------------------------------------------------------------- */

func TestParseAdmonition(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n:::{note}\nA source file is documentation too.\n:::\n")

	admonition := document.Blocks[1]
	if admonition.Kind != BlockAdmonition || admonition.Anchor != "note" {
		t.Fatalf("expected a note admonition, got %s %q", admonition.Kind, admonition.Anchor)
	}
	page := RenderPage(document)
	if !strings.Contains(page.Body, "class=\"admonition note\"") {
		t.Fatalf("expected a framed note, got:\n%s", page.Body)
	}
}

func TestParseDirectiveKeepsItsNameAndArguments(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n:::{toctree}\n:maxdepth: 2\n\nindex\n:::\n")

	directive := document.Blocks[1]
	if directive.Kind != BlockDirective || directive.Anchor != "toctree" {
		t.Fatalf("expected a toctree directive, got %s %q", directive.Kind, directive.Anchor)
	}
	if !strings.Contains(RenderPage(document).Body, "class=\"directive toctree\"") {
		t.Fatal("expected the directive in the page")
	}
}

func TestParseDirectiveWithArguments(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n:::{toctree} extra\nindex\n:::\n")

	directive := document.Blocks[1]
	if directive.Text != "extra" {
		t.Fatalf("expected the directive arguments, got %q", directive.Text)
	}
}

func TestParseCodeDirectiveKeepsItsBodyVerbatim(t *testing.T) {
	// A code block shows what an author wrote, so what is inside it is not read
	// as documentation: a role written there is an example, not a reference.
	document := Parse("docs/index.md", "# Index\n\n:::{code-block} markdown\nSee {ref}`nowhere` here.\n:::\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "class=\"xref\"") {
		t.Fatalf("expected the example to stay text, got:\n%s", page.Body)
	}
	if !strings.Contains(page.Body, "class=\"language-markdown\"") {
		t.Fatalf("expected a code block with its language, got:\n%s", page.Body)
	}
	if len(document.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostic from an example, got %+v", document.Diagnostics)
	}
	roles := collectRoles(document)
	if len(roles) != 0 {
		t.Fatalf("expected the code block to hold no cross-references, got %+v", roles)
	}
}

/* -------------------------------------------------------------------------- */
/*  Cross-reference roles                                                     */
/* -------------------------------------------------------------------------- */

func TestParseRoleInMySTSpelling(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nSee {ref}`deployment`.\n")

	page := RenderPage(document)
	if !strings.Contains(page.Body, "class=\"xref\"") || !strings.Contains(page.Body, ">deployment<") {
		t.Fatalf("expected a cross-reference, got:\n%s", page.Body)
	}
}

func TestParseRoleInClassicSpelling(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nSee :ref:`deployment`.\n")

	if !strings.Contains(RenderPage(document).Body, "class=\"xref\"") {
		t.Fatalf("expected a cross-reference, got:\n%s", RenderPage(document).Body)
	}
}

func TestParseRoleWithAnExplicitTarget(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\nSee {ref}`the guide <deployment>`.\n")

	page := RenderPage(document)
	if !strings.Contains(page.Body, "the guide") || !strings.Contains(page.Body, "href=\"#deployment\"") {
		t.Fatalf("expected the label and the target, got:\n%s", page.Body)
	}
}

func TestParseKeepsAnUnfinishedRoleAsText(t *testing.T) {
	// A half-written role is text, not a broken link: a page that shows the
	// author what they typed is easier to fix than a page with a dead link.
	document := Parse("docs/index.md", "# Index\n\nA {ref}` without a closing backtick.\n")

	if !strings.Contains(renderInlineText(document.Blocks[1]), "{ref}`") {
		t.Fatal("expected the unfinished role to stay as text")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func hasDiagnostic(document *Document, substring string) bool {
	for _, diagnostic := range document.Diagnostics {
		if strings.Contains(diagnostic.Message, substring) {
			return true
		}
	}
	return false
}

func renderInlineText(node *Node) string {
	var b strings.Builder
	for _, child := range node.Children {
		if child.Text != "" {
			b.WriteString(child.Text)
		}
		b.WriteString(renderInlineText(child))
	}
	return b.String()
}

func renderInlineTextSlice(nodes []*Node) string {
	var b strings.Builder
	for _, node := range nodes {
		b.WriteString(renderInlineText(node))
	}
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Regressions                                                               */
/* -------------------------------------------------------------------------- */

func TestParseOrderedListKeepsTheBodyAndNotTheMarker(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n1. first\n2. second\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "<li>.") || strings.Contains(page.Body, "<li>)") {
		t.Fatalf("expected the marker to be dropped, got:\n%s", page.Body)
	}
	if !strings.Contains(page.Body, "<li>first</li>") {
		t.Fatalf("expected the item body, got:\n%s", page.Body)
	}
}

func TestParseTableRendersTheHeaderOnce(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n| Field | Type |\n|-------|------|\n| x | u8 |\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "<p>| Field | Type |</p>") {
		t.Fatalf("expected the header only inside the table, got:\n%s", page.Body)
	}
	if strings.Count(page.Body, "<tbody>") != 1 || strings.Count(page.Body, "</tbody>") != 1 {
		t.Fatalf("expected one table body, got:\n%s", page.Body)
	}
	if strings.Count(page.Body, "<thead>") != 1 {
		t.Fatalf("expected one table head, got:\n%s", page.Body)
	}
}

func TestParseDirectivesNest(t *testing.T) {
	document := Parse("docs/index.md", "# Index\n\n:::{note}\nouter\n\n:::{tip}\ninner\n:::\n:::\n")

	page := RenderPage(document)
	if !strings.Contains(page.Body, "class=\"admonition note\"") || !strings.Contains(page.Body, "class=\"admonition tip\"") {
		t.Fatalf("expected both directives, got:\n%s", page.Body)
	}
	if strings.Contains(page.Body, "<p>inner<br />") {
		t.Fatalf("expected the inner directive to keep its body, got:\n%s", page.Body)
	}
}

func TestParseKeepsFormattingAfterAStrayMarker(t *testing.T) {
	// One character that starts no construct must not stop the scan: the rest of
	// the paragraph is still read.
	document := Parse("docs/index.md", "# Index\n\nA } b *c* and `d`.\n")

	page := RenderPage(document)
	if !strings.Contains(page.Body, "<em>c</em>") || !strings.Contains(page.Body, "<code>d</code>") {
		t.Fatalf("expected the formatting after the stray character, got:\n%s", page.Body)
	}
}

func TestDiagnosticNamesAnUnlocatedProblem(t *testing.T) {
	// A diagnostic with no file still says something: printing a blank line
	// would report a problem by showing nothing at all.
	if got := (Diagnostic{Line: 3, Message: "boom"}).String(); got != "line 3: boom" {
		t.Fatalf("expected the line and the message, got %q", got)
	}
	if got := (Diagnostic{Message: "boom"}).String(); got != "boom" {
		t.Fatalf("expected the message, got %q", got)
	}
	if got := (Diagnostic{Path: "p.md", Line: 3, Message: "boom"}).String(); got != "p.md:3: boom" {
		t.Fatalf("expected the located form, got %q", got)
	}
}

// A colon and a code span next to each other is prose, not the classic role
// spelling.
//
// `: `Some“ has the same shape as `:ref:`Some“ — a colon, a name, a backtick,
// a target, a backtick — and the only thing that tells them apart is what the name
// is. A sentence that puts a code span, a colon and another code span together is
// ordinary English about code, and reading it as a role turned `Option[T]`:
// `Some` into a cross-reference to a target called "Some" that no page declares:
// a false alarm in a normal build and a hard failure under `-strict`.
func TestAColonBeforeACodeSpanIsProseAndNotARole(t *testing.T) {
	document := Parse("docs/index.md",
		"# Types\n\n| M | F |\n|---|---|\n| `core.option` | `Option[T]`: `Some`, `None` |\n"+
			"There is no `else if`: an `else` whose block is one `if`.\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "class=\"xref\"") {
		t.Fatalf("prose was read as a cross-reference:\n%s", page.Body)
	}
	for _, wanted := range []string{"Some", "None", "else"} {
		if !strings.Contains(page.Body, wanted) {
			t.Errorf("expected %q to survive as text, got:\n%s", wanted, page.Body)
		}
	}
}

// The two spellings of a role are still roles, including one whose name carries a
// digit or a dash, which is how a namespaced role is written.
func TestARoleNameMayCarryDigitsAndDashes(t *testing.T) {
	for _, name := range []string{"ref", "doc", "py-class", "myst_role", "x2"} {
		if !isRoleName(name) {
			t.Errorf("%q should name a role", name)
		}
	}
	for _, name := range []string{"", " ", " ref", "re f", "re:f", "-ref", "2ref"} {
		if isRoleName(name) {
			t.Errorf("%q should not name a role", name)
		}
	}
}
