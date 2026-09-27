//
// @file api.go
// @brief Generating an API reference from Rho sources.
//
// The language documents itself twice. The prose lives in doc-as-code sources,
// which a person writes; the interface lives in the code, which a machine can
// read. This file is the machine half: it parses Rho sources with the compiler's
// own front end and writes a reference of the modules, functions, structs and
// externs it finds.
//
// Generating the reference from the sources is the only way it stays true. A
// reference copied into prose is a reference that is wrong the day the code
// changes, and a reader who finds one wrong stops trusting the rest.
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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	rhoparser "rho/src/frontend/parser"
)

/* -------------------------------------------------------------------------- */
/*  The reference model                                                       */
/* -------------------------------------------------------------------------- */

// ApiModule is one Rho module in the reference.
type ApiModule struct {
	// Name is the module name declared by the source, which is the name an
	// import uses.
	Name string
	// Path is where the source was read from.
	Path string
	// Functions are the functions it declares, sorted by name.
	Functions []ApiFunction
	// Structs are the structs it declares, sorted by name.
	Structs []ApiStruct
	// Externs are the functions the host supplies, which is what a port does.
	Externs []ApiExtern
}

// ApiFunction is one declared function.
type ApiFunction struct {
	Name       string
	Parameters []ApiParameter
	Result     string
}

// ApiParameter is one parameter, with its name and its type.
type ApiParameter struct {
	Name string
	Type string
}

// Signature is the declaration as it is written, which is what a reader copies.
func (f ApiFunction) Signature() string {
	parts := make([]string, 0, len(f.Parameters))
	for _, parameter := range f.Parameters {
		parts = append(parts, parameter.Name+": "+parameter.Type)
	}
	return "fn " + f.Name + "(" + strings.Join(parts, ", ") + ") " + f.Result
}

// ApiStruct is one declared struct and its fields.
type ApiStruct struct {
	Name   string
	Fields []ApiParameter
}

// ApiExtern is one function the host supplies for a module.
type ApiExtern struct {
	Name       string
	Parameters []ApiParameter
	Result     string
}

/* -------------------------------------------------------------------------- */
/*  Reading sources                                                           */
/* -------------------------------------------------------------------------- */

// ReadApiSources parses every Rho source under a directory and returns the
// modules they declare, sorted by module name.
//
// A source that does not parse is skipped and reported by the caller through
// the diagnostics, because a reference page for a file that does not compile is
// worse than one page fewer.
func ReadApiSources(root string) ([]ApiModule, []Diagnostic) {
	modules := []ApiModule{}
	diagnostics := []Diagnostic{}
	seen := map[string]bool{}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".kilo", "node_modules", "build", "vendor", "tests":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".rho") {
			return nil
		}
		// A test source exercises the language; it is not part of the
		// interface, and a test-only syntax should not be reported as a
		// source the reference could not read.
		if isTestSource(path) {
			return nil
		}
		module, diagnostic, ok := readApiModule(path)
		if !ok {
			if diagnostic.Message != "" {
				diagnostics = append(diagnostics, diagnostic)
			}
			return nil
		}
		// A module is declared once: an include brings a file's declarations
		// into another file, and the reference names the module, not the file.
		if seen[module.Name] {
			return nil
		}
		seen[module.Name] = true
		modules = append(modules, module)
		return nil
	})

	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })
	return modules, diagnostics
}

// isTestSource reports whether a source is a test rather than a module.
func isTestSource(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, "_test.rho") || strings.HasPrefix(base, "test_")
}

// readApiModule parses one source into its reference entries.
func readApiModule(path string) (ApiModule, Diagnostic, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ApiModule{}, Diagnostic{Path: path, Message: err.Error()}, false
	}
	tokens, err := lexer.Lex(string(data))
	if err != nil {
		return ApiModule{}, Diagnostic{Path: path, Line: lineOf(err), Message: "lexing: " + err.Error()}, false
	}
	module, err := rhoparser.Parse(tokens)
	if err != nil {
		return ApiModule{}, Diagnostic{Path: path, Line: lineOf(err), Message: "parsing: " + err.Error()}, false
	}

	entry := ApiModule{Name: module.Name, Path: path}
	for _, declaration := range module.Decls {
		switch decl := declaration.(type) {
		case ast.FuncDecl:
			// An extern is a function the host supplies, which is what a port
			// reaches, so it is listed apart from the functions Rho declares.
			if decl.Extern {
				entry.Externs = append(entry.Externs, ApiExtern{
					Name:       decl.Name,
					Parameters: parameters(decl.Params),
					Result:     decl.ReturnType,
				})
				continue
			}
			entry.Functions = append(entry.Functions, ApiFunction{
				Name:       decl.Name,
				Parameters: parameters(decl.Params),
				Result:     decl.ReturnType,
			})
		case ast.StructDecl:
			entry.Structs = append(entry.Structs, ApiStruct{Name: decl.Name, Fields: fields(decl.Fields)})
		}
	}
	sort.Slice(entry.Functions, func(i, j int) bool { return entry.Functions[i].Name < entry.Functions[j].Name })
	sort.Slice(entry.Structs, func(i, j int) bool { return entry.Structs[i].Name < entry.Structs[j].Name })
	sort.Slice(entry.Externs, func(i, j int) bool { return entry.Externs[i].Name < entry.Externs[j].Name })
	return entry, Diagnostic{}, true
}

func fields(declared []ast.Field) []ApiParameter {
	out := make([]ApiParameter, 0, len(declared))
	for _, field := range declared {
		out = append(out, ApiParameter{Name: field.Name, Type: field.Type})
	}
	return out
}

func parameters(params []ast.Param) []ApiParameter {
	out := make([]ApiParameter, 0, len(params))
	for _, param := range params {
		out = append(out, ApiParameter{Name: param.Name, Type: param.Type})
	}
	return out
}

// lineOf pulls a source line out of a diagnostic, which carries one when it
// knows it.
func lineOf(err error) int {
	type located interface{ Line() int }
	if located, ok := err.(located); ok {
		return located.Line()
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Rendering                                                                 */
/* -------------------------------------------------------------------------- */

// ApiTarget is the target a generated reference page is addressed by.
func ApiTarget(module string) string {
	return "api-" + headingSlug(module)
}

// RenderApiModule writes a reference page for one module as a document, so it
// travels through the same renderer as a page an author wrote.
func RenderApiModule(module ApiModule) *Document {
	document := &Document{
		Path:   module.Path,
		Title:  module.Name,
		Target: ApiTarget(module.Name),
	}

	document.Blocks = append(document.Blocks,
		heading(1, module.Name, document.Target),
		paragraph("The module `module "+module.Name+"`, read from `"+module.Path+"`. This page is generated from the source, so it describes what the code declares."),
	)

	if len(module.Functions) > 0 {
		document.Blocks = append(document.Blocks, heading(2, "Functions", ""))
		for _, function := range module.Functions {
			document.Blocks = append(document.Blocks, heading(3, function.Name+"()", ""))
			document.Blocks = append(document.Blocks, codeBlock("rho", function.Signature()))
		}
	}
	if len(module.Structs) > 0 {
		document.Blocks = append(document.Blocks, heading(2, "Structs", ""))
		table := &Node{Kind: BlockTable}
		header := &Node{Kind: BlockRow, Text: "header"}
		header.Children = []*Node{cell("Struct"), cell("Fields")}
		table.Children = append(table.Children, header)
		for _, structure := range module.Structs {
			row := &Node{Kind: BlockRow}
			row.Children = []*Node{
				cell(structure.Name),
				cell(fieldList(structure.Fields)),
			}
			table.Children = append(table.Children, row)
		}
		document.Blocks = append(document.Blocks, table)
	}
	if len(module.Externs) > 0 {
		document.Blocks = append(document.Blocks, heading(2, "Externs", ""))
		document.Blocks = append(document.Blocks,
			paragraph("Externs are supplied by the host rather than declared in Rho, which is how a port reaches a service."),
		)
		table := &Node{Kind: BlockTable}
		header := &Node{Kind: BlockRow, Text: "header"}
		header.Children = []*Node{cell("Extern"), cell("Signature")}
		table.Children = append(table.Children, header)
		for _, extern := range module.Externs {
			row := &Node{Kind: BlockRow}
			signature := "extern fn " + extern.Name + "("
			parts := []string{}
			for _, parameter := range extern.Parameters {
				parts = append(parts, parameter.Name+": "+parameter.Type)
			}
			signature += strings.Join(parts, ", ") + ") " + extern.Result
			row.Children = []*Node{cell(extern.Name), cell("`" + signature + "`")}
			table.Children = append(table.Children, row)
		}
		document.Blocks = append(document.Blocks, table)
	}

	document.collectHeadings()
	return document
}

// RenderApiIndex writes the reference index: every module the scan found, with
// what it declares, so a reader can see the shape of the library at a glance.
func RenderApiIndex(modules []ApiModule, section string) *Document {
	document := &Document{
		Path:   "std",
		Title:  "API reference",
		Target: "api",
	}
	document.Blocks = append(document.Blocks,
		heading(1, "API reference", "api"),
		paragraph("The reference is generated from the Rho sources in this tree, so every entry is the declaration the compiler sees."),
	)
	if len(modules) == 0 {
		document.Blocks = append(document.Blocks, paragraph(":::{note}\nNo Rho source was found, so the reference is empty.\n:::"))
		document.collectHeadings()
		return document
	}
	table := &Node{Kind: BlockTable}
	header := &Node{Kind: BlockRow, Text: "header"}
	header.Children = []*Node{cell("Module"), cell("Functions"), cell("Structs"), cell("Externs"), cell("Source")}
	table.Children = append(table.Children, header)
	for _, module := range modules {
		row := &Node{Kind: BlockRow}
		row.Children = []*Node{
			cell("{ref}`" + module.Name + " <" + ApiTarget(module.Name) + ">`"),
			cell(itoa(len(module.Functions))),
			cell(itoa(len(module.Structs))),
			cell(itoa(len(module.Externs))),
			cell("`" + module.Path + "`"),
		}
		table.Children = append(table.Children, row)
	}
	document.Blocks = append(document.Blocks, table)
	document.collectHeadings()
	_ = section
	return document
}

func cell(text string) *Node {
	return &Node{Kind: BlockCell, Children: []*Node{{Kind: InlineText, Text: text}}}
}

func fieldList(fields []ApiParameter) string {
	parts := []string{}
	for _, field := range fields {
		parts = append(parts, field.Name+": "+field.Type)
	}
	return strings.Join(parts, ", ")
}

func heading(level int, text, target string) *Node {
	node := &Node{
		Kind:     BlockHeading,
		Level:    level,
		Text:     text,
		Target:   target,
		ID:       target,
		Children: parseInline(text),
	}
	if node.ID == "" {
		node.ID = headingSlug(text)
	}
	return node
}

func paragraph(text string) *Node {
	return &Node{Kind: BlockParagraph, Children: parseInline(text)}
}

// codeBlock is a rendered code block. A signature is written as a block rather
// than as a paragraph holding a fence, so it keeps its own line breaks and its
// language instead of collapsing into inline code.
func codeBlock(lang, body string) *Node {
	return &Node{
		Kind:     BlockCode,
		Lang:     lang,
		Children: []*Node{{Kind: BlockHTML, Text: body}},
	}
}

/* -------------------------------------------------------------------------- */
/*  Search text                                                               */
/* -------------------------------------------------------------------------- */

// ApiSearchText is the text of a reference page for the search index, which
// has to carry the names a reader would search for even though they appear as
// markup on the page.
func ApiSearchText(module ApiModule) string {
	parts := []string{module.Name}
	for _, function := range module.Functions {
		parts = append(parts, function.Name, function.Signature())
	}
	for _, structure := range module.Structs {
		parts = append(parts, structure.Name, fieldList(structure.Fields))
	}
	for _, extern := range module.Externs {
		parts = append(parts, extern.Name)
	}
	return strings.Join(parts, " ")
}

// EscapeTarget is exported for the command, which reports a target in a log
// line where markup would be noise.
func EscapeTarget(target string) string { return html.EscapeString(target) }

/* -------------------------------------------------------------------------- */
/*  A reference as a set of pages                                             */
/* -------------------------------------------------------------------------- */

// ApiSet is a generated reference: an index page and one page per module.
//
// It is built apart from the site, because the reference is a different kind of
// page from a written one: it comes from the code rather than from a person, and
// it is marked as generated so a reader can tell the two apart.
type ApiSet struct {
	// Section is where the reference appears in the sidebar.
	Section string
	// Modules are the modules the scan found, sorted by name.
	Modules []ApiModule
	// Diagnostics are the sources that could not be read.
	Diagnostics []Diagnostic
}

// ReadApiSet generates a reference for a tree.
//
// A tree that holds no Rho source yields an empty set rather than an error: a
// repository that documents a language without shipping sources of it is a
// repository with prose documentation, which is still documentation.
func ReadApiSet(root, section string) ApiSet {
	set := ApiSet{Section: section}
	if root == "" {
		return set
	}
	if set.Section == "" {
		set.Section = "api"
	}
	modules, diagnostics := ReadApiSources(root)
	// A path a reader can use is one they can resolve: the site is written for
	// a tree, so a page names its source relative to that tree rather than
	// printing the machine it was built on.
	for index := range modules {
		if relative, err := filepath.Rel(root, modules[index].Path); err == nil {
			modules[index].Path = filepath.ToSlash(relative)
		}
	}
	set.Modules = modules
	set.Diagnostics = diagnostics
	return set
}

// Empty reports whether the scan found nothing to document.
func (a ApiSet) Empty() bool { return len(a.Modules) == 0 }

// documents are the reference pages, in site order: the index first, then one
// page per module, both in the reference section.
func (a ApiSet) documents() []Document {
	documents := []Document{}
	if a.Empty() {
		return documents
	}
	index := RenderApiIndex(a.Modules, a.Section)
	documents = append(documents, *index)
	for _, module := range a.Modules {
		documents = append(documents, *RenderApiModule(module))
	}
	return documents
}
