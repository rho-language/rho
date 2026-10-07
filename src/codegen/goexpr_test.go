//
// @file goexpr_test.go
// @brief Tests for translating Rho IR expressions from C syntax to Go.
//
// Rho IR carries C-flavoured expressions, so the Go backend depends on these
// rewrites being correct: compound literals, casts, the rho_concat join
// helper, C stdio calls and snake_case names.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Test: Compound Literals                                                   */
/* -------------------------------------------------------------------------- */

// (PID){ a, b } has no Go form; it becomes a composite literal.
func TestToGoExprCompoundLiteral(t *testing.T) {
	got := ToGoExpr("(PID){ pid.kp, pid.ki, alpha }")
	want := "PID{ pid.kp, pid.ki, alpha }"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprNestedCompoundLiteral(t *testing.T) {
	// Field names inside the literal are struct fields, so snake_case becomes
	// lowerCamel just like any other member access.
	got := ToGoExpr("(Kalman1D){ initial, 1.0, process_noise, measurement_noise }")
	want := "Kalman1D{ initial, 1.0, processNoise, measurementNoise }"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: C Casts                                                             */
/* -------------------------------------------------------------------------- */

func TestToGoExprNumericCast(t *testing.T) {
	if got := ToGoExpr("(int32_t)x"); got != "int32(x)" {
		t.Fatalf("got %q, want %q", got, "int32(x)")
	}
}

func TestToGoExprDoubleCast(t *testing.T) {
	if got := ToGoExpr("(double)total"); got != "float64(total)" {
		t.Fatalf("got %q, want %q", got, "float64(total)")
	}
}

func TestToGoExprCharPointerCast(t *testing.T) {
	if got := ToGoExpr("(const char *)name"); got != "string(name)" {
		t.Fatalf("got %q, want %q", got, "string(name)")
	}
}

// A void cast carries no Go equivalent and must be dropped, not mistranslated.
func TestToGoExprDropsVoidCast(t *testing.T) {
	if got := ToGoExpr("(void)0"); got != "0" {
		t.Fatalf("got %q, want %q", got, "0")
	}
}

// Parentheses that only group an expression must survive untouched.
func TestToGoExprKeepsGrouping(t *testing.T) {
	if got := ToGoExpr("(setpoint - measurement)"); got != "(setpoint - measurement)" {
		t.Fatalf("got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: rho_concat                                                          */
/* -------------------------------------------------------------------------- */

// Go's + covers both the numeric and the string case rho_concat joined.
func TestToGoExprRhoConcatNumbers(t *testing.T) {
	got := ToGoExpr("rho_concat(filter.a, filter.b)")
	want := "(filter.a + filter.b)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprRhoConcatStrings(t *testing.T) {
	got := ToGoExpr("rho_concat(prefix(), name)")
	want := "(prefix() + name)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Nested calls must all be rewritten, not just the outer one.
func TestToGoExprRhoConcatNested(t *testing.T) {
	got := ToGoExpr("rho_concat(rho_concat(a, b), c)")
	want := "((a + b) + c)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprRhoConcatWithFunctionCallArg(t *testing.T) {
	// A Rho local named `error` is escaped everywhere it appears, including
	// nested inside a rewritten call.
	got := ToGoExpr("rho_concat((pid.kp * error), clamp_f64(x, 0.0, 1.0))")
	want := "((pid.kp * error_) + clampF64(x, 0.0, 1.0))"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: C Library Calls                                                     */
/* -------------------------------------------------------------------------- */

func TestToGoExprPrintf(t *testing.T) {
	got := ToGoExpr("printf(\"%s\\n\", \"hello\")")
	want := "fmt.Printf(\"%s\\n\", \"hello\")"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprSprintf(t *testing.T) {
	if got := ToGoExpr("sprintf(\"%d\", x)"); got != "fmt.Sprintf(\"%d\", x)" {
		t.Fatalf("got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: snake_case Names                                                    */
/* -------------------------------------------------------------------------- */

func TestToGoExprMemberAccess(t *testing.T) {
	if got := ToGoExpr("filter.previous_error"); got != "filter.previousError" {
		t.Fatalf("got %q, want %q", got, "filter.previousError")
	}
}

func TestToGoExprFunctionName(t *testing.T) {
	if got := ToGoExpr("clamp_f64(x, 0.0, 1.0)"); got != "clampF64(x, 0.0, 1.0)" {
		t.Fatalf("got %q", got)
	}
}

func TestToGoExprDeepFunctionName(t *testing.T) {
	if got := ToGoExpr("std_math_basic_min_u32(a, b)"); got != "stdMathBasicMinU32(a, b)" {
		t.Fatalf("got %q", got)
	}
}

// Exported type names carry no underscores and must not be rewritten.
func TestToGoExprKeepsTypeNames(t *testing.T) {
	if got := ToGoExpr("MovingAverage4{ x }"); got != "MovingAverage4{ x }" {
		t.Fatalf("got %q", got)
	}
}

// A Rho local named `error` collides with a Go predeclared identifier; every
// reference has to be escaped consistently, not just the declaration.
func TestToGoExprEscapesReservedName(t *testing.T) {
	if got := ToGoExpr("(setpoint - measurement)"); got != "(setpoint - measurement)" {
		t.Fatalf("got %q", got)
	}
	if got := ToGoExpr("error"); got != "error_" {
		t.Fatalf("got %q, want %q", got, "error_")
	}
	if got := ToGoExpr("error * dt"); got != "error_ * dt" {
		t.Fatalf("got %q, want %q", got, "error_ * dt")
	}
}

// Predeclared names that Go allows shadowing are left alone, so a genuine
// call to a builtin still resolves. `len` is not among them: Rho types it as a
// u32 while Go's returns an int, so its call carries a conversion.
func TestToGoExprKeepsShadowableBuiltins(t *testing.T) {
	for _, expr := range []string{"min(a, b)", "append(s, t)"} {
		if got := ToGoExpr(expr); got != expr {
			t.Fatalf("%s: got %q, want unchanged", expr, got)
		}
	}
}

// Literals and keywords must pass through unchanged.
func TestToGoExprKeepsLiterals(t *testing.T) {
	for _, expr := range []string{"true", "false", "0.0", "1.0", "-1"} {
		got := ToGoExpr(expr)
		if got != expr {
			t.Fatalf("%q: got %q, want unchanged", expr, got)
		}
	}
}

func TestToGoExprNULLBecomesNil(t *testing.T) {
	if got := ToGoExpr("NULL"); got != "nil" {
		t.Fatalf("got %q, want %q", got, "nil")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: String Literals Are Preserved                                       */
/* -------------------------------------------------------------------------- */

// Escapes and format specifiers inside a literal must survive untouched,
// including text that looks like code.
func TestToGoExprPreservesStringLiterals(t *testing.T) {
	cases := []string{
		`"hello from rho"`,
		`"%s\n"`,
		`"a_b_c"`,
		`"rho_concat(x, y)"`,
		`"clamp_f64(a, b)"`,
		`"error"`,
	}
	for _, expr := range cases {
		if got := ToGoExpr(expr); got != expr {
			t.Fatalf("%s: got %q, want unchanged", expr, got)
		}
	}
}

func TestToGoExprPrintfKeepsFormatString(t *testing.T) {
	got := ToGoExpr(`printf("%s\n", name)`)
	want := `fmt.Printf("%s\n", name)`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Control Flow Expressions                                            */
/* -------------------------------------------------------------------------- */

func TestToGoExprConditions(t *testing.T) {
	cases := map[string]string{
		"!filter.initialized":                     "!filter.initialized",
		"(filter.index == 0)":                     "(filter.index == 0)",
		"((x <= threshold) && (x >= -threshold))": "((x <= threshold) && (x >= -threshold))",
		"(filter.count == 0)":                     "(filter.count == 0)",
	}
	for expr, want := range cases {
		if got := ToGoExpr(expr); got != want {
			t.Fatalf("%s: got %q, want %q", expr, got, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Helpers                                                             */
/* -------------------------------------------------------------------------- */

func TestLowerCamel(t *testing.T) {
	cases := map[string]string{
		"previous_error":         "previousError",
		"clamp_f64":              "clampF64",
		"min_u32":                "minU32",
		"std_math_basic_min_u32": "stdMathBasicMinU32",
		"already":                "already",
		"":                       "",
	}
	for in, want := range cases {
		if got := lowerCamel(in); got != want {
			t.Fatalf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestSplitTopLevel(t *testing.T) {
	got := splitTopLevel("a, f(b, c), d")
	want := []string{"a", "f(b, c)", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("part %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// A comma inside a string literal must not split the argument list.
func TestSplitTopLevelIgnoresStringComma(t *testing.T) {
	got := splitTopLevel(`"a, b", c`)
	if len(got) != 2 {
		t.Fatalf("expected 2 parts, got %v", got)
	}
}

func TestGoTypeForCName(t *testing.T) {
	if got := goTypeForCName("double"); got != "float64" {
		t.Errorf("double: got %q", got)
	}
	if got := goTypeForCName("int32_t"); got != "int32" {
		t.Errorf("int32_t: got %q", got)
	}
	if got := goTypeForCName("string"); got != "string" {
		t.Errorf("string: got %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Cast Operands With Selectors                                        */
/* -------------------------------------------------------------------------- */

// A cast applies to the whole operand. Truncating it at the '.' would leave a
// dangling member access and produce uncompilable Go.
func TestToGoExprCastOnMemberAccess(t *testing.T) {
	got := ToGoExpr("(int32_t)filter.count")
	want := "int32(filter.count)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprCastOnIndex(t *testing.T) {
	got := ToGoExpr("(double)values[index]")
	want := "float64(values[index])"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToGoExprCastOnParenOperand(t *testing.T) {
	// The grouping parens are preserved, which is redundant but valid Go.
	got := ToGoExpr("(double)(a + b)")
	if !strings.HasPrefix(got, "float64(") || !strings.Contains(got, "a + b") {
		t.Fatalf("got %q", got)
	}
}

// A decimal point is part of the literal, not a selector.
func TestToGoExprFloatLiteralNotSelector(t *testing.T) {
	got := ToGoExpr("(double)2.5")
	if got != "float64(2.5)" {
		t.Fatalf("got %q", got)
	}
}

func TestToGoExprUnsignedCharCast(t *testing.T) {
	if got := ToGoExpr("(unsigned char)value"); got != "uint8(value)" {
		t.Fatalf("got %q", got)
	}
}

func TestToGoExprLenIsUint32(t *testing.T) {
	// Rho types len as u32, but Go's len returns an int, so the result is
	// converted or a comparison against a u32 will not compile.
	if got := ToGoExpr("len(data)"); got != "uint32(len(data))" {
		t.Fatalf("expected uint32(len(data)), got %q", got)
	}
}

func TestToGoExprFileBuiltins(t *testing.T) {
	if got := ToGoExpr("read_file(path)"); got != "rhoReadFile(path)" {
		t.Fatalf("expected rhoReadFile(path), got %q", got)
	}
	if got := ToGoExpr("write_file(path, data)"); got != "rhoWriteFile(path, data)" {
		t.Fatalf("expected rhoWriteFile(path, data), got %q", got)
	}
	if got := ToGoExpr("get_args()"); got != "rhoGetArgs()" {
		t.Fatalf("expected rhoGetArgs(), got %q", got)
	}
}

func TestToGoExprCastCoversCall(t *testing.T) {
	// A C cast binds to the whole postfix call, so the operand is `len(data)`
	// and the Go conversion has to wrap all of it. `len` itself converts to
	// Rho's u32, so the result is a double conversion, which is correct.
	got := ToGoExpr("(int32_t)len(data)")
	if got != "int32(uint32(len(data)))" {
		t.Fatalf("expected int32(uint32(len(data))), got %q", got)
	}
}

func TestToGoExprCastCoversIndexedExpr(t *testing.T) {
	got := ToGoExpr("(int32_t)values[0]")
	if got != "int32(values[0])" {
		t.Fatalf("expected int32(values[0]), got %q", got)
	}
}

func TestGoTypeForRhoNameSlice(t *testing.T) {
	if got := goTypeForRhoName("[u8]"); got != "[]uint8" {
		t.Fatalf("expected []uint8, got %q", got)
	}
	if got := goTypeForRhoName("[u32]"); got != "[]uint32" {
		t.Fatalf("expected []uint32, got %q", got)
	}
}

func TestGoTypeForRhoNamePrimitive(t *testing.T) {
	cases := map[string]string{
		"u8":     "uint8",
		"u32":    "uint32",
		"i32":    "int32",
		"f64":    "float64",
		"bool":   "bool",
		"string": "string",
	}
	for name, want := range cases {
		if got := goTypeForRhoName(name); got != want {
			t.Fatalf("%s: expected %q, got %q", name, want, got)
		}
	}
}

func TestGoTypeForIRSliceIsSliceNotPointer(t *testing.T) {
	slice := ir.Type{Kind: ir.TypeSlice, Name: "[u8]", Args: []ir.Type{{Kind: ir.TypeU8, Name: "u8"}}}
	if got := goTypeForIR(slice); got != "[]uint8" {
		t.Fatalf("expected []uint8 for a slice, got %q", got)
	}
	pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[u8]", Args: []ir.Type{{Kind: ir.TypeU8, Name: "u8"}}}
	if got := goTypeForIR(pointer); got != "*uint8" {
		t.Fatalf("expected *uint8 for a pointer, got %q", got)
	}
}
