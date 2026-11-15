//
// @file deprecations.go
// @brief A warning at every use of something marked `@deprecated`.
//
// A declaration whose doc comment carries `// @deprecated use X instead` still
// compiles and still works. What changes is that every use of it from outside
// the module that declares it is a warning, naming the declaration, saying what
// the comment says, and pointing at the declaration in a note:
//
//	src/main.rho:7:5: warning: read_all is deprecated: use read_bytes instead
//	lib/files.rho:12:1: note: read_all is declared here
//
// The module that declares it goes on using it without being told: a module
// keeping an old name working is not a module that needs to move off it. An
// import of a module whose header carries `@deprecated` is a warning at the
// import.
//
// This is the first half of the promise docs/stability.md makes: a change that
// breaks a program is a deprecation warning for at least one minor release
// before it is a refusal.
//
// The uses are found by walking the declarations once, after the names are
// registered, the way the checker resolves them: a local of the same name hides
// a function or a constant, an unqualified call means the caller's own module's
// function first, and a qualified one the module its alias names. A use inside
// a position-less node is reported at the nearest enclosing node that has one,
// which for `old(x)` and `Mode.Old` is where the expression begins.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"rho/src/frontend/ast"
)

// DeprecatedCode is the code a deprecation warning carries, so an editor can
// draw the use struck through and a tool can filter on it.
const DeprecatedCode = "deprecated"

// deprecatedDecl is one declaration marked deprecated.
type deprecatedDecl struct {
	kind  string // function, struct, enum, variant, const, type
	name  string // as a use names it: `old`, `Mode.Old`
	note  string
	owner string
	pos   ast.Position
}

// deprecations is what the walk resolves uses against.
type deprecations struct {
	funcs    map[string]*deprecatedDecl // by declaration position
	types    map[string]*deprecatedDecl // struct, enum and alias, by name
	consts   map[string]*deprecatedDecl
	variants map[string]*deprecatedDecl // by `Enum.Variant`
	// bareVariants are deprecated variants whose name no other enum's variant
	// has, so a pattern naming one bare (`Old =>`) can only mean it.
	bareVariants map[string]*deprecatedDecl
}

func (d *deprecations) empty() bool {
	return len(d.funcs) == 0 && len(d.types) == 0 && len(d.consts) == 0 && len(d.variants) == 0
}

// collectDeprecations reads the deprecated declarations of a program.
func collectDeprecations(m ast.Module) *deprecations {
	d := &deprecations{
		funcs: map[string]*deprecatedDecl{}, types: map[string]*deprecatedDecl{},
		consts: map[string]*deprecatedDecl{}, variants: map[string]*deprecatedDecl{},
		bareVariants: map[string]*deprecatedDecl{},
	}
	variantNames := map[string]int{}
	for _, decl := range m.Decls {
		switch x := decl.(type) {
		case ast.FuncDecl:
			if x.Deprecated != nil {
				d.funcs[x.Pos.String()] = &deprecatedDecl{kind: "function", name: written(x.Owner, x.Name), note: x.Deprecated.Note, owner: x.Owner, pos: x.Pos}
			}
		case ast.StructDecl:
			if x.Deprecated != nil && !x.Enum {
				d.types[x.Name] = &deprecatedDecl{kind: "struct", name: written(x.Owner, x.Name), note: x.Deprecated.Note, owner: x.Owner, pos: x.Pos}
			}
		case ast.TypeDecl:
			if x.Deprecated != nil {
				d.types[x.Name] = &deprecatedDecl{kind: "type", name: written(x.Owner, x.Name), note: x.Deprecated.Note, owner: x.Owner, pos: x.Pos}
			}
		case ast.ConstDecl:
			if x.Deprecated != nil {
				d.consts[x.Name] = &deprecatedDecl{kind: "const", name: written(x.Owner, x.Name), note: x.Deprecated.Note, owner: x.Owner, pos: x.Pos}
			}
		case ast.EnumDecl:
			if x.Deprecated != nil {
				d.types[x.Name] = &deprecatedDecl{kind: "enum", name: written(x.Owner, x.Name), note: x.Deprecated.Note, owner: x.Owner, pos: x.Pos}
			}
			for _, variant := range x.Variants {
				variantNames[variant.Name]++
				if variant.Deprecated != nil {
					d.variants[x.Name+"."+variant.Name] = &deprecatedDecl{kind: "variant", name: written(x.Owner, x.Name) + "." + variant.Name, note: variant.Deprecated.Note, owner: x.Owner, pos: variant.Pos}
				}
			}
		}
	}
	for key, decl := range d.variants {
		name := key[strings.IndexByte(key, '.')+1:]
		if variantNames[name] == 1 {
			d.bareVariants[name] = decl
		}
	}
	return d
}

// written is a declaration's name as its module wrote it. An imported
// module's types and constants are compiled under a name of their own
// (`lib_Config` for `Config` in `lib`), which is not the name a reader knows.
func written(owner, name string) string {
	if owner == "" {
		return name
	}
	return strings.TrimPrefix(name, strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(owner)+"_")
}

// warnDeprecations warns at every use of a deprecated declaration from outside
// its module, and at every import of a deprecated module.
func (tc *TypeChecker) warnDeprecations(m ast.Module) {
	for _, imported := range m.DeprecatedImports {
		message := "module " + imported.Module + " is deprecated"
		if imported.Note != "" {
			message += ": " + imported.Note
		}
		tc.warnDeprecated(imported.Pos, message, Note{Position: imported.Decl, Message: "module " + imported.Module + " is declared here"})
	}
	d := collectDeprecations(m)
	if d.empty() {
		return
	}
	for _, decl := range m.Decls {
		w := &deprecationWalk{tc: tc, d: d}
		switch x := decl.(type) {
		case ast.FuncDecl:
			w.owner, w.file = x.Owner, x.Pos.File
			w.typeParams = setOf(x.TypeParams)
			for _, param := range x.Params {
				w.typeString(param.Type, w.place(param.Pos, x.Pos))
			}
			w.typeString(x.ReturnType, x.Pos)
			w.locals = map[string]bool{}
			for _, param := range x.Params {
				w.locals[param.Name] = true
			}
			collectLocals(x.Body, w.locals)
			w.stmts(x.Body, x.Pos)
		case ast.StructDecl:
			if x.Enum {
				continue
			}
			w.owner, w.file = x.Owner, x.Pos.File
			w.typeParams = setOf(x.TypeParams)
			for _, field := range x.Fields {
				w.typeString(field.Type, w.place(field.Pos, x.Pos))
			}
		case ast.EnumDecl:
			w.owner, w.file = x.Owner, x.Pos.File
			w.typeParams = setOf(x.TypeParams)
			for _, variant := range x.Variants {
				for _, field := range variant.Fields {
					w.typeString(field.Type, w.place(field.Pos, w.place(variant.Pos, x.Pos)))
				}
			}
		case ast.ConstDecl:
			w.owner, w.file = x.Owner, x.Pos.File
			w.typeString(x.Type.String(), x.Pos)
			w.expr(x.Value, x.Pos)
		case ast.TypeDecl:
			w.owner, w.file = x.Owner, x.Pos.File
			if x.Type.Name != x.Name {
				w.typeString(x.Type.String(), x.Pos)
			}
		case ast.RouteDecl:
			w.expr(ast.IdentifierExpr{Name: x.Handler}, x.Pos)
		}
	}
}

// warnDeprecated records a deprecation warning with its note, once per place
// and message.
func (tc *TypeChecker) warnDeprecated(at ast.Position, message string, note Note) {
	before := len(tc.warnings)
	tc.warn(at, message)
	if len(tc.warnings) > before {
		added := tc.warnings[len(tc.warnings)-1]
		added.Code = DeprecatedCode
		added.Notes = append(added.Notes, note)
	}
}

// deprecationWalk is the walk over one declaration.
type deprecationWalk struct {
	tc         *TypeChecker
	d          *deprecations
	owner      string
	file       string
	locals     map[string]bool
	typeParams map[string]bool
}

// place is at when it is known, and otherwise the enclosing position.
func (w *deprecationWalk) place(at, enclosing ast.Position) ast.Position {
	if at.Valid() {
		return at
	}
	return enclosing
}

// use warns about one use of a deprecated declaration, unless the use is in
// the declaration's own module.
func (w *deprecationWalk) use(decl *deprecatedDecl, at ast.Position) {
	if decl == nil || !at.Valid() || sameModule(decl.owner, decl.pos.File, w.owner, at.File) {
		return
	}
	message := decl.name + " is deprecated"
	if decl.note != "" {
		message += ": " + decl.note
	}
	w.tc.warnDeprecated(at, message, Note{Position: decl.pos, Message: decl.name + " is declared here"})
}

// typeString warns about every deprecated type a written type names.
func (w *deprecationWalk) typeString(written string, at ast.Position) {
	for _, name := range typeNames(written) {
		if w.typeParams[name] {
			continue
		}
		w.use(w.d.types[name], at)
	}
}

func (w *deprecationWalk) typeExprs(types []ast.TypeExpr, at ast.Position) {
	for _, t := range types {
		w.typeString(t.String(), at)
	}
}

func (w *deprecationWalk) stmts(stmts []ast.Stmt, enclosing ast.Position) {
	for _, stmt := range stmts {
		w.stmt(stmt, enclosing)
	}
}

func (w *deprecationWalk) stmt(stmt ast.Stmt, enclosing ast.Position) {
	if stmt == nil {
		return
	}
	at := w.place(stmt.Position(), enclosing)
	switch s := stmt.(type) {
	case ast.BlockStmt:
		w.stmts(s.Stmts, at)
	case ast.LetStmt:
		w.typeString(s.Type.String(), at)
		w.expr(s.Value, at)
	case ast.AssignStmt:
		w.expr(s.Target, at)
		w.expr(s.Value, at)
	case ast.ExprStmt:
		w.expr(s.Expr, at)
	case ast.ReturnStmt:
		w.expr(s.Value, at)
	case ast.IfStmt:
		w.expr(s.Condition, at)
		w.stmts(s.Then, at)
		w.stmts(s.Else, at)
	case ast.WhileStmt:
		w.expr(s.Condition, at)
		w.stmts(s.Body, at)
	case ast.ForStmt:
		w.expr(s.Collection, at)
		w.expr(s.Step, at)
		w.stmts(s.Body, at)
	case ast.MatchStmt:
		w.expr(s.Value, at)
		for _, arm := range s.Arms {
			armAt := w.place(arm.Pos, at)
			if !arm.Wildcard {
				w.pattern(arm.Pattern, armAt)
			}
			w.stmts(arm.Body, armAt)
		}
	}
}

// pattern warns about the variants and enums a pattern names. The names it
// binds are not uses of anything.
func (w *deprecationWalk) pattern(pattern ast.Expr, enclosing ast.Position) {
	if pattern == nil {
		return
	}
	at := w.place(pattern.Position(), enclosing)
	switch p := pattern.(type) {
	case ast.IdentifierExpr:
		if !w.locals[p.Name] {
			w.use(w.d.bareVariants[p.Name], at)
			w.use(w.constNamed(p.Name), at)
		}
	case ast.MemberExpr:
		w.variant(p, at)
	case ast.CallExpr:
		switch callee := p.Callee.(type) {
		case ast.IdentifierExpr:
			w.use(w.d.bareVariants[callee.Name], at)
		case ast.MemberExpr:
			w.variant(callee, at)
		}
		for _, arg := range p.Args {
			w.pattern(arg, at)
		}
	case ast.StructLiteralExpr:
		w.structLiteralType(p.Type, at)
		for _, value := range p.Values {
			w.pattern(value, at)
		}
	default:
		w.expr(pattern, at)
	}
}

// variant is `Enum.Variant`, or `Option[u32].None`: the enum and the variant.
func (w *deprecationWalk) variant(member ast.MemberExpr, at ast.Position) bool {
	var enumName string
	switch object := member.Object.(type) {
	case ast.IdentifierExpr:
		enumName = object.Name
	case ast.IndexExpr:
		if ident, ok := object.Object.(ast.IdentifierExpr); ok {
			enumName = ident.Name
		}
	}
	if enumName == "" || w.locals[enumName] || w.tc.enums[enumName] == nil {
		return false
	}
	w.use(w.d.types[enumName], at)
	w.use(w.d.variants[enumName+"."+member.Name], at)
	w.typeExprs(member.TypeArgs, at)
	return true
}

// structLiteralType is the type a struct literal builds: a struct, or the
// variant of an enum written `Shape.Rect { ... }`.
func (w *deprecationWalk) structLiteralType(t ast.TypeExpr, at ast.Position) {
	name := t.Name
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		enumName := name[:dot]
		if i := strings.LastIndexByte(enumName, '.'); i >= 0 {
			enumName = enumName[i+1:]
		}
		w.use(w.d.types[enumName], at)
		w.use(w.d.variants[enumName+"."+name[dot+1:]], at)
	} else {
		w.use(w.d.types[name], at)
	}
	w.typeExprs(t.Args, at)
}

func (w *deprecationWalk) exprs(exprs []ast.Expr, at ast.Position) {
	for _, expr := range exprs {
		w.expr(expr, at)
	}
}

func (w *deprecationWalk) expr(expr ast.Expr, enclosing ast.Position) {
	if expr == nil {
		return
	}
	at := w.place(expr.Position(), enclosing)
	switch e := expr.(type) {
	case ast.IdentifierExpr:
		w.identifier(e.Name, at)
	case ast.UnaryExpr:
		w.expr(e.Value, at)
	case ast.BinaryExpr:
		w.expr(e.Left, at)
		w.expr(e.Right, at)
	case ast.CallExpr:
		w.expr(e.Callee, at)
		w.typeExprs(e.TypeArgs, at)
		w.exprs(e.Args, at)
	case ast.MemberExpr:
		if w.variant(e, at) {
			return
		}
		if object, ok := e.Object.(ast.IdentifierExpr); ok && !w.locals[object.Name] {
			if owner, known := w.tc.moduleAliases[object.Name]; known && owner != "" {
				if entry, found := w.tc.owned[owner][e.Name]; found {
					w.use(w.d.funcs[entry.pos.String()], at)
				}
				return
			}
		}
		w.expr(e.Object, at)
	case ast.IndexExpr:
		w.expr(e.Object, at)
		if len(e.TypeArgs) > 0 {
			w.typeExprs(e.TypeArgs, at)
		} else {
			w.expr(e.Index, at)
		}
	case ast.SliceExpr:
		w.expr(e.Object, at)
		w.expr(e.Low, at)
		w.expr(e.High, at)
	case ast.CastExpr:
		w.typeString(e.Type.String(), at)
		w.expr(e.Value, at)
	case ast.StructLiteralExpr:
		w.structLiteralType(e.Type, at)
		w.exprs(e.Values, at)
	case ast.ArrayLiteralExpr:
		w.exprs(e.Values, at)
		if e.Repeat != nil {
			w.expr(*e.Repeat, at)
		}
	case ast.IfExpr:
		w.expr(e.Condition, at)
		w.expr(e.Then, at)
		w.expr(e.Else, at)
	case ast.FuncLitExpr:
		for _, param := range e.Params {
			w.typeString(param.Type, w.place(param.Pos, at))
		}
		w.typeString(e.ReturnType, at)
		w.stmts(e.Body, at)
	}
}

// identifier is a name used as a value: a local, a constant, a function, or a
// type used as a constructor.
func (w *deprecationWalk) identifier(name string, at ast.Position) {
	if w.locals[name] {
		return
	}
	if decl := w.constNamed(name); decl != nil {
		w.use(decl, at)
		return
	}
	if entry, ok := w.tc.owned[w.owner][name]; ok {
		w.use(w.d.funcs[entry.pos.String()], at)
		return
	}
	if entry, ok := w.tc.funcs[name]; ok {
		w.use(w.d.funcs[entry.pos.String()], at)
		return
	}
	w.use(w.d.types[name], at)
}

// constNamed is the deprecated constant a name is, when the name is the
// program's constant of that name.
func (w *deprecationWalk) constNamed(name string) *deprecatedDecl {
	decl := w.d.consts[name]
	if decl == nil {
		return nil
	}
	if declared, ok := w.tc.moduleConsts[name]; !ok || declared.Pos.String() != decl.pos.String() {
		return nil
	}
	return decl
}

// collectLocals records every name a body binds: its lets, its loop
// variables, the parameters of its closures and the names its patterns bind. A
// local of a function's or a constant's name hides it wherever in the body it
// is declared, which can only leave a use unreported, never report one that is
// not there.
func collectLocals(stmts []ast.Stmt, locals map[string]bool) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case ast.BlockStmt:
			collectLocals(s.Stmts, locals)
		case ast.LetStmt:
			locals[s.Name] = true
		case ast.IfStmt:
			collectLocals(s.Then, locals)
			collectLocals(s.Else, locals)
		case ast.WhileStmt:
			collectLocals(s.Body, locals)
		case ast.ForStmt:
			locals[s.Name] = true
			collectLocals(s.Body, locals)
		case ast.MatchStmt:
			for _, arm := range s.Arms {
				if arm.Pattern != nil {
					patternBindings(arm.Pattern, locals)
				}
				collectLocals(arm.Body, locals)
			}
		}
		visitExprs(stmt, func(expr ast.Expr) {
			if lit, ok := expr.(ast.FuncLitExpr); ok {
				for _, param := range lit.Params {
					locals[param.Name] = true
				}
				collectLocals(lit.Body, locals)
			}
		})
	}
}

// patternBindings records the lower-case names a pattern's values bind.
func patternBindings(pattern ast.Expr, locals map[string]bool) {
	var values []ast.Expr
	switch p := pattern.(type) {
	case ast.CallExpr:
		values = p.Args
	case ast.StructLiteralExpr:
		values = p.Values
	}
	for _, value := range values {
		if ident, ok := value.(ast.IdentifierExpr); ok {
			if ident.Name != "_" {
				locals[ident.Name] = true
			}
			continue
		}
		patternBindings(value, locals)
	}
}

// typeNames are the names a written type is made of: `Map[Key, Slice[Old]]`
// is Map, Key, Slice and Old.
func typeNames(written string) []string {
	var names []string
	start := -1
	for i := 0; i <= len(written); i++ {
		identifier := i < len(written) && (written[i] == '_' || written[i] >= 'a' && written[i] <= 'z' ||
			written[i] >= 'A' && written[i] <= 'Z' || start >= 0 && written[i] >= '0' && written[i] <= '9')
		if identifier {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			names = append(names, written[start:i])
			start = -1
		}
	}
	return names
}

func setOf(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// sameModule reports whether a use is in the module that declares what it
// uses: the same owner, and for the program's own sources — which have none —
// the same module.yml.
func sameModule(declOwner, declFile, useOwner, useFile string) bool {
	if declOwner != useOwner {
		return false
	}
	if declOwner != "" {
		return true
	}
	return moduleRoot(declFile) == moduleRoot(useFile)
}

var moduleRoots sync.Map

// moduleRoot is the folder of the nearest module.yml above a file, or the
// file's own folder when there is none.
func moduleRoot(file string) string {
	if file == "" {
		return ""
	}
	dir := filepath.Dir(file)
	if cached, ok := moduleRoots.Load(dir); ok {
		return cached.(string)
	}
	root := dir
	for candidate := dir; ; {
		if _, err := os.Stat(filepath.Join(candidate, "module.yml")); err == nil {
			root = candidate
			break
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}
	moduleRoots.Store(dir, root)
	return root
}
