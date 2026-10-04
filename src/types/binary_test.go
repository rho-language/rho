//
// @file binary_test.go
// @brief Tests for operator lowering, in particular numeric promotion.
//
// C promotes mismatched numeric operands implicitly and has no string
// concatenation operator; Go does the opposite in both cases. The lowering has
// to satisfy both backends at once:
//
//   - string `+` lowers to the rho_concat runtime helper
//   - numeric `+` lowers to the host operator
//   - mismatched numeric operands get an explicit cast, which is valid C and
//     the only form Go accepts
//   - a numeric literal is an untyped constant and adopts the other operand
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"strings"
	"testing"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// lowerFunctionToIR type checks a module and returns the IR for the named
// function, so lowering decisions can be inspected directly.
func lowerFunctionToIR(t *testing.T, source, want string) ir.FuncIR {
	t.Helper()
	module := parseSource(t, source)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("typecheck failed: %v\n%s", err, source)
	}
	out := tc.ToIR(*module)
	for _, fn := range out.Funcs {
		if fn.Name == want {
			return fn
		}
	}
	t.Fatalf("function %q not found in:\n%s", want, source)
	return ir.FuncIR{}
}

// findFunc returns a named function from generated IR.
func findFunc(t *testing.T, module ir.ModuleIR, name string) ir.FuncIR {
	t.Helper()
	for _, fn := range module.Funcs {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("function %q not found in generated IR", name)
	return ir.FuncIR{}
}

func parseSource(t *testing.T, source string) *ast.Module {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing failed: %v\n%s", err, source)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing failed: %v\n%s", err, source)
	}
	return module
}

// instrText joins every string an instruction carries, for substring checks.
func instrText(fn ir.FuncIR) string {
	var b strings.Builder
	var walk func([]ir.Instr)
	walk = func(list []ir.Instr) {
		for _, instr := range list {
			b.WriteString(instr.Dest)
			b.WriteString(instr.Type)
			b.WriteString(instr.Condition)
			for _, arg := range instr.Args {
				b.WriteString(" ")
				b.WriteString(arg)
			}
			b.WriteString("\n")
			walk(instr.Then)
			walk(instr.Else)
		}
	}
	walk(fn.Body)
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Test: String Concatenation Uses the Runtime Helper                         */
/* -------------------------------------------------------------------------- */

func TestStringConcatUsesRhoConcat(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn label() string {
    return "v";
}

fn main() void {
    let greeting: string = label() + "x";
    print(greeting);
}
`, "main")
	text := instrText(fn)
	if !strings.Contains(text, "rho_concat(") {
		t.Fatalf("string + should use the runtime helper, got:\n%s", text)
	}
}

func TestStringConcatNested(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn main() void {
    let out: string = "a" + "b" + "c";
    print(out);
}
`, "main")
	if !strings.Contains(instrText(fn), "rho_concat(") {
		t.Fatalf("nested string + should use the runtime helper:\n%s", instrText(fn))
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Numeric Addition Uses the Host Operator                             */
/* -------------------------------------------------------------------------- */

// The original defect: numeric + was lowered to a string helper, so the
// generated C passed doubles to a `char*` parameter and failed to compile.
func TestNumericAdditionDoesNotUseRhoConcat(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn main() void {
    let total: f64 = 1.0 + 2.0;
    print(total);
}
`, "main")
	text := instrText(fn)
	if strings.Contains(text, "rho_concat(") {
		t.Fatalf("numeric + must not use the string helper, got:\n%s", text)
	}
	if !strings.Contains(text, "1.0 + 2.0") {
		t.Fatalf("expected a plain addition, got:\n%s", text)
	}
}

func TestNumericAdditionOfVariables(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn main() void {
    let a: f64 = 1.0;
    let b: f64 = 2.0;
    let total: f64 = a + b;
    print(total);
}
`, "main")
	text := instrText(fn)
	if strings.Contains(text, "rho_concat(") {
		t.Fatalf("numeric variable addition must not use the string helper:\n%s", text)
	}
	if !strings.Contains(text, "a + b") {
		t.Fatalf("expected a plain addition, got:\n%s", text)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Numeric Promotion                                                   */
/* -------------------------------------------------------------------------- */

// Go has no implicit promotion, so a mixed expression needs an explicit cast.
func TestMixedArithmeticIsCast(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

struct Pair {
    total: f64
    count: u32
}

fn mean(p: Pair) f64 {
    return p.total / p.count;
}

fn main() void {
}
`, "mean")
	text := instrText(fn)
	if !strings.Contains(text, "(double)p.count") {
		t.Fatalf("expected the u32 operand to be widened to double, got:\n%s", text)
	}
}

// A numeric literal is an untyped constant, so it adopts the other operand's
// type instead of forcing a promotion.
func TestLiteralAdoptsOtherOperandType(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn bump(count: u32) u32 {
    return count + 1;
}

fn main() void {
}
`, "bump")
	text := instrText(fn)
	if strings.Contains(text, "(int32_t)") {
		t.Fatalf("a literal must not drive promotion, got:\n%s", text)
	}
	if !strings.Contains(text, "count + 1") {
		t.Fatalf("expected an unpromoted addition, got:\n%s", text)
	}
}

func TestSameTypedArithmeticIsNotCast(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn add(a: f64, b: f64) f64 {
    return a + b;
}

fn main() void {
}
`, "add")
	if strings.Contains(instrText(fn), "(double)") {
		t.Fatalf("same-typed arithmetic needs no cast:\n%s", instrText(fn))
	}
}

// A numeric literal is an untyped constant that adapts to the other operand
// in both C and Go, so `count < 4` needs no cast even though the literal
// parses as a signed integer.
func TestLiteralComparisonIsNotCast(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn is_small(count: u32) bool {
    return count < 4;
}

fn main() void {
}
`, "is_small")
	if strings.Contains(instrText(fn), "(int32_t)") {
		t.Fatalf("a literal must not force a cast, got:\n%s", instrText(fn))
	}
	if !strings.Contains(instrText(fn), "count < 4") {
		t.Fatalf("expected the comparison unchanged, got:\n%s", instrText(fn))
	}
}

// A comparison between two named values of different numeric types does need
// a cast, otherwise Go rejects it.
func TestMixedNamedComparisonIsCast(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

struct Pair {
    total: f64
    count: u32
}

	fn is_small(p: Pair, limit: f64) bool {
    return p.count < limit;
}

fn main() void {
}
`, "is_small")
	if !strings.Contains(instrText(fn), "(double)p.count") {
		t.Fatalf("expected the u32 operand to be widened, got:\n%s", instrText(fn))
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Promotion Helpers                                                   */
/* -------------------------------------------------------------------------- */

func TestPromotedKind(t *testing.T) {
	if got := promotedKind(ir.TypeU32, ir.TypeF64); got != ir.TypeF64 {
		t.Errorf("u32/f64 should promote to f64, got %v", got)
	}
	if got := promotedKind(ir.TypeF64, ir.TypeU32); got != ir.TypeF64 {
		t.Errorf("f64/u32 should promote to f64, got %v", got)
	}
	if got := promotedKind(ir.TypeU32, ir.TypeU32); got != ir.TypeU32 {
		t.Errorf("equal kinds should be unchanged, got %v", got)
	}
}

func TestCTypeForKind(t *testing.T) {
	cases := map[ir.TypeKind]string{
		ir.TypeU32:  "uint32_t",
		ir.TypeI32:  "int32_t",
		ir.TypeF64:  "double",
		ir.TypeBool: "bool",
	}
	for kind, want := range cases {
		if got := cTypeForKind(kind); got != want {
			t.Errorf("kind %v: got %q, want %q", kind, got, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Call Site Naming                                                    */
/* -------------------------------------------------------------------------- */

// A function belonging to an imported module is emitted under its qualified
// name, so its call sites have to use the same name or the generated code does
// not link. This is the second half of the C generation failure: definitions
// were qualified while call sites were not.
func TestCallSiteUsesQualifiedCName(t *testing.T) {
	module := parseSource(t, `module app

fn main() void {
    let value: f64 = clamp_f64(1.0, 0.0, 1.0);
    print(value);
}
`)

	tc := New()
	// Register the function the way module loading does: under a qualified key
	// and the bare name, both carrying the qualified C name.
	entry := functionType{
		params: []ir.Type{
			{Kind: ir.TypeF64}, {Kind: ir.TypeF64}, {Kind: ir.TypeF64},
		},
		result: ir.Type{Kind: ir.TypeF64},
		cName:  "std_math_basic_clamp_f64",
	}
	tc.funcs["std.math.basic.clamp_f64"] = entry
	tc.funcs["clamp_f64"] = entry

	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("typecheck failed: %v", err)
	}
	out := tc.ToIR(*module)
	main := findFunc(t, out, "main")
	text := instrText(main)
	if !strings.Contains(text, "std_math_basic_clamp_f64(") {
		t.Fatalf("expected the call to use the qualified name, got:\n%s", text)
	}
}

// A function declared in the same module keeps its bare name, so ordinary
// calls are untouched.
func TestCallSiteKeepsLocalName(t *testing.T) {
	module := parseSource(t, `module app

fn clamp(value: f64) f64 {
    return value;
}

fn main() void {
    let out: f64 = clamp(1.0);
    print(out);
}
`)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("typecheck failed: %v", err)
	}
	out := tc.ToIR(*module)
	main := findFunc(t, out, "main")
	text := instrText(main)
	if !strings.Contains(text, "clamp(1.0)") {
		t.Fatalf("expected the bare name at the call site, got:\n%s", text)
	}
	if strings.Contains(text, "app_clamp") {
		t.Fatalf("a local function must not be qualified, got:\n%s", text)
	}
}
