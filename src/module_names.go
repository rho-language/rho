//
// @file module_names.go
// @brief Each imported module's types and constants are its own.
//
// A program is compiled as one list of declarations, the importing module's and
// every module it imports. A function already belongs to the module that
// declared it; this pass does the same for a struct, an enum, a type alias and a
// constant, so two modules that each declare a `Config` or a `LIMIT` have two of
// them, and an application's own `Config` is never replaced by one it imported.
//
// An imported module's name is given a name of its own (`app_a_Config` for
// `Config` in `app.a`), and every reference to it is rewritten to that name
// before the checker reads the program:
//
//   - inside the module, its own declaration is the one meant;
//   - elsewhere, a name only one module declares is that module's, and a name the
//     importing code declares itself is its own;
//   - `a.Config` and `a.LIMIT` always mean module a's, which is how a name two
//     modules share is written.
//
// A module that publishes `exports` lets another module name only what it lists.
// The standard library is the language's own vocabulary — `Request`, `Option` —
// and keeps the names it has.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/frontend/ast"
)

// moduleNames is what the pass knows about the program's declarations.
type moduleNames struct {
	// types and consts are each module's own names, by module and then by the
	// name written, mapped to the name they are compiled under.
	types  map[string]map[string]string
	consts map[string]map[string]string
	// typeOwners and constOwners are the modules that declare a name.
	typeOwners  map[string][]string
	constOwners map[string][]string
	// globalTypes and globalConsts are declared by the importing code or the
	// standard library, and keep their names.
	globalTypes  map[string]bool
	globalConsts map[string]bool
	// aliases maps the name code uses for a module — the last segment of its
	// name — to the module, or to "" when two modules share it.
	aliases map[string]string
	exports map[string][]string
}

// ownedModule reports whether a module's names are its own: every imported
// module except the standard library.
func ownedModule(owner string) bool {
	return owner != "" && owner != "std" && !strings.HasPrefix(owner, "std.")
}

// ownModuleNames gives every imported module's types and constants names of
// their own and rewrites the references to them.
func ownModuleNames(module *ast.Module) error {
	names := &moduleNames{
		types:        map[string]map[string]string{},
		consts:       map[string]map[string]string{},
		typeOwners:   map[string][]string{},
		constOwners:  map[string][]string{},
		globalTypes:  map[string]bool{},
		globalConsts: map[string]bool{},
		aliases:      map[string]string{},
		exports:      module.Exports,
	}
	declareType := func(owner, name string) {
		if !ownedModule(owner) {
			names.globalTypes[name] = true
			return
		}
		if names.types[owner] == nil {
			names.types[owner] = map[string]string{}
		}
		if _, seen := names.types[owner][name]; !seen {
			names.types[owner][name] = ownedName(owner, name)
			names.typeOwners[name] = append(names.typeOwners[name], owner)
		}
	}
	for _, decl := range module.Decls {
		switch declaration := decl.(type) {
		case ast.StructDecl:
			declareType(declaration.Owner, declaration.Name)
		case ast.EnumDecl:
			declareType(declaration.Owner, declaration.Name)
		case ast.TypeDecl:
			declareType(declaration.Owner, declaration.Name)
		case ast.MessageDecl:
			names.globalTypes[declaration.Name] = true
		case ast.ConstDecl:
			owner := declaration.Owner
			if !ownedModule(owner) {
				names.globalConsts[declaration.Name] = true
				continue
			}
			if names.consts[owner] == nil {
				names.consts[owner] = map[string]string{}
			}
			if _, seen := names.consts[owner][declaration.Name]; !seen {
				names.consts[owner][declaration.Name] = ownedName(owner, declaration.Name)
				names.constOwners[declaration.Name] = append(names.constOwners[declaration.Name], owner)
			}
		}
	}
	if len(names.types) == 0 && len(names.consts) == 0 {
		return nil
	}
	owners := map[string]bool{}
	for owner := range names.types {
		owners[owner] = true
	}
	for owner := range names.consts {
		owners[owner] = true
	}
	for owner := range owners {
		alias := owner[strings.LastIndex(owner, ".")+1:]
		if previous, taken := names.aliases[alias]; taken && previous != owner {
			names.aliases[alias] = ""
			continue
		}
		names.aliases[alias] = owner
	}

	for index, decl := range module.Decls {
		rewritten, err := names.declaration(decl)
		if err != nil {
			return err
		}
		module.Decls[index] = rewritten
	}
	return nil
}

// ownedName is the name a module's own declaration is compiled under.
func ownedName(owner, name string) string {
	return strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(owner) + "_" + name
}

// exported reports whether a module lets other modules name one of its names.
func (names *moduleNames) exported(owner, name string) bool {
	list, listed := names.exports[owner]
	if !listed {
		return true
	}
	for _, entry := range list {
		if entry == name {
			return true
		}
	}
	return false
}

// notExported is the refusal of a name a module keeps to itself, with what it
// does export.
func (names *moduleNames) notExported(owner, name string) error {
	return fmt.Errorf("%s is not exported by %s (it exports %s)", name, owner, strings.Join(names.exports[owner], ", "))
}

// scope is where a reference is resolved: the module whose code it is, the
// names that code declares locally, and the type parameters of a generic.
type scope struct {
	owner      string
	locals     map[string]bool
	typeParams map[string]bool
}

// resolve is the name a written type or constant is compiled under. own is the
// table of the kind asked for; a name that is not one of them is returned as it
// was, for the checker to accept or report.
func (names *moduleNames) resolve(written string, at scope, own map[string]map[string]string, owners map[string][]string, globals map[string]bool, kind string) (string, error) {
	if dot := strings.Index(written, "."); dot > 0 {
		alias, name := written[:dot], written[dot+1:]
		owner, known := names.aliases[alias]
		if !known {
			return written, nil
		}
		if owner == "" {
			return "", fmt.Errorf("%s: more than one imported module is called %q", written, alias)
		}
		compiled, declared := own[owner][name]
		if !declared {
			return "", fmt.Errorf("%s: module %s declares no %s %s", written, owner, kind, name)
		}
		if owner != at.owner && !names.exported(owner, name) {
			return "", names.notExported(owner, name)
		}
		return compiled, nil
	}
	if at.typeParams[written] {
		return written, nil
	}
	if compiled, mine := own[at.owner][written]; mine {
		return compiled, nil
	}
	if globals[written] {
		return written, nil
	}
	declaring := owners[written]
	switch len(declaring) {
	case 0:
		return written, nil
	case 1:
		owner := declaring[0]
		if !names.exported(owner, written) {
			return "", names.notExported(owner, written)
		}
		return own[owner][written], nil
	}
	sorted := append([]string(nil), declaring...)
	sort.Strings(sorted)
	qualified := make([]string, len(sorted))
	for i, owner := range sorted {
		qualified[i] = owner[strings.LastIndex(owner, ".")+1:] + "." + written
	}
	return "", fmt.Errorf("%s is declared by %s; write %s", written, strings.Join(sorted, " and "), strings.Join(qualified, " or "))
}

func (names *moduleNames) typeName(written string, at scope) (string, error) {
	return names.resolve(written, at, names.types, names.typeOwners, names.globalTypes, "type")
}

func (names *moduleNames) constName(written string, at scope) (string, error) {
	return names.resolve(written, at, names.consts, names.constOwners, names.globalConsts, "constant")
}

// typeString rewrites every type name in a written type — `Config`,
// `[Config]`, `Option[a.Config]`, `Config[4]`.
func (names *moduleNames) typeString(written string, at scope) (string, error) {
	var out strings.Builder
	for i := 0; i < len(written); {
		if !identStart(written[i]) {
			out.WriteByte(written[i])
			i++
			continue
		}
		start := i
		for i < len(written) && identPart(written[i]) {
			i++
		}
		for i+1 < len(written) && written[i] == '.' && identStart(written[i+1]) {
			i++
			for i < len(written) && identPart(written[i]) {
				i++
			}
		}
		resolved, err := names.typeName(written[start:i], at)
		if err != nil {
			return "", err
		}
		out.WriteString(resolved)
	}
	return out.String(), nil
}

func identStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func identPart(c byte) bool {
	return identStart(c) || (c >= '0' && c <= '9')
}

func (names *moduleNames) typeExpr(expr ast.TypeExpr, at scope) (ast.TypeExpr, error) {
	if expr.Name != "" {
		resolved, err := names.typeString(expr.Name, at)
		if err != nil {
			return expr, err
		}
		expr.Name = resolved
	}
	if len(expr.Args) > 0 {
		args := make([]ast.TypeExpr, len(expr.Args))
		for i, arg := range expr.Args {
			resolved, err := names.typeExpr(arg, at)
			if err != nil {
				return expr, err
			}
			args[i] = resolved
		}
		expr.Args = args
	}
	return expr, nil
}

// declaration rewrites one declaration, in the scope of the module that
// declared it.
func (names *moduleNames) declaration(decl ast.Decl) (ast.Decl, error) {
	wrap := func(name, owner string, err error) error {
		where := "the program"
		if owner != "" {
			where = "module " + owner
		}
		return fmt.Errorf("%s, in %s: %w", where, name, err)
	}
	switch declaration := decl.(type) {
	case ast.StructDecl:
		at := scope{owner: declaration.Owner, typeParams: setOf(declaration.TypeParams)}
		if compiled, mine := names.types[declaration.Owner][declaration.Name]; mine {
			declaration.Name = compiled
		}
		fields, err := names.fields(declaration.Fields, at)
		if err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		declaration.Fields = fields
		if declaration.TypeParamDefaults, err = names.defaults(declaration.TypeParamDefaults, at); err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		return declaration, nil
	case ast.EnumDecl:
		if compiled, mine := names.types[declaration.Owner][declaration.Name]; mine {
			declaration.Name = compiled
		}
		return declaration, nil
	case ast.TypeDecl:
		at := scope{owner: declaration.Owner}
		if compiled, mine := names.types[declaration.Owner][declaration.Name]; mine {
			declaration.Name = compiled
		}
		target, err := names.typeExpr(declaration.Type, at)
		if err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		declaration.Type = target
		return declaration, nil
	case ast.ConstDecl:
		at := scope{owner: declaration.Owner}
		if compiled, mine := names.consts[declaration.Owner][declaration.Name]; mine {
			declaration.Name = compiled
		}
		var err error
		if declaration.Type, err = names.typeExpr(declaration.Type, at); err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		if declaration.Value != nil {
			if declaration.Value, err = names.expr(declaration.Value, at); err != nil {
				return decl, wrap(declaration.Name, declaration.Owner, err)
			}
		}
		return declaration, nil
	case ast.MessageDecl:
		fields, err := names.fields(declaration.Fields, scope{})
		if err != nil {
			return decl, wrap(declaration.Name, "", err)
		}
		declaration.Fields = fields
		return declaration, nil
	case ast.RpcDecl:
		var err error
		if declaration.Request, err = names.fields(declaration.Request, scope{}); err != nil {
			return decl, wrap(declaration.Name, "", err)
		}
		if declaration.Response, err = names.fields(declaration.Response, scope{}); err != nil {
			return decl, wrap(declaration.Name, "", err)
		}
		return declaration, nil
	case ast.FuncDecl:
		at := scope{owner: declaration.Owner, typeParams: setOf(declaration.TypeParams), locals: localNames(declaration)}
		params := make([]ast.Param, len(declaration.Params))
		for i, param := range declaration.Params {
			var err error
			if param.Type, err = names.typeString(param.Type, at); err != nil {
				return decl, wrap(declaration.Name, declaration.Owner, err)
			}
			if param.TypeExpr, err = names.typeExpr(param.TypeExpr, at); err != nil {
				return decl, wrap(declaration.Name, declaration.Owner, err)
			}
			params[i] = param
		}
		declaration.Params = params
		var err error
		if declaration.ReturnType, err = names.typeString(declaration.ReturnType, at); err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		if declaration.TypeParamDefaults, err = names.defaults(declaration.TypeParamDefaults, at); err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		if declaration.Body, err = names.stmts(declaration.Body, at); err != nil {
			return decl, wrap(declaration.Name, declaration.Owner, err)
		}
		return declaration, nil
	}
	return decl, nil
}

func (names *moduleNames) fields(fields []ast.Field, at scope) ([]ast.Field, error) {
	out := make([]ast.Field, len(fields))
	for i, field := range fields {
		var err error
		if field.Type, err = names.typeString(field.Type, at); err != nil {
			return nil, err
		}
		if field.TypeExpr, err = names.typeExpr(field.TypeExpr, at); err != nil {
			return nil, err
		}
		out[i] = field
	}
	return out, nil
}

func (names *moduleNames) defaults(defaults map[string]string, at scope) (map[string]string, error) {
	if len(defaults) == 0 {
		return defaults, nil
	}
	out := make(map[string]string, len(defaults))
	for param, written := range defaults {
		resolved, err := names.typeString(written, at)
		if err != nil {
			return nil, err
		}
		out[param] = resolved
	}
	return out, nil
}

func setOf(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, name := range list {
		set[name] = true
	}
	return set
}

// localNames are the names a function declares for itself: its parameters, its
// lets and its loop variables, anywhere in its body. A constant or a module
// written with one of those names inside the function is the local.
func localNames(fn ast.FuncDecl) map[string]bool {
	locals := map[string]bool{}
	for _, param := range fn.Params {
		locals[param.Name] = true
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, statement := range stmts {
			switch stmt := statement.(type) {
			case ast.LetStmt:
				locals[stmt.Name] = true
			case ast.ForStmt:
				locals[stmt.Name] = true
				walk(stmt.Body)
			case ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case ast.WhileStmt:
				walk(stmt.Body)
			case ast.BlockStmt:
				walk(stmt.Stmts)
			case ast.MatchStmt:
				for _, arm := range stmt.Arms {
					walk(arm.Body)
				}
			}
		}
	}
	walk(fn.Body)
	return locals
}

func (names *moduleNames) stmts(stmts []ast.Stmt, at scope) ([]ast.Stmt, error) {
	if len(stmts) == 0 {
		return stmts, nil
	}
	out := make([]ast.Stmt, len(stmts))
	for i, statement := range stmts {
		rewritten, err := names.stmt(statement, at)
		if err != nil {
			return nil, err
		}
		out[i] = rewritten
	}
	return out, nil
}

func (names *moduleNames) stmt(statement ast.Stmt, at scope) (ast.Stmt, error) {
	var err error
	switch stmt := statement.(type) {
	case ast.LetStmt:
		if stmt.Type, err = names.typeExpr(stmt.Type, at); err != nil {
			return nil, err
		}
		if stmt.Value, err = names.expr(stmt.Value, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.AssignStmt:
		if stmt.Target, err = names.expr(stmt.Target, at); err != nil {
			return nil, err
		}
		if stmt.Value, err = names.expr(stmt.Value, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.ExprStmt:
		if stmt.Expr, err = names.expr(stmt.Expr, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.ReturnStmt:
		if stmt.Value, err = names.expr(stmt.Value, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.IfStmt:
		if stmt.Condition, err = names.expr(stmt.Condition, at); err != nil {
			return nil, err
		}
		if stmt.Then, err = names.stmts(stmt.Then, at); err != nil {
			return nil, err
		}
		if stmt.Else, err = names.stmts(stmt.Else, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.WhileStmt:
		if stmt.Condition, err = names.expr(stmt.Condition, at); err != nil {
			return nil, err
		}
		if stmt.Body, err = names.stmts(stmt.Body, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.ForStmt:
		if stmt.Collection, err = names.expr(stmt.Collection, at); err != nil {
			return nil, err
		}
		if stmt.Step, err = names.expr(stmt.Step, at); err != nil {
			return nil, err
		}
		if stmt.Body, err = names.stmts(stmt.Body, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.BlockStmt:
		if stmt.Stmts, err = names.stmts(stmt.Stmts, at); err != nil {
			return nil, err
		}
		return stmt, nil
	case ast.MatchStmt:
		if stmt.Value, err = names.expr(stmt.Value, at); err != nil {
			return nil, err
		}
		arms := make([]ast.MatchArm, len(stmt.Arms))
		for i, arm := range stmt.Arms {
			if arm.Pattern, err = names.expr(arm.Pattern, at); err != nil {
				return nil, err
			}
			if arm.Body, err = names.stmts(arm.Body, at); err != nil {
				return nil, err
			}
			arms[i] = arm
		}
		stmt.Arms = arms
		return stmt, nil
	}
	return statement, nil
}

// expr rewrites the constants and types an expression names. A function is
// not touched: the checker resolves a call through the module that owns it.
func (names *moduleNames) expr(expression ast.Expr, at scope) (ast.Expr, error) {
	if expression == nil {
		return nil, nil
	}
	var err error
	switch expr := expression.(type) {
	case ast.IdentifierExpr:
		if at.locals[expr.Name] {
			return expr, nil
		}
		resolved, err := names.constName(expr.Name, at)
		if err != nil {
			return nil, err
		}
		if resolved == expr.Name {
			// A type in a value's place: an enum's name before its variant, or
			// the argument of sizeof.
			if resolved, err = names.typeName(expr.Name, at); err != nil {
				return nil, err
			}
		}
		expr.Name = resolved
		return expr, nil
	case ast.MemberExpr:
		// `a.LIMIT`, `a.Color` and `a.Color.Red` name what module a declares.
		if object, isIdentifier := expr.Object.(ast.IdentifierExpr); isIdentifier && !at.locals[object.Name] {
			if owner, isModule := names.aliases[object.Name]; isModule && owner != "" {
				qualified := object.Name + "." + expr.Name
				if _, isConst := names.consts[owner][expr.Name]; isConst {
					resolved, err := names.constName(qualified, at)
					if err != nil {
						return nil, err
					}
					return ast.IdentifierExpr{Node: expr.Node, Name: resolved}, nil
				}
				if _, isType := names.types[owner][expr.Name]; isType {
					resolved, err := names.typeName(qualified, at)
					if err != nil {
						return nil, err
					}
					return ast.IdentifierExpr{Node: expr.Node, Name: resolved}, nil
				}
				// Anything else is a call into the module, which the checker
				// resolves.
				return expr, nil
			}
		}
		if expr.Object, err = names.expr(expr.Object, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.CallExpr:
		// The callee is a function, unless it is a member of something that is
		// not a module.
		if _, isIdentifier := expr.Callee.(ast.IdentifierExpr); !isIdentifier {
			if expr.Callee, err = names.expr(expr.Callee, at); err != nil {
				return nil, err
			}
		}
		if len(expr.TypeArgs) > 0 {
			args := make([]ast.TypeExpr, len(expr.TypeArgs))
			for i, arg := range expr.TypeArgs {
				if args[i], err = names.typeExpr(arg, at); err != nil {
					return nil, err
				}
			}
			expr.TypeArgs = args
		}
		if expr.Args, err = names.exprs(expr.Args, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.UnaryExpr:
		if expr.Value, err = names.expr(expr.Value, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.BinaryExpr:
		if expr.Left, err = names.expr(expr.Left, at); err != nil {
			return nil, err
		}
		if expr.Right, err = names.expr(expr.Right, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.IndexExpr:
		if expr.Object, err = names.expr(expr.Object, at); err != nil {
			return nil, err
		}
		if expr.Index, err = names.expr(expr.Index, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.CastExpr:
		if expr.Type, err = names.typeExpr(expr.Type, at); err != nil {
			return nil, err
		}
		if expr.Value, err = names.expr(expr.Value, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.StructLiteralExpr:
		if expr.Type, err = names.typeExpr(expr.Type, at); err != nil {
			return nil, err
		}
		if expr.Values, err = names.exprs(expr.Values, at); err != nil {
			return nil, err
		}
		return expr, nil
	case ast.ArrayLiteralExpr:
		if expr.Values, err = names.exprs(expr.Values, at); err != nil {
			return nil, err
		}
		if expr.Repeat != nil {
			repeat, err := names.expr(*expr.Repeat, at)
			if err != nil {
				return nil, err
			}
			expr.Repeat = &repeat
		}
		return expr, nil
	case ast.IfExpr:
		if expr.Condition, err = names.expr(expr.Condition, at); err != nil {
			return nil, err
		}
		if expr.Then, err = names.expr(expr.Then, at); err != nil {
			return nil, err
		}
		if expr.Else, err = names.expr(expr.Else, at); err != nil {
			return nil, err
		}
		return expr, nil
	}
	return expression, nil
}

func (names *moduleNames) exprs(list []ast.Expr, at scope) ([]ast.Expr, error) {
	if len(list) == 0 {
		return list, nil
	}
	out := make([]ast.Expr, len(list))
	for i, expression := range list {
		rewritten, err := names.expr(expression, at)
		if err != nil {
			return nil, err
		}
		out[i] = rewritten
	}
	return out, nil
}
