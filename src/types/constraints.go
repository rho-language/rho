//
// @file constraints.go
// @brief Constraints on type parameters: what a generic body may do with a T.
//
// A generic body is checked once, with each type parameter standing for every
// type it may be specialised at, and then lowered once per specialisation with
// the parameters substituted. What the body may do with a `T` is therefore what
// every type it may become supports, and that is what a constraint says:
//
//	fn sum[T: Number](xs: Slice[T]) T
//	fn max[T: Ordered](a: T, b: T) T
//	fn index_of[T: Equal](xs: Slice[T], x: T) Option[u32]
//
// The constraints are the language's own, a short list with fixed meanings:
//
//	Integer   i8 … i64, u8 … u64        + - * / %  & | ^  unary -  ordering
//	Float     f32, f64                  + - * /  unary -  ordering
//	Number    Integer or Float          + - * /  unary -  ordering
//	Ordered   Number, string            < <= > >=  == !=
//	Equal     Ordered, bool, plain enum == !=
//
// Each one implies the ones below it, so `T: Integer` may be passed where
// `U: Number` is wanted. Several may be written, `T: Ordered + Equal`, and a
// type argument has to satisfy each of them.
//
// A specialisation is lowered exactly as the same code written at the concrete
// type would be, so a `+` on a `T` specialised at i8 is the i8 `+`: it wraps,
// and a `/` by zero stops the program, on both backends.
//
// A type argument that does not satisfy a constraint is refused where the call
// is written, naming the constraint and the type, rather than surfacing later
// as an error in a specialised body the program never wrote.
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

/* -------------------------------------------------------------------------- */
/*  The constraints                                                           */
/* -------------------------------------------------------------------------- */

// constraintOrder is every constraint, from the narrowest to the widest, which
// is the order a message lists them in.
var constraintOrder = []string{"Integer", "Float", "Number", "Ordered", "Equal"}

// constraintDescription says which types satisfy a constraint, for a message.
var constraintDescription = map[string]string{
	"Integer": "an integer type (i8, i16, i32, i64, u8, u16, u32, u64)",
	"Float":   "a floating-point type (f32, f64)",
	"Number":  "a number type (an integer type, f32 or f64)",
	"Ordered": "a number type or string",
	"Equal":   "a number type, bool, string or an enum without data",
}

// constraintImplies says which constraints a constraint includes: a type that
// satisfies the key satisfies every value.
var constraintImplies = map[string][]string{
	"Integer": {"Integer", "Number", "Ordered", "Equal"},
	"Float":   {"Float", "Number", "Ordered", "Equal"},
	"Number":  {"Number", "Ordered", "Equal"},
	"Ordered": {"Ordered", "Equal"},
	"Equal":   {"Equal"},
}

// knownConstraint reports whether a name is one of the language's constraints.
func knownConstraint(name string) bool {
	_, ok := constraintImplies[name]
	return ok
}

// boundNames are the constraint names written on a parameter.
func boundNames(bounds []ast.TypeBound) []string {
	names := make([]string, len(bounds))
	for i, bound := range bounds {
		names[i] = bound.Name
	}
	return names
}

// checkBounds refuses a constraint nothing defines, at its name, and one
// written on a name that is not a type parameter of the declaration.
func checkBounds(fn ast.FuncDecl) error {
	for _, param := range fn.TypeParams {
		for _, bound := range fn.TypeParamBounds[param] {
			if !knownConstraint(bound.Name) {
				return locate(fmt.Errorf(msgUnknownConstraint, bound.Name, param, fn.Name, strings.Join(constraintOrder, ", ")), bound.Pos, "")
			}
		}
	}
	return nil
}

// typeSatisfies reports whether a concrete type satisfies one constraint.
func (tc *TypeChecker) typeSatisfies(typ ir.Type, constraint string) bool {
	if typ.ArrayLen > 0 {
		return false
	}
	// A type parameter of the generic being checked satisfies what its own
	// constraints imply: `T: Integer` may be passed on as a `U: Number`.
	if bounds, isParam := tc.typeParamBounds(typ); isParam {
		for _, bound := range bounds {
			for _, implied := range constraintImplies[bound] {
				if implied == constraint {
					return true
				}
			}
		}
		return false
	}
	integer := integerBits(typ.Kind) > 0 && !typ.Enum
	float := (typ.Kind == ir.TypeF32 || typ.Kind == ir.TypeF64) && !typ.Enum
	switch constraint {
	case "Integer":
		return integer
	case "Float":
		return float
	case "Number":
		return integer || float
	case "Ordered":
		return integer || float || typ.Kind == ir.TypeString
	case "Equal":
		if integer || float || typ.Kind == ir.TypeString || typ.Kind == ir.TypeBool {
			return true
		}
		if typ.Enum {
			info, ok := tc.enums[baseTypeName(typ.Name)]
			return ok && info.Plain
		}
		return false
	}
	return false
}

// checkTypeArgBounds refuses a specialisation of a generic function whose type
// arguments do not satisfy its constraints, at the place it is asked for.
func (tc *TypeChecker) checkTypeArgBounds(decl ast.FuncDecl, subs map[string]ir.Type, at ast.Position) error {
	for _, param := range decl.TypeParams {
		bounds := decl.TypeParamBounds[param]
		if len(bounds) == 0 {
			continue
		}
		actual, bound := subs[param]
		if !bound || actual.Kind == ir.TypeInvalid || actual.Name == poisonName {
			continue
		}
		for _, constraint := range bounds {
			if !knownConstraint(constraint.Name) {
				// Reported at the declaration.
				continue
			}
			if tc.typeSatisfies(actual, constraint.Name) {
				continue
			}
			what := constraintDescription[constraint.Name]
			if params, isParam := tc.typeParamBounds(actual); isParam {
				return withNote(locate(fmt.Errorf(msgTypeParamUnconstrainedArg, decl.Name, param, constraint.Name, actual.Name, constraintText(params), actual.Name, constraint.Name), at, ""),
					at, declaredAt("generic function "+decl.Name, decl.Pos))
			}
			return withNote(locate(fmt.Errorf(msgConstraintUnsatisfied, decl.Name, param, constraint.Name, typeName(actual), constraint.Name, what), at, ""),
				at, declaredAt("generic function "+decl.Name, decl.Pos))
		}
	}
	return nil
}

// constraintText is a list of constraints as a message says it.
func constraintText(bounds []string) string {
	if len(bounds) == 0 {
		return "no constraint"
	}
	return strings.Join(bounds, " + ")
}

/* -------------------------------------------------------------------------- */
/*  Inside a generic body                                                     */
/* -------------------------------------------------------------------------- */

// typeParamBounds reports whether a type is a type parameter of the generic
// body being checked, and its constraints when it is.
func (tc *TypeChecker) typeParamBounds(typ ir.Type) ([]string, bool) {
	if typ.Kind != ir.TypeStruct || typ.ArrayLen > 0 || len(tc.checkingTypeParams) == 0 {
		return nil, false
	}
	if _, substituted := tc.currentTypeArgs[typ.Name]; substituted {
		return nil, false
	}
	for _, param := range tc.checkingTypeParams {
		if param == typ.Name {
			return tc.checkingBounds[param], true
		}
	}
	return nil, false
}

// boundsAllow reports whether a parameter's constraints include one.
func boundsAllow(bounds []string, constraint string) bool {
	for _, bound := range bounds {
		for _, implied := range constraintImplies[bound] {
			if implied == constraint {
				return true
			}
		}
	}
	return false
}

// operatorConstraint is the constraint an operator needs of its operands.
func operatorConstraint(op string) string {
	switch op {
	case "+", "-", "*", "/":
		return "Number"
	case "%", "&", "|", "^":
		return "Integer"
	case "<", "<=", ">", ">=":
		return "Ordered"
	case "==", "!=":
		return "Equal"
	}
	return ""
}

// typeParamLiteral checks a written number meeting a type parameter. The
// literal becomes the parameter's type in every specialisation, so it has to
// mean the same in each: an integer every number type holds (0 to 127) when
// the parameter is a number. Anything else is converted explicitly, `T(0.5)`,
// which says the conversion happens and checks it against each type.
func typeParamLiteral(literal ast.Expr, param string, bounds []string) error {
	if !boundsAllow(bounds, "Number") {
		return locate(fmt.Errorf(msgTypeParamLiteralNotNumber, param, constraintText(bounds), param), literal.Position(), "")
	}
	if fitsEveryNumber(literal, bounds) {
		return nil
	}
	return locate(fmt.Errorf(msgTypeParamLiteralRange, exprText(literal), param, constraintText(bounds), param), literal.Position(), "")
}

// fitsEveryNumber reports whether a written number fits every type a
// parameter with these constraints may be: 0 to 127 for an integer, and any
// number for a parameter that is only ever a float.
func fitsEveryNumber(literal ast.Expr, bounds []string) bool {
	number, ok := literal.(ast.NumberExpr)
	if !ok {
		return false
	}
	onlyFloat := boundsAllow(bounds, "Float") && !boundsAllow(bounds, "Integer")
	if strings.ContainsAny(number.Value, ".eE") && !strings.HasPrefix(number.Value, "0x") {
		return false
	}
	value, err := strconv.ParseInt(number.Value, 0, 64)
	if err != nil {
		return false
	}
	if onlyFloat {
		// An integer below 2^24 is exact in an f32 as in an f64.
		return value >= 0 && value <= 1<<24
	}
	return value >= 0 && value <= 127
}

// exprText is a short spelling of a literal for a message.
func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case ast.NumberExpr:
		return e.Value
	case ast.UnaryExpr:
		return e.Op + exprText(e.Value)
	}
	return "the constant"
}

// typeParamBinary types a binary operator with a type parameter on one side or
// both. handled is false when neither side is one, and the operator is typed
// as for concrete values.
func (tc *TypeChecker) typeParamBinary(value ast.BinaryExpr, left, right ir.Type) (ir.Type, bool, error) {
	leftBounds, leftParam := tc.typeParamBounds(left)
	rightBounds, rightParam := tc.typeParamBounds(right)
	if !leftParam && !rightParam {
		return ir.Type{}, false, nil
	}
	if value.Op == "&&" || value.Op == "||" {
		return ir.Type{}, false, nil
	}
	param, bounds, other, otherExpr := left, leftBounds, right, value.Right
	if !leftParam {
		param, bounds, other, otherExpr = right, rightBounds, left, value.Left
	}
	if value.Op == "<<" || value.Op == ">>" {
		return ir.Type{}, true, locate(fmt.Errorf(msgTypeParamShift, value.Op, param.Name), value.Position(), "")
	}
	needed := operatorConstraint(value.Op)
	if needed == "" {
		return ir.Type{}, true, locate(fmt.Errorf("operator %s is not defined on %s, a type parameter", value.Op, param.Name), value.Position(), "")
	}
	// `==` and `!=` were accepted on any type parameter before constraints
	// existed, and are checked at each specialisation as they were; `T: Equal`
	// checks them where the generic is called instead.
	if (value.Op == "==" || value.Op == "!=") && len(bounds) == 0 {
		return ir.Type{}, false, nil
	}
	if !boundsAllow(bounds, needed) {
		suggestion := needed
		if needed == "Number" && boundsAllow(bounds, "Ordered") {
			suggestion = "Number"
		}
		written := param.Name + ": " + suggestion
		if len(bounds) > 0 {
			written = param.Name + ": " + constraintText(bounds) + " + " + suggestion
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgTypeParamOperator, value.Op, param.Name, constraintText(bounds), suggestion, written), value.Position(), "")
	}
	// The other side is the same parameter, or a written number that means
	// the same in every specialisation.
	switch {
	case leftParam && rightParam:
		if left.Name != right.Name {
			return ir.Type{}, true, locate(fmt.Errorf(msgTypeParamMixed, value.Op, left.Name, right.Name), value.Position(), "")
		}
	case isUntypedConstant(otherExpr):
		if err := typeParamLiteral(otherExpr, param.Name, bounds); err != nil {
			return ir.Type{}, true, err
		}
	default:
		if other.Kind == ir.TypeInvalid || other.Name == poisonName {
			break
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgTypeParamWithConcrete, value.Op, param.Name, typeName(other), param.Name), value.Position(), "")
	}
	switch value.Op {
	case "==", "!=", "<", "<=", ">", ">=":
		return ir.Type{Kind: ir.TypeBool, Name: "bool"}, true, nil
	}
	return param, true, nil
}

// typeParamNegation types unary minus on a type parameter.
func (tc *TypeChecker) typeParamNegation(value ast.UnaryExpr, inner ir.Type) (ir.Type, bool, error) {
	bounds, isParam := tc.typeParamBounds(inner)
	if !isParam {
		return ir.Type{}, false, nil
	}
	if !boundsAllow(bounds, "Number") {
		written := inner.Name + ": Number"
		if len(bounds) > 0 {
			written = inner.Name + ": " + constraintText(bounds) + " + Number"
		}
		return ir.Type{}, true, locate(fmt.Errorf(msgTypeParamOperator, "unary -", inner.Name, constraintText(bounds), "Number", written), value.Position(), "")
	}
	return inner, true, nil
}

// typeParamConversion checks `T(x)` and `x as T` in a generic body, where T is
// one of its type parameters. A conversion is to every type T may be, so the
// parameter has to be a number and the value a number (or a T); a written
// number has to fit every type T may be.
func (tc *TypeChecker) typeParamConversion(target ir.Type, operand ast.Expr, scope map[string]ir.Type, at ast.Position) error {
	bounds, isParam := tc.typeParamBounds(target)
	if !isParam {
		return nil
	}
	operandType, err := tc.exprType(operand, scope)
	if err != nil {
		return err
	}
	if operandType.Name == target.Name && operandType.Kind == target.Kind {
		return nil
	}
	if len(bounds) == 0 {
		// Before constraints, a conversion to an unconstrained parameter was
		// accepted and checked at each specialisation; it still is.
		return nil
	}
	if !boundsAllow(bounds, "Number") {
		return locate(fmt.Errorf(msgTypeParamConversion, target.Name, constraintText(bounds), target.Name), at, "")
	}
	if isUntypedConstant(operand) {
		if number, ok := operand.(ast.NumberExpr); ok && strings.Contains(number.Value, ".") && boundsAllow(bounds, "Float") && !boundsAllow(bounds, "Integer") {
			return nil
		}
		if !fitsEveryNumber(operand, bounds) {
			return locate(fmt.Errorf(msgTypeParamLiteralRange, exprText(operand), target.Name, constraintText(bounds), target.Name), operand.Position(), "")
		}
		return nil
	}
	if !numeric(operandType) {
		if _, otherParam := tc.typeParamBounds(operandType); otherParam && boundsAllow(tc.checkingBounds[operandType.Name], "Number") {
			return nil
		}
		return locate(fmt.Errorf(msgTypeParamConversionFrom, typeName(operandType), target.Name), at, "")
	}
	return nil
}
