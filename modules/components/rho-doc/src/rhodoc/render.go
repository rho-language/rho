//
// @file render.go
// @brief The HTML renderer.
//
// A document is rendered as a page in the shape a documentation reader
// expects: a title, an anchor on every heading, a table of contents, and blocks
// that read the way they are written. The output is a static page, so it can be
// served by anything that serves files, and the stylesheet travels with the
// site rather than living in a server.
//
// Rendering is separate from parsing, so a plugin or a theme can replace either
// half. The renderer never reads Markdown and the parser never sees HTML.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"html"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Page rendering                                                            */
/* -------------------------------------------------------------------------- */

// Page is a rendered page: the body HTML plus what a layout needs around it.
type Page struct {
	Title    string
	Target   string
	Body     string
	Sections []Section
	// Breadcrumb is the trail of section names leading to the page.
	Breadcrumb []string
}

// RenderPage turns a document into the body of a page.
func RenderPage(document *Document) Page {
	r := &renderer{}
	body := strings.Builder{}
	body.WriteString("<section class=\"document\">\n")
	for _, block := range document.Blocks {
		r.render(&body, block)
	}
	body.WriteString("</section>\n")
	return Page{
		Title:    document.Title,
		Target:   document.Target,
		Body:     body.String(),
		Sections: document.Sections,
	}
}

type renderer struct{}

// render writes one node. An unknown kind writes its own text, so a construct
// the renderer does not know about is still visible on the page.
func (r *renderer) render(b *strings.Builder, node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case BlockHeading:
		level := node.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		id := ""
		if node.ID != "" {
			id = " id=\"" + html.EscapeString(node.ID) + "\""
		}
		b.WriteString("<h" + itoa(level) + id + ">")
		r.renderChildren(b, node)
		b.WriteString("</h" + itoa(level) + ">\n")

	case BlockParagraph:
		b.WriteString("<p>")
		r.renderChildren(b, node)
		b.WriteString("</p>\n")

	case BlockCode:
		class := ""
		if node.Lang != "" {
			class = " class=\"language-" + html.EscapeString(node.Lang) + "\""
		}
		b.WriteString("<pre><code" + class + ">")
		b.WriteString(html.EscapeString(nodeText(node)))
		b.WriteString("</code></pre>\n")

	case BlockQuote:
		b.WriteString("<blockquote>\n")
		r.renderChildren(b, node)
		b.WriteString("</blockquote>\n")

	case BlockAdmonition:
		b.WriteString("<div class=\"admonition " + html.EscapeString(node.Anchor) + "\">\n")
		b.WriteString("<p class=\"admonition-title\">" + html.EscapeString(title(node.Anchor)) + "</p>\n")
		r.renderChildren(b, node)
		b.WriteString("</div>\n")

	case BlockDirective:
		b.WriteString("<div class=\"directive " + html.EscapeString(node.Anchor) + "\"")
		if node.Text != "" {
			b.WriteString(" data-arguments=\"" + html.EscapeString(node.Text) + "\"")
		}
		b.WriteString(">\n")
		if node.Text != "" {
			b.WriteString("<p class=\"directive-arguments\"><code>" + html.EscapeString(node.Text) + "</code></p>\n")
		}
		r.renderChildren(b, node)
		b.WriteString("</div>\n")

	case BlockList:
		tag := "ul"
		if node.Ordered {
			tag = "ol"
		}
		b.WriteString("<" + tag + ">\n")
		for _, item := range node.Children {
			b.WriteString("<li>")
			r.renderItem(b, item)
			b.WriteString("</li>\n")
		}
		b.WriteString("</" + tag + ">\n")

	case BlockListItem:
		b.WriteString("<li>")
		r.renderItem(b, node)
		b.WriteString("</li>\n")

	case BlockTable:
		b.WriteString("<table>\n")
		for _, row := range node.Children {
			r.renderRow(b, row)
		}
		// The header row opens the body, so a table that has one has a body to
		// close.
		if len(node.Children) > 0 && node.Children[0].Text == "header" {
			b.WriteString("</tbody>\n")
		}
		b.WriteString("</table>\n")

	case BlockRow:
		r.renderRow(b, node)

	case BlockCell:
		r.renderChildren(b, node)

	case InlineText:
		b.WriteString(renderText(node.Text))

	case InlineCode:
		b.WriteString("<code>" + html.EscapeString(node.Text) + "</code>")

	case InlineStrong:
		b.WriteString("<strong>")
		r.renderChildren(b, node)
		b.WriteString("</strong>")

	case InlineEmphasis:
		b.WriteString("<em>")
		r.renderChildren(b, node)
		b.WriteString("</em>")

	case InlineLink:
		href := safeHref(node.Href)
		// An image carries its own text rather than children.
		if node.Text != "" && len(node.Children) == 0 {
			b.WriteString("<a href=\"" + href + "\">" + html.EscapeString(node.Text) + "</a>")
			return
		}
		b.WriteString("<a href=\"" + href + "\">")
		r.renderChildren(b, node)
		b.WriteString("</a>")

	case InlineRole:
		// A role resolves at site level, where every target is known. Here it
		// is rendered as its label, and the site builder rewrites the href.
		b.WriteString("<a class=\"xref\" href=\"#" + html.EscapeString(xrefAnchor(node.Href)) + "\">")
		if node.Text != "" {
			b.WriteString(html.EscapeString(node.Text))
		} else {
			b.WriteString(html.EscapeString(node.Href))
		}
		b.WriteString("</a>")

	case InlineBreak:
		b.WriteString("<br />\n")

	case BlockHTML:
		// Raw text a block carries verbatim: a code body, or markup an author
		// wrote for a plugin to interpret. It is escaped, never trusted.
		b.WriteString(html.EscapeString(node.Text))

	default:
		r.renderChildren(b, node)
	}
}

// renderChildren writes the children of a node, inserting the newlines a list
// item and a paragraph need so the output reads as markup rather than one line.
func (r *renderer) renderChildren(b *strings.Builder, node *Node) {
	for index, child := range node.Children {
		if index > 0 && (child.Kind == BlockParagraph || child.Kind == BlockList) {
			b.WriteString("\n")
		}
		r.render(b, child)
	}
}

// renderItem writes the body of a list item. An item that is one paragraph
// stays inline, the way Markdown reads.
func (r *renderer) renderItem(b *strings.Builder, item *Node) {
	blocks := item.Children
	if len(blocks) == 1 && blocks[0].Kind == BlockParagraph {
		r.renderChildren(b, blocks[0])
		return
	}
	r.renderChildren(b, item)
}

// renderRow writes a row. The header row opens the head and then the body, so
// the table has one of each and the body starts where the head ends.
func (r *renderer) renderRow(b *strings.Builder, row *Node) {
	if row.Text == "header" {
		b.WriteString("<thead>\n<tr>")
		for _, cell := range row.Children {
			b.WriteString("<th>")
			r.renderChildren(b, cell)
			b.WriteString("</th>")
		}
		b.WriteString("</tr>\n</thead>\n<tbody>\n")
		return
	}
	b.WriteString("<tr>")
	for _, cell := range row.Children {
		b.WriteString("<td>")
		r.renderChildren(b, cell)
		b.WriteString("</td>")
	}
	b.WriteString("</tr>\n")
}

// renderText writes a text run, turning a newline into a line break, so a
// paragraph written over several source lines reads as several lines.
func renderText(text string) string {
	escaped := html.EscapeString(text)
	escaped = strings.ReplaceAll(escaped, "\n", "<br />\n")
	return escaped
}

// safeHref is a link destination a page may carry.
//
// A source is text an author wrote, but a generated site is served to readers:
// a `javascript:` or `data:` destination in a link would run in the reader's
// browser, so only the schemes a documentation page has any business carrying
// are written, and anything else becomes plain text. A relative path, a
// fragment and an absolute http(s) or mailto destination are all allowed.
func safeHref(href string) string {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" {
		return ""
	}
	lowered := strings.ToLower(trimmed)
	// A relative destination, an absolute path and a fragment are what a page
	// links to inside a site.
	if !strings.Contains(lowered, ":") || strings.HasPrefix(lowered, "./") || strings.HasPrefix(lowered, "../") || strings.HasPrefix(lowered, "/") || strings.HasPrefix(lowered, "#") {
		return html.EscapeString(trimmed)
	}
	for _, allowed := range []string{"http:", "https:", "mailto:"} {
		if strings.HasPrefix(lowered, allowed) {
			return html.EscapeString(trimmed)
		}
	}
	// Anything else — javascript:, data:, vbscript: — is not a destination, so
	// it stays as the text it was written as.
	return ""
}

// xrefAnchor is the fragment a cross-reference points at. A role names a
// target, and a target that reads as a page is addressed by its own fragment.
func xrefAnchor(target string) string {
	return strings.TrimSpace(strings.TrimPrefix(target, "#"))
}

// title names an admonition the way a reader expects to see it.
func title(name string) string {
	if name == "" {
		return "Note"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// nodeText is the text a block carries, which a code block holds verbatim.
func nodeText(node *Node) string {
	if node.Text != "" {
		return node.Text
	}
	if len(node.Children) == 1 && node.Children[0].Kind == BlockHTML {
		return node.Children[0].Text
	}
	return ""
}
