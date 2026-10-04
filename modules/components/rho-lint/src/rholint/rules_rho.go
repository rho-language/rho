//
// @file rules_rho.go
// @brief The rules over the Rho sources, through the compiler's own parser.
//
// This is the engine that makes `rho lint` worth running on a machine with nothing
// installed, and it is worth running for a particular reason: it is the only engine
// in the run that can see the program *before* it became C. Everything the C
// analyzers say about a generated translation is a finding about the generator's
// output; everything this says is a finding about a line somebody wrote.
//
// Which is not the same set of findings. clang-tidy will report a shadowed local in
// the generated C of a Rho program that shadows nothing in the Rho, because the
// generator introduces locals of its own. This reports the shadow in the source and
// points at the line that causes it, which is the line a person can fix.
//
// Every rule here is decidable from the tree alone. That is a constraint rather
// than a limitation to apologise for: a rule needing types, a call graph or a
// preprocessed view is a rule that would be wrong sometimes, and a lint tool that is
// wrong sometimes is a tool people learn to distrust wholesale. The checks that need
// a front end are clang-tidy's and cppcheck's job, which is what the plugins are for.
//
// A Rho program that must produce nothing from this engine is the test material,
// and so is a C program this engine is never pointed at, because there is nothing
// in it to point at. Both are exercised by the engine's own tests.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"fmt"
	"regexp"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

// The rules over Rho sources.
//
// Every name says which engine it is from, because these two engines share one
// namespace with a third — the C rules — and `unused-import` in one of them is not a
// rule in the other.
const (
	// RhoParse is a file the compiler cannot read.
	//
	// An error rather than a warning, and the reason is worth stating: this is the
	// compiler's own parser, so a file it rejects is a file the build rejects. Every
	// other rule here would find nothing in it, and reporting a clean file because
	// the analyzer could not read it is the one outcome a lint tool must not produce.
	RhoParse = "rho-parse"
	// RhoModuleName is a module whose declared name is not its file's name.
	RhoModuleName = "rho-module-name"
	// RhoShadowedBinding is a `let` whose name is already bound outside it.
	RhoShadowedBinding = "rho-shadowed-binding"
	// RhoUnreachable is a statement after one that always leaves the block.
	RhoUnreachable = "rho-unreachable"
	// RhoEmptyBlock is a block with nothing in it.
	RhoEmptyBlock = "rho-empty-block"
	// RhoUnusedImport is an import nothing in the file names.
	RhoUnusedImport = "rho-unused-import"
	// RhoTodoComment is a marker a person left for themselves. The C engine has one
	// of its own by the same name and a different value, because the two engines
	// share one namespace.
	RhoTodoComment = "rho-todo-comment"
)

// RhoRules is what the `rho` engine checks.
//
// Ordered by how much a finding costs to be wrong about. A file that does not parse
// is first, because it decides whether the rest of the rules can say anything at all;
// then the defects; then the notes.
func RhoRules() RuleSet {
	return RuleSet{
		Name:     EngineRho,
		Language: LanguageRho,
		Rules: []Check{
			{
				ID:          RhoParse,
				Severity:    SeverityError,
				Description: "A source the compiler cannot read, which the build cannot compile either",
				Apply:       rhoParse,
			},
			{
				ID:          RhoModuleName,
				Severity:    SeverityWarning,
				Description: "A module whose declared name is not the name of the file it is in",
				Apply:       rhoModuleName,
			},
			{
				ID:          RhoShadowedBinding,
				Severity:    SeverityWarning,
				Description: "A let whose name is already bound further out, so two bindings share one name",
				Apply:       rhoShadowedBinding,
			},
			{
				ID:          RhoUnreachable,
				Severity:    SeverityNote,
				Description: "A statement after one that always leaves the block, which can never run",
				Apply:       rhoUnreachable,
			},
			{
				ID:          RhoEmptyBlock,
				Severity:    SeverityNote,
				Description: "A block with nothing in it, which reads as one something was meant to go into",
				Apply:       rhoEmptyBlock,
			},
			{
				ID:          RhoUnusedImport,
				Severity:    SeverityNote,
				Description: "An import nothing in this file names",
				Apply:       rhoUnusedImport,
			},
			{
				ID:          RhoTodoComment,
				Severity:    SeverityNote,
				Description: "A TODO, FIXME, XXX or HACK somebody left in a comment",
				Apply:       rhoTodo,
			},
		},
	}
}

// todoMarker is the comment marker both engines look for.
var todoMarker = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

// declarationLine finds where a declaration is written, by its own spelling.
//
// Used for the module and the imports only, and the reason is in the comment on
// `ModulePosition`: those two nodes are the ones the parser does not position, and
// finding the line by the same two keywords the parser requires is exact rather than
// a guess.
func declarationLine(source Source, keyword string, name string) (int, int) {
	pattern := regexp.MustCompile(`(?m)^[ \t]*` + keyword + `[ \t]+` +
		regexp.QuoteMeta(name) + `\b`)
	location := pattern.FindStringIndex(source.Text)
	if location == nil {
		return 0, 0
	}
	line := strings.Count(source.Text[:location[0]], "\n") + 1
	column := location[1] - (strings.LastIndex(source.Text[:location[1]], "\n") + 1)
	return line, column
}

/* -------------------------------------------------------------------------- */
/*  Parsing                                                                    */
/* -------------------------------------------------------------------------- */

// parseRho is a source as the compiler's parser reads it.
//
// Every rule goes through this one function rather than through the parser
// directly, because two rules disagreeing about whether a file parses produces a
// report saying a file has a shadowed binding in it and has a syntax error in it at
// the same time.
func parseRho(source Source) (*ast.Module, error) {
	tokens, err := lexer.Lex(source.Text)
	if err != nil {
		return nil, err
	}
	return parser.Parse(tokens)
}

func rhoParse(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoParse)
	if _, err := parseRho(source); err != nil {
		return []Diagnostic{set.Report(rule, source, 0, 0,
			"the compiler cannot read this file: "+err.Error())}
	}
	return nil
}

// tree is a source parsed once, shared by the rules that need it.
//
// Read once per file rather than once per rule: a file with seven rules would
// otherwise be lexed and parsed seven times, and the parse is the only expensive
// thing a run does. Returns nil for a file that does not parse, which every
// non-parse rule treats as nothing to say — the parse rule is the one that reports
// it, once, rather than seven times.
func tree(source Source) *ast.Module {
	module, err := parseRho(source)
	if err != nil {
		return nil
	}
	return module
}

/* -------------------------------------------------------------------------- */
/*  Module name                                                                */
/* -------------------------------------------------------------------------- */

// rhoModuleName is a module whose declared name is not its file's name.
//
// A real defect with a real consequence and no false positives. A Rho module is
// found by name, so a file called `main.rho` declaring `module hello_world` is a
// module no `import hello_world` can reach: every import of it fails, and the
// failure is a message about a module that does not exist rather than about this
// file.
//
// A module name may be dotted — `module device.hal.gpio` — and only its last
// segment is the file's name. Comparing the whole string to the stem would report
// every module in a tree of them, which is the shape a real project has.
func rhoModuleName(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoModuleName)
	module := tree(source)
	if module == nil {
		return nil
	}
	declared := strings.TrimSpace(module.Name)
	if declared == "" {
		return nil
	}
	stem := fileStem(source.Path)
	segment := declared
	if dot := strings.LastIndex(segment, "."); dot >= 0 {
		segment = segment[dot+1:]
	}
	if segment == stem {
		return nil
	}
	line, column := declarationLine(source, "module", declared)
	return []Diagnostic{set.Report(rule, source, line, column, fmt.Sprintf(
		"this file is %s.rho and declares itself %s, so an import of %s cannot find it",
		stem, declared, declared))}
}

// fileStem is a path without its directory or its extension.
func fileStem(path string) string {
	base := path
	if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
		base = base[slash+1:]
	}
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}
	return base
}

/* -------------------------------------------------------------------------- */
/*  Imports                                                                    */
/* -------------------------------------------------------------------------- */

// rhoUnusedImport is an import nothing in the file names.
//
// The rule with the most room to be wrong here, and it is worth saying exactly why
// it is nevertheless right, because a generated foreign interface is the case:
//
//	import ffi
//	...
//	device_init()
//
// `ffi` is not named by anything, and it is not unused — its declarations were
// generated from the platform's headers into `build/generated/modules/ffi.rho` and
// are in scope by virtue of the import. So an import whose module the *build*
// generated is never reported: that is a module whose declarations exist whether or
// not this file names them, and a rule that reported it would fire on every Rho
// application with a C interface, which is most of them.
//
// What is left is a note rather than a warning, because the rest is still only a
// claim about names: a dependency can be imported for a `feature`, for a route, or
// for a declaration another file reads. So this says that nothing here names it,
// which a person can check in a second — which is the actual question.
func rhoUnusedImport(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoUnusedImport)
	module := tree(source)
	if module == nil || len(module.Imports) == 0 {
		return nil
	}

	// Every identifier the file names outside its imports.
	names := map[string]bool{}
	for _, declaration := range module.Decls {
		countNamesIn(declaration, names)
	}

	found := []Diagnostic{}
	for _, imported := range module.Imports {
		name := strings.TrimSpace(imported.Name)
		if name == "" {
			continue
		}
		// A dotted name is used as its last segment: `import os.platform` is written
		// `platform`, and the path is only where it came from.
		segment := name
		if dot := strings.LastIndex(segment, "."); dot >= 0 {
			segment = segment[dot+1:]
		}
		if names[segment] || source.Generated[segment] || source.Generated[name] {
			continue
		}
		line, column := declarationLine(source, "import", name)
		found = append(found, set.Report(rule, source, line, column, fmt.Sprintf(
			"%s is imported and nothing in this file names it", name)))
	}
	return found
}

// countNamesIn is every identifier a declaration mentions.
//
// A walk over the whole tree rather than over the statements, because an import is
// used by a call's callee, by a field's type, and by the name of a route handler —
// and a check that looked only at the statements would report every import of a file
// whose calls are all inside expressions.
func countNamesIn(node any, names map[string]bool) {
	switch one := node.(type) {
	case nil:
		return
	case ast.TypeDecl:
		countTypeNames(one.Type, names)
	case ast.StructDecl:
		names[one.Name] = true
		for _, field := range one.Fields {
			names[field.Name] = true
			countTypeNames(field.TypeExpr, names)
		}
	case ast.FuncDecl:
		names[one.Name] = true
		for _, param := range one.Params {
			names[param.Name] = true
			countTypeNames(param.TypeExpr, names)
		}
		countTypeNames(ast.TypeExpr{Name: one.ReturnType}, names)
		for _, statement := range one.Body {
			countNamesIn(statement, names)
		}
	case ast.RouteDecl:
		names[one.Handler] = true
	case ast.BlockStmt:
		for _, statement := range one.Stmts {
			countNamesIn(statement, names)
		}
	case ast.LetStmt:
		names[one.Name] = true
		countNamesIn(one.Value, names)
	case ast.AssignStmt:
		countNamesIn(one.Target, names)
		countNamesIn(one.Value, names)
	case ast.ExprStmt:
		countNamesIn(one.Expr, names)
	case ast.ReturnStmt:
		countNamesIn(one.Value, names)
	case ast.IfStmt:
		countNamesIn(one.Condition, names)
		for _, statement := range one.Then {
			countNamesIn(statement, names)
		}
		for _, statement := range one.Else {
			countNamesIn(statement, names)
		}
	case ast.WhileStmt:
		countNamesIn(one.Condition, names)
		for _, statement := range one.Body {
			countNamesIn(statement, names)
		}
	case ast.BreakStmt:
	case ast.IdentifierExpr:
		names[one.Name] = true
	case ast.NumberExpr, ast.StringExpr, ast.BoolExpr:
	case ast.UnaryExpr:
		countNamesIn(one.Value, names)
	case ast.BinaryExpr:
		countNamesIn(one.Left, names)
		countNamesIn(one.Right, names)
	case ast.CallExpr:
		countNamesIn(one.Callee, names)
		for _, argument := range one.Args {
			countNamesIn(argument, names)
		}
		for _, argument := range one.TypeArgs {
			countTypeNames(argument, names)
		}
	case ast.MemberExpr:
		countNamesIn(one.Object, names)
		names[one.Name] = true
	case ast.IndexExpr:
		countNamesIn(one.Object, names)
		countNamesIn(one.Index, names)
	case ast.CastExpr:
		countTypeNames(one.Type, names)
		countNamesIn(one.Value, names)
	case ast.StructLiteralExpr:
		countTypeNames(one.Type, names)
		for _, value := range one.Values {
			countNamesIn(value, names)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range one.Values {
			countNamesIn(value, names)
		}
	case ast.IfExpr:
		countNamesIn(one.Condition, names)
		countNamesIn(one.Then, names)
		countNamesIn(one.Else, names)
	}
}

// countTypeNames is every identifier a type names.
func countTypeNames(one ast.TypeExpr, names map[string]bool) {
	if name := strings.TrimSpace(one.Name); name != "" {
		names[name] = true
	}
	for _, argument := range one.Args {
		countTypeNames(argument, names)
	}
}

/* -------------------------------------------------------------------------- */
/*  Bindings                                                                   */
/* -------------------------------------------------------------------------- */

// rhoShadowedBinding is a `let` whose name is already bound outside it.
//
// Scoped to one function, and that scoping is what makes the rule sound. Two
// functions each binding `count` is not a shadow — they are two locals that happen to
// share a name, and a rule that said otherwise would fire on every project and be
// turned off within the week. Within one function the inner binding is the one the
// code below it can see and the outer one is unreachable from there, which is the
// defect.
func rhoShadowedBinding(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoShadowedBinding)
	module := tree(source)
	if module == nil {
		return nil
	}
	found := []Diagnostic{}
	for _, declaration := range module.Decls {
		function, ok := declaration.(ast.FuncDecl)
		if !ok {
			continue
		}
		for _, one := range shadowedIn(function.Body, map[string]bool{}) {
			found = append(found, set.Report(rule, source, one.Pos.Line, one.Pos.Column, fmt.Sprintf(
				"this %q is already bound further out, so only the one bound here is reachable from here",
				one.Name)))
		}
	}
	return found
}

// shadowedIn is the `let`s in a list of statements that shadow an outer binding.
//
// Walked with the outer bindings threaded down rather than collected first: the
// answer for a statement inside an `if` depends on what encloses that `if`, and
// collecting every name in the function first would report every second `let` in it.
func shadowedIn(statements []ast.Stmt, outer map[string]bool) []ast.LetStmt {
	found := []ast.LetStmt{}
	// The bindings this block introduces, kept apart from the ones it may shadow.
	// A second `let x` in the *same* block is a redeclaration rather than a shadow,
	// and neither rule here reports it: this parser accepts it, and asserting what
	// the generator does with it is not something a lint tool should be claiming.
	here := map[string]bool{}
	for _, statement := range statements {
		binding, ok := statement.(ast.LetStmt)
		if !ok {
			continue
		}
		if outer[binding.Name] || here[binding.Name] {
			found = append(found, binding)
		}
		here[binding.Name] = true
	}

	enclosing := map[string]bool{}
	for name := range outer {
		enclosing[name] = true
	}
	for name := range here {
		enclosing[name] = true
	}

	for _, statement := range statements {
		switch one := statement.(type) {
		case ast.IfStmt:
			found = append(found, shadowedIn(one.Then, enclosing)...)
			found = append(found, shadowedIn(one.Else, enclosing)...)
		case ast.WhileStmt:
			found = append(found, shadowedIn(one.Body, enclosing)...)
		case ast.BlockStmt:
			found = append(found, shadowedIn(one.Stmts, enclosing)...)
		}
	}
	return found
}

/* -------------------------------------------------------------------------- */
/*  Statements                                                                 */
/* -------------------------------------------------------------------------- */

// rhoUnreachable is a statement after one that always leaves the block.
//
// Only the statements whose effect is unconditional, and each is checked for the
// reason it applies. `while true` counts and a `while` on a condition does not:
// a condition may be false on the first test, which is exactly what a Rho program
// relies on when it writes `while` instead of `while true`.
func rhoUnreachable(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoUnreachable)
	module := tree(source)
	if module == nil {
		return nil
	}
	found := []Diagnostic{}
	for _, declaration := range module.Decls {
		function, ok := declaration.(ast.FuncDecl)
		if !ok {
			continue
		}
		for _, one := range unreachableIn(function.Body) {
			found = append(found, set.Report(rule, source, one.Position().Line, one.Position().Column,
				"nothing after this can run: the block has already been left"))
		}
	}
	return found
}

func unreachableIn(statements []ast.Stmt) []ast.Stmt {
	// The tail of a block, once something in it always leaves.
	for index, statement := range statements {
		if !leaves(statement) {
			continue
		}
		found := []ast.Stmt{}
		found = append(found, statements[index+1:]...)
		for _, nested := range nestedBlocks(statement) {
			found = append(found, unreachableIn(nested)...)
		}
		return found
	}

	// No statement here leaves unconditionally, so the block's tail is reachable —
	// and each nested block is judged on its own, because a `return` inside an `if`
	// does not make the statements after the `if` unreachable. It does not: the `if`
	// may not have been taken.
	found := []ast.Stmt{}
	for _, statement := range statements {
		for _, nested := range nestedBlocks(statement) {
			found = append(found, unreachableIn(nested)...)
		}
	}
	return found
}

// nestedBlocks are the statement lists inside a statement, which are judged
// separately from the list that holds them.
func nestedBlocks(statement ast.Stmt) [][]ast.Stmt {
	switch one := statement.(type) {
	case ast.IfStmt:
		return [][]ast.Stmt{one.Then, one.Else}
	case ast.WhileStmt:
		return [][]ast.Stmt{one.Body}
	case ast.BlockStmt:
		return [][]ast.Stmt{one.Stmts}
	default:
		return nil
	}
}

// leaves reports whether a statement always leaves the block it is in.
func leaves(statement ast.Stmt) bool {
	switch one := statement.(type) {
	case ast.ReturnStmt, ast.BreakStmt:
		return true
	case ast.WhileStmt:
		return isTrueCondition(one.Condition) && !hasBreak(one.Body)
	default:
		return false
	}
}

// isTrueCondition reports whether a condition is literally true.
//
// Both spellings, because Rho has a `true` literal and a program may write the
// identifier — and both mean the same loop.
func isTrueCondition(condition ast.Expr) bool {
	switch one := condition.(type) {
	case ast.BoolExpr:
		return one.Value
	case ast.IdentifierExpr:
		return one.Name == "true"
	default:
		return false
	}
}

func hasBreak(statements []ast.Stmt) bool {
	for _, statement := range statements {
		if _, ok := statement.(ast.BreakStmt); ok {
			return true
		}
		for _, nested := range nestedBlocks(statement) {
			if hasBreak(nested) {
				return true
			}
		}
	}
	return false
}

// rhoEmptyBlock is a block with nothing in it.
//
// A note and not a warning, because an empty block is a legitimate thing to write:
// an `else` with nothing in it says "and do nothing else", which is a statement
// about the `if` rather than a gap in it. What the rule is for is a block that reads
// as though something was meant to go in it, and a person decides that in a second.
func rhoEmptyBlock(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoEmptyBlock)
	module := tree(source)
	if module == nil {
		return nil
	}
	found := []Diagnostic{}
	for _, declaration := range module.Decls {
		function, ok := declaration.(ast.FuncDecl)
		if !ok {
			continue
		}
		for _, block := range emptyBlocks(function.Body) {
			found = append(found, set.Report(rule, source, block.Line, block.Column,
				"this block is empty; an `else` with nothing in it says do nothing, and an `if` "+
					"that means nothing should not be written"))
		}
	}
	return found
}

// emptyBlocks are the empty blocks in a list of statements, as places.
func emptyBlocks(statements []ast.Stmt) []place {
	found := []place{}
	for _, statement := range statements {
		switch one := statement.(type) {
		case ast.BlockStmt:
			if len(one.Stmts) == 0 {
				found = append(found, place{Line: one.Pos.Line, Column: one.Pos.Column})
			}
		case ast.IfStmt:
			found = append(found, emptyBlocks(one.Then)...)
			found = append(found, emptyBlocks(one.Else)...)
		case ast.WhileStmt:
			found = append(found, emptyBlocks(one.Body)...)
		}
	}
	return found
}

/* -------------------------------------------------------------------------- */
/*  Comments                                                                   */
/* -------------------------------------------------------------------------- */

// rhoTodo is a marker left in a comment.
//
// Scanned over the text rather than over the token stream, and the one thing that
// matters about that is strings: `print("set the TODO flag")` is not a finding, and
// any scan that looks at lines without knowing which of them are inside a string
// literal would report it. So the walk knows where a comment starts — a `//` that is
// not inside a string — and a comment's marker is inside it by definition.
//
// Block comments are not scanned. They carry the file banners this toolchain writes,
// and a TODO in a banner is a statement about the generator rather than about the
// program.
func rhoTodo(source Source) []Diagnostic {
	set := RhoRules()
	rule, _ := set.Find(RhoTodoComment)
	found := []Diagnostic{}

	line, column := 1, 1
	quoted := false
	escaped := false
	for index := 0; index < len(source.Text); index++ {
		character := source.Text[index]
		switch {
		case escaped:
			escaped = false
		case character == '\\' && quoted:
			escaped = true
		case character == '"':
			quoted = !quoted
		case quoted:
			// Inside a string literal: nothing here can start a comment or a line.
		case character == '/' && index+1 < len(source.Text) && source.Text[index+1] == '/':
			marker := todoMarker.FindStringIndex(source.Text[index+2:])
			if marker != nil {
				found = append(found, set.Report(rule, source, line, column,
					fmt.Sprintf("%s left in a comment; it is either done or it is a comment",
						source.Text[index+2+marker[0]:index+2+marker[1]])))
			}
			// To the end of the line, which is where the comment ends.
			for index+1 < len(source.Text) && source.Text[index+1] != '\n' {
				index++
				column++
			}
		case character == '\n':
			line++
			column = 1
			continue
		}
		column++
	}
	return found
}
