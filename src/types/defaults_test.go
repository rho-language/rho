//
// @file defaults_test.go
// @brief Tests for type-less bindings and default type parameters.
//
// A `let` may omit its type annotation and take it from the value, and a
// generic parameter may declare a default so an application that cannot deduce
// it still compiles.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
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

// localType returns the lowered type name recorded for a local binding,
// which is what the backends consume.
func localType(t *testing.T, source, target string) (ir.Type, ir.ModuleIR) {
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
				return ir.Type{Kind: ir.TypeStruct, Name: instr.Type}, out
			}
		}
	}
	t.Fatalf("local %q not found in generated IR", target)
	return ir.Type{}, out
}

/* -------------------------------------------------------------------------- */
/*  Test: Type-less Bindings                                                  */
/* -------------------------------------------------------------------------- */

func TestTypeLessBindingInfersFromValue(t *testing.T) {
	got, _ := localType(t, `module app

fn main() void {
    let total = 1 + 2
    print(total);
}
`, "total")
	if got.Name != "int32_t" {
		t.Fatalf("expected the integer literal to lower to int32_t, got %#v", got)
	}
}

func TestTypeLessBindingInfersString(t *testing.T) {
	got, _ := localType(t, `module app

fn main() void {
    let label = "hi"
    print(label);
}
`, "label")
	if got.Name != "const char*" {
		t.Fatalf("expected a string binding to lower to const char*, got %#v", got)
	}
}

// A type-less binding to a generic application takes the deduced specialisation.
func TestTypeLessBindingInfersGeneric(t *testing.T) {
	got, _ := localType(t, `module app

struct Box[T] {
    value: T
}

fn wrap[T](v: T) Box[T] {
    return Box[T]{ v }
}

fn main() void {
    let b = wrap(1)
    print("done");
}
`, "b")
	if got.Name != "Box_i32" {
		t.Fatalf("expected the deduced Box_i32, got %#v", got)
	}
}

// An inferred local must be usable afterwards, which means the scope records
// its real type rather than an empty one.
func TestTypeLessBindingIsUsableAfterwards(t *testing.T) {
	module := parseSource(t, `module app

fn main() void {
    let total = 1 + 2
    if total == 3 {
        print("ok");
    }
}
`)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("an inferred local should be usable afterwards: %v", err)
	}
}

// A binding with neither an annotation nor a value has nothing to infer from.
func TestTypeLessBindingWithoutValueIsRejected(t *testing.T) {
	module := parseSource(t, `module app

fn main() void {
    let orphan
    print("done");
}
`)
	tc := New()
	err := tc.CheckModule(*module)
	if err == nil {
		t.Fatal("expected an error for a binding with no type and no value")
	}
	if !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("expected the binding named in the error, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Default Type Parameters                                             */
/* -------------------------------------------------------------------------- */

// A parameter with a default is filled when deduction cannot resolve it.
func TestDefaultTypeParameterFillsUndeduced(t *testing.T) {
	_, out := localType(t, `module app

struct Pair[A, B = Unit] {
    first: A
    second: B
}

fn make[A, B = Unit](a: A) Pair[A, B] {
    return Pair[A, B]{ a, default(B) }
}

fn main() void {
    let p = make(7)
    print("done");
}
`, "p")
	if !structNames(out)["Pair_i32_Unit"] {
		t.Fatalf("expected the default to appear in the mangled name, got %v", structNames(out))
	}
	if !hasFunc(out, "make_i32_Unit") {
		t.Fatalf("expected make_i32_Unit, got %v", funcNames(out))
	}
}

// A partial written application and the deduced one must mangle identically,
// otherwise the definition and the reference disagree.
func TestPartialApplicationMatchesDeducedMangling(t *testing.T) {
	_, out := localType(t, `module app

struct Pair[A, B = Unit] {
    first: A
    second: B
}

fn make[A, B = Unit](a: A) Pair[A, B] {
    return Pair[A, B]{ a, default(B) }
}

fn main() void {
    let p: Pair[u32] = make(9)
    print("done");
}
`, "p")
	names := structNames(out)
	if !names["Pair_u32_Unit"] {
		t.Fatalf("expected the written Pair[u32] to expand to Pair_u32_Unit, got %v", names)
	}
	if names["Pair_u32"] {
		t.Fatalf("a partial application must not mangle to a second name: %v", names)
	}
}

// The default type is synthesised so the backends have something to emit.
func TestDefaultTypeIsSynthesised(t *testing.T) {
	_, out := localType(t, `module app

struct Pair[A, B = Unit] {
    first: A
    second: B
}

fn main() void {
    let p: Pair[u32] = Pair[u32]{ 1, default(Unit) }
    print("done");
}
`, "p")
	if !structNames(out)["Unit"] {
		t.Fatalf("expected a Unit definition, got %v", structNames(out))
	}
}

// A default type the module defines itself is used as written, not synthesised.
func TestModuleDefinedDefaultTypeIsNotSynthesised(t *testing.T) {
	_, out := localType(t, `module app

struct Opaque {
    tag: u32
}

struct Pair[A, B = Opaque] {
    first: A
    second: B
}

fn main() void {
    let p: Pair[u32] = Pair[u32]{ 1, Opaque{ 0 } }
    print("done");
}
`, "p")
	// Opaque is declared, so no second definition may be emitted for it.
	count := 0
	for _, s := range out.Structs {
		if s.Name == "Opaque" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one Opaque definition, got %d", count)
	}
	if structNames(out)["Pair_u32_Opaque"] != true {
		t.Fatalf("expected Pair_u32_Opaque, got %v", structNames(out))
	}
}

// A parameter with neither a binding nor a default is still an error.
func TestParameterWithoutDefaultStillReported(t *testing.T) {
	module := parseSource(t, `module app

struct Pair[A, B] {
    first: A
    second: B
}

fn make[A, B](a: A) Pair[A, B] {
    return Pair[A, B]{ a, default(B) }
}

fn main() void {
    make(7)
    print("done");
}
`)
	tc := New()
	err := tc.CheckModule(*module)
	if err == nil {
		t.Fatal("expected an error for a parameter with no default")
	}
	if !strings.Contains(err.Error(), "B") {
		t.Fatalf("expected the unresolved parameter named, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Helpers                                                              */
/* -------------------------------------------------------------------------- */

func TestApplyDefaultsFillsOnlyUnbound(t *testing.T) {
	tc := New()
	subs := map[string]ir.Type{"A": {Kind: ir.TypeU32, Name: "u32"}}
	tc.applyDefaults([]string{"A", "B"}, map[string]string{"B": "Unit"}, subs)
	if subs["A"].Name != "u32" {
		t.Fatalf("an existing binding must not be overwritten, got %#v", subs["A"])
	}
	if _, ok := subs["B"]; !ok {
		t.Fatalf("expected B filled from its default, got %v", subs)
	}
}

func TestUndeducedParamsIgnoresDefaults(t *testing.T) {
	tc := New()
	subs := map[string]ir.Type{"A": {Kind: ir.TypeU32, Name: "u32"}}
	got := tc.undeducedParams([]string{"A", "B"}, map[string]string{"B": "Unit"}, subs)
	if len(got) != 0 {
		t.Fatalf("expected no unresolved parameters, got %v", got)
	}
	got = tc.undeducedParams([]string{"A", "B", "C"}, nil, subs)
	if len(got) != 2 {
		t.Fatalf("expected B and C unresolved, got %v", got)
	}
}

func TestExpandWithDefaults(t *testing.T) {
	tc := New()
	partial := []ir.Type{{Kind: ir.TypeU32, Name: "u32"}}
	full := tc.expandWithDefaults([]string{"A", "B"}, map[string]string{"B": "Unit"}, partial)
	if full == nil || len(full) != 2 {
		t.Fatalf("expected the application completed, got %v", full)
	}
	if full[1].Name != "Unit" {
		t.Fatalf("expected B defaulted to Unit, got %#v", full[1])
	}
	// A parameter with no default cannot be completed.
	if got := tc.expandWithDefaults([]string{"A", "B"}, nil, partial); got != nil {
		t.Fatalf("expected nil when a parameter has no default, got %v", got)
	}
}
