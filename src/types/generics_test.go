//
// @file generics_test.go
// @brief Tests for generic instantiation and monomorphisation.
//
// Generic declarations are templates. Each concrete application is expanded
// into its own specialised struct and function, with type parameters
// substituted and the name mangled so the C and Go backends emit one
// definition per instantiation.
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
/*  Test: Name Mangling                                                        */
/* -------------------------------------------------------------------------- */

// Mangled names have to be valid identifiers in both C and Go.
func TestMangleTypeName(t *testing.T) {
	cases := map[string]string{
		"Result[u32, u32]":   "Result_u32_u32",
		"Option[u32]":        "Option_u32",
		"PID":                "PID",
		"Box[string]":        "Box_string",
		"Pair[u32, Ptr[u8]]": "Pair_u32_Ptr_u8",
		"":                   "",
	}
	for in, want := range cases {
		if got := mangleTypeName(in); got != want {
			t.Errorf("mangleTypeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMangleGeneric(t *testing.T) {
	if got := mangleGeneric("Ok", nil); got != "Ok" {
		t.Errorf("a call with no type arguments should keep its name, got %q", got)
	}
	got := mangleGeneric("Ok", []ir.Type{{Kind: ir.TypeU32, Name: "u32"}, {Kind: ir.TypeU32, Name: "u32"}})
	if got != "Ok_u32_u32" {
		t.Errorf("got %q, want %q", got, "Ok_u32_u32")
	}
}

// Two applications of the same generic must mangle differently, otherwise the
// emitted definitions would collide.
func TestMangleDistinguishesArguments(t *testing.T) {
	a := mangleGeneric("Box", []ir.Type{{Kind: ir.TypeU32, Name: "u32"}})
	b := mangleGeneric("Box", []ir.Type{{Kind: ir.TypeString, Name: "string"}})
	if a == b {
		t.Fatalf("distinct applications mangled identically: %q", a)
	}
}

func TestSplitTypeArgs(t *testing.T) {
	cases := map[string][]string{
		"u32":                  {"u32"},
		"u32, u32":             {"u32", "u32"},
		"Option[u32], E":       {"Option[u32]", "E"},
		"Map[string, u32]":     {"Map[string, u32]"},
		"Option[u32, Ptr[u8]]": {"Option[u32, Ptr[u8]]"},
	}
	for in, want := range cases {
		got := splitTypeArgs(in)
		if len(got) != len(want) {
			t.Fatalf("splitTypeArgs(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitTypeArgs(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Substitution                                                         */
/* -------------------------------------------------------------------------- */

// A direct type parameter hit is a map lookup.
func TestSubstituteTypeDirect(t *testing.T) {
	tc := New()
	subs := map[string]ir.Type{"T": {Kind: ir.TypeU32, Name: "u32"}}
	got := tc.substituteType(ir.Type{Kind: ir.TypeStruct, Name: "T"}, subs)
	if got.Name != "u32" {
		t.Fatalf("got %#v", got)
	}
}

// A parameter buried inside another application must still be replaced.
func TestSubstituteTypeNested(t *testing.T) {
	tc := New()
	subs := map[string]ir.Type{"E": {Kind: ir.TypeString, Name: "string"}}
	got := tc.substituteType(ir.Type{Kind: ir.TypeStruct, Name: "Result[Option[u32], E]"}, subs)
	if !strings.Contains(got.Name, "string") {
		t.Fatalf("expected E replaced inside the application, got %q", got.Name)
	}
	if !strings.Contains(got.Name, "Option[u32]") {
		t.Fatalf("expected the concrete inner application preserved, got %q", got.Name)
	}
}

func TestSubstituteTypeNoOp(t *testing.T) {
	tc := New()
	// A non-struct type is never a type parameter.
	if got := tc.substituteType(ir.Type{Kind: ir.TypeF64, Name: "f64"}, map[string]ir.Type{"T": {Name: "u32"}}); got.Name != "f64" {
		t.Fatalf("got %#v", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Monomorphisation                                                     */
/* -------------------------------------------------------------------------- */

// One application produces exactly one specialised struct.
func TestMonomorphiseEmitsOnePerApplication(t *testing.T) {
	out := compileGeneric(t, `module app

struct Box[T] {
    value: T
}

fn use() void {
    let a: Box[u32] = Box[u32]{ 1 }
    let b: Box[string] = Box[string]{ "x" }
    print("done");
}
`)
	names := structNames(out)
	if len(names) != 2 {
		t.Fatalf("expected two specialisations, got %v", names)
	}
	if !names["Box_u32"] || !names["Box_string"] {
		t.Fatalf("expected Box_u32 and Box_string, got %v", names)
	}
}

// The same application used many times is emitted once.
func TestMonomorphiseDeduplicates(t *testing.T) {
	out := compileGeneric(t, `module app

struct Box[T] {
    value: T
}

fn use() void {
    let a: Box[u32] = Box[u32]{ 1 }
    let b: Box[u32] = Box[u32]{ 2 }
    let c: Box[u32] = Box[u32]{ 3 }
    print("done");
}
`)
	if got := len(structNames(out)); got != 1 {
		t.Fatalf("expected one specialisation, got %d", got)
	}
}

// Each specialisation substitutes the field types.
func TestMonomorphiseSubstitutesFields(t *testing.T) {
	out := compileGeneric(t, `module app

struct Box[T] {
    value: T
}

fn use() void {
    let a: Box[string] = Box[string]{ "x" }
    print("done");
}
`)
	for _, s := range out.Structs {
		if s.Name != "Box_string" {
			continue
		}
		if s.Fields[0].Type.Name != "string" {
			t.Fatalf("expected the field substituted to string, got %#v", s.Fields[0].Type)
		}
		return
	}
	t.Fatalf("Box_string was not emitted: %v", structNames(out))
}

// A generic function is specialised per application too.
func TestMonomorphiseGenericFunction(t *testing.T) {
	out := compileGeneric(t, `module app

fn twice[T](v: T) T {
    return v
}

fn use() void {
    let a: u32 = twice[u32](1)
    let b: string = twice[string]("x")
    print("done");
}
`)
	found := map[string]bool{}
	for _, fn := range out.Funcs {
		found[fn.Name] = true
	}
	if !found["twice_u32"] || !found["twice_string"] {
		t.Fatalf("expected both function specialisations, got %v", found)
	}
}

// A specialisation substitutes parameter and return types.
func TestMonomorphiseFunctionSignature(t *testing.T) {
	out := compileGeneric(t, `module app

fn twice[T](v: T) T {
    return v
}

fn use() void {
    let a: string = twice[string]("x")
    print("done");
}
`)
	for _, fn := range out.Funcs {
		if fn.Name != "twice_string" {
			continue
		}
		if fn.Params[0].Type.Name != "string" || fn.ReturnType.Name != "string" {
			t.Fatalf("expected string parameter and return, got %+v", fn)
		}
		return
	}
	t.Fatal("twice_string was not emitted")
}

// An application discovered inside a specialisation is itself expanded, which
// is what makes a generic calling another generic work.
func TestMonomorphiseNestedInstantiation(t *testing.T) {
	out := compileGeneric(t, `module app

struct Inner[T] {
    value: T
}

struct Outer[T] {
    inner: Inner[T]
}

fn use() void {
    let a: Outer[u32] = Outer[u32]{ Inner[u32]{ 1 } }
    print("done");
}
`)
	names := structNames(out)
	if !names["Inner_u32"] || !names["Outer_u32"] {
		t.Fatalf("expected nested expansion of both, got %v", names)
	}
}

// An application only reachable through a generic function is still emitted.
func TestMonomorphiseReachesThroughGenericFunction(t *testing.T) {
	out := compileGeneric(t, `module app

struct Box[T] {
    value: T
}

fn make[T](v: T) Box[T] {
    return Box[T]{ v }
}

fn use() void {
    let a: Box[u32] = make[u32](1)
    print("done");
}
`)
	if !structNames(out)["Box_u32"] {
		t.Fatalf("expected Box_u32 to be reached through make[u32], got %v", structNames(out))
	}
}

// A generic that is never applied emits nothing.
func TestMonomorphiseSkipsUnusedGeneric(t *testing.T) {
	out := compileGeneric(t, `module app

struct Box[T] {
    value: T
}

fn use() void {
    print("done");
}
`)
	if got := len(structNames(out)); got != 0 {
		t.Fatalf("expected an unused generic to emit nothing, got %d", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: default(T)                                                           */
/* -------------------------------------------------------------------------- */

// default(T) is the zero value of T, which Result and Option depend on.
//
// A struct's zero value is written with empty braces rather than with a single
// zero: `{0}` is how C zeroes a struct's first member and means nothing in Go,
// where a literal has to name every field it sets. `{}` is valid in both.
func TestZeroValueFor(t *testing.T) {
	tc := New()
	// A specialisation with fields is zeroed; the caller registers them.
	tc.fields["Option_u32"] = map[string]ir.Type{"value": {Kind: ir.TypeU32, Name: "u32"}}
	cases := map[string]string{
		"bool":        "false",
		"u32":         "0",
		"i32":         "0",
		"f64":         "0",
		"string":      `""`,
		"Option[u32]": "(Option_u32){}",
	}
	for in, want := range cases {
		if got := tc.zeroValueFor(in); got != want {
			t.Errorf("zeroValueFor(%q) = %q, want %q", in, got, want)
		}
	}
}

// A struct with no fields, such as the synthesised Unit used as a generic
// parameter default, is initialised empty rather than with an excess element.
func TestZeroValueForEmptyStruct(t *testing.T) {
	tc := New()
	tc.fields["Unit"] = map[string]ir.Type{}
	if got := tc.zeroValueFor("Unit"); got != "(Unit){}" {
		t.Fatalf("got %q, want %q", got, "(Unit){}")
	}
}

// default(T) lowers to the zero value of T rather than a call.
func TestDefaultLowersToZeroValue(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn main() void {
    let n: u32 = default(u32)
    print(n);
}
`, "main")
	text := instrText(fn)
	if !strings.Contains(text, "uint32_t 0") {
		t.Fatalf("expected default(u32) to lower to 0, got: %s", text)
	}
	if strings.Contains(text, "default(") {
		t.Fatalf("default must not survive as a call, got: %s", text)
	}
}

func TestDefaultStringLowersToNull(t *testing.T) {
	fn := lowerFunctionToIR(t, `module app

fn main() void {
    let s: string = default(string)
    print(s);
}
`, "main")
	if !strings.Contains(instrText(fn), `const char* ""`) {
		t.Fatalf("expected default(string) to lower to an empty string, got: %s", instrText(fn))
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// compileGeneric parses, checks and lowers a module, returning its IR.
func compileGeneric(t *testing.T, source string) ir.ModuleIR {
	t.Helper()
	module := parseSource(t, source)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("typecheck failed: %v\n%s", err, source)
	}
	return tc.ToIR(*module)
}

func structNames(module ir.ModuleIR) map[string]bool {
	names := map[string]bool{}
	for _, s := range module.Structs {
		names[s.Name] = true
	}
	return names
}

// Two applications of the same generic can mangle to the same name, and they
// are the same type: `Option[Ptr[Thing]]` written in a source and the mangled
// `Option_Ptr_Thing` a foreign declaration carries are one type reached two
// ways, seeded as two instantiations because their arguments are spelled
// differently. A backend emits one definition per name, so the second would be
// a second definition of the first — and the C one rejects that as a
// duplicate.
func TestMonomorphiseEmitsOnePerName(t *testing.T) {
	out := compileGeneric(t, `module app

struct Thing {
    value: u32
}

struct Option[T] {
    is_some: bool
    value: T
}

extern fn get_thing(name: Ptr[u8]) Option[Ptr[Thing]]

fn use() void {
    let a: Option[Ptr[Thing]] = Option[Ptr[Thing]]{ false, 0 }
    print("done");
}
`)
	count := 0
	for _, structure := range out.Structs {
		if structure.Name == "Option_Ptr_Thing" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one Option_Ptr_Thing, got %d: %v", count, structNames(out))
	}
}

// Two *different* types that mangle to the same name are a collision, and both
// are kept: dropping one of them would leave a program that compiles and means
// something other than it says. The backend's own duplicate check is what
// reports them.
func TestTwoTypesWithOneNameAreBothKept(t *testing.T) {
	out := compileGeneric(t, `module app

struct Thing {
    value: u32
}

struct Option[T] {
    is_some: bool
    value: T
}

struct Option_Ptr_Thing {
    other: u32
}

fn use() void {
    let a: Option[Ptr[Thing]] = Option[Ptr[Thing]]{ false, 0 }
    print("done");
}
`)
	count := 0
	for _, structure := range out.Structs {
		if structure.Name == "Option_Ptr_Thing" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected both types with the name to be kept, got %d: %v", count, structNames(out))
	}
}
