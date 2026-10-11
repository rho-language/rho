package parser

import (
	"fmt"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

type Parser struct {
	lx      *lexer.Lexer
	current lexer.Token
	nextTok lexer.Token
	tokens  []lexer.Token
	tokenAt int

	// valueNames are the names the module binds at the top level. A brace after
	// one of them opens a block, not a struct literal: `while i < MAX {` is a
	// loop, and reading `MAX {` as a literal would swallow its body.
	valueNames map[string]bool
	// applyNames are the names that can take type arguments: the types and the
	// functions, plus the ones the language supplies. Only a name here can be
	// applied, so `a[i] {` is an index before a block and `Box[u8] {` is a
	// literal. A parameter named `a` is in neither set, which is what tells the
	// two apart.
	applyNames map[string]bool
	// noStructLiteral is set while a condition is read: the `{` after
	// `if x == NAME` opens the body, never a struct literal. It is the only
	// context where the two are ambiguous, and a name declared in another file
	// cannot be told from a type by the leading-capital rule alone.
	noStructLiteral bool
}

func New(lx *lexer.Lexer) *Parser {
	p := &Parser{lx: lx}
	p.readToken()
	p.readToken()
	return p
}

func Parse(tokens []lexer.Token) (mod *ast.Module, err error) {
	p := &Parser{
		tokens:     tokens,
		valueNames: collectValueNames(tokens),
		applyNames: collectApplyNames(tokens),
	}
	p.readToken()
	p.readToken()
	return p.Parse()
}

// protocolAndBuiltinTypes are the types and constructors the language supplies,
// which a module uses without declaring. A literal or an application of one has
// no declaration for a scan to find, so the names are carried here.
var protocolAndBuiltinTypes = []string{
	"Request", "Response",
	"String", "Slice", "Ptr", "Option", "Result", "Array", "Map", "Range",
	"RS485", "UART", "I2C", "SPI", "GPIO", "Timer", "Modbus", "TimeSpec",
	"Some", "None", "Ok", "Err",
}

// collectApplyNames scans for the names that can take a type argument: what the
// module declares as a type or a function, and what the language supplies.
func collectApplyNames(tokens []lexer.Token) map[string]bool {
	names := map[string]bool{}
	for _, name := range protocolAndBuiltinTypes {
		names[name] = true
	}
	for i := 0; i+1 < len(tokens); i++ {
		switch tokens[i].Kind {
		case lexer.TokenStruct, lexer.TokenType, lexer.TokenFn, lexer.TokenExtern:
			if tokens[i+1].Kind == lexer.TokenIdent {
				names[tokens[i+1].Lex] = true
			}
		}
	}
	return names
}

// collectValueNames scans the token stream for the names a module binds at the
// top level, so a brace after one can be told from a block.
//
// Nesting is tracked because a local binding inside a function is not a
// module-level one, and treating it as such would stop its name from ever being
// a type.
func collectValueNames(tokens []lexer.Token) map[string]bool {
	names := map[string]bool{}
	depth := 0
	for i := 0; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case lexer.TokenLBrace:
			depth++
		case lexer.TokenRBrace:
			if depth > 0 {
				depth--
			}
		case lexer.TokenConst, lexer.TokenLet, lexer.TokenVar:
			if depth == 0 && i+1 < len(tokens) && tokens[i+1].Kind == lexer.TokenIdent {
				names[tokens[i+1].Lex] = true
			}
		}
	}
	return names
}

func (p *Parser) Parse() (mod *ast.Module, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			mod = nil
			if recoveredErr, ok := recovered.(error); ok {
				err = recoveredErr
				return
			}
			err = fmt.Errorf("%v", recovered)
		}
	}()
	if p.current.Kind == lexer.TokenEOF {
		return nil, fmt.Errorf("expected module declaration")
	}
	mod = p.parseModuleDecl()
	mod.Namespace = p.parseNamespace()
	mod.Includes = p.parseIncludes()
	mod.Imports = p.parseImports()
	mod.Features = p.parseFeatures()
	mod.Decls = p.parseTopLevelDecls()
	return mod, nil
}

// parseNamespace reads an optional `namespace a.b.c;` after the module name. It
// is the version a schema's names are qualified by, and it is written once for
// the file rather than repeated on every declaration. `namespace` is contextual,
// so a program may still use the word as an ordinary name.
func (p *Parser) parseNamespace() string {
	if p.current.Kind != lexer.TokenIdent || p.current.Lex != "namespace" {
		return ""
	}
	p.readToken()
	parts := []string{p.name()}
	for p.current.Kind == lexer.TokenDot {
		p.readToken()
		parts = append(parts, p.name())
	}
	if p.current.Kind == lexer.TokenSemicolon {
		p.readToken()
	}
	return strings.Join(parts, ".")
}

// intLiteral reads an integer literal. An enum's values are written, not counted,
// so a value that is not a number is an error where it is written.
func (p *Parser) intLiteral() int {
	value, err := strconv.Atoi(p.current.Lex)
	if err != nil {
		panic(NewErrorf(p.current, "expected an integer, got %q", p.current.Lex))
	}
	p.readToken()
	return value
}

func (p *Parser) parseFeatures() []string {
	features := []string{}
	for p.current.Kind == lexer.TokenIdent && p.current.Lex == "feature" {
		p.readToken()
		p.expect(lexer.TokenString)
		features = append(features, p.current.Lex)
		p.readToken()
		if p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	return features
}

func (p *Parser) parseIncludes() []ast.Include {
	includes := []ast.Include{}
	for p.current.Kind == lexer.TokenInclude {
		p.readToken()
		p.expect(lexer.TokenString)
		includes = append(includes, ast.Include{Path: p.current.Lex})
		p.readToken()
		if p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	return includes
}

func (p *Parser) readToken() {
	p.current = p.nextTok
	if p.tokens != nil {
		if p.tokenAt < len(p.tokens) {
			p.nextTok = p.tokens[p.tokenAt]
			p.tokenAt++
		} else {
			p.nextTok = lexer.Token{Kind: lexer.TokenEOF}
		}
		return
	}
	p.nextTok = p.lx.Next()
}

func (p *Parser) expect(kind lexer.Kind) {
	if p.current.Kind != kind {
		panic(NewErrorf(p.current, "expected %v, got %v (%q)", kind, p.current.Kind, p.current.Lex))
	}
}

func isName(kind lexer.Kind) bool {
	return kind == lexer.TokenIdent || (kind >= lexer.TokenModule && kind <= lexer.TokenType)
}

func (p *Parser) name() string {
	if !isName(p.current.Kind) {
		panic(NewErrorf(p.current, "expected name, got %q", p.current.Lex))
	}
	name := p.current.Lex
	p.readToken()
	return name
}

func (p *Parser) parseModuleDecl() *ast.Module {
	p.expect(lexer.TokenModule)
	p.readToken()
	parts := []string{p.name()}
	for p.current.Kind == lexer.TokenDot {
		p.readToken()
		parts = append(parts, p.name())
	}
	return &ast.Module{Name: strings.Join(parts, ".")}
}

func (p *Parser) parseImports() []ast.Import {
	imports := []ast.Import{}
	for p.current.Kind == lexer.TokenImport {
		p.readToken()
		parts := []string{p.name()}
		for p.current.Kind == lexer.TokenDot {
			p.readToken()
			parts = append(parts, p.name())
		}
		imports = append(imports, ast.Import{Name: strings.Join(parts, ".")})
	}
	return imports
}

// parseTopLevelDecls reads the declarations at the top of a file.
//
// A declaration is positioned where its keyword is, which is where a reader
// expects a declaration to be: the first character of `fn`, not of its name. The
// position is taken before the declaration is parsed, because parsing it moves
// past every token that belongs to it.
func (p *Parser) parseTopLevelDecls() []ast.Decl {
	decls := []ast.Decl{}
	for p.current.Kind != lexer.TokenEOF {
		start := p.position()
		var decl ast.Decl
		switch p.current.Kind {
		case lexer.TokenStruct:
			decl = p.parseStructDecl()
		case lexer.TokenEnum:
			decl = p.parseEnumDecl()
		// `message` and `rpc` are contextual keywords: they name a declaration
		// only where one begins, so a field or a variable may still be called
		// one. Reserving them would rename code that already used the words.
		case lexer.TokenIdent:
			switch p.current.Lex {
			case "message":
				decl = p.parseMessageDecl()
			case "rpc":
				decl = p.parseRpcDecl()
			default:
				panic(NewErrorf(p.current, "unexpected token in top-level: %q", p.current.Lex))
			}
		case lexer.TokenFn:
			decl = p.parseFuncDecl()
		case lexer.TokenExtern:
			decl = p.parseFuncDeclWithExtern()
		case lexer.TokenType:
			decl = p.parseTypeDecl()
		case lexer.TokenRoute:
			decl = p.parseRouteDecl()
		case lexer.TokenLet, lexer.TokenConst, lexer.TokenVar:
			decl = p.parseConstDecl()
		default:
			panic(NewErrorf(p.current, "unexpected token in top-level: %q", p.current.Lex))
		}
		p.locate(&decl, start)
		decls = append(decls, decl)
	}
	return decls
}

// position is where the lexer currently is, in a form the tree can carry.
func (p *Parser) position() ast.Position {
	return ast.Position{Offset: p.current.Pos, Line: p.current.Line, Column: p.current.Column}
}

// locate puts a declaration at the place its keyword was.
//
// The position is where the declaration began, which the parse functions have
// already moved past, so it is taken before they run and set afterwards. A node
// can be any of the four kinds, and asking each in turn is four type assertions
// rather than a change to every parse function.
func (p *Parser) locate(node *ast.Decl, at ast.Position) {
	switch n := (*node).(type) {
	case ast.TypeDecl:
		n.Pos = at
		*node = n
	case ast.RouteDecl:
		n.Pos = at
		*node = n
	case ast.StructDecl:
		n.Pos = at
		*node = n
	case ast.FuncDecl:
		n.Pos = at
		*node = n
	}
}

// parseRouteDecl parses a route binding, as in
//
//	route GET "/api/v1/modules/{name}" => get_module
func (p *Parser) parseRouteDecl() ast.Decl {
	p.readToken() // consume 'route'
	method := strings.ToUpper(p.name())
	p.expect(lexer.TokenString)
	// A string token carries the path without its quotes.
	path := p.current.Lex
	p.readToken()
	p.expect(lexer.TokenFatArrow)
	p.readToken()
	handler := p.name()
	return ast.RouteDecl{Method: method, Path: path, Handler: handler}
}

func (p *Parser) parseTypeDecl() ast.Decl {
	p.readToken()
	name := p.name()
	if p.current.Kind != lexer.TokenAssign {
		if p.current.Kind == lexer.TokenLBracket {
			_, _ = p.parseTypeParams()
		}
		return ast.TypeDecl{Name: name, Type: ast.TypeExpr{Name: name}}
	}
	p.expect(lexer.TokenAssign)
	p.readToken()
	return ast.TypeDecl{Name: name, Type: p.parseType()}
}

// parseIfStmt parses an if statement, including `else if` and `else`.
//
// The cursor is on `if`. An `else if` is an `else` whose body is a single `if`,
// which is how it is written and how the two-branch form both backends emit
// already expresses it — so it nests rather than becoming a third shape.
func (p *Parser) parseIfStmt() ast.Stmt {
	p.readToken()
	condition := p.parseCondition()
	thenBody := p.parseBlock()
	elseBody := []ast.Stmt{}
	if p.current.Kind == lexer.TokenElse {
		p.readToken()
		if p.current.Kind == lexer.TokenIf {
			elseBody = []ast.Stmt{p.parseIfStmt()}
		} else {
			elseBody = p.parseBlock()
		}
	}
	return ast.IfStmt{Condition: condition, Then: thenBody, Else: elseBody}
}

// parseForStmt parses:
//
//	for name in collection { body }
//	for name in reverse collection { body }
//	for name in collection step n { body }
//
// `in` is not a keyword, so it is matched as the word it is; `reverse` and
// `step` are, and are read where they appear.
func (p *Parser) parseForStmt() ast.Stmt {
	p.readToken() // consume 'for'
	name := p.name()
	if !(p.current.Kind == lexer.TokenIdent && p.current.Lex == "in") {
		panic(NewErrorf(p.current, "expected 'in' after the loop variable, got %q", p.current.Lex))
	}
	p.readToken()
	reverse := false
	if p.current.Kind == lexer.TokenReverse {
		reverse = true
		p.readToken()
	}
	collection := p.parseCondition()
	var step ast.Expr
	if p.current.Kind == lexer.TokenStep {
		p.readToken()
		step = p.parseCondition()
	}
	return ast.ForStmt{Name: name, Collection: collection, Reverse: reverse, Step: step, Body: p.parseBlock()}
}

// parseMatchStmt parses:
//
//	match value {
//	    1 => print("one")
//	    _ => print("other")
//	}
//
// A pattern is a literal to compare against, or `_` for anything. The body is a
// block or a single expression.
func (p *Parser) parseMatchStmt() ast.Stmt {
	p.readToken() // consume 'match'
	value := p.parseCondition()
	p.expect(lexer.TokenLBrace)
	p.readToken()
	arms := []ast.MatchArm{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated match body"))
		}
		arm := ast.MatchArm{}
		if p.current.Kind == lexer.TokenIdent && p.current.Lex == "_" {
			arm.Wildcard = true
			p.readToken()
		} else {
			arm.Pattern = p.parseExpr(0)
		}
		p.expect(lexer.TokenFatArrow)
		p.readToken()
		if p.current.Kind == lexer.TokenLBrace {
			arm.Body = p.parseBlock()
		} else {
			expression := p.parseExpr(0)
			arm.Body = []ast.Stmt{ast.ExprStmt{Expr: expression}}
		}
		arms = append(arms, arm)
		if p.current.Kind == lexer.TokenComma || p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	p.readToken()
	return ast.MatchStmt{Value: value, Arms: arms}
}

// parseConstDecl parses a module-level binding:
//
//	const NAME: T = value
//
// The type annotation is optional, matching a local binding.
func (p *Parser) parseConstDecl() ast.Decl {
	p.readToken()
	name := p.name()
	var declaredType ast.TypeExpr
	if p.current.Kind == lexer.TokenColon {
		p.readToken()
		declaredType = p.parseType()
	}
	var value ast.Expr
	if p.current.Kind == lexer.TokenAssign {
		p.readToken()
		value = p.parseExpr(0)
	}
	p.consumeSemi()
	return ast.ConstDecl{Name: name, Type: declaredType, Value: value}
}

// parseTypeParams parses a bracketed type parameter list, including any
// defaults: `[T, E = Unit]`.
func (p *Parser) parseTypeParams() ([]string, map[string]string) {
	params := []string{}
	defaults := map[string]string{}
	if p.current.Kind != lexer.TokenLBracket {
		return params, defaults
	}
	p.readToken()
	for p.current.Kind != lexer.TokenRBracket {
		name := p.name()
		params = append(params, name)
		// An optional default, written after an equals sign.
		if p.current.Kind == lexer.TokenAssign {
			p.readToken()
			defaults[name] = p.parseType().String()
		}
		if p.current.Kind == lexer.TokenComma {
			p.readToken()
		}
	}
	p.readToken()
	return params, defaults
}

func (p *Parser) parseType() ast.TypeExpr {
	// Dynamic array type: [u8], [u32], etc.
	if p.current.Kind == lexer.TokenLBracket {
		p.readToken()
		inner := p.parseType()
		p.expect(lexer.TokenRBracket)
		p.readToken()
		return ast.TypeExpr{
			Name: "[" + inner.Name + "]",
		}
	}
	typeExpr := ast.TypeExpr{Name: p.name()}
	closing := lexer.TokenRBracket
	if p.current.Kind == lexer.TokenLess {
		closing = lexer.TokenGreater
	}
	if p.current.Kind != lexer.TokenLBracket && p.current.Kind != lexer.TokenLess {
		return typeExpr
	}
	p.readToken()
	// A bracket is two different things depending on what is inside it. A number
	// on its own is a length: `u8[8]` is eight of them, and `SetError[4]` is
	// four. A number after a comma is a value argument, because that is how a
	// declaration says how many of something it holds: `Array[u8, 4]`.
	//
	// Reading every number as a length swapped the element type for the count, so
	// `Array[u8, 4]` parsed as an array of eight-byte things of length `u8` — a
	// name no declaration could be built from, and a type the backend had never
	// heard of.
	afterComma := false
	for p.current.Kind != closing {
		if p.current.Kind == lexer.TokenNumber && !afterComma {
			length, err := strconv.Atoi(p.current.Lex)
			if err != nil {
				panic(NewErrorf(p.current, "invalid array length"))
			}
			typeExpr.ArrayLen = &length
			p.readToken()
		} else if p.current.Kind == lexer.TokenNumber {
			// A value argument, which is how a declaration says how many of
			// something it holds. It is kept as a name rather than a length,
			// because it names the parameter it is substituted for and not a
			// dimension of the element.
			argument := ast.TypeExpr{Name: p.current.Lex}
			typeExpr.Args = append(typeExpr.Args, argument)
			p.readToken()
			afterComma = false
		} else {
			typeExpr.Args = append(typeExpr.Args, p.parseType())
			afterComma = false
		}
		if p.current.Kind == lexer.TokenComma {
			p.readToken()
			afterComma = true
		}
	}
	p.readToken()
	// Further dimensions: `u32[2][2]` is an array of arrays. Each is read into
	// Dims, so the written name keeps every dimension the program wrote.
	for p.current.Kind == lexer.TokenLBracket {
		p.readToken()
		if p.current.Kind != lexer.TokenNumber {
			panic(NewErrorf(p.current, "an array dimension must be a number"))
		}
		dimension, err := strconv.Atoi(p.current.Lex)
		if err != nil {
			panic(NewErrorf(p.current, "invalid array length"))
		}
		typeExpr.Dims = append(typeExpr.Dims, dimension)
		p.readToken()
		p.expect(lexer.TokenRBracket)
		p.readToken()
	}
	return typeExpr
}

func (p *Parser) parseStructDecl() ast.Decl {
	p.readToken()
	name := p.name()
	typeParams, typeDefaults := p.parseTypeParams()
	p.expect(lexer.TokenLBrace)
	p.readToken()
	fields := []ast.Field{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated struct body"))
		}
		fieldName := p.name()
		p.expect(lexer.TokenColon)
		p.readToken()
		fieldType := p.parseType()
		field := ast.Field{Name: fieldName, Type: fieldType.String(), TypeExpr: fieldType}
		// A bit field says how many bits it uses: `flag: u8 : 1`.
		if p.current.Kind == lexer.TokenColon {
			p.readToken()
			if p.current.Kind != lexer.TokenNumber {
				panic(NewErrorf(p.current, "a bit field width must be a number"))
			}
			width, err := strconv.Atoi(p.current.Lex)
			if err != nil {
				panic(NewErrorf(p.current, "invalid bit field width"))
			}
			field.BitWidth = &width
			p.readToken()
		}
		fields = append(fields, field)
		if p.current.Kind == lexer.TokenComma || p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	p.readToken()
	return ast.StructDecl{Name: name, Fields: fields, TypeParams: typeParams, TypeParamDefaults: typeDefaults}
}

// parseFieldList reads `name: Type` entries until the closing brace, which it
// consumes. It is the body of a struct, a message, and an rpc's request and
// response, because a field is a field wherever it is written.
func (p *Parser) parseFieldList() []ast.Field {
	fields := []ast.Field{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated field list"))
		}
		fieldName := p.name()
		p.expect(lexer.TokenColon)
		p.readToken()
		fieldType := p.parseType()
		fields = append(fields, ast.Field{Name: fieldName, Type: fieldType.String(), TypeExpr: fieldType})
		if p.current.Kind == lexer.TokenComma || p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	p.readToken() // the closing brace
	return fields
}

// parseEnumDecl reads:
//
//	enum Severity {
//	    INFO = 0
//	    WARN = 1
//	}
func (p *Parser) parseEnumDecl() ast.Decl {
	p.readToken()
	name := p.name()
	p.expect(lexer.TokenLBrace)
	p.readToken()
	variants := []ast.EnumVariant{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated enum body"))
		}
		variantName := p.name()
		value := 0
		if p.current.Kind == lexer.TokenAssign {
			p.readToken()
			p.expect(lexer.TokenNumber)
			value = p.intLiteral()
		}
		variants = append(variants, ast.EnumVariant{Name: variantName, Value: value})
		if p.current.Kind == lexer.TokenComma || p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	p.readToken()
	return ast.EnumDecl{Name: name, Variants: variants}
}

// parseMessageDecl reads `message Name { fields }`.
func (p *Parser) parseMessageDecl() ast.Decl {
	p.readToken()
	name := p.name()
	p.expect(lexer.TokenLBrace)
	p.readToken()
	return ast.MessageDecl{Name: name, Fields: p.parseFieldList()}
}

// parseRpcDecl reads:
//
//	rpc SetMotorSpeed {
//	    request  { fields }
//	    response { fields }
//	}
func (p *Parser) parseRpcDecl() ast.Decl {
	p.readToken()
	name := p.name()
	request := ast.RpcDecl{Name: name}

	p.expect(lexer.TokenLBrace)
	p.readToken()
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated rpc body"))
		}
		section := p.name()
		p.expect(lexer.TokenLBrace)
		p.readToken()
		fields := p.parseFieldList()
		switch section {
		case "request":
			request.Request = fields
		case "response":
			request.Response = fields
		default:
			panic(NewErrorf(p.current, "expected request or response in rpc %s, got %q", name, section))
		}
	}
	p.readToken()
	return request
}

func (p *Parser) parseFuncDecl() ast.Decl {
	return p.parseFuncDeclWithExtern()
}

func (p *Parser) parseFuncDeclWithExtern() ast.Decl {
	extern := false
	if p.current.Kind == lexer.TokenFn {
		p.readToken()
	} else {
		extern = true
		p.readToken()
		p.expect(lexer.TokenFn)
		p.readToken()
	}
	name := p.name()
	typeParams, typeDefaults := p.parseTypeParams()
	p.expect(lexer.TokenLParen)
	p.readToken()
	params := []ast.Param{}
	for p.current.Kind != lexer.TokenRParen {
		paramName := p.name()
		p.expect(lexer.TokenColon)
		p.readToken()
		paramType := p.parseType()
		params = append(params, ast.Param{Name: paramName, Type: paramType.String(), TypeExpr: paramType})
		if p.current.Kind == lexer.TokenComma {
			p.readToken()
		}
	}
	p.readToken()
	returnType := ast.TypeExpr{Name: "void"}
	if p.current.Kind == lexer.TokenArrow {
		p.readToken()
		returnType = p.parseType()
	} else if p.current.Kind == lexer.TokenIdent {
		returnType = p.parseType()
	}
	body := []ast.Stmt{}
	if !extern {
		body = p.parseBlock()
	} else if p.current.Kind == lexer.TokenSemicolon {
		p.readToken()
	}
	return ast.FuncDecl{Name: name, Params: params, ReturnType: returnType.String(), TypeParams: typeParams, TypeParamDefaults: typeDefaults, Extern: extern, Body: body}
}

// parseCondition reads an if or while condition. The brace that follows it
// opens the body, so a name at the end of the condition is not the head of a
// struct literal — `if id == MESSAGE_ID_TELEMETRY {` is a comparison and a
// block, and reading `MESSAGE_ID_TELEMETRY {` as a literal would swallow the
// body. The name could not be resolved here even in principle: it may be a
// constant declared in another file, which this parser never sees.
func (p *Parser) parseCondition() ast.Expr {
	previous := p.noStructLiteral
	p.noStructLiteral = true
	condition := p.parseExpr(0)
	p.noStructLiteral = previous
	return condition
}

func (p *Parser) parseBlock() []ast.Stmt {
	p.expect(lexer.TokenLBrace)
	p.readToken()
	body := []ast.Stmt{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic(NewErrorf(p.current, "unterminated block"))
		}
		body = append(body, p.parseStmt())
	}
	p.readToken()
	return body
}

// blockExpr reads a block used where an expression is wanted. The block holds
// exactly one expression — or one `if`, which is how a nested if expression is
// written and which is a value for the same reason the outer one is.
func blockExpr(statements []ast.Stmt) ast.Expr {
	if len(statements) == 1 {
		switch statement := statements[0].(type) {
		case ast.ExprStmt:
			return statement.Expr
		case ast.IfStmt:
			if len(statement.Else) == 0 {
				panic(NewError(lexer.Token{}, "an if used as an expression needs an else"))
			}
			return ast.IfExpr{
				Condition: statement.Condition,
				Then:      blockExpr(statement.Then),
				Else:      blockExpr(statement.Else),
			}
		}
	}
	panic(NewError(lexer.Token{}, "expression block must contain one expression"))
}

// parseStmt reads one statement.
//
// A statement is positioned where its first token is, taken before the statement
// is read because reading it moves past every token that belongs to it. Setting
// it here rather than in each case means a new kind of statement is positioned by
// having been added, rather than by remembering.
func (p *Parser) parseStmt() ast.Stmt {
	start := p.position()
	var stmt ast.Stmt
	switch p.current.Kind {
	case lexer.TokenLet, lexer.TokenConst, lexer.TokenVar:
		p.readToken()
		name := p.name()
		// The type annotation is optional: `let x = expr` infers it, while
		// `let x: T = expr` states it and can drive generic deduction.
		var declaredType ast.TypeExpr
		if p.current.Kind == lexer.TokenColon {
			p.readToken()
			declaredType = p.parseType()
		}
		var value ast.Expr
		if p.current.Kind == lexer.TokenAssign {
			p.readToken()
			value = p.parseExpr(0)
		}
		p.consumeSemi()
		stmt = ast.LetStmt{Name: name, Type: declaredType, Value: value}
	case lexer.TokenReturn:
		p.readToken()
		if p.current.Kind == lexer.TokenRBrace || p.current.Kind == lexer.TokenSemicolon {
			p.consumeSemi()
			stmt = ast.ReturnStmt{}
			break
		}
		value := p.parseExpr(0)
		p.consumeSemi()
		stmt = ast.ReturnStmt{Value: value}
	case lexer.TokenIf:
		stmt = p.parseIfStmt()
	case lexer.TokenWhile:
		p.readToken()
		condition := p.parseCondition()
		stmt = ast.WhileStmt{Condition: condition, Body: p.parseBlock()}
	case lexer.TokenFor:
		stmt = p.parseForStmt()
	case lexer.TokenMatch:
		stmt = p.parseMatchStmt()
	case lexer.TokenBreak:
		p.readToken()
		p.consumeSemi()
		stmt = ast.BreakStmt{}
	default:
		left := p.parseExpr(0)
		if p.current.Kind == lexer.TokenAssign {
			op := p.current.Lex
			p.readToken()
			value := p.parseExpr(0)
			p.consumeSemi()
			stmt = ast.AssignStmt{Target: left, Value: value, Op: op}
			break
		}
		p.consumeSemi()
		stmt = ast.ExprStmt{Expr: left}
	}
	// A semicolon is optional after any statement, including one whose body is a
	// block: `if c { }` and `if c { };` are the same statement. The cases that
	// need it have consumed it already, and this is a no-op for them.
	p.consumeSemi()
	p.place(&stmt, start)
	return stmt
}

// place puts a statement at the place it began.
//
// A statement is any of the kinds, and asking each in turn is a type switch
// rather than a change to every case above: the position is where the statement
// started, which reading it has already moved past.
func (p *Parser) place(stmt *ast.Stmt, at ast.Position) {
	switch n := (*stmt).(type) {
	case ast.LetStmt:
		n.Pos = at
		*stmt = n
	case ast.ReturnStmt:
		n.Pos = at
		*stmt = n
	case ast.IfStmt:
		n.Pos = at
		*stmt = n
	case ast.WhileStmt:
		n.Pos = at
		*stmt = n
	case ast.ForStmt:
		n.Pos = at
		*stmt = n
	case ast.MatchStmt:
		n.Pos = at
		*stmt = n
	case ast.BreakStmt:
		n.Pos = at
		*stmt = n
	case ast.AssignStmt:
		n.Pos = at
		*stmt = n
	case ast.ExprStmt:
		n.Pos = at
		*stmt = n
	}
}

func (p *Parser) consumeSemi() {
	if p.current.Kind == lexer.TokenSemicolon {
		p.readToken()
	}
}

var precedence = map[lexer.Kind]int{
	lexer.TokenOr: 1, lexer.TokenAnd: 2,
	lexer.TokenEqual: 3, lexer.TokenNotEqual: 3,
	lexer.TokenLess: 4, lexer.TokenLessEqual: 4, lexer.TokenGreater: 4, lexer.TokenGreaterEqual: 4,
	lexer.TokenPlus: 5, lexer.TokenMinus: 5,
	lexer.TokenBitOr: 5, lexer.TokenCaret: 5, lexer.TokenBitAnd: 6,
	lexer.TokenStar: 7, lexer.TokenSlash: 7, lexer.TokenPercent: 7,
}

func (p *Parser) parseExpr(minPrecedence int) ast.Expr {
	left := p.parsePrefix()
	for {
		currentPrecedence, ok := precedence[p.current.Kind]
		if !ok || currentPrecedence < minPrecedence {
			break
		}
		op := p.current.Lex
		start := left.Position()
		p.readToken()
		left = p.placeExpr(
			ast.BinaryExpr{Op: op, Left: left, Right: p.parseExpr(currentPrecedence + 1)},
			start)
	}
	return left
}

// parsePrefix reads the first thing in an expression.
//
// The position is taken before anything is read, and applied on the way out, so
// that an expression carries where it began rather than where the parser had
// reached by the time it was finished.
func (p *Parser) parsePrefix() ast.Expr {
	start := p.position()
	var expression ast.Expr
	switch p.current.Kind {
	case lexer.TokenNumber:
		expression = ast.NumberExpr{Value: p.current.Lex}
		p.readToken()
	case lexer.TokenString:
		expression = ast.StringExpr{Value: p.current.Lex}
		p.readToken()
	case lexer.TokenTrue, lexer.TokenFalse:
		expression = ast.BoolExpr{Value: p.current.Kind == lexer.TokenTrue}
		p.readToken()
	case lexer.TokenMinus, lexer.TokenNot, lexer.TokenStar, lexer.TokenBitAnd, lexer.TokenBitNot:
		op := p.current.Lex
		p.readToken()
		expression = ast.UnaryExpr{Op: op, Value: p.parsePrefix()}
	case lexer.TokenIf:
		p.readToken()
		condition := p.parseExpr(0)
		then := p.parseBlock()
		if p.current.Kind != lexer.TokenElse {
			panic(NewErrorf(p.current, "if expression requires else"))
		}
		p.readToken()
		var elseExpr ast.Expr
		if p.current.Kind == lexer.TokenLBrace {
			elseExpr = blockExpr(p.parseBlock())
		} else {
			elseExpr = p.parseExpr(0)
		}
		expression = ast.IfExpr{Condition: condition, Then: blockExpr(then), Else: elseExpr}
	case lexer.TokenLParen:
		p.readToken()
		expression = p.parseExpr(0)
		p.expect(lexer.TokenRParen)
		p.readToken()
	case lexer.TokenLBracket:
		p.readToken()
		// An empty dynamic-array literal carries no element type.
		if p.current.Kind == lexer.TokenRBracket {
			p.readToken()
			expression = ast.ArrayLiteralExpr{Values: []ast.Expr{}}
			break
		}
		first := p.parseExpr(0)
		// A repeat literal fills a fixed-size array: `[value; count]`.
		if p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
			count := p.parseExpr(0)
			p.expect(lexer.TokenRBracket)
			p.readToken()
			expression = ast.ArrayLiteralExpr{Values: []ast.Expr{first}, Repeat: &count}
			break
		}
		values := []ast.Expr{first}
		for p.current.Kind != lexer.TokenRBracket {
			if p.current.Kind == lexer.TokenEOF {
				panic(NewErrorf(p.current, "unterminated array literal"))
			}
			if p.current.Kind == lexer.TokenComma {
				p.readToken()
			}
			values = append(values, p.parseExpr(0))
		}
		p.readToken()
		expression = ast.ArrayLiteralExpr{Values: values}
	case lexer.TokenIdent:
		expression = ast.IdentifierExpr{Name: p.current.Lex}
		p.readToken()
	default:
		panic(NewErrorf(p.current, "expected expression, got %q", p.current.Lex))
	}
	// Postfix calls and field accesses extend what was read, so they are applied
	// before the position is: `a.b()` is one expression that begins at `a`.
	return p.placeExpr(p.parsePostfix(expression), start)
}

// placeExpr puts an expression at the place it began.
//
// An expression is any of the kinds, and asking each in turn is a type switch
// rather than a change to every case in parsePrefix.
func (p *Parser) placeExpr(expr ast.Expr, at ast.Position) ast.Expr {
	switch e := expr.(type) {
	case ast.IdentifierExpr:
		e.Pos = at
		return e
	case ast.NumberExpr:
		e.Pos = at
		return e
	case ast.StringExpr:
		e.Pos = at
		return e
	case ast.BoolExpr:
		e.Pos = at
		return e
	case ast.UnaryExpr:
		e.Pos = at
		return e
	case ast.BinaryExpr:
		e.Pos = at
		return e
	case ast.CallExpr:
		e.Pos = at
		return e
	case ast.MemberExpr:
		e.Pos = at
		return e
	case ast.IndexExpr:
		e.Pos = at
		return e
	case ast.CastExpr:
		e.Pos = at
		return e
	case ast.StructLiteralExpr:
		e.Pos = at
		return e
	case ast.ArrayLiteralExpr:
		e.Pos = at
		return e
	case ast.IfExpr:
		e.Pos = at
		return e
	}
	return expr
}

func (p *Parser) parsePostfix(expression ast.Expr) ast.Expr {
	for {
		switch p.current.Kind {
		case lexer.TokenIdent:
			if p.current.Lex != "as" {
				return expression
			}
			p.readToken()
			target := p.parseType()
			expression = ast.CastExpr{Type: target, Value: expression}
		case lexer.TokenDot:
			p.readToken()
			expression = ast.MemberExpr{Object: expression, Name: p.name()}
		case lexer.TokenLBracket:
			application := p.bracketOpensApplication(expression)
			p.readToken()
			if p.current.Kind == lexer.TokenNumber || !application {
				index := p.parseExpr(0)
				p.expect(lexer.TokenRBracket)
				p.readToken()
				expression = ast.IndexExpr{Object: expression, Index: index}
				continue
			}
			args := []ast.TypeExpr{}
			for p.current.Kind != lexer.TokenRBracket {
				args = append(args, p.parseType())
				if p.current.Kind == lexer.TokenComma {
					p.readToken()
				}
			}
			p.readToken()
			if p.current.Kind == lexer.TokenLParen {
				p.readToken()
				callArgs := []ast.Expr{}
				for p.current.Kind != lexer.TokenRParen {
					callArgs = append(callArgs, p.parseExpr(0))
					if p.current.Kind == lexer.TokenComma {
						p.readToken()
					}
				}
				p.readToken()
				expression = ast.CallExpr{Callee: expression, TypeArgs: args, Args: callArgs}
			} else if p.current.Kind == lexer.TokenLBrace {
				p.readToken()
				values := []ast.Expr{}
				for p.current.Kind != lexer.TokenRBrace {
					values = append(values, p.parseExpr(0))
					if p.current.Kind == lexer.TokenComma {
						p.readToken()
					}
				}
				p.readToken()
				expression = ast.StructLiteralExpr{Type: ast.TypeExpr{Name: expression.(ast.IdentifierExpr).Name, Args: args}, Values: values}
			} else {
				index := ast.IdentifierExpr{Name: args[0].Name}
				expression = ast.IndexExpr{Object: expression, Index: index}
			}
		case lexer.TokenLBrace:
			// A condition's brace opens the body, whatever the name before it
			// looks like.
			if p.noStructLiteral {
				return expression
			}
			identifier, isIdentifier := expression.(ast.IdentifierExpr)
			if !isIdentifier || !p.isTypeName(identifier.Name) {
				return expression
			}
			p.readToken()
			values := []ast.Expr{}
			for p.current.Kind != lexer.TokenRBrace {
				values = append(values, p.parseExpr(0))
				if p.current.Kind == lexer.TokenComma {
					p.readToken()
				}
			}
			p.readToken()
			expression = ast.StructLiteralExpr{Type: ast.TypeExpr{Name: identifier.Name}, Values: values}
		case lexer.TokenLParen:
			p.readToken()
			args := []ast.Expr{}
			for p.current.Kind != lexer.TokenRParen {
				args = append(args, p.parseExpr(0))
				if p.current.Kind == lexer.TokenComma {
					p.readToken()
				}
			}
			p.readToken()
			expression = ast.CallExpr{Callee: expression, Args: args}
		default:
			return expression
		}
	}
}

// isTypeApplication reports whether `name[...]` names a generic type rather
// than indexing a value.
//
// Rho spells a type with a leading capital, so `Ptr[u8]` is an application and
// `data[offset]` is an index. Without the case test every indexed value would
// be read as a type and its index parsed as a type argument, which fails as
// soon as the index is arithmetic.
// isTypeName reports whether a name can be the head of a struct literal.
//
// A type is written with a leading capital, so that is the test. The exception
// is a module-level binding: `MAX {` after a comparison opens a block, and a
// capitalised constant is exactly how that is written.
func (p *Parser) isTypeName(name string) bool {
	if p.valueNames != nil && p.valueNames[name] {
		return false
	}
	// A leading capital is the whole test. It used to require two characters,
	// which read `P { 1 }` as a comparison followed by a block and made a
	// one-letter type name — `struct P` — impossible to write a literal of.
	return len(name) >= 1 && name[0] >= 'A' && name[0] <= 'Z'
}

// isUpperName reports whether a word is written with a leading capital, which
// is how the language writes a type name.
func isUpperName(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

// isScalarTypeName reports whether a word is a built-in scalar type name, which
// can only be a type, never a value a program could index by.
func isScalarTypeName(name string) bool {
	switch name {
	case "u8", "u16", "u32", "u64",
		"i8", "i16", "i32", "i64",
		"f32", "f64", "bool", "string":
		return true
	}
	return false
}

func isTypeApplication(expression ast.Expr) bool {
	identifier, ok := expression.(ast.IdentifierExpr)
	if !ok || identifier.Name == "" {
		return false
	}
	first := identifier.Name[0]
	return first >= 'A' && first <= 'Z'
}

// bracketOpensApplication reports whether the bracket at the cursor opens a
// type-argument list rather than an index.
//
// The two share a spelling, so the decision is made by what follows the closing
// bracket: a call or a brace is an application — `twice[u32](1)`, `Box[u32]{v}`
// — and anything else is an index, `data[offset]`. It is a lookahead because
// the contents cannot tell them apart: `u32` is a type when applied and a name
// when indexed.
func (p *Parser) bracketOpensApplication(expression ast.Expr) bool {
	identifier, isIdentifier := expression.(ast.IdentifierExpr)
	if !isIdentifier {
		// Only a bare name can be applied: `suffix.ptr[i]` indexes a field.
		return false
	}
	if p.valueNames != nil && p.valueNames[identifier.Name] {
		// A module-level binding is a value, so a bracket on it is an index.
		return false
	}
	if p.tokens == nil {
		// A streamed parse has no lookahead, so the leading capital is the best
		// that can be said.
		return isTypeApplication(expression)
	}
	// What is inside the bracket says what the bracket is. A scalar type name
	// can only be a type argument — no program has a value called `u32` — so
	// `from_u64[u32](x)` is an application even though the name is lower-case
	// and declared in another module the parser has not read, which is what a
	// declared name alone could not tell it.
	if p.nextTok.Kind == lexer.TokenIdent && isScalarTypeName(p.nextTok.Lex) {
		return true
	}
	// A capitalized name is a type by convention, so `f[SomeType](x)` is an
	// application too — the case the parser could not decide before, because the
	// function is imported and its declaration was never read. A capitalized
	// name the module binds to a value is a constant, and an index.
	if p.nextTok.Kind == lexer.TokenIdent && isUpperName(p.nextTok.Lex) &&
		!(p.valueNames != nil && p.valueNames[p.nextTok.Lex]) {
		return true
	}
	if p.applyNames != nil && !p.applyNames[identifier.Name] {
		// A name that cannot take a type argument — a parameter, a local — has
		// its bracket read as an index. `if data[i] {` is an index before a
		// block, not a literal whose body is the block.
		return false
	}
	// current is `[` at tokens[tokenAt-2]; nextTok is the first token inside.
	open := p.tokenAt - 2
	if open < 0 || open >= len(p.tokens) || p.tokens[open].Kind != lexer.TokenLBracket {
		return false
	}
	depth := 0
	for i := open; i < len(p.tokens); i++ {
		switch p.tokens[i].Kind {
		case lexer.TokenLBracket:
			depth++
		case lexer.TokenRBracket:
			depth--
			if depth == 0 {
				if i+1 < len(p.tokens) {
					next := p.tokens[i+1].Kind
					return next == lexer.TokenLParen || next == lexer.TokenLBrace
				}
				return false
			}
		}
	}
	return false
}
