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
			err = fmt.Errorf("%v", recovered)
		}
	}()
	if p.current.Kind == lexer.TokenEOF {
		return nil, fmt.Errorf("expected module declaration")
	}
	mod = p.parseModuleDecl()
	mod.Includes = p.parseIncludes()
	mod.Imports = p.parseImports()
	mod.Decls = p.parseTopLevelDecls()
	return mod, nil
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
		panic(fmt.Sprintf("expected %v at byte %d, got %v (%q)", kind, p.current.Pos, p.current.Kind, p.current.Lex))
	}
}

func isName(kind lexer.Kind) bool {
	return kind == lexer.TokenIdent || (kind >= lexer.TokenModule && kind <= lexer.TokenType)
}

func (p *Parser) name() string {
	if !isName(p.current.Kind) {
		panic(fmt.Sprintf("expected name at byte %d, got %q", p.current.Pos, p.current.Lex))
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

func (p *Parser) parseTopLevelDecls() []ast.Decl {
	decls := []ast.Decl{}
	for p.current.Kind != lexer.TokenEOF {
		switch p.current.Kind {
		case lexer.TokenStruct:
			decls = append(decls, p.parseStructDecl())
		case lexer.TokenFn:
			decls = append(decls, p.parseFuncDecl())
		case lexer.TokenExtern:
			decls = append(decls, p.parseFuncDeclWithExtern())
		case lexer.TokenType:
			decls = append(decls, p.parseTypeDecl())
		default:
			panic(fmt.Sprintf("unexpected token in top-level: %q", p.current.Lex))
		}
	}
	return decls
}

func (p *Parser) parseTypeDecl() ast.Decl {
	p.readToken()
	name := p.name()
	p.expect(lexer.TokenAssign)
	p.readToken()
	return ast.TypeDecl{Name: name, Type: p.parseType()}
}

func (p *Parser) parseTypeParams() []string {
	params := []string{}
	if p.current.Kind != lexer.TokenLBracket {
		return params
	}
	p.readToken()
	for p.current.Kind != lexer.TokenRBracket {
		params = append(params, p.name())
		if p.current.Kind == lexer.TokenComma {
			p.readToken()
		}
	}
	p.readToken()
	return params
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
	for p.current.Kind != closing {
		if p.current.Kind == lexer.TokenNumber {
			length, err := strconv.Atoi(p.current.Lex)
			if err != nil {
				panic("invalid array length")
			}
			typeExpr.ArrayLen = &length
			p.readToken()
		} else {
			typeExpr.Args = append(typeExpr.Args, p.parseType())
		}
		if p.current.Kind == lexer.TokenComma {
			p.readToken()
		}
	}
	p.readToken()
	return typeExpr
}

func (p *Parser) parseStructDecl() ast.Decl {
	p.readToken()
	name := p.name()
	typeParams := p.parseTypeParams()
	p.expect(lexer.TokenLBrace)
	p.readToken()
	fields := []ast.Field{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic("unterminated struct body")
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
	return ast.StructDecl{Name: name, Fields: fields, TypeParams: typeParams}
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
	typeParams := p.parseTypeParams()
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
	}
	return ast.FuncDecl{Name: name, Params: params, ReturnType: returnType.String(), TypeParams: typeParams, Extern: extern, Body: body}
}

func (p *Parser) parseBlock() []ast.Stmt {
	p.expect(lexer.TokenLBrace)
	p.readToken()
	body := []ast.Stmt{}
	for p.current.Kind != lexer.TokenRBrace {
		if p.current.Kind == lexer.TokenEOF {
			panic("unterminated block")
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
	panic("expression block must contain one expression")
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.current.Kind {
	case lexer.TokenLet:
		p.readToken()
		name := p.name()
		p.expect(lexer.TokenColon)
		p.readToken()
		declaredType := p.parseType()
		var value ast.Expr
		if p.current.Kind == lexer.TokenAssign {
			p.readToken()
			value = p.parseExpr(0)
		}
		p.consumeSemi()
		return ast.LetStmt{Name: name, Type: declaredType, Value: value}
	case lexer.TokenReturn:
		p.readToken()
		if p.current.Kind == lexer.TokenRBrace || p.current.Kind == lexer.TokenSemicolon {
			p.consumeSemi()
			return ast.ReturnStmt{}
		}
		value := p.parseExpr(0)
		p.consumeSemi()
		return ast.ReturnStmt{Value: value}
	case lexer.TokenIf:
		p.readToken()
		condition := p.parseExpr(0)
		thenBody := p.parseBlock()
		elseBody := []ast.Stmt{}
		if p.current.Kind == lexer.TokenElse {
			p.readToken()
			elseBody = p.parseBlock()
		}
		return ast.IfStmt{Condition: condition, Then: thenBody, Else: elseBody}
	case lexer.TokenWhile:
		p.readToken()
		condition := p.parseExpr(0)
		return ast.WhileStmt{Condition: condition, Body: p.parseBlock()}
	case lexer.TokenBreak:
		p.readToken()
		p.consumeSemi()
		return ast.BreakStmt{}
	default:
		left := p.parseExpr(0)
		if p.current.Kind == lexer.TokenAssign {
			op := p.current.Lex
			p.readToken()
			value := p.parseExpr(0)
			p.consumeSemi()
			return ast.AssignStmt{Target: left, Value: value, Op: op}
		}
		p.consumeSemi()
		return ast.ExprStmt{Expr: left}
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

func (p *Parser) parsePrefix() ast.Expr {
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
			panic("if expression requires else")
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
	case lexer.TokenIdent:
		expression = ast.IdentifierExpr{Name: p.current.Lex}
		p.readToken()
	default:
		panic(fmt.Sprintf("expected expression, got %q", p.current.Lex))
	}
	return p.parsePostfix(expression)
}

func (p *Parser) parsePostfix(expression ast.Expr) ast.Expr {
	for {
		switch p.current.Kind {
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
