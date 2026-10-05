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
}

func New(lx *lexer.Lexer) *Parser {
	p := &Parser{lx: lx}
	p.readToken()
	p.readToken()
	return p
}

func Parse(tokens []lexer.Token) (mod *ast.Module, err error) {
	p := &Parser{tokens: tokens}
	p.readToken()
	p.readToken()
	return p.Parse()
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
	mod.Includes = p.parseIncludes()
	mod.Imports = p.parseImports()
	mod.Features = p.parseFeatures()
	mod.Decls = p.parseTopLevelDecls()
	return mod, nil
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
		case lexer.TokenFn:
			decl = p.parseFuncDecl()
		case lexer.TokenExtern:
			decl = p.parseFuncDeclWithExtern()
		case lexer.TokenType:
			decl = p.parseTypeDecl()
		case lexer.TokenRoute:
			decl = p.parseRouteDecl()
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
		fields = append(fields, ast.Field{Name: fieldName, Type: fieldType.String(), TypeExpr: fieldType})
		if p.current.Kind == lexer.TokenComma || p.current.Kind == lexer.TokenSemicolon {
			p.readToken()
		}
	}
	p.readToken()
	return ast.StructDecl{Name: name, Fields: fields, TypeParams: typeParams, TypeParamDefaults: typeDefaults}
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

func blockExpr(statements []ast.Stmt) ast.Expr {
	if len(statements) == 1 {
		if exprStmt, ok := statements[0].(ast.ExprStmt); ok {
			return exprStmt.Expr
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
	case lexer.TokenLet:
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
		p.readToken()
		condition := p.parseExpr(0)
		thenBody := p.parseBlock()
		elseBody := []ast.Stmt{}
		if p.current.Kind == lexer.TokenElse {
			p.readToken()
			elseBody = p.parseBlock()
		}
		stmt = ast.IfStmt{Condition: condition, Then: thenBody, Else: elseBody}
	case lexer.TokenWhile:
		p.readToken()
		condition := p.parseExpr(0)
		stmt = ast.WhileStmt{Condition: condition, Body: p.parseBlock()}
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
		p.readToken()
		left = ast.BinaryExpr{Op: op, Left: left, Right: p.parseExpr(currentPrecedence + 1)}
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
		values := []ast.Expr{}
		for p.current.Kind != lexer.TokenRBracket {
			if p.current.Kind == lexer.TokenEOF {
				panic(NewErrorf(p.current, "unterminated array literal"))
			}
			values = append(values, p.parseExpr(0))
			if p.current.Kind == lexer.TokenComma {
				p.readToken()
			}
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
			p.readToken()
			if p.current.Kind == lexer.TokenNumber || !isTypeApplication(expression) {
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
			identifier, isIdentifier := expression.(ast.IdentifierExpr)
			if !isIdentifier || len(identifier.Name) < 2 || identifier.Name[0] < 'A' || identifier.Name[0] > 'Z' {
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

func isTypeApplication(expression ast.Expr) bool {
	_, ok := expression.(ast.IdentifierExpr)
	return ok
}
