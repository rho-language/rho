//
// @file enums.go
// @brief Enums: a value that is one of a fixed set of variants.
//
// An enum is declared once and comes in two shapes, decided by whether any
// variant carries data.
//
// A plain enum is a number. Each variant has one — written, `A = 1`, or counted
// on from the one before — and the enum is an integer of the narrowest width
// that holds them all. That is what a C header and a wire protocol agree on, so
// a plain enum crosses either as itself. To the checker it is still a type of
// its own: a Color is not a number and not a Shape, and it becomes one only
// through a conversion (`u32(c)` one way, `Color(n)` the other, which checks).
//
// An enum whose variants carry data is a tag and the values of every variant:
// in C the variants share their storage in a union, in Go they sit side by side
// in a struct (ir.FieldIR.Variant). Either way it is a value — copied when it is
// assigned, passed or returned — and the backends agree on what it holds
// because both are built from the same helpers: a constructor per variant, and
// for the standard Option and Result the accessors code written before they
// were enums reads (`.is_some`, `.value`, `.is_ok`, `.ok`, `.err`). Those two
// also keep the layout their structs had — the flag as the tag, each value
// under its old field name — so Go code a port carries still reads them.
//
// The checker lays a data enum out as a struct (StructDecl.Enum) and lets the
// struct machinery do what it already does — specialise a generic one, order
// the declarations, follow the strings in it for the C arena — and keeps what
// only an enum has here: its variants, how one is built, and how a match takes
// one apart.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"fmt"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// enumVariant is one variant of an enum, as the checker reads it.
type enumVariant struct {
	Name string
	// Tag is what a value of the variant holds in its tag: the variant's index
	// for an enum that carries data, its number for a plain one.
	Tag int
	// Fields are the values it carries. A tuple-like variant's are named f0,
	// f1, ..., which is the name each backend stores them under.
	Fields []ast.Field
	// Named says the fields were declared with names, `Rect { w: i32 }`.
	Named bool
	Pos   ast.Position
}

// enumInfo is one enum the program declares.
type enumInfo struct {
	Decl  ast.EnumDecl
	Name  string
	Owner string
	// Plain says no variant carries data: the enum is an integer.
	Plain bool
	// Params and Defaults are a generic enum's type parameters.
	Params   []string
	Defaults map[string]string
	Variants []enumVariant
	// Kind is the integer a plain enum is, or the width of a data enum's tag.
	Kind ir.TypeKind
	// TagField is the field a data enum's tag is stored in: `tag`, or for the
	// standard Option and Result the flag their structs had (`is_some`,
	// `is_ok`), a bool that is false for the first variant and true for the
	// second.
	TagField string
	// Stored names, for Option and Result, the field each variant's one value
	// is kept in by its own name (`value`, `ok`, `err`), so their layout in Go
	// is the one their structs had and Go code written against it still reads.
	Stored map[string]string
}

// tagIsFlag says the tag is the bool an Option's or a Result's struct had.
func (info *enumInfo) tagIsFlag() bool { return info.TagField != "" && info.TagField != "tag" }

// tagValue is what a value of a variant holds in its tag, as an expression.
func (info *enumInfo) tagValue(variant *enumVariant) string {
	if info.tagIsFlag() {
		if variant.Tag == 1 {
			return "true"
		}
		return "false"
	}
	return strconv.Itoa(variant.Tag)
}

// payloadField is where a variant's value is kept, relative to a value of the
// enum: its own name for Option and Result, `as.<Variant>.<field>` otherwise.
func (info *enumInfo) payloadField(variant *enumVariant, field ast.Field) string {
	if stored := info.Stored[variant.Name]; stored != "" {
		return stored
	}
	return "as." + variant.Name + "." + field.Name
}

// compatLayout is the layout the standard Option and Result keep from when
// they were structs: the flag that was their first field as the tag, and each
// value under the field name it had. Only an enum of exactly their shape has it.
func compatLayout(decl ast.EnumDecl) (string, map[string]string, bool) {
	if len(decl.Variants) != 2 {
		return "", nil, false
	}
	first, second := decl.Variants[0], decl.Variants[1]
	switch {
	case decl.Name == "Option" && len(decl.TypeParams) == 1 &&
		first.Name == "None" && len(first.Fields) == 0 &&
		second.Name == "Some" && len(second.Fields) == 1 && !second.Named:
		return "is_some", map[string]string{"Some": "value"}, true
	case decl.Name == "Result" && len(decl.TypeParams) == 2 &&
		first.Name == "Err" && len(first.Fields) == 1 && !first.Named &&
		second.Name == "Ok" && len(second.Fields) == 1 && !second.Named:
		return "is_ok", map[string]string{"Err": "err", "Ok": "ok"}, true
	}
	return "", nil, false
}

// variant is the variant of that name, or nil.
func (info *enumInfo) variant(name string) *enumVariant {
	for i := range info.Variants {
		if info.Variants[i].Name == name {
			return &info.Variants[i]
		}
	}
	return nil
}

// variantNames lists the variants as a program writes them, `Shape.Circle`.
func (info *enumInfo) variantNames(only map[string]bool) []string {
	out := []string{}
	for _, variant := range info.Variants {
		if only == nil || only[variant.Name] {
			out = append(out, info.Name+"."+variant.Name)
		}
	}
	return out
}

// newEnumInfo reads a declaration.
func newEnumInfo(decl ast.EnumDecl) *enumInfo {
	info := &enumInfo{
		Decl:     decl,
		Name:     decl.Name,
		Owner:    decl.Owner,
		Plain:    !decl.CarriesData(),
		Params:   decl.TypeParams,
		Defaults: decl.TypeParamDefaults,
	}
	values := make([]int, 0, len(decl.Variants))
	for index, written := range decl.Variants {
		variant := enumVariant{Name: written.Name, Tag: index, Named: written.Named, Pos: written.Pos}
		if info.Plain {
			variant.Tag = written.Value
			values = append(values, written.Value)
		}
		for i, field := range written.Fields {
			if !written.Named {
				field.Name = "f" + strconv.Itoa(i)
			}
			variant.Fields = append(variant.Fields, field)
		}
		info.Variants = append(info.Variants, variant)
	}
	if info.Plain {
		info.Kind = plainEnumKind(values)
	} else if len(decl.Variants) <= 256 {
		info.Kind = ir.TypeU8
	} else {
		info.Kind = ir.TypeU16
	}
	info.TagField = "tag"
	if flag, stored, ok := compatLayout(decl); ok {
		info.TagField, info.Stored = flag, stored
	}
	return info
}

// plainEnumKind is the narrowest integer that holds every value: unsigned when
// none is negative, which is what a tag or a protocol field usually is.
func plainEnumKind(values []int) ir.TypeKind {
	low, high := 0, 0
	for i, value := range values {
		if i == 0 || value < low {
			low = value
		}
		if i == 0 || value > high {
			high = value
		}
	}
	if low >= 0 {
		switch {
		case high <= 0xFF:
			return ir.TypeU8
		case high <= 0xFFFF:
			return ir.TypeU16
		case high <= 0xFFFFFFFF:
			return ir.TypeU32
		}
		return ir.TypeU64
	}
	switch {
	case low >= -128 && high <= 127:
		return ir.TypeI8
	case low >= -32768 && high <= 32767:
		return ir.TypeI16
	case low >= -2147483648 && high <= 2147483647:
		return ir.TypeI32
	}
	return ir.TypeI64
}

// kindName is the Rho name of an integer kind.
func kindName(kind ir.TypeKind) string {
	switch kind {
	case ir.TypeU8:
		return "u8"
	case ir.TypeU16:
		return "u16"
	case ir.TypeU32:
		return "u32"
	case ir.TypeU64:
		return "u64"
	case ir.TypeI8:
		return "i8"
	case ir.TypeI16:
		return "i16"
	case ir.TypeI32:
		return "i32"
	}
	return "i64"
}

// layout is the struct a data enum is stored as: its tag, then the values of
// each variant, each field marked with the variant it belongs to. A field's
// name carries the variant too, so two variants' `f0` are two fields to the
// checker's tables; a backend is handed the plain name and the variant apart.
func (info *enumInfo) layout() ast.StructDecl {
	tag := kindName(info.Kind)
	if info.tagIsFlag() {
		tag = "bool"
	}
	fields := []ast.Field{{Name: info.TagField, Type: tag, TypeExpr: ast.TypeExpr{Name: tag}}}
	variants := info.Variants
	if info.Name == "Result" && len(info.Stored) > 0 {
		// The struct had `ok` before `err`, and its fields keep their order.
		variants = []enumVariant{info.Variants[1], info.Variants[0]}
	}
	for _, variant := range variants {
		for _, field := range variant.Fields {
			stored := field
			if name := info.Stored[variant.Name]; name != "" {
				stored.Name = name
				stored.Shared = true
				fields = append(fields, stored)
				continue
			}
			stored.Name = variant.Name + "." + field.Name
			stored.Variant = variant.Name
			fields = append(fields, stored)
		}
	}
	decl := ast.StructDecl{
		Name:              info.Name,
		Owner:             info.Owner,
		Fields:            fields,
		TypeParams:        info.Params,
		TypeParamDefaults: info.Defaults,
		Enum:              true,
	}
	decl.Pos = info.Decl.Pos
	return decl
}

// storedField is how a field of an enum's layout is handed to a backend: its
// own name, and the variant it belongs to.
func storedField(field ast.Field) (string, string) {
	if field.Variant == "" {
		return field.Name, ""
	}
	return strings.TrimPrefix(field.Name, field.Variant+"."), field.Variant
}

// expandEnums records the enums a module declares and adds the layout of each
// that carries data, which the struct passes then register, specialise and
// emit. A plain enum is registered as the integer type it is.
//
// It is called by CheckModule and ToIR alike, on the module each is given; the
// declarations are added to a copy, so the caller's module is not changed.
func (tc *TypeChecker) expandEnums(m ast.Module) ast.Module {
	if tc.enums == nil {
		tc.enums = map[string]*enumInfo{}
	}
	decls := make([]ast.Decl, 0, len(m.Decls))
	for _, decl := range m.Decls {
		decls = append(decls, decl)
		enum, ok := decl.(ast.EnumDecl)
		if !ok {
			continue
		}
		info := newEnumInfo(enum)
		tc.enums[enum.Name] = info
		if info.Plain {
			tc.types[enum.Name] = ir.Type{Kind: info.Kind, Name: enum.Name, Enum: true}
			continue
		}
		decls = append(decls, info.layout())
	}
	m.Decls = decls
	return m
}

/* -------------------------------------------------------------------------- */
/*  Declarations                                                              */
/* -------------------------------------------------------------------------- */

// checkEnum validates an enum declaration.
func (tc *TypeChecker) checkEnum(decl ast.EnumDecl) error {
	info := tc.enums[decl.Name]
	if info == nil {
		return nil
	}
	if primitiveTypeName(decl.Name) {
		return locate(fmt.Errorf("enum %s redeclares the built-in type %s; choose another name", decl.Name, decl.Name), decl.Pos, "enum "+decl.Name)
	}
	if len(decl.Variants) == 0 {
		return locate(fmt.Errorf("enum %s has no variants: a value of an enum is one of its variants, so it needs at least one", decl.Name), decl.Pos, "enum "+decl.Name)
	}
	seen := map[string]bool{}
	for _, variant := range decl.Variants {
		if seen[variant.Name] {
			return locate(fmt.Errorf("enum %s declares %s twice; give each variant its own name", decl.Name, variant.Name), variant.Pos, "")
		}
		seen[variant.Name] = true
	}
	if info.Plain {
		if len(decl.TypeParams) > 0 {
			return locate(fmt.Errorf("enum %s has type parameters but no variant carries a value, so there is nothing for them to be; remove [%s], or give a variant a value of that type", decl.Name, strings.Join(decl.TypeParams, ", ")), decl.Pos, "")
		}
		values := map[int]string{}
		for _, variant := range decl.Variants {
			if other, taken := values[variant.Value]; taken {
				return locate(fmt.Errorf("%s.%s is %d, which %s.%s already is; a match could not tell them apart, so give each variant its own number", decl.Name, variant.Name, variant.Value, decl.Name, other), variant.Pos, "")
			}
			values[variant.Value] = variant.Name
		}
		return nil
	}
	carrier := ""
	for _, variant := range decl.Variants {
		if len(variant.Fields) > 0 {
			carrier = variant.Name
			break
		}
	}
	for _, variant := range decl.Variants {
		if variant.Explicit {
			return locate(fmt.Errorf("%s.%s is given the number %d, but %s carries data (in %s.%s), and the variants of an enum that carries data have no numbers; remove `= %d`",
				decl.Name, variant.Name, variant.Value, decl.Name, decl.Name, carrier, variant.Value), variant.Pos, "")
		}
		names := map[string]bool{}
		for _, field := range variant.Fields {
			if variant.Named {
				if names[field.Name] {
					return locate(fmt.Errorf("%s.%s declares the value %s twice", decl.Name, variant.Name, field.Name), field.Pos, "")
				}
				names[field.Name] = true
			}
			if baseTypeName(field.TypeExpr.Name) == decl.Name || field.TypeExpr.Name == decl.Name {
				return locate(fmt.Errorf("%s.%s holds a %s, which would make every %s infinitely large; hold it through a Ptr[%s], or keep an index into an array of them",
					decl.Name, variant.Name, decl.Name, decl.Name, decl.Name), field.Pos, "")
			}
			isParam := false
			for _, param := range decl.TypeParams {
				if field.TypeExpr.Name == param {
					isParam = true
				}
			}
			if isParam {
				continue
			}
			if _, ok := tc.resolveType(field.Type); !ok {
				return locate(fmt.Errorf("unknown type %q in %s.%s: no struct, enum, alias or built-in type has that name; declare it, import the module that does, or check the spelling",
					field.Type, decl.Name, variant.Name), field.Pos, "")
			}
		}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Types                                                                     */
/* -------------------------------------------------------------------------- */

// enumOf is the enum a type is, with the type arguments of a generic one, or
// nil when the type is not an enum.
func (tc *TypeChecker) enumOf(t ir.Type) (*enumInfo, []ir.Type) {
	if len(tc.enums) == 0 || t.ArrayLen > 0 || (t.Kind != ir.TypeStruct && !t.Enum) {
		return nil, nil
	}
	name := t.Name
	if info, ok := tc.enums[name]; ok && len(info.Params) == 0 {
		return info, nil
	}
	if spec, ok := tc.specialisations[name]; ok {
		if info, isEnum := tc.enums[spec.Base]; isEnum && len(info.Params) > 0 {
			return info, tc.expandWithDefaults(info.Params, info.Defaults, spec.Args)
		}
	}
	if open := strings.IndexByte(name, '['); open > 0 && strings.HasSuffix(name, "]") {
		if info, ok := tc.enums[name[:open]]; ok && len(info.Params) > 0 {
			parts := splitTypeArgs(name[open+1 : len(name)-1])
			args := make([]ir.Type, len(parts))
			for i, part := range parts {
				typ, found := tc.resolveType(part)
				if !found {
					typ = ir.Type{Kind: ir.TypeStruct, Name: part}
				}
				args[i] = tc.substituteType(typ, tc.currentTypeArgs)
			}
			if expanded := tc.expandWithDefaults(info.Params, info.Defaults, args); expanded != nil {
				args = expanded
			}
			return info, args
		}
	}
	// An optional pointer may reach here by its mangled name alone, from a
	// foreign signature that wrote it so.
	if IsOptionalPointer(name) {
		if info, ok := tc.enums["Option"]; ok && len(info.Params) == 1 {
			payload := npoPayload(name)
			return info, []ir.Type{{Kind: ir.TypePtr, Name: "Ptr[" + payload + "]", Args: []ir.Type{{Kind: ir.TypeStruct, Name: payload}}}}
		}
	}
	return nil, nil
}

// isDataEnum reports whether a type is an enum whose variants carry data.
func (tc *TypeChecker) isDataEnum(t ir.Type) bool {
	info, _ := tc.enumOf(t)
	return info != nil && !info.Plain
}

// enumSubs pairs a generic enum's parameters with its arguments.
func (tc *TypeChecker) enumSubs(info *enumInfo, args []ir.Type) map[string]ir.Type {
	if len(info.Params) == 0 {
		return nil
	}
	return tc.buildSubstitutionFromTypes(info.Params, info.Defaults, args)
}

// payloadTypes are the types of the values a variant carries, in a
// specialisation of the enum. Lowering asks for the types a backend spells —
// a nested application mangled and recorded for expansion — and checking for
// the written form, which records nothing.
func (tc *TypeChecker) payloadTypes(info *enumInfo, args []ir.Type, variant *enumVariant, lowering bool) []ir.Type {
	subs := tc.enumSubs(info, args)
	out := make([]ir.Type, len(variant.Fields))
	for i, field := range variant.Fields {
		written := field.TypeExpr.String()
		if field.TypeExpr.Name == "" {
			written = field.Type
		}
		if lowering {
			out[i] = tc.declaredTypeWith(written, subs)
			continue
		}
		typ := tc.typeFromExpr(field.TypeExpr, info.Params)
		if field.TypeExpr.Name == "" {
			typ = tc.typeFor(field.Type)
		}
		out[i] = tc.substituteType(typ, subs)
	}
	return out
}

// enumType is the type of a value of an enum at the given arguments. Checking
// names a generic one as it is written, `Option[u32]`, which is how the checker
// names every application; lowering resolves it to the specialisation a
// backend is handed, and records it for expansion.
func (tc *TypeChecker) enumType(info *enumInfo, args []ir.Type, lowering bool) ir.Type {
	if len(info.Params) == 0 {
		return tc.types[info.Name]
	}
	names := make([]string, len(args))
	for i, arg := range args {
		names[i] = typeName(arg)
	}
	written := info.Name + "[" + strings.Join(names, ", ") + "]"
	if lowering {
		return tc.declaredTypeWith(written, nil)
	}
	return ir.Type{Kind: ir.TypeStruct, Name: written}
}

/* -------------------------------------------------------------------------- */
/*  Construction                                                              */
/* -------------------------------------------------------------------------- */

// enumConstruct is a variant of an enum written as a value: `Shape.Empty`,
// `Shape.Circle(3)`, `Shape.Rect { 2, 3 }`, `Option[u32].None`.
type enumConstruct struct {
	info     *enumInfo
	variant  string
	typeArgs []ast.TypeExpr
	args     []ast.Expr
	// bare says it was written without values; braces that the values were
	// written in braces.
	bare   bool
	braces bool
	pos    ast.Position
}

func (c enumConstruct) written() string { return c.info.Name + "." + c.variant }

// enumNamed is the enum an expression names, when it names one: `Shape`, or
// `geometry.Shape` for one an imported module declares. A local of that name is
// a value and names nothing.
func (tc *TypeChecker) enumNamed(object ast.Expr, scope map[string]ir.Type) (*enumInfo, bool) {
	if len(tc.enums) == 0 {
		return nil, false
	}
	switch o := object.(type) {
	case ast.IdentifierExpr:
		if _, local := scope[o.Name]; local {
			return nil, false
		}
		if _, constant := tc.moduleConsts[o.Name]; constant {
			return nil, false
		}
		info, ok := tc.enums[o.Name]
		return info, ok
	case ast.MemberExpr:
		module, ok := o.Object.(ast.IdentifierExpr)
		if !ok {
			return nil, false
		}
		if _, local := scope[module.Name]; local {
			return nil, false
		}
		if _, isModule := tc.moduleAliases[module.Name]; !isModule {
			return nil, false
		}
		info, isEnum := tc.enums[o.Name]
		return info, isEnum
	}
	return nil, false
}

// enumConstruction reads an expression as a variant written as a value.
func (tc *TypeChecker) enumConstruction(expr ast.Expr, scope map[string]ir.Type) (enumConstruct, bool) {
	switch e := expr.(type) {
	case ast.MemberExpr:
		info, ok := tc.enumNamed(e.Object, scope)
		if !ok {
			return enumConstruct{}, false
		}
		return enumConstruct{info: info, variant: e.Name, typeArgs: e.TypeArgs, bare: true, pos: e.Pos}, true
	case ast.CallExpr:
		member, ok := e.Callee.(ast.MemberExpr)
		if !ok {
			return enumConstruct{}, false
		}
		info, ok := tc.enumNamed(member.Object, scope)
		if !ok {
			return enumConstruct{}, false
		}
		typeArgs := member.TypeArgs
		if len(e.TypeArgs) > 0 {
			typeArgs = e.TypeArgs
		}
		return enumConstruct{info: info, variant: member.Name, typeArgs: typeArgs, args: e.Args, pos: e.Pos}, true
	case ast.StructLiteralExpr:
		dot := strings.LastIndexByte(e.Type.Name, '.')
		if dot < 0 || len(tc.enums) == 0 {
			return enumConstruct{}, false
		}
		enumName := e.Type.Name[:dot]
		if at := strings.LastIndexByte(enumName, '.'); at >= 0 {
			enumName = enumName[at+1:]
		}
		info, ok := tc.enums[enumName]
		if !ok {
			return enumConstruct{}, false
		}
		return enumConstruct{info: info, variant: e.Type.Name[dot+1:], typeArgs: e.Type.Args, args: e.Values, braces: true, pos: e.Pos}, true
	}
	return enumConstruct{}, false
}

// isEnumConstruction reports whether an expression is a variant written as a
// value, which takes its type arguments from where it is used when it does not
// write them.
func (tc *TypeChecker) isEnumConstruction(expr ast.Expr, scope map[string]ir.Type) bool {
	_, ok := tc.enumConstruction(expr, scope)
	return ok
}

// exprTypeFor types a value written where a type is wanted: a variant of a
// generic enum deduces from it, anything else is typed on its own.
func (tc *TypeChecker) exprTypeFor(expr ast.Expr, scope map[string]ir.Type, want ir.Type) (ir.Type, error) {
	if tc.isEnumConstruction(expr, scope) {
		typ, err := tc.exprTypeExpected(expr, scope, want)
		if err != nil {
			return typ, locate(err, expr.Position(), "")
		}
		return typ, nil
	}
	return tc.exprType(expr, scope)
}

// lowerExprFor is exprTypeFor's lowering.
func (tc *TypeChecker) lowerExprFor(expr ast.Expr, scope map[string]ir.Type, want ir.Type) string {
	if tc.isEnumConstruction(expr, scope) {
		return tc.lowerExprExpected(expr, scope, want)
	}
	return tc.lowerExpr(expr, scope)
}

// constructionVariant is the variant a construction names, checked against
// how it was written.
func (tc *TypeChecker) constructionVariant(c enumConstruct) (*enumVariant, error) {
	variant := c.info.variant(c.variant)
	if variant == nil {
		return nil, locate(fmt.Errorf("%s has no variant %s; its variants are %s", c.info.Name, c.variant, strings.Join(c.info.variantNames(nil), ", ")), c.pos, "")
	}
	switch {
	case c.bare && len(variant.Fields) > 0:
		return nil, locate(fmt.Errorf("%s carries %s (%s); build it with %s(...)", c.written(), plural(len(variant.Fields), "value"), fieldTypesText(variant), c.written()), c.pos, "")
	case !c.bare && len(variant.Fields) == 0:
		return nil, locate(fmt.Errorf("%s carries no value; write it as %s", c.written(), c.written()), c.pos, "")
	case !c.bare && len(c.args) != len(variant.Fields):
		return nil, locate(fmt.Errorf("%s carries %s (%s), and this passes %d", c.written(), plural(len(variant.Fields), "value"), fieldTypesText(variant), len(c.args)), c.pos, "")
	case c.braces && !variant.Named:
		return nil, locate(fmt.Errorf("%s carries unnamed values; build it with %s(...)", c.written(), c.written()), c.pos, "")
	}
	return variant, nil
}

// fieldTypesText spells what a variant carries: `i32, string` or
// `w: i32, h: i32`.
func fieldTypesText(variant *enumVariant) string {
	parts := make([]string, len(variant.Fields))
	for i, field := range variant.Fields {
		if variant.Named {
			parts[i] = field.Name + ": " + field.Type
		} else {
			parts[i] = field.Type
		}
	}
	return strings.Join(parts, ", ")
}

// enumArgs are the type arguments of a construction: those written, or those
// deduced from where it is used and from its values.
func (tc *TypeChecker) enumArgs(c enumConstruct, variant *enumVariant, expected ir.Type, scope map[string]ir.Type) ([]ir.Type, error) {
	info := c.info
	if len(info.Params) == 0 {
		if len(c.typeArgs) > 0 {
			return nil, locate(fmt.Errorf("%s is not generic, so %s takes no type arguments", info.Name, c.written()), c.pos, "")
		}
		return nil, nil
	}
	if len(c.typeArgs) > 0 {
		args := tc.expandWithDefaults(info.Params, info.Defaults, tc.resolveTypeArgs(c.typeArgs))
		if args == nil || len(args) != len(info.Params) {
			return nil, locate(fmt.Errorf("%s takes %s (%s), and %s writes %d", info.Name, plural(len(info.Params), "type argument"), strings.Join(info.Params, ", "), c.written(), len(c.typeArgs)), c.pos, "")
		}
		return args, nil
	}
	subs := map[string]ir.Type{}
	if expected.Kind != ir.TypeInvalid {
		if wanted, args := tc.enumOf(tc.substituteType(expected, tc.currentTypeArgs)); wanted == info {
			for i, param := range info.Params {
				if i < len(args) {
					subs[param] = args[i]
				}
			}
		}
	}
	for i, arg := range c.args {
		if i >= len(variant.Fields) || len(subs) == len(info.Params) {
			break
		}
		actual, err := tc.exprType(arg, scope)
		if err != nil {
			return nil, err
		}
		declared := tc.typeFromExpr(variant.Fields[i].TypeExpr, info.Params)
		tc.unifyTypeArgs(info.Params, declared, actual, subs)
	}
	args := tc.orderedArgs(info.Params, info.Defaults, subs)
	if args == nil {
		missing := tc.undeducedParams(info.Params, info.Defaults, subs)
		return nil, locate(fmt.Errorf("cannot tell which %s %s is (%s unresolved); write it, as in %s[%s].%s, or give the value a declared type",
			info.Name, c.written(), strings.Join(missing, ", "), info.Name, strings.Join(info.Params, ", "), c.variant), c.pos, "")
	}
	return args, nil
}

// enumConstructionType checks a construction and types it.
func (tc *TypeChecker) enumConstructionType(c enumConstruct, scope map[string]ir.Type, expected ir.Type) (ir.Type, error) {
	variant, err := tc.constructionVariant(c)
	if err != nil {
		return ir.Type{}, err
	}
	args, err := tc.enumArgs(c, variant, expected, scope)
	if err != nil {
		return ir.Type{}, err
	}
	payload := tc.payloadTypes(c.info, args, variant, false)
	for i, arg := range c.args {
		actual, err := tc.exprTypeFor(arg, scope, payload[i])
		if err != nil {
			return ir.Type{}, err
		}
		if !assignable(payload[i], actual) {
			what := fmt.Sprintf("value %d", i+1)
			if variant.Named {
				what = variant.Fields[i].Name
			}
			return ir.Type{}, locate(fmt.Errorf("%s of %s has type %s, want %s; %s", what, c.written(), typeName(actual), typeName(payload[i]), conversionHint(actual, payload[i])), arg.Position(), "")
		}
		if err := checkConstantFits(arg, payload[i]); err != nil {
			return ir.Type{}, locate(err, arg.Position(), "")
		}
	}
	return tc.enumType(c.info, args, false), nil
}

// plainValue is a plain enum's variant as an expression both backends read: the
// number, converted to the enum's integer.
func plainValue(info *enumInfo, variant *enumVariant) string {
	return "((" + cTypeForKind(info.Kind) + ")(" + strconv.Itoa(variant.Tag) + "))"
}

// lowerEnumConstruction lowers a construction: a plain variant is its number,
// an optional pointer is the pointer, and anything else is a call to the
// variant's constructor.
func (tc *TypeChecker) lowerEnumConstruction(c enumConstruct, scope map[string]ir.Type, expected ir.Type) string {
	variant := c.info.variant(c.variant)
	if variant == nil {
		return "0"
	}
	if c.info.Plain {
		return plainValue(c.info, variant)
	}
	args, err := tc.enumArgs(c, variant, expected, scope)
	if err != nil {
		args = tc.orderedArgs(c.info.Params, c.info.Defaults, map[string]ir.Type{})
	}
	typ := tc.enumType(c.info, args, true)
	payload := tc.payloadTypes(c.info, args, variant, true)
	values := make([]string, len(c.args))
	for i, arg := range c.args {
		values[i] = tc.converted(tc.lowerExprFor(arg, scope, payload[i]), arg, scope, payload[i])
	}
	if IsOptionalPointer(mangleTypeName(typ.Name)) {
		if len(values) == 1 {
			return values[0]
		}
		return "NULL"
	}
	return tc.enumConstructor(c.info, args, typ, variant) + "(" + strings.Join(values, ", ") + ")"
}

/* -------------------------------------------------------------------------- */
/*  Helpers both backends call                                                */
/* -------------------------------------------------------------------------- */

// enumHelper records a function the lowering calls, once by name, and returns
// the name. The helpers are ordinary functions in the IR, so each backend
// emits them as it emits any other.
func (tc *TypeChecker) enumHelper(fn ir.FuncIR) string {
	if tc.enumHelpers == nil {
		tc.enumHelpers = map[string]ir.FuncIR{}
	}
	if _, done := tc.enumHelpers[fn.CName]; !done {
		tc.enumHelpers[fn.CName] = fn
		tc.enumHelperOrder = append(tc.enumHelperOrder, fn.CName)
	}
	return fn.CName
}

// enumHelperFuncs are the helpers recorded, in the order they were first used.
func (tc *TypeChecker) enumHelperFuncs() []ir.FuncIR {
	out := make([]ir.FuncIR, 0, len(tc.enumHelperOrder))
	for _, name := range tc.enumHelperOrder {
		out = append(out, tc.enumHelpers[name])
	}
	return out
}

// enumHelperName is the name of one of an enum's helpers.
func enumHelperName(typ ir.Type, what string) string {
	return "rho_enum_" + mangleTypeName(typ.Name) + "_" + what
}

// enumConstructor is the function that builds a variant: the value zeroed, its
// tag set and its values stored.
func (tc *TypeChecker) enumConstructor(info *enumInfo, args []ir.Type, typ ir.Type, variant *enumVariant) string {
	name := enumHelperName(typ, variant.Name)
	if tc.enumHelpers != nil {
		if _, done := tc.enumHelpers[name]; done {
			return name
		}
	}
	mangled := mangleTypeName(typ.Name)
	payload := tc.payloadTypes(info, args, variant, true)
	params := make([]ir.ParamIR, len(payload))
	body := []ir.Instr{
		{Op: ir.OpLet, Dest: "rho_e", Type: mangled, TypeName: mangled, Args: []string{"(" + mangled + "){}"}},
		{Op: ir.OpAssign, Dest: "rho_e." + info.TagField, Args: []string{info.tagValue(variant)}},
	}
	for i, field := range variant.Fields {
		param := "p" + strconv.Itoa(i)
		params[i] = ir.ParamIR{Name: param, Type: payload[i]}
		body = append(body, ir.Instr{Op: ir.OpAssign, Dest: "rho_e." + info.payloadField(variant, field), Args: []string{param}})
	}
	body = append(body, ir.Instr{Op: ir.OpReturn, Args: []string{"rho_e"}})
	return tc.enumHelper(ir.FuncIR{Name: name, CName: name, Params: params, ReturnType: ir.Type{Kind: ir.TypeStruct, Name: mangled}, Body: body})
}

// enumGetter is the function that reads a variant's first value, or the zero
// value of its type when the enum holds another variant. It is what the field
// a struct-shaped Option or Result had (`.value`, `.ok`, `.err`) reads now,
// and it answers what that field held: the zero value when it is not set.
func (tc *TypeChecker) enumGetter(info *enumInfo, args []ir.Type, typ ir.Type, variant *enumVariant) string {
	name := enumHelperName(typ, "get_"+variant.Name)
	if tc.enumHelpers != nil {
		if _, done := tc.enumHelpers[name]; done {
			return name
		}
	}
	mangled := mangleTypeName(typ.Name)
	payload := tc.payloadTypes(info, args, variant, true)
	field := variant.Fields[0]
	body := []ir.Instr{
		{Op: ir.OpIf, Condition: tc.enumTagTest(info, typ, "p0", variant), Then: []ir.Instr{
			{Op: ir.OpReturn, Args: []string{"p0." + info.payloadField(variant, field)}},
		}},
		{Op: ir.OpReturn, Args: []string{tc.zeroValueFor(typeName(payload[0]))}},
	}
	return tc.enumHelper(ir.FuncIR{Name: name, CName: name,
		Params:     []ir.ParamIR{{Name: "p0", Type: ir.Type{Kind: ir.TypeStruct, Name: mangled}}},
		ReturnType: payload[0], Body: body})
}

// enumConverter is the function `Color(n)` calls: the variant whose number n
// is, or a stop naming the enum when n is none of them.
func (tc *TypeChecker) enumConverter(info *enumInfo) string {
	typ := tc.types[info.Name]
	name := enumHelperName(typ, "from")
	if tc.enumHelpers != nil {
		if _, done := tc.enumHelpers[name]; done {
			return name
		}
	}
	body := []ir.Instr{}
	for i := range info.Variants {
		variant := &info.Variants[i]
		body = append(body, ir.Instr{Op: ir.OpIf, Condition: "(p0 == " + strconv.Itoa(variant.Tag) + ")", Then: []ir.Instr{
			{Op: ir.OpReturn, Args: []string{plainValue(info, variant)}},
		}})
	}
	body = append(body,
		ir.Instr{Op: ir.OpExpr, Args: []string{"panicf(" + strconv.Quote(info.Name+"(n): n is not a value of "+info.Name) + ")"}},
		ir.Instr{Op: ir.OpReturn, Args: []string{plainValue(info, &info.Variants[0])}})
	return tc.enumHelper(ir.FuncIR{Name: name, CName: name,
		Params:     []ir.ParamIR{{Name: "p0", Type: ir.Type{Kind: ir.TypeI64, Name: "i64"}}},
		ReturnType: typ, Body: body})
}

/* -------------------------------------------------------------------------- */
/*  Option and Result, as they were written before they were enums            */
/* -------------------------------------------------------------------------- */

// compatField is a field the struct-shaped Option or Result had, and what it
// is on the enum: a test of a variant, or that variant's value.
type compatField struct {
	variant string
	test    bool
}

// compatFields are the fields code written against the struct-shaped Option
// and Result reads. They are kept as read-only accessors, so that code keeps
// working unchanged; new code takes the value apart with match.
var compatFields = map[string]map[string]compatField{
	"Option": {
		"is_some": {variant: "Some", test: true},
		"value":   {variant: "Some"},
	},
	"Result": {
		"is_ok": {variant: "Ok", test: true},
		"ok":    {variant: "Ok"},
		"err":   {variant: "Err"},
	},
}

// compatField is the accessor a member names on an Option or a Result, when
// it names one.
func (tc *TypeChecker) compatFieldOf(info *enumInfo, name string) (compatField, *enumVariant, bool) {
	fields, ok := compatFields[info.Name]
	if !ok {
		return compatField{}, nil, false
	}
	field, ok := fields[name]
	if !ok {
		return compatField{}, nil, false
	}
	variant := info.variant(field.variant)
	if variant == nil || (!field.test && len(variant.Fields) == 0) {
		return compatField{}, nil, false
	}
	return field, variant, true
}

// enumMemberType types a member read on a value of a data enum.
func (tc *TypeChecker) enumMemberType(object ir.Type, info *enumInfo, args []ir.Type, name string) (ir.Type, error) {
	if field, variant, ok := tc.compatFieldOf(info, name); ok {
		if field.test {
			return ir.Type{Kind: ir.TypeBool, Name: "bool"}, nil
		}
		return tc.payloadTypes(info, args, variant, false)[0], nil
	}
	hint := fmt.Sprintf("take it apart with match, as in `match v { %s => ... }`", tc.examplePattern(info))
	if _, compat := compatFields[info.Name]; compat {
		names := []string{}
		for field := range compatFields[info.Name] {
			names = append(names, field)
		}
		sortStrings(names)
		hint += "; the fields " + strings.Join(names, ", ") + " are kept for code written before " + info.Name + " was an enum"
	}
	return ir.Type{}, fmt.Errorf("%s is an enum and has no field %q: %s", typeName(object), name, hint)
}

// examplePattern is a pattern a message can show for an enum.
func (tc *TypeChecker) examplePattern(info *enumInfo) string {
	for _, variant := range info.Variants {
		if len(variant.Fields) == 0 {
			continue
		}
		if variant.Named {
			names := make([]string, len(variant.Fields))
			for i, field := range variant.Fields {
				names[i] = field.Name
			}
			return info.Name + "." + variant.Name + " { " + strings.Join(names, ", ") + " }"
		}
		names := make([]string, len(variant.Fields))
		for i := range variant.Fields {
			names[i] = string(rune('a' + i))
		}
		return info.Name + "." + variant.Name + "(" + strings.Join(names, ", ") + ")"
	}
	return info.Name + "." + info.Variants[0].Name
}

// lowerEnumMember lowers a member read on a data enum: an accessor of an
// Option or a Result.
func (tc *TypeChecker) lowerEnumMember(object ast.Expr, objectType ir.Type, info *enumInfo, args []ir.Type, name string, scope map[string]ir.Type) (string, bool) {
	field, variant, ok := tc.compatFieldOf(info, name)
	if !ok {
		return "", false
	}
	receiver := tc.lowerExpr(object, scope)
	if field.test {
		return tc.enumTagTest(info, objectType, receiver, variant), true
	}
	typ := tc.enumType(info, args, true)
	if IsOptionalPointer(mangleTypeName(typ.Name)) {
		return "(" + receiver + ")", true
	}
	return tc.enumGetter(info, args, typ, variant) + "(" + receiver + ")", true
}

// compatLiteral says how `Option[T]{ is_some, value }` and
// `Result[T, E]{ is_ok, ok, err }` — the literals of the struct-shaped Option
// and Result — read: the flag, and the variant it names when true and false.
func (tc *TypeChecker) compatLiteral(info *enumInfo) (flag string, variants []string, ok bool) {
	switch info.Name {
	case "Option":
		return "is_some", []string{"Some", "None"}, info.variant("Some") != nil && info.variant("None") != nil
	case "Result":
		return "is_ok", []string{"Ok", "Err"}, info.variant("Ok") != nil && info.variant("Err") != nil
	}
	return "", nil, false
}

// lowerCompatLiteral lowers such a literal to the function that builds the
// variant its flag names.
func (tc *TypeChecker) lowerCompatLiteral(info *enumInfo, args []ir.Type, values []string) string {
	typ := tc.enumType(info, args, true)
	mangled := mangleTypeName(typ.Name)
	name := enumHelperName(typ, "literal")
	if tc.enumHelpers == nil || tc.enumHelpers[name].CName == "" {
		_, names, _ := tc.compatLiteral(info)
		yes, no := info.variant(names[0]), info.variant(names[1])
		params := []ir.ParamIR{{Name: "p0", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}}}
		yesPayload := tc.payloadTypes(info, args, yes, true)
		params = append(params, ir.ParamIR{Name: "p1", Type: yesPayload[0]})
		yesCall := tc.enumConstructor(info, args, typ, yes) + "(p1)"
		noCall := tc.enumConstructor(info, args, typ, no) + "()"
		if len(no.Fields) > 0 {
			noPayload := tc.payloadTypes(info, args, no, true)
			params = append(params, ir.ParamIR{Name: "p2", Type: noPayload[0]})
			noCall = tc.enumConstructor(info, args, typ, no) + "(p2)"
		}
		body := []ir.Instr{
			{Op: ir.OpIf, Condition: "p0", Then: []ir.Instr{{Op: ir.OpReturn, Args: []string{yesCall}}}},
			{Op: ir.OpReturn, Args: []string{noCall}},
		}
		tc.enumHelper(ir.FuncIR{Name: name, CName: name, Params: params, ReturnType: ir.Type{Kind: ir.TypeStruct, Name: mangled}, Body: body})
	}
	want := len(tc.enumHelpers[name].Params)
	for len(values) < want {
		values = append(values, tc.zeroValueFor(typeName(tc.enumHelpers[name].Params[len(values)].Type)))
	}
	return name + "(" + strings.Join(values[:want], ", ") + ")"
}

/* -------------------------------------------------------------------------- */
/*  Operators and conversions                                                 */
/* -------------------------------------------------------------------------- */

// enumOperands refuses what an operator cannot do with an enum: compare two
// values of an enum that carries data, order or compute with a plain one, or
// compare two different enums.
func (tc *TypeChecker) enumOperands(expr ast.BinaryExpr, left, right ir.Type) error {
	leftInfo, _ := tc.enumOf(left)
	rightInfo, _ := tc.enumOf(right)
	if leftInfo == nil && rightInfo == nil {
		return nil
	}
	info := leftInfo
	if info == nil {
		info = rightInfo
	}
	switch expr.Op {
	case "==", "!=":
		if !info.Plain {
			return locate(fmt.Errorf("cannot compare %s values with %s: %s carries data, and a value is compared by taking it apart; test the variant with match or `if let`, as in `if let %s = v { ... }`",
				info.Name, expr.Op, info.Name, tc.examplePattern(info)), expr.Pos, "")
		}
		if leftInfo != rightInfo {
			other := right
			if leftInfo != info {
				other = left
			}
			return locate(fmt.Errorf("cannot compare %s with %s using %s; %s", article(info.Name), article(typeName(other)), expr.Op, conversionHint(other, tc.types[info.Name])), expr.Pos, "")
		}
		return nil
	case "<", "<=", ">", ">=":
		return locate(fmt.Errorf("cannot order %s values with %s: an enum is one of its variants, not a quantity; compare their numbers, as in u32(a) %s u32(b), if that is what is meant", info.Name, expr.Op, expr.Op), expr.Pos, "")
	}
	return locate(fmt.Errorf("operator %s does not apply to %s: an enum is one of its variants, not a number; convert it with i32(...) to compute with its number", expr.Op, info.Name), expr.Pos, "")
}

/* -------------------------------------------------------------------------- */
/*  Patterns                                                                  */
/* -------------------------------------------------------------------------- */

// patternBinding is a name a pattern binds, and the type of what it binds.
type patternBinding struct {
	name string
	typ  ir.Type
	pos  ast.Position
}

// variantPattern is a pattern that names a variant, read apart: the enum it
// names (nil when unqualified, `Circle(r)` against a Shape), the variant, and
// the values written inside it.
type variantPattern struct {
	info     *enumInfo
	variant  string
	typeArgs []ast.TypeExpr
	values   []ast.Expr
	bare     bool
	braces   bool
	pos      ast.Position
}

// readVariantPattern reads a pattern as a variant, when it is written as one.
func (tc *TypeChecker) readVariantPattern(pattern ast.Expr, scope map[string]ir.Type) (variantPattern, bool) {
	switch p := pattern.(type) {
	case ast.IdentifierExpr:
		return variantPattern{variant: p.Name, bare: true, pos: p.Pos}, true
	case ast.MemberExpr:
		info, ok := tc.enumNamed(p.Object, scope)
		if !ok {
			return variantPattern{}, false
		}
		return variantPattern{info: info, variant: p.Name, typeArgs: p.TypeArgs, bare: true, pos: p.Pos}, true
	case ast.CallExpr:
		switch callee := p.Callee.(type) {
		case ast.IdentifierExpr:
			return variantPattern{variant: callee.Name, values: p.Args, pos: p.Pos}, true
		case ast.MemberExpr:
			info, ok := tc.enumNamed(callee.Object, scope)
			if !ok {
				return variantPattern{}, false
			}
			return variantPattern{info: info, variant: callee.Name, typeArgs: callee.TypeArgs, values: p.Args, pos: p.Pos}, true
		}
	case ast.StructLiteralExpr:
		name := p.Type.Name
		vp := variantPattern{values: p.Values, braces: true, pos: p.Pos}
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			enumName := name[:dot]
			if at := strings.LastIndexByte(enumName, '.'); at >= 0 {
				enumName = enumName[at+1:]
			}
			info, ok := tc.enums[enumName]
			if !ok {
				return variantPattern{}, false
			}
			vp.info, vp.variant = info, name[dot+1:]
			return vp, true
		}
		vp.variant = name
		return vp, true
	}
	return variantPattern{}, false
}

// checkPattern checks a pattern against a value of a type and collects the
// names it binds. It reports the variant the pattern tests ("" for one that
// matches any value) and whether it matches every value of that variant.
//
// top says the pattern is a whole arm's, where a bare name that is no variant
// is a mistake rather than a binding.
func (tc *TypeChecker) checkPattern(pattern ast.Expr, valueType ir.Type, scope map[string]ir.Type, top bool, bindings *[]patternBinding) (string, bool, error) {
	info, args := tc.enumOf(valueType)
	if ident, ok := pattern.(ast.IdentifierExpr); ok {
		if ident.Name == "_" {
			return "", true, nil
		}
		isVariant := info != nil && info.variant(ident.Name) != nil
		if !isVariant && !top {
			*bindings = append(*bindings, patternBinding{name: ident.Name, typ: valueType, pos: ident.Pos})
			return "", true, nil
		}
	}
	if info == nil {
		// A value that is no enum is compared: a literal, or a constant.
		patternType, err := tc.exprType(pattern, scope)
		if err != nil {
			return "", false, err
		}
		if !assignable(valueType, patternType) {
			return "", false, locate(fmt.Errorf("this pattern is %s and cannot match %s; %s", article(typeName(patternType)), article(typeName(valueType)), conversionHint(patternType, valueType)), pattern.Position(), "")
		}
		return "", false, nil
	}
	vp, ok := tc.readVariantPattern(pattern, scope)
	if !ok {
		return "", false, locate(fmt.Errorf("this pattern cannot match %s: write one of its variants, as in %s, or `_` for any", article(typeName(valueType)), tc.examplePattern(info)), pattern.Position(), "")
	}
	if vp.info != nil && vp.info != info {
		return "", false, locate(fmt.Errorf("%s.%s is %s, and the value matched is %s", vp.info.Name, vp.variant, article(vp.info.Name), article(typeName(valueType))), vp.pos, "")
	}
	variant := info.variant(vp.variant)
	if variant == nil {
		return "", false, locate(fmt.Errorf("%s has no variant %s; its variants are %s", info.Name, vp.variant, strings.Join(info.variantNames(nil), ", ")), vp.pos, "")
	}
	written := info.Name + "." + variant.Name
	if vp.bare {
		return variant.Name, true, nil
	}
	if len(variant.Fields) == 0 {
		return "", false, locate(fmt.Errorf("%s carries no value, so there is nothing to take apart; write %s", written, written), vp.pos, "")
	}
	payload := tc.payloadTypes(info, args, variant, false)
	if vp.braces {
		if !variant.Named {
			return "", false, locate(fmt.Errorf("%s carries unnamed values; take it apart by position, as in %s(a)", written, written), vp.pos, "")
		}
		for _, value := range vp.values {
			ident, isName := value.(ast.IdentifierExpr)
			if !isName {
				return "", false, locate(fmt.Errorf("a pattern in braces names the values it binds, as in %s { %s }; take values apart by position to compare or nest", written, variant.Fields[0].Name), value.Position(), "")
			}
			found := -1
			for i, field := range variant.Fields {
				if field.Name == ident.Name {
					found = i
				}
			}
			if found < 0 {
				return "", false, locate(fmt.Errorf("%s has no value named %s; it carries %s", written, ident.Name, fieldTypesText(variant)), ident.Pos, "")
			}
			*bindings = append(*bindings, patternBinding{name: ident.Name, typ: payload[found], pos: ident.Pos})
		}
		return variant.Name, true, nil
	}
	if len(vp.values) != len(variant.Fields) {
		return "", false, locate(fmt.Errorf("%s carries %s (%s), and this pattern takes it apart into %d; write one name, literal or `_` for each", written, plural(len(variant.Fields), "value"), fieldTypesText(variant), len(vp.values)), vp.pos, "")
	}
	irrefutable := true
	for i, value := range vp.values {
		inner, whole, err := tc.checkPattern(value, payload[i], scope, false, bindings)
		if err != nil {
			return "", false, err
		}
		// A nested variant matches every value only when it is the only
		// variant its enum has.
		if innerInfo, _ := tc.enumOf(payload[i]); inner != "" && innerInfo != nil && len(innerInfo.Variants) > 1 {
			whole = false
		}
		if !whole {
			irrefutable = false
		}
	}
	return variant.Name, irrefutable, nil
}

// patternBindings are the names a match arm binds, quietly: for a reader that
// only needs the scope of the arm's body.
func (tc *TypeChecker) patternBindings(pattern ast.Expr, valueType ir.Type, scope map[string]ir.Type) []patternBinding {
	if info, _ := tc.enumOf(valueType); info == nil {
		return nil
	}
	bindings := []patternBinding{}
	if _, _, err := tc.checkPattern(pattern, valueType, scope, true, &bindings); err != nil {
		return nil
	}
	return bindings
}

// warn records a warning about a place, while a module is being checked.
func (tc *TypeChecker) warn(at ast.Position, message string) {
	if tc.collect == nil {
		return
	}
	key := at.String() + "\x00" + message
	for _, existing := range tc.warnings {
		if existing.Position.String()+"\x00"+existing.Message == key {
			return
		}
	}
	tc.warnings = append(tc.warnings, &Error{Position: at, Message: message})
}

// Warnings are what checking a module found worth saying that does not stop
// it compiling: an arm of a match that can never run, say.
func (tc *TypeChecker) Warnings() []*Error {
	return tc.warnings
}

// checkEnumMatch checks a match on a value of an enum: each arm's pattern,
// each body with the names its pattern binds, that every value is handled, and
// which arms can never run. It reports whether the match always leaves the
// block.
func (tc *TypeChecker) checkEnumMatch(stmt ast.MatchStmt, info *enumInfo, valueType ir.Type, scope map[string]ir.Type, result ir.Type, inLoop bool) (bool, error) {
	terminates := true
	types := []ir.Type{valueType}
	rows := [][]spacePattern{}
	for _, arm := range stmt.Arms {
		armScope := cloneScope(scope)
		at := arm.Pos
		row := []spacePattern{wildPattern}
		if !arm.Wildcard {
			if !at.Valid() {
				at = arm.Pattern.Position()
			}
			bindings := []patternBinding{}
			if _, _, err := tc.checkPattern(arm.Pattern, valueType, scope, true, &bindings); err != nil {
				return false, locate(err, arm.Pattern.Position(), "")
			}
			bound := map[string]bool{}
			for _, binding := range bindings {
				if bound[binding.name] {
					return false, locate(fmt.Errorf("this pattern binds %s twice; give each value its own name", binding.name), binding.pos, "")
				}
				bound[binding.name] = true
				if backendReservedName(binding.name) {
					return false, locate(fmt.Errorf("binding %q: %s", binding.name, backendReservedReason(binding.name)), binding.pos, "")
				}
				armScope[binding.name] = binding.typ
			}
			row = []spacePattern{tc.toSpace(arm.Pattern, valueType, scope)}
		}
		if !(stmt.IfLet && arm.Wildcard) && !tc.useful(rows, types, row, 0) {
			if arm.Wildcard {
				tc.warn(at, fmt.Sprintf("unreachable arm: the arms above handle every value of %s, so `_` never runs; remove it", info.Name))
			} else {
				tc.warn(at, "unreachable arm: the arms above already match every value this one would, so it never runs; remove it or move it above them")
			}
		}
		rows = append(rows, row)
		ends, err := tc.checkStatements(arm.Body, armScope, result, inLoop)
		if err != nil {
			return false, err
		}
		if !ends {
			terminates = false
		}
	}
	exhaustive := !tc.useful(rows, types, []spacePattern{wildPattern}, 0)
	if !exhaustive && !stmt.IfLet {
		unhandled, unnamed := tc.unhandledVariants(info, valueType, rows)
		parts := []string{}
		missing := []string{}
		partial := []string{}
		for _, name := range unhandled {
			if unnamed[name] {
				missing = append(missing, info.Name+"."+name)
			} else {
				partial = append(partial, info.Name+"."+name)
			}
		}
		if len(missing) == 1 {
			parts = append(parts, missing[0]+" is missing")
		} else if len(missing) > 1 {
			parts = append(parts, joinNames(missing)+" are missing")
		}
		if len(partial) > 0 {
			parts = append(parts, "not every "+joinNames(partial)+" is matched")
		}
		return false, locate(fmt.Errorf("this match does not handle every variant of %s: %s; add an arm for each, or a `_ =>` arm for the rest", info.Name, strings.Join(parts, ", and ")), stmt.Pos, "")
	}
	return exhaustive && terminates, nil
}

// joinNames is a list as a sentence writes it: `a`, `a and b`, `a, b and c`.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

/* -------------------------------------------------------------------------- */
/*  Lowering a match                                                          */
/* -------------------------------------------------------------------------- */

// loweredBinding is a name a pattern binds, as the expression it stands for.
type loweredBinding struct {
	name   string
	access string
	typ    ir.Type
}

// enumTagTest is the test that a value is a variant.
func (tc *TypeChecker) enumTagTest(info *enumInfo, valueType ir.Type, access string, variant *enumVariant) string {
	if info.Plain {
		return "(" + access + " == " + plainValue(info, variant) + ")"
	}
	if IsOptionalPointer(mangleTypeName(valueType.Name)) {
		if len(variant.Fields) > 0 {
			return "((" + access + ") != NULL)"
		}
		return "((" + access + ") == NULL)"
	}
	if info.tagIsFlag() {
		if variant.Tag == 1 {
			return "((" + access + ")." + info.TagField + ")"
		}
		return "(!(" + access + ")." + info.TagField + ")"
	}
	return "((" + access + ")." + info.TagField + " == " + strconv.Itoa(variant.Tag) + ")"
}

// enumPayload is where a variant's value is in a value of the enum.
func enumPayload(info *enumInfo, valueType ir.Type, access string, variant *enumVariant, field ast.Field) string {
	if IsOptionalPointer(mangleTypeName(valueType.Name)) {
		return access
	}
	return "(" + access + ")." + info.payloadField(variant, field)
}

// lowerPattern lowers a pattern against the value an access expression reads:
// the tests it makes, in the order they are safe to make — a value of a
// variant is read only after its tag is tested — and the names it binds.
func (tc *TypeChecker) lowerPattern(pattern ast.Expr, access string, valueType ir.Type, scope map[string]ir.Type, tests *[]string, bindings *[]loweredBinding) {
	info, args := tc.enumOf(valueType)
	if ident, ok := pattern.(ast.IdentifierExpr); ok {
		if ident.Name == "_" {
			return
		}
		if info == nil || info.variant(ident.Name) == nil {
			*bindings = append(*bindings, loweredBinding{name: ident.Name, access: access, typ: valueType})
			return
		}
	}
	if info == nil {
		// A literal or a constant, compared the way `==` written by hand is:
		// a string with the runtime's string equality.
		tc.lowerSeq++
		name := "rho_pat_" + strconv.Itoa(tc.lowerSeq)
		if tc.lowerAliases == nil {
			tc.lowerAliases = map[string]string{}
		}
		tc.lowerAliases[name] = access
		local := cloneScope(scope)
		local[name] = valueType
		*tests = append(*tests, tc.lowerExpr(ast.BinaryExpr{Op: "==", Left: ast.IdentifierExpr{Name: name}, Right: pattern}, local))
		delete(tc.lowerAliases, name)
		return
	}
	vp, ok := tc.readVariantPattern(pattern, scope)
	if !ok {
		return
	}
	variant := info.variant(vp.variant)
	if variant == nil {
		return
	}
	*tests = append(*tests, tc.enumTagTest(info, valueType, access, variant))
	if vp.bare || len(variant.Fields) == 0 {
		return
	}
	payload := tc.payloadTypes(info, args, variant, false)
	if vp.braces {
		for _, value := range vp.values {
			ident, isName := value.(ast.IdentifierExpr)
			if !isName {
				continue
			}
			for i, field := range variant.Fields {
				if field.Name == ident.Name {
					*bindings = append(*bindings, loweredBinding{name: ident.Name, access: enumPayload(info, valueType, access, variant, field), typ: payload[i]})
				}
			}
		}
		return
	}
	for i, value := range vp.values {
		if i >= len(variant.Fields) {
			break
		}
		tc.lowerPattern(value, enumPayload(info, valueType, access, variant, variant.Fields[i]), payload[i], scope, tests, bindings)
	}
}

// lowerEnumMatch lowers a match on an enum to a chain of ifs over a copy of the
// value, each testing its arm's pattern; the names a pattern binds stand for
// the parts of the copy they name. A match that handles every variant without
// a wildcard runs its last arm as the else, so a backend sees a chain every
// path of which runs an arm.
func (tc *TypeChecker) lowerEnumMatch(stmt ast.MatchStmt, info *enumInfo, valueType ir.Type, scope map[string]ir.Type) []ir.Instr {
	tc.lowerSeq++
	valueName := "rho_match_value_" + strconv.Itoa(tc.lowerSeq)

	wrap := tc.arenaWrapFor(stmt.Value, valueType, scope)
	// A generic enum's copy is declared as its specialisation, which is then
	// emitted whether or not anything else names it.
	declared := valueType
	if _, args := tc.enumOf(valueType); len(info.Params) > 0 {
		declared = tc.enumType(info, args, true)
	}
	out := tc.arenaWrapped(wrap, ir.Instr{
		Op:       ir.OpLet,
		Dest:     valueName,
		Type:     tc.cTypeFor(declared),
		TypeName: typeName(declared),
		Args:     []string{tc.arenaReleasing(wrap, func() string { return tc.lowerExpr(stmt.Value, scope) })},
	})
	scope[valueName] = valueType

	// A match that handles every value reaches its last arm only with a value
	// that arm matches, so the last arm needs no test of its own.
	last := -1
	wildcard := false
	for i, arm := range stmt.Arms {
		if arm.Wildcard {
			wildcard = true
		} else {
			last = i
		}
	}
	exhaustive := !wildcard && !tc.useful(tc.matchRows(stmt, valueType, scope), []ir.Type{valueType}, []spacePattern{wildPattern}, 0)
	if tc.lowerAliases == nil {
		tc.lowerAliases = map[string]string{}
	}

	tail := []ir.Instr{}
	for i := len(stmt.Arms) - 1; i >= 0; i-- {
		arm := stmt.Arms[i]
		bodyScope := cloneScope(scope)
		if arm.Wildcard {
			tail = tc.lowerStatements(arm.Body, bodyScope)
			continue
		}
		tests := []string{}
		bindings := []loweredBinding{}
		tc.lowerPattern(arm.Pattern, valueName, valueType, scope, &tests, &bindings)
		type saved struct {
			value string
			had   bool
		}
		previous := map[string]saved{}
		for _, binding := range bindings {
			value, had := tc.lowerAliases[binding.name]
			if _, done := previous[binding.name]; !done {
				previous[binding.name] = saved{value, had}
			}
			tc.lowerAliases[binding.name] = binding.access
			bodyScope[binding.name] = binding.typ
		}
		body := tc.lowerStatements(arm.Body, bodyScope)
		for name, before := range previous {
			if before.had {
				tc.lowerAliases[name] = before.value
			} else {
				delete(tc.lowerAliases, name)
			}
		}
		if exhaustive && i == last {
			tail = body
			continue
		}
		condition := "true"
		if len(tests) > 0 {
			condition = "(" + strings.Join(tests, " && ") + ")"
		}
		tail = []ir.Instr{{Op: ir.OpIf, Condition: condition, Then: body, Else: tail}}
	}
	return append(out, tail...)
}

/* -------------------------------------------------------------------------- */
/*  Conversions and literals                                                  */
/* -------------------------------------------------------------------------- */

// checkEnumConversion refuses a conversion an enum does not have: a data enum
// to or from anything, and a number cast to a plain enum, which `Color(n)`
// does with a check.
//
// It is also what keeps an enum that carries data out of a database column or
// a wire field: those hold a number, and such an enum is not one.
func (tc *TypeChecker) checkEnumConversion(value ast.Expr, scope map[string]ir.Type, target ir.Type, isCast bool) error {
	source, err := tc.exprType(value, scope)
	if err != nil {
		return nil
	}
	sourceInfo, _ := tc.enumOf(source)
	targetInfo, _ := tc.enumOf(target)
	if sourceInfo == nil && targetInfo == nil {
		return nil
	}
	if sourceInfo != nil && sourceInfo == targetInfo {
		return nil
	}
	if sourceInfo != nil && !sourceInfo.Plain {
		return locate(fmt.Errorf("%s carries data and is not a number, so it cannot become %s; take it apart with match and convert the values it carries — a database column or a wire field holds those, never the enum itself",
			typeName(source), article(typeName(target))), value.Position(), "")
	}
	if targetInfo != nil && !targetInfo.Plain {
		return locate(fmt.Errorf("nothing converts to %s, which carries data; build one of its variants, as in %s", targetInfo.Name, tc.examplePattern(targetInfo)), value.Position(), "")
	}
	if targetInfo != nil && isCast {
		return locate(fmt.Errorf("a cast does not make a number %s; write %s(n), which stops the program if n is none of its values", article(targetInfo.Name), targetInfo.Name), value.Position(), "")
	}
	if targetInfo != nil && sourceInfo != nil {
		return locate(fmt.Errorf("%s does not convert to %s; convert through the number, as in %s(u32(v))", sourceInfo.Name, targetInfo.Name, targetInfo.Name), value.Position(), "")
	}
	return nil
}

// enumConversionCall reads `Name(...)` where Name is an enum, when it is not a
// function of that name.
func (tc *TypeChecker) enumConversionCall(call ast.CallExpr, scope map[string]ir.Type) (*enumInfo, bool) {
	ident, ok := call.Callee.(ast.IdentifierExpr)
	if !ok || len(tc.enums) == 0 {
		return nil, false
	}
	if _, local := scope[ident.Name]; local {
		return nil, false
	}
	if _, isFunc := tc.lookupFunc(ident.Name); isFunc {
		return nil, false
	}
	info, ok := tc.enums[ident.Name]
	return info, ok
}

// enumConversionType types `Color(n)`: a number made into a plain enum.
func (tc *TypeChecker) enumConversionType(call ast.CallExpr, scope map[string]ir.Type) (ir.Type, bool, error) {
	info, ok := tc.enumConversionCall(call, scope)
	if !ok {
		return ir.Type{}, false, nil
	}
	if !info.Plain {
		return ir.Type{}, true, locate(fmt.Errorf("%s is an enum that carries data and is built from one of its variants, as in %s", info.Name, tc.examplePattern(info)), call.Pos, "")
	}
	if len(call.Args) != 1 {
		return ir.Type{}, true, locate(fmt.Errorf("%s(n) makes a number into %s, and takes one number", info.Name, article(info.Name)), call.Pos, "")
	}
	source, err := tc.exprType(call.Args[0], scope)
	if err != nil {
		return ir.Type{}, true, err
	}
	target := tc.types[info.Name]
	if sourceInfo, _ := tc.enumOf(source); sourceInfo == info {
		return target, true, nil
	}
	if integerBits(source.Kind) == 0 || source.Enum {
		return ir.Type{}, true, locate(fmt.Errorf("%s(n) makes a whole number into %s, and this is %s; %s", info.Name, article(info.Name), article(typeName(source)), conversionHint(source, target)), call.Args[0].Position(), "")
	}
	if value, constant := constantInteger(call.Args[0]); constant && enumVariantWithValue(info, value) == nil {
		return ir.Type{}, true, locate(fmt.Errorf("%d is not a value of %s; its values are %s", value, info.Name, plainValuesText(info)), call.Args[0].Position(), "")
	}
	return target, true, nil
}

// constantInteger is the value of an integer literal, negated or not.
func constantInteger(expr ast.Expr) (int, bool) {
	switch e := expr.(type) {
	case ast.NumberExpr:
		value, err := strconv.ParseInt(strings.ReplaceAll(e.Value, "_", ""), 0, 64)
		return int(value), err == nil
	case ast.UnaryExpr:
		if e.Op == "-" {
			value, ok := constantInteger(e.Value)
			return -value, ok
		}
	}
	return 0, false
}

// enumVariantWithValue is the variant of a plain enum with a number, or nil.
func enumVariantWithValue(info *enumInfo, value int) *enumVariant {
	for i := range info.Variants {
		if info.Variants[i].Tag == value {
			return &info.Variants[i]
		}
	}
	return nil
}

// plainValuesText lists a plain enum's variants with their numbers.
func plainValuesText(info *enumInfo) string {
	parts := make([]string, len(info.Variants))
	for i, variant := range info.Variants {
		parts[i] = fmt.Sprintf("%s = %d", variant.Name, variant.Tag)
	}
	return strings.Join(parts, ", ")
}

// lowerEnumConversion lowers `Color(n)`: a literal known to be a value is that
// value, anything else goes through the function that checks it.
func (tc *TypeChecker) lowerEnumConversion(call ast.CallExpr, scope map[string]ir.Type) (string, bool) {
	info, ok := tc.enumConversionCall(call, scope)
	if !ok || !info.Plain || len(call.Args) != 1 {
		return "", false
	}
	if source, err := tc.exprType(call.Args[0], scope); err == nil {
		if sourceInfo, _ := tc.enumOf(source); sourceInfo == info {
			return tc.lowerExpr(call.Args[0], scope), true
		}
	}
	if value, constant := constantInteger(call.Args[0]); constant {
		if variant := enumVariantWithValue(info, value); variant != nil {
			return plainValue(info, variant), true
		}
	}
	return tc.enumConverter(info) + "((int64_t)(" + tc.lowerExpr(call.Args[0], scope) + "))", true
}

// enumLiteralType types a struct literal whose type is an enum that carries
// data: the literal of the struct-shaped Option or Result, which still builds
// the variant its flag names, or a mistake for any other enum.
func (tc *TypeChecker) enumLiteralType(literal ast.StructLiteralExpr, scope map[string]ir.Type, expected ir.Type) (ir.Type, bool, error) {
	info, ok := tc.enums[literal.Type.Name]
	if !ok || info.Plain {
		return ir.Type{}, false, nil
	}
	args, err := tc.compatLiteralArgs(info, literal, scope, expected)
	if err != nil {
		return ir.Type{}, true, err
	}
	typ := tc.enumType(info, args, false)
	// The optional pointer's literal is the pointer, and is lowered as it was.
	if IsOptionalPointer(mangleTypeName(typ.Name)) {
		return typ, true, nil
	}
	flag, _, _ := tc.compatLiteral(info)
	if len(literal.Values) > 0 {
		flagType, err := tc.exprType(literal.Values[0], scope)
		if err != nil {
			return ir.Type{}, true, err
		}
		if flagType.Kind != ir.TypeBool {
			return ir.Type{}, true, locate(fmt.Errorf("the first value of a %s literal is %s, which is a bool; build the value with one of its variants instead, as in %s", info.Name, flag, tc.examplePattern(info)), literal.Values[0].Position(), "")
		}
	}
	for _, value := range literal.Values[1:] {
		if _, err := tc.exprType(value, scope); err != nil {
			return ir.Type{}, true, err
		}
	}
	return typ, true, nil
}

// compatLiteralArgs are the type arguments of an Option or Result literal,
// written or deduced from the expected type and the values.
func (tc *TypeChecker) compatLiteralArgs(info *enumInfo, literal ast.StructLiteralExpr, scope map[string]ir.Type, expected ir.Type) ([]ir.Type, error) {
	flag, names, ok := tc.compatLiteral(info)
	if !ok || len(literal.Values) < 2 || len(literal.Values) > len(names)+1 {
		return nil, locate(fmt.Errorf("%s is an enum: build a value with one of its variants, as in %s", info.Name, tc.examplePattern(info)), literal.Pos, "")
	}
	_ = flag
	if len(literal.Type.Args) > 0 {
		args := tc.expandWithDefaults(info.Params, info.Defaults, tc.resolveTypeArgs(literal.Type.Args))
		if args == nil {
			return nil, locate(fmt.Errorf("%s takes %s", info.Name, plural(len(info.Params), "type argument")), literal.Pos, "")
		}
		return args, nil
	}
	subs := map[string]ir.Type{}
	if expected.Kind != ir.TypeInvalid {
		if wanted, args := tc.enumOf(tc.substituteType(expected, tc.currentTypeArgs)); wanted == info {
			for i, param := range info.Params {
				if i < len(args) {
					subs[param] = args[i]
				}
			}
		}
	}
	for i, name := range names {
		if i+1 >= len(literal.Values) {
			break
		}
		variant := info.variant(name)
		if variant == nil || len(variant.Fields) == 0 {
			continue
		}
		actual, err := tc.exprType(literal.Values[i+1], scope)
		if err != nil {
			return nil, err
		}
		tc.unifyTypeArgs(info.Params, tc.typeFromExpr(variant.Fields[0].TypeExpr, info.Params), actual, subs)
	}
	args := tc.orderedArgs(info.Params, info.Defaults, subs)
	if args == nil {
		return nil, locate(fmt.Errorf("cannot deduce the type arguments of this %s literal; write them, as in %s[%s]{ ... }", info.Name, info.Name, strings.Join(info.Params, ", ")), literal.Pos, "")
	}
	return args, nil
}

// lowerEnumLiteral lowers an Option or Result literal.
func (tc *TypeChecker) lowerEnumLiteral(literal ast.StructLiteralExpr, scope map[string]ir.Type, expected ir.Type) (string, bool) {
	info, ok := tc.enums[literal.Type.Name]
	if !ok || info.Plain {
		return "", false
	}
	args, err := tc.compatLiteralArgs(info, literal, scope, expected)
	if err != nil {
		return "", false
	}
	typ := tc.enumType(info, args, true)
	if IsOptionalPointer(mangleTypeName(typ.Name)) {
		return tc.lowerOptionalPointerLiteral(literal, typ.Name, scope), true
	}
	values := make([]string, len(literal.Values))
	for i, value := range literal.Values {
		values[i] = tc.lowerExpr(value, scope)
	}
	return tc.lowerCompatLiteral(info, args, values), true
}

// exampleConstruction is a value of an enum a message can show.
func (tc *TypeChecker) exampleConstruction(t ir.Type) string {
	info, _ := tc.enumOf(t)
	if info == nil {
		return "..."
	}
	for _, variant := range info.Variants {
		if len(variant.Fields) > 0 {
			return info.Name + "." + variant.Name + "(...)"
		}
	}
	return info.Name + "." + info.Variants[0].Name
}

/* -------------------------------------------------------------------------- */
/*  Exhaustiveness                                                            */
/* -------------------------------------------------------------------------- */

// spacePattern is a pattern reduced to what decides which values it matches:
// anything, or a constructor — a variant, `true` or `false`, or a literal —
// with a pattern for each value the constructor carries.
//
// It is what the usefulness check (Maranget, "Warnings for pattern matching")
// reads: an arm is unreachable when its pattern is not useful after the arms
// above it, and a match is exhaustive when `_` is not useful after all of them.
// A nested pattern, `Slot.Filled(Pair.One(Shape.Circle(r)))`, and a literal one,
// `Value.Flag(true)`, are decided the same way as a variant written alone.
type spacePattern struct {
	wild bool
	ctor string
	args []spacePattern
}

var wildPattern = spacePattern{wild: true}

func wilds(n int) []spacePattern {
	out := make([]spacePattern, n)
	for i := range out {
		out[i] = wildPattern
	}
	return out
}

// toSpace reduces a pattern against a value of a type.
func (tc *TypeChecker) toSpace(pattern ast.Expr, valueType ir.Type, scope map[string]ir.Type) spacePattern {
	info, args := tc.enumOf(valueType)
	switch p := pattern.(type) {
	case ast.IdentifierExpr:
		if p.Name == "_" || info == nil || info.variant(p.Name) == nil {
			return wildPattern
		}
	case ast.BoolExpr:
		if p.Value {
			return spacePattern{ctor: "true"}
		}
		return spacePattern{ctor: "false"}
	}
	if info == nil {
		// A number, a string or a constant: one value of many, which only `_`
		// or a binding covers.
		return spacePattern{ctor: "=" + tc.lowerExpr(pattern, scope)}
	}
	vp, ok := tc.readVariantPattern(pattern, scope)
	if !ok {
		return wildPattern
	}
	variant := info.variant(vp.variant)
	if variant == nil {
		return wildPattern
	}
	out := spacePattern{ctor: variant.Name, args: wilds(len(variant.Fields))}
	if vp.bare || vp.braces || len(vp.values) != len(variant.Fields) {
		return out
	}
	payload := tc.payloadTypes(info, args, variant, false)
	for i, value := range vp.values {
		out.args[i] = tc.toSpace(value, payload[i], scope)
	}
	return out
}

// spaceCtor is one constructor of a type: its name and the types of what it
// carries.
type spaceCtor struct {
	name  string
	types []ir.Type
}

// spaceCtors are every constructor of a type, and whether that is all of
// them: an enum's variants and a bool's two values are; a number or a string
// has more values than a match lists.
func (tc *TypeChecker) spaceCtors(t ir.Type) ([]spaceCtor, bool) {
	if t.Kind == ir.TypeBool {
		return []spaceCtor{{name: "true"}, {name: "false"}}, true
	}
	info, args := tc.enumOf(t)
	if info == nil {
		return nil, false
	}
	out := make([]spaceCtor, len(info.Variants))
	for i := range info.Variants {
		out[i] = spaceCtor{name: info.Variants[i].Name, types: tc.payloadTypes(info, args, &info.Variants[i], false)}
	}
	return out, true
}

// ctorTypes are the types of what a constructor of a type carries.
func (tc *TypeChecker) ctorTypes(t ir.Type, name string, arity int) []ir.Type {
	if ctors, _ := tc.spaceCtors(t); ctors != nil {
		for _, ctor := range ctors {
			if ctor.name == name && len(ctor.types) == arity {
				return ctor.types
			}
		}
	}
	out := make([]ir.Type, arity)
	return out
}

// specialise keeps the rows that can match a constructor, with its values in
// place of the first column.
func specialise(rows [][]spacePattern, name string, arity int) [][]spacePattern {
	out := [][]spacePattern{}
	for _, row := range rows {
		head := row[0]
		switch {
		case head.wild:
			out = append(out, append(wilds(arity), row[1:]...))
		case head.ctor == name && len(head.args) == arity:
			out = append(out, append(append([]spacePattern{}, head.args...), row[1:]...))
		}
	}
	return out
}

// useful reports whether a row of patterns matches some value that none of
// the rows before it does.
func (tc *TypeChecker) useful(rows [][]spacePattern, types []ir.Type, row []spacePattern, depth int) bool {
	if len(row) == 0 {
		return len(rows) == 0
	}
	if depth > 64 {
		return true
	}
	head := row[0]
	if !head.wild {
		argTypes := tc.ctorTypes(types[0], head.ctor, len(head.args))
		return tc.useful(specialise(rows, head.ctor, len(head.args)), append(append([]ir.Type{}, argTypes...), types[1:]...),
			append(append([]spacePattern{}, head.args...), row[1:]...), depth+1)
	}
	ctors, complete := tc.spaceCtors(types[0])
	if complete {
		used := map[string]bool{}
		for _, r := range rows {
			if !r[0].wild {
				used[r[0].ctor] = true
			}
		}
		all := true
		for _, ctor := range ctors {
			if !used[ctor.name] {
				all = false
			}
		}
		if all {
			for _, ctor := range ctors {
				if tc.useful(specialise(rows, ctor.name, len(ctor.types)), append(append([]ir.Type{}, ctor.types...), types[1:]...),
					append(wilds(len(ctor.types)), row[1:]...), depth+1) {
					return true
				}
			}
			return false
		}
	}
	rest := [][]spacePattern{}
	for _, r := range rows {
		if r[0].wild {
			rest = append(rest, r[1:])
		}
	}
	return tc.useful(rest, types[1:], row[1:], depth+1)
}

// matchRows are a match's arms reduced, a wildcard arm as `_`.
func (tc *TypeChecker) matchRows(stmt ast.MatchStmt, valueType ir.Type, scope map[string]ir.Type) [][]spacePattern {
	rows := make([][]spacePattern, len(stmt.Arms))
	for i, arm := range stmt.Arms {
		if arm.Wildcard {
			rows[i] = []spacePattern{wildPattern}
			continue
		}
		rows[i] = []spacePattern{tc.toSpace(arm.Pattern, valueType, scope)}
	}
	return rows
}

// unhandledVariants are the variants of an enum some value of which no row
// matches, and of those the ones no row names at all.
func (tc *TypeChecker) unhandledVariants(info *enumInfo, valueType ir.Type, rows [][]spacePattern) (unhandled []string, unnamed map[string]bool) {
	unnamed = map[string]bool{}
	types := []ir.Type{valueType}
	for _, ctor := range func() []spaceCtor { c, _ := tc.spaceCtors(valueType); return c }() {
		probe := []spacePattern{{ctor: ctor.name, args: wilds(len(ctor.types))}}
		if !tc.useful(rows, types, probe, 0) {
			continue
		}
		unhandled = append(unhandled, ctor.name)
		named := false
		for _, row := range rows {
			if !row[0].wild && row[0].ctor == ctor.name {
				named = true
			}
		}
		if !named {
			unnamed[ctor.name] = true
		}
	}
	return unhandled, unnamed
}
