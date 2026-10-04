//
// @file extract.go
// @brief Reading a Rho service's HTTP surface out of its sources.
//
// The reading uses the compiler's own lexer, parser and AST, so a route is read
// here exactly as the compiler read it. A second dialect of Rho would be a
// second opinion about what a program says, and this file would eventually
// disagree with the build that ships it.
//
// What is read is the declaration a person wrote:
//
//	route GET "/modules/{id}" => show_module
//
// and, from the handler that serves it, the values it reads:
//
//	let id: u32 = path_u32(req, "id")
//	let q: string = query_str(req, "q")
//
// The second is where a typed parameter comes from. There is no annotation in
// Rho, and adding one for this would put a web framework's vocabulary into the
// language; instead the type is the one the handler asked for, and the value
// is converted by the statement the call site implies. An OpenAPI document
// derived from that says what the program actually does rather than what a
// separate declaration says it might.
//
// Comments are read from the source text rather than from the AST, because the
// lexer discards them: a comment is not a value and the compiler has no use for
// one, but a documentation tool has no use for a route that cannot describe
// itself. They are attached by position, which is why this file keeps the
// sources it read alongside the declarations it found.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorouter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	rhoparser "rho/src/frontend/parser"
)

// Diagnostic is a problem found while reading a project, reported against the
// file it was found in rather than returned as a failure. A source that does
// not parse is skipped: a route sheet for a file that does not compile is worth
// less than the ones that do.
type Diagnostic struct {
	Level   string `json:"level"`
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

// ReadOptions say which sources to read and what to skip.
type ReadOptions struct {
	// Root is the project directory. Sources are read from it, and the paths
	// written into the spec are relative to it.
	Root string
	// Target names the target a document describes, when a project serves more
	// than one HTTP API. Empty takes the first target of kind server.
	Target string
	// NoSecurity reads the sources without asking the descriptor how a caller
	// proves who it is, which is what a caller wants when the descriptor is
	// not the thing being described.
	NoSecurity bool
}

// ReadResult is a spec and whatever could not be read.
type ReadResult struct {
	Spec        Spec
	Diagnostics []Diagnostic
}

// Read reads every route a project declares into a Spec.
//
// The sources are the ones the module graph reaches: the entry point and
// everything it imports within the project. Reading a file no build would
// compile would report routes that are not in the service.
func Read(options ReadOptions) (ReadResult, error) {
	result := ReadResult{}
	if options.Root == "" {
		return result, fmt.Errorf("a project directory is required")
	}
	manifest, err := readManifest(options.Root)
	if err != nil {
		return result, err
	}

	files, err := sourceFiles(options.Root, manifest)
	if err != nil {
		return result, err
	}

	result.Spec = Spec{
		Version:       SpecVersion,
		Module:        manifest.Module,
		Title:         manifest.Title,
		Description:   manifest.Description,
		ModuleVersion: manifest.ModuleVersion,
		Source:        options.Root,
		Operations:    []Operation{},
	}

	// Every source is parsed before any route is described, because describing
	// one needs the whole project: a handler that delegates to a function in
	// another file is served by the same function either way, and its statuses
	// are as much part of the operation as the ones its own body names.
	project := newProject()
	sources := []parsedSource{}
	for _, file := range files {
		source, diagnostics := readFile(options.Root, file)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		if source == nil {
			continue
		}
		sources = append(sources, *source)
		project.add(*source)
	}

	for _, source := range sources {
		for _, decl := range source.module.Decls {
			route, ok := decl.(ast.RouteDecl)
			if !ok {
				continue
			}
			result.Operations(project.operations(source, route))
		}
	}

	if !options.NoSecurity {
		section, fromPipeline, found, err := readTargetAPI(options.Root, options.Target)
		if err != nil {
			// A descriptor that does not parse is worth saying so about, but
			// it is not a reason to refuse to describe the routes.
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Level: "warning", File: DescriptorFileName,
				Message: fmt.Sprintf("the descriptor does not parse: %v", err)})
			return result, nil
		}
		if found {
			result.Spec.Security = resolveSecurity(section, fromPipeline)
		}
	}
	return result, nil
}

// Operations adds what a read found to the spec.
func (r *ReadResult) Operations(found []Operation) {
	r.Spec.Operations = append(r.Spec.Operations, found...)
}

// sourceFiles lists the project's Rho sources, in a stable order, skipping the
// directories no build compiles.
func sourceFiles(root string, manifest manifestInfo) ([]string, error) {
	out := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".kilo", "build", "node_modules", "vendor", "static":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".rho") {
			return nil
		}
		base := filepath.Base(path)
		// A test is a client of the service rather than part of it, and a
		// fixture is not a route anyone serves.
		if strings.HasSuffix(base, "_test.rho") || strings.HasPrefix(base, "test_") {
			return nil
		}
		if strings.HasPrefix(filepath.Base(filepath.Dir(path)), "tests") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	sort.Strings(out)
	if manifest.Main != "" {
		// The entry point is read first, so the operations of the module a
		// reader cares about come before the ones it imports.
		entry := filepath.Join(root, filepath.FromSlash(manifest.Main))
		for i, file := range out {
			if file == entry {
				out = append([]string{entry}, append(out[:i:i], out[i+1:]...)...)
				break
			}
		}
	}
	return out, nil
}

// parsedSource is one source that was read, kept with what reading it produced
// so that a route can be described with the rest of the project in hand.
type parsedSource struct {
	// path is the path as a spec writes it.
	path string
	// module is what the compiler's parser produced.
	module *ast.Module
	// comments are the source's comments, by position.
	comments *commentIndex
}

// readFile reads one source into what the parser said about it.
//
// A source that cannot be read is a diagnostic rather than a failure: one file
// that does not parse is not a reason to refuse to describe the others.
func readFile(root, path string) (*parsedSource, []Diagnostic) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, []Diagnostic{{Level: "warning", File: relative(root, path),
			Message: fmt.Sprintf("cannot read the source: %v", err)}}
	}
	tokens, lexErr := lexer.Lex(string(content))
	if lexErr != nil {
		return nil, []Diagnostic{{Level: "warning", File: relative(root, path),
			Message: fmt.Sprintf("the source does not lex: %v", lexErr)}}
	}
	module, err := rhoparser.Parse(tokens)
	if err != nil {
		return nil, []Diagnostic{{Level: "warning", File: relative(root, path),
			Message: fmt.Sprintf("the source does not parse: %v", err)}}
	}
	return &parsedSource{
		path:     relative(root, path),
		module:   module,
		comments: newCommentIndex(string(content)),
	}, nil
}

// project is a whole project's functions, so that a route can be described with
// more than the body its handler happens to have.
//
// A handler that delegates is the common shape rather than the exception: a
// route renders a page, and the page is built by a function beside it. Reading
// only the handler's own body would describe such a service as answering
// nothing, which is a description that is worse than useless — it is a
// description a client would act on.
type project struct {
	// handlers are the functions that take a request and answer with a
	// response, by the name a route names them.
	handlers map[string]handlerInfo
	// calls is the local functions each function calls, by name.
	calls map[string][]string
	// answers is what every function that answers with a response names, by
	// name. It is not only the handlers: a route commonly delegates to a
	// function that builds the response from arguments rather than from the
	// request, and that function's statuses are the operation's statuses.
	answers map[string][]Response
	// byName is every function, by name, for resolving what a handler
	// delegates to.
	byName map[string]ast.FuncDecl
	// owners is the file each function was declared in, so a route can be
	// tagged by the module it belongs to.
	owners map[string]string
}

// newProject returns an empty project.
func newProject() *project {
	return &project{
		handlers: map[string]handlerInfo{},
		calls:    map[string][]string{},
		answers:  map[string][]Response{},
		byName:   map[string]ast.FuncDecl{},
		owners:   map[string]string{},
	}
}

// add reads one source's functions into the project.
func (p *project) add(source parsedSource) {
	for _, decl := range source.module.Decls {
		fn, ok := decl.(ast.FuncDecl)
		if !ok || fn.Extern {
			continue
		}
		name := fn.Name
		if fn.Owner != "" {
			name = fn.Owner + "." + fn.Name
		}
		p.byName[name] = fn
		p.owners[name] = source.path
		p.calls[name] = calledNames(fn.Body)
		if fn.ReturnType == "Response" {
			p.answers[name] = collectResponses(fn.Body)
		}
		if servesRequest(fn) {
			p.handlers[name] = readHandler(source.comments, fn)
		}
	}
}

// operations describes the routes one source declares.
func (p *project) operations(source parsedSource, route ast.RouteDecl) []Operation {
	return []Operation{p.operation(source, route)}
}

// responses is what an operation answers with.
//
// A handler that names its own statuses answers with those. A handler that
// delegates answers with what the functions it calls answer with, which is the
// one level of indirection a service written the way this toolchain invites has
// and the level beyond which a reading stops being a reading: a status chosen
// three calls down is a status no static reading can promise, and inventing one
// would be worse than saying nothing.
func (p *project) responses(name string, handler handlerInfo) []Response {
	if len(handler.Responses) > 0 {
		return handler.Responses
	}
	out := []Response{}
	seen := map[string]bool{}
	for _, called := range p.calls[name] {
		if seen[called] {
			continue
		}
		seen[called] = true
		out = append(out, p.answers[called]...)
	}
	return mergeResponses(out)
}

// calledNames returns the distinct function names a body calls, in the order it
// calls them.
func calledNames(body []ast.Stmt) []string {
	seen := map[string]bool{}
	out := []string{}
	walkStatements(body, func(call ast.CallExpr) {
		callee, ok := call.Callee.(ast.IdentifierExpr)
		if !ok || seen[callee.Name] {
			return
		}
		seen[callee.Name] = true
		out = append(out, callee.Name)
	})
	return out
}

// servesRequest reports whether a function is a handler: one that takes the
// request and answers with a response. A function that merely takes a string is
// not a handler, however much a route would like it to be.
func servesRequest(fn ast.FuncDecl) bool {
	if len(fn.Params) != 1 {
		return false
	}
	return fn.Params[0].Type == "Request" && fn.ReturnType == "Response"
}

// handlerInfo is what a handler says and does about the request it serves.
type handlerInfo struct {
	// Summary is the handler's own comment, which is the description when the
	// comment runs to more than one line.
	Summary string
	// Tag groups the operation with the others from the same module.
	Tag string
	// Parameters are the values the handler reads, in the order it reads them.
	Parameters []Parameter
	// Responses are the statuses the handler answers with.
	Responses []Response
	// NeedsAuth is true when the handler checks a credential, so the operation
	// is one a caller may need a token for.
	NeedsAuth bool
}

// readHandler reads what a handler reads out of a request, and what it answers.
func readHandler(comments *commentIndex, fn ast.FuncDecl) handlerInfo {
	info := handlerInfo{
		Summary: comments.above(comments.lineOfFunction(fn.Name)),
		Tag:     moduleTag(fn),
	}
	// A handler that checks a credential may need one. It is "may" rather than
	// "does" because the check can be conditional — token issuance is open
	// until a deployment closes it — and a document has a way to say exactly
	// that: a security entry with an empty alternative means a caller may
	// present a credential or may not.
	needsAuth := false
	walkStatements(fn.Body, func(call ast.CallExpr) {
		if callee, ok := call.Callee.(ast.IdentifierExpr); ok && readsCredential(callee.Name) {
			needsAuth = true
		}
		parameter, isParameter := parameterOf(call)
		if !isParameter {
			return
		}
		key := parameter.In + ":" + parameter.Name
		if containsParameter(info.Parameters, key) {
			return
		}
		info.Parameters = append(info.Parameters, parameter)
	})

	info.Responses = mergeResponses(collectResponses(fn.Body))
	// A path wildcard is required and nothing else can be decided here. The
	// language has no way to say which value a guard was for: `bind_ok(req)`
	// takes the request and nothing else, so a guard cannot be attributed to the
	// read it guards. A value that a handler reads is therefore reported as
	// optional unless the router guarantees it, which is the one requirement the
	// route itself states.
	for i := range info.Parameters {
		if info.Parameters[i].In == InPath {
			info.Parameters[i].Required = true
		}
	}
	info.NeedsAuth = needsAuth
	return info
}

// readsCredential reports whether a helper is one a handler checks a credential
// with. Whether it reads one, checks one or parses one, the operation is one
// where a credential may be needed, which is what a document says.
func readsCredential(name string) bool {
	for _, known := range credentialReads {
		if known == name {
			return true
		}
	}
	return false
}

// containsParameter reports whether a list already carries a value, compared on
// where it comes from and what it is called.
func containsParameter(parameters []Parameter, key string) bool {
	for _, parameter := range parameters {
		if parameter.In+":"+parameter.Name == key {
			return true
		}
	}
	return false
}

// collectResponses reads every status a body answers with, whether it comes
// from a helper taking one or from a Response written directly.
func collectResponses(statements []ast.Stmt) []Response {
	out := []Response{}
	// A status is written as a call whose argument is one, or as a response
	// written directly. The second is an expression rather than a call, so
	// both shapes are found by walking the whole body for either.
	walkStatements(statements, func(call ast.CallExpr) {
		if status, ok := responseAt(call); ok {
			out = appendResponse(out, status, contentTypeOf(call))
		}
	})
	walkExpressions(statements, func(expression ast.Expr) {
		if status, contentType, ok := literalResponse(expression); ok {
			out = appendResponse(out, status, contentType)
		}
	})
	return out
}

// walkExpressions visits every expression in a body, in the order it is
// written, so a document lists statuses in the order a reader reaches them.
func walkExpressions(statements []ast.Stmt, visit func(ast.Expr)) {
	// Every call is an expression too, so the body is walked for expressions
	// and each call reached through that walk carries its own subtree.
	walkStatements(statements, func(call ast.CallExpr) {
		visit(call.Callee)
		for _, argument := range call.Args {
			walkExpressionArgument(argument, visit)
		}
	})
	for _, statement := range statements {
		switch node := statement.(type) {
		case ast.LetStmt:
			visit(node.Value)
		case ast.AssignStmt:
			visit(node.Value)
		case ast.ExprStmt:
			visit(node.Expr)
		case ast.ReturnStmt:
			visit(node.Value)
		case ast.IfStmt:
			visit(node.Condition)
		case ast.WhileStmt:
			visit(node.Condition)
		}
	}
}

// moduleTag is the module a function belongs to, read from its owner.
func moduleTag(fn ast.FuncDecl) string {
	if fn.Owner == "" {
		return ""
	}
	if index := strings.LastIndex(fn.Owner, "."); index >= 0 {
		return fn.Owner[index+1:]
	}
	return fn.Owner
}

// readRoute builds the operation a route declaration describes, taking what it
// can from the handler and filling in what only the declaration says.
func (p *project) operation(source parsedSource, route ast.RouteDecl) Operation {

	declared := source.comments.above(source.comments.lineOfRoute(route.Method, route.Path))
	operation := Operation{
		ID:      route.Handler,
		Method:  strings.ToUpper(route.Method),
		Path:    route.Path,
		Handler: route.Handler,
		Source:  source.path,
		Tag:     moduleOf(source.path),
	}
	operation.Summary, operation.Description = splitSummary(declared)

	// A path wildcard is a parameter whether or not the handler reads it: the
	// router will not have matched the route without it, so it is always
	// required, and its type is string unless the handler converts it.
	for _, name := range wildcardNames(route.Path) {
		operation.Parameters = append(operation.Parameters, Parameter{
			Name: name, In: InPath, Type: "string", Required: true,
		})
	}

	handler, found := p.handlers[route.Handler]
	if !found {
		return operation
	}
	// A handler's own comment describes the operation, and is split the same
	// way a route's is: the first line heads it and the rest is the prose.
	if summary, description := splitSummary(handler.Summary); summary != "" {
		if operation.Summary == "" {
			operation.Summary = summary
		}
		if operation.Description == "" {
			operation.Description = description
		}
	}
	if operation.Tag == "" {
		operation.Tag = handler.Tag
	}
	operation.Deprecated = strings.Contains(strings.ToLower(declared), "deprecated")

	// The handler's reading wins over the wildcard's default, because the
	// handler is where the type was asked for. The wildcard entries written
	// above are placeholders: a path always carries a value, but the type is
	// the one the handler asked to receive it as.
	read := map[string]Parameter{}
	for _, parameter := range handler.Parameters {
		read[parameter.In+":"+parameter.Name] = parameter
	}
	for i, parameter := range operation.Parameters {
		if found, named := read[parameter.In+":"+parameter.Name]; named {
			operation.Parameters[i].Type = found.Type
			operation.Parameters[i].Description = found.Description
		}
	}
	for _, parameter := range handler.Parameters {
		// A body value belongs to the body, not to the parameter list: a
		// consumer that had to know to filter one out would be a consumer that
		// reads the same thing twice, and the document says the same.
		if parameter.In == InBody {
			continue
		}
		if parameter.In == InPath && hasParameter(operation.Parameters, InPath, parameter.Name) {
			continue
		}
		operation.Parameters = append(operation.Parameters, parameter)
	}
	operation.RequestBody = bodyOf(handler.Parameters)
	operation.Responses = p.responses(route.Handler, handler)
	operation.NeedsAuth = handler.NeedsAuth
	return operation
}

// hasParameter reports whether a list already carries a value.
func hasParameter(parameters []Parameter, in, name string) bool {
	for _, parameter := range parameters {
		if parameter.In == in && parameter.Name == name {
			return true
		}
	}
	return false
}

// bodyOf describes what a handler reads out of the body, or returns nil for a
// handler that reads none. A handler that reads the body whole is serving a
// document or a package, not a form.
func bodyOf(parameters []Parameter) *Body {
	body := &Body{ContentType: "application/json"}
	seen := false
	for _, parameter := range parameters {
		if parameter.In != InBody {
			continue
		}
		seen = true
		if parameter.Name == "" {
			body.Raw = true
			body.ContentType = "application/octet-stream"
			continue
		}
		body.Fields = append(body.Fields, parameter)
	}
	if !seen {
		return nil
	}
	return body
}

// mergeResponses collapses the statuses a handler answers with into one entry
// each, keeping the first description written for one.
func mergeResponses(responses []Response) []Response {
	seen := map[int]bool{}
	out := []Response{}
	for _, response := range responses {
		if seen[response.Status] {
			continue
		}
		seen[response.Status] = true
		out = append(out, response)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Status < out[j].Status })
	return out
}

// appendResponse records a status a handler answers with, reading the standard
// library's own name for it so a document says what the code means.
func appendResponse(responses []Response, status int, contentType string) []Response {
	return append(responses, Response{
		Status:      status,
		Description: statusName(status),
		ContentType: contentType,
	})
}

// wildcardNames returns the `{name}` placeholders in a route pattern. A trailing
// `{name...}` names the rest of the path and is written without the dots,
// because that is the name a handler asks for.
func wildcardNames(pattern string) []string {
	out := []string{}
	for i := 0; i < len(pattern); {
		if pattern[i] != '{' {
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			break
		}
		// A catch-all carries the same name without its dots, because that is
		// the name a handler asks for.
		name := strings.TrimSuffix(pattern[i+1:i+end], "...")
		if name != "" {
			out = append(out, name)
		}
		i += end + 1
	}
	return out
}

// relative returns a path as it is written in a spec, relative to the project.
func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

/* -------------------------------------------------------------------------- */
/*  Walking                                                                    */
/* -------------------------------------------------------------------------- */

// walkStatements visits every call in a handler body, wherever it is written.
func walkStatements(statements []ast.Stmt, visit func(ast.CallExpr)) {
	for _, statement := range statements {
		walkStatement(statement, visit)
	}
}

// walkStatement visits every call one statement contains.
func walkStatement(statement ast.Stmt, visit func(ast.CallExpr)) {
	switch node := statement.(type) {
	case ast.LetStmt:
		walkExpression(node.Value, visit)
	case ast.AssignStmt:
		walkExpression(node.Value, visit)
	case ast.ExprStmt:
		walkExpression(node.Expr, visit)
	case ast.ReturnStmt:
		walkExpression(node.Value, visit)
	case ast.IfStmt:
		walkExpression(node.Condition, visit)
		walkStatements(node.Then, visit)
		walkStatements(node.Else, visit)
	case ast.WhileStmt:
		walkExpression(node.Condition, visit)
		walkStatements(node.Body, visit)
	}
}

// walkExpressionArgument visits every expression one argument contains.
func walkExpressionArgument(expression ast.Expr, visit func(ast.Expr)) {
	visit(expression)
	walkCallArguments(expression, visit)
}

// walkCallArguments visits the arguments of every call an expression contains.
func walkCallArguments(expression ast.Expr, visit func(ast.Expr)) {
	switch node := expression.(type) {
	case ast.CallExpr:
		for _, argument := range node.Args {
			walkExpressionArgument(argument, visit)
		}
	case ast.UnaryExpr:
		walkCallArguments(node.Value, visit)
	case ast.BinaryExpr:
		walkCallArguments(node.Left, visit)
		walkCallArguments(node.Right, visit)
	case ast.MemberExpr:
		walkCallArguments(node.Object, visit)
	case ast.IndexExpr:
		walkCallArguments(node.Object, visit)
		walkCallArguments(node.Index, visit)
	case ast.CastExpr:
		walkCallArguments(node.Value, visit)
	case ast.StructLiteralExpr:
		for _, value := range node.Values {
			walkCallArguments(value, visit)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range node.Values {
			walkCallArguments(value, visit)
		}
	case ast.IfExpr:
		walkCallArguments(node.Condition, visit)
		walkCallArguments(node.Then, visit)
		walkCallArguments(node.Else, visit)
	}
}

// walkExpression visits every call one expression contains.
func walkExpression(expression ast.Expr, visit func(ast.CallExpr)) {
	switch node := expression.(type) {
	case ast.CallExpr:
		visit(node)
		for _, argument := range node.Args {
			walkExpression(argument, visit)
		}
	case ast.UnaryExpr:
		walkExpression(node.Value, visit)
	case ast.BinaryExpr:
		walkExpression(node.Left, visit)
		walkExpression(node.Right, visit)
	case ast.MemberExpr:
		walkExpression(node.Object, visit)
	case ast.IndexExpr:
		walkExpression(node.Object, visit)
		walkExpression(node.Index, visit)
	case ast.CastExpr:
		walkExpression(node.Value, visit)
	case ast.StructLiteralExpr:
		for _, value := range node.Values {
			walkExpression(value, visit)
		}
	case ast.ArrayLiteralExpr:
		for _, value := range node.Values {
			walkExpression(value, visit)
		}
	case ast.IfExpr:
		walkExpression(node.Condition, visit)
		walkExpression(node.Then, visit)
		walkExpression(node.Else, visit)
	}
}

// stringArgument returns a string literal argument, or "" when the argument is
// anything else. A name built at run time cannot be read here, and a value read
// that way is documented by hand rather than by this tool.
func stringArgument(arguments []ast.Expr, index int) string {
	if index >= len(arguments) {
		return ""
	}
	literal, ok := arguments[index].(ast.StringExpr)
	if !ok {
		return ""
	}
	return literal.Value
}
