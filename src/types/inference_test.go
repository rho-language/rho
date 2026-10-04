//
// @file inference_test.go
// @brief Tests for generic type-argument inference.
//
// An application written without type arguments is deduced from its field
// values, its call arguments, or the expected type at the use site. Both the
// checking and the lowering pass use one shared resolver, so the deduced
// arguments and the emitted specialisation name cannot diverge.
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

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// deduce compiles a program and returns the type of the local named target,
// which is where an inferred application shows up.
func deduce(t *testing.T, source, target string) (ir.Type, ir.ModuleIR) {
	t.Helper()
	module := parseSource(t, source)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("typecheck failed: %v\n%s", err, source)
	}
	out := tc.ToIR(*module)

	for _, fn := range out.Funcs {
		if fn.Name != "main" {
			continue
		}
		for _, instr := range fn.Body {
			if instr.Op == ir.OpLet && instr.Dest == target {
				return tc.typeFor(instr.Type), out
			}
		}
	}
	t.Fatalf("local %q not found in generated IR", target)
	return ir.Type{}, out
}

/* -------------------------------------------------------------------------- */
/*  Test: Deduction From Field Values                                         */
/* -------------------------------------------------------------------------- */

func TestInferStructFromFieldValue(t *testing.T) {
	got, _ := deduce(t, `module app

struct Box[T] {
    value: T
}

fn main() void {
    let a: Box[i32] = Box{ 1 }
    print("done");
}
`, "a")
	if got.Name != "Box_i32" {
		t.Fatalf("expected Box_i32 deduced from the field value, got %#v", got)
	}
}

func TestInferStructFromStringField(t *testing.T) {
	got, _ := deduce(t, `module app

struct Box[T] {
    value: T
}

fn main() void {
    let b: Box[string] = Box{ "hi" }
    print("done");
}
`, "b")
	if got.Name != "Box_string" {
		t.Fatalf("expected Box_string, got %#v", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Deduction From Call Arguments                                       */
/* -------------------------------------------------------------------------- */

func TestInferCallFromArgument(t *testing.T) {
	_, out := deduce(t, `module app

struct Box[T] {
    value: T
}

fn wrap[T](v: T) Box[T] {
    return Box[T]{ v }
}

fn main() void {
    let c: Box[i32] = wrap(1)
    print("done");
}
`, "c")
	if !structNames(out)["Box_i32"] {
		t.Fatalf("expected Box_i32 to be reached through wrap(1), got %v", structNames(out))
	}
	if !hasFunc(out, "wrap_i32") {
		t.Fatalf("expected the wrap specialisation, got %v", funcNames(out))
	}
}

// A parameter used twice must deduce consistently.
func TestInferCallFromRepeatedParameter(t *testing.T) {
	_, out := deduce(t, `module app

fn pair[T](a: T, b: T) T {
    return a
}

fn main() void {
    let x: u32 = pair(1, 2)
    print(x);
}
`, "x")
	if !hasFunc(out, "pair_u32") {
		t.Fatalf("expected pair_u32, got %v", funcNames(out))
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: The Expected Type Wins                                              */
/* -------------------------------------------------------------------------- */

// A literal alone deduces i32, but the declared type pins the argument.
func TestExpectedTypeOverridesLiteralDefault(t *testing.T) {
	got, _ := deduce(t, `module app

struct Box[T] {
    value: T
}

fn main() void {
    let d: Box[u32] = Box{ 1 }
    print("done");
}
`, "d")
	if got.Name != "Box_u32" {
		t.Fatalf("expected the declared Box_u32 to win over a literal default, got %#v", got)
	}
}

// The expected type supplies a parameter the arguments cannot, which is what
// makes `Ok(7)` work when the binding says Result[u32, u32].
func TestExpectedTypeSuppliesUndeducibleParameter(t *testing.T) {
	_, out := deduce(t, `module app

struct Result[T, E] {
    is_ok: bool
    ok: T
    err: E
}

fn Ok[T, E](v: T) Result[T, E] {
    return Result[T, E]{ true, v, default(E) }
}

fn main() void {
    let good: Result[u32, u32] = Ok(7)
    print("done");
}
`, "good")
	if !structNames(out)["Result_u32_u32"] {
		t.Fatalf("expected Result_u32_u32, got %v", structNames(out))
	}
	if !hasFunc(out, "Ok_u32_u32") {
		t.Fatalf("expected Ok_u32_u32, got %v", funcNames(out))
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Checking and Lowering Agree                                          */
/* -------------------------------------------------------------------------- */

// The mangled name in the emitted IR must match the type the checker inferred,
// otherwise the definition and the reference disagree.
func TestInferenceAgreesAcrossPasses(t *testing.T) {
	_, out := deduce(t, `module app

struct Box[T] {
    value: T
}

fn main() void {
    let a: Box[u32] = Box{ 1 }
    if a.value == 1 {
        print("ok");
    }
}
`, "a")
	if !structNames(out)["Box_u32"] {
		t.Fatalf("expected Box_u32 to be emitted, got %v", structNames(out))
	}
}

// Member access on an inferred local must type check, which means the
// specialisation's fields are reachable under its mangled name.
func TestMemberAccessOnInferredLocal(t *testing.T) {
	module := parseSource(t, `module app

struct Box[T] {
    value: T
}

fn main() void {
    let a: Box[string] = Box{ "hi" }
    if a.value == "hi" {
        print("ok");
    }
}
`)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("member access on an inferred local should type check: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Undeducible Applications Are Reported                               */
/* -------------------------------------------------------------------------- */

// A parameter with no source cannot be guessed, so the error names it.
func TestUndeducibleParameterIsReported(t *testing.T) {
	module := parseSource(t, `module app

struct Result[T, E] {
    is_ok: bool
    ok: T
    err: E
}

fn Ok[T, E](v: T) Result[T, E] {
    return Result[T, E]{ true, v, default(E) }
}

fn main() void {
    Ok(7)
    print("done");
}
`)
	tc := New()
	err := tc.CheckModule(*module)
	if err == nil {
		t.Fatal("expected an error for an undeducible type argument")
	}
	if !strings.Contains(err.Error(), "E") {
		t.Fatalf("expected the unresolved parameter named, got %v", err)
	}
	if !strings.Contains(err.Error(), "explicitly") {
		t.Fatalf("expected the message to suggest explicit arguments, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unification                                                          */
/* -------------------------------------------------------------------------- */

func TestUnifyBindsParameter(t *testing.T) {
	subs := map[string]ir.Type{}
	param := ir.Type{Kind: ir.TypeStruct, Name: "T"}
	actual := ir.Type{Kind: ir.TypeU32, Name: "u32"}
	if !unify(param, actual, []string{"T"}, subs) {
		t.Fatal("expected unification to succeed")
	}
	if subs["T"].Name != "u32" {
		t.Fatalf("expected T bound to u32, got %#v", subs["T"])
	}
}

// Binding a parameter twice with different types must not silently succeed.
func TestUnifyRejectsConflictingBinding(t *testing.T) {
	subs := map[string]ir.Type{"T": {Kind: ir.TypeU32, Name: "u32"}}
	param := ir.Type{Kind: ir.TypeStruct, Name: "T"}
	if unify(param, ir.Type{Kind: ir.TypeString, Name: "string"}, []string{"T"}, subs) {
		t.Fatal("expected a conflicting rebinding to be rejected")
	}
}

func TestUnifyNestedApplication(t *testing.T) {
	subs := map[string]ir.Type{}
	param := ir.Type{Kind: ir.TypeStruct, Name: "Result[Option[T], E]"}
	actual := ir.Type{Kind: ir.TypeStruct, Name: "Result[Option[u32], string]"}
	if !unify(param, actual, []string{"T", "E"}, subs) {
		t.Fatal("expected nested unification to succeed")
	}
	if subs["T"].Name != "u32" {
		t.Errorf("expected T=u32, got %#v", subs["T"])
	}
	if subs["E"].Name != "string" {
		t.Errorf("expected E=string, got %#v", subs["E"])
	}
}

func TestUnifyRejectsMismatchedBase(t *testing.T) {
	subs := map[string]ir.Type{}
	param := ir.Type{Kind: ir.TypeStruct, Name: "Result[T, E]"}
	actual := ir.Type{Kind: ir.TypeStruct, Name: "Option[u32]"}
	if unify(param, actual, []string{"T", "E"}, subs) {
		t.Fatal("expected mismatched generic bases to be rejected")
	}
}

func TestOrderedArgsRequiresEveryParameter(t *testing.T) {
	tc := New()
	if got := tc.orderedArgs([]string{"T", "E"}, nil, map[string]ir.Type{"T": {Name: "u32"}}); got != nil {
		t.Fatalf("expected nil when a parameter is unbound, got %v", got)
	}
	got := tc.orderedArgs([]string{"T", "E"}, nil, map[string]ir.Type{
		"E": {Kind: ir.TypeU32, Name: "u32"},
		"T": {Kind: ir.TypeU32, Name: "u32"},
	})
	if len(got) != 2 || got[0].Name != "u32" || got[1].Name != "u32" {
		t.Fatalf("expected both arguments in declaration order, got %v", got)
	}
}

func TestSplitApplication(t *testing.T) {
	base, args, ok := splitApplication("Result[u32, string]")
	if !ok || base != "Result" || len(args) != 2 {
		t.Fatalf("unexpected split: %q %v %v", base, args, ok)
	}
	if args[0].Name != "u32" || args[1].Name != "string" {
		t.Fatalf("unexpected args: %v", args)
	}
	if _, _, ok := splitApplication("PID"); ok {
		t.Fatal("a plain name is not an application")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func funcNames(module ir.ModuleIR) []string {
	out := make([]string, 0, len(module.Funcs))
	for _, fn := range module.Funcs {
		out = append(out, fn.Name)
	}
	return out
}

func hasFunc(module ir.ModuleIR, name string) bool {
	for _, fn := range module.Funcs {
		if fn.Name == name {
			return true
		}
	}
	return false
}
