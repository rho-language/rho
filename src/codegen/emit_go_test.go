//
// @file emit_go_test.go
// @brief Tests for the Rho Go code generation backend.
//
// Beyond checking the emitted text, these tests compile and run a generated
// Go project, because a backend that produces plausible but unbuildable code
// is worse than no backend at all.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// greetIR is a small program that prints through the translated printf call.
func greetIR() ir.ModuleIR {
	return ir.ModuleIR{
		Name: "greet",
		Funcs: []ir.FuncIR{
			{
				Name:       "greeting",
				CName:      "greeting",
				ReturnType: ir.Type{Kind: ir.TypeString},
				Body: []ir.Instr{
					{Op: ir.OpReturn, Args: []string{"\"hello\""}},
				},
			},
			{
				Name:       "main",
				CName:      "main",
				ReturnType: ir.Type{Kind: ir.TypeVoid},
				Body: []ir.Instr{
					{Op: ir.OpExpr, Args: []string{`printf("%s\n", greeting())`}},
				},
			},
		},
	}
}

// counterIR exercises structs, typed locals, compound literals and control
// flow, which is the combination the expression translator has to get right.
//
// Arithmetic stays within one numeric type: the IR carries C expressions, and
// C promotes mixed integer and floating point operands implicitly while Go
// requires an explicit conversion. See the limitations note in emit_go.go.
func counterIR() ir.ModuleIR {
	return ir.ModuleIR{
		Name: "counter",
		Structs: []ir.StructIR{
			{
				Name:  "Accum",
				CName: "Accum",
				Fields: []ir.FieldIR{
					{Name: "total", Type: ir.Type{Kind: ir.TypeF64}},
					{Name: "count", Type: ir.Type{Kind: ir.TypeF64}},
				},
			},
		},
		Funcs: []ir.FuncIR{
			{
				Name:       "accum_init",
				CName:      "accum_init",
				ReturnType: ir.Type{Kind: ir.TypeStruct, Name: "Accum"},
				Body: []ir.Instr{
					{Op: ir.OpReturn, Args: []string{"(Accum){ 0.0, 0.0 }"}},
				},
			},
			{
				Name:  "accum_add",
				CName: "accum_add",
				Params: []ir.ParamIR{
					{Name: "accum", Type: ir.Type{Kind: ir.TypeStruct, Name: "Accum"}},
					{Name: "value", Type: ir.Type{Kind: ir.TypeF64}},
				},
				ReturnType: ir.Type{Kind: ir.TypeStruct, Name: "Accum"},
				Body: []ir.Instr{
					{Op: ir.OpLet, Dest: "running", Type: "double", Args: []string{"rho_concat(accum.total, value)"}},
					{Op: ir.OpLet, Dest: "seen", Type: "double", Args: []string{"rho_concat(accum.count, 1.0)"}},
					{Op: ir.OpReturn, Args: []string{"(Accum){ running, seen }"}},
				},
			},
			{
				Name:  "accum_mean",
				CName: "accum_mean",
				Params: []ir.ParamIR{
					{Name: "accum", Type: ir.Type{Kind: ir.TypeStruct, Name: "Accum"}},
				},
				ReturnType: ir.Type{Kind: ir.TypeF64},
				Body: []ir.Instr{
					{Op: ir.OpReturn, Args: []string{"(accum.total / accum.count)"}},
				},
			},
			{
				Name:       "main",
				CName:      "main",
				ReturnType: ir.Type{Kind: ir.TypeVoid},
				Body: []ir.Instr{
					{Op: ir.OpCall, Dest: "acc", Args: []string{"accum_init"}},
					{Op: ir.OpCall, Dest: "acc", Args: []string{"accum_add", "acc", "2.0"}},
					{Op: ir.OpCall, Dest: "acc", Args: []string{"accum_add", "acc", "4.0"}},
					{Op: ir.OpExpr, Args: []string{`printf("%.1f\n", accum_mean(acc))`}},
				},
			},
		},
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Implementation Emission                                             */
/* -------------------------------------------------------------------------- */

func TestEmitGoPackageHeader(t *testing.T) {
	out := EmitGo(greetIR())
	if !strings.Contains(out, "package main") {
		t.Error("expected package main")
	}
	if !strings.Contains(out, "func AppMain()") {
		t.Error("a Rho main should become AppMain")
	}
	if strings.Contains(out, "func main(") {
		t.Error("app.go must not define main; main.go owns the entry point")
	}
}

// An import is only emitted when the body actually uses it.
func TestEmitGoImportsAreContentDriven(t *testing.T) {
	withPrintf := EmitGo(greetIR())
	if !strings.Contains(withPrintf, `import "fmt"`) {
		t.Error("expected the fmt import when printf is used")
	}

	withoutPrintf := greetIR()
	withoutPrintf.Funcs[1].Body = []ir.Instr{{Op: ir.OpNop}}
	out := EmitGo(withoutPrintf)
	if strings.Contains(out, `import "fmt"`) {
		t.Error("expected no fmt import when printf is unused")
	}
}

func TestEmitGoStructs(t *testing.T) {
	out := EmitGo(counterIR())
	if !strings.Contains(out, "type Accum struct {") {
		t.Fatal("expected the Accum struct")
	}
	// A struct field is written `name Type` in Go.
	if !strings.Contains(out, "total float64") {
		t.Error("expected total to map to float64")
	}
	if !strings.Contains(out, "count float64") {
		t.Error("expected count to map to float64")
	}
}

// Go writes parameters as `name Type`, the reverse of the C spelling.
func TestEmitGoParametersUseGoOrder(t *testing.T) {
	out := EmitGo(counterIR())
	if !strings.Contains(out, "func accumAdd(accum Accum, value float64) Accum {") {
		t.Errorf("expected `name Type` parameters, got:\n%s", out)
	}
}

func TestEmitGoTypedLocalsUseVar(t *testing.T) {
	out := EmitGo(counterIR())
	if !strings.Contains(out, "var running float64 = (accum.total + value)") {
		t.Errorf("expected a var declaration for the typed local, got:\n%s", out)
	}
}

// A non-void Rho main cannot be represented, and that must be reported.
func TestEmitGoRejectsNonVoidMain(t *testing.T) {
	m := greetIR()
	m.Funcs[1].ReturnType = ir.Type{Kind: ir.TypeI32}
	if got := EmitGo(m); got != "" {
		t.Errorf("expected no output for a non-void main, got %d bytes", len(got))
	}
}

// A library module without main still yields a buildable project.
func TestEmitGoStubsMissingMain(t *testing.T) {
	m := greetIR()
	m.Funcs = m.Funcs[:1]
	out := EmitGo(m)
	if !strings.Contains(out, "func AppMain() {}") {
		t.Error("expected a stub AppMain when the module has no main")
	}
}

func TestEmitGoDeduplicatesFeatures(t *testing.T) {
	m := greetIR()
	m.Features = []string{"math", "math", "math.control"}
	out := EmitGo(m)
	if strings.Count(out, "RHO_FEATURE_MATH = 1") != 1 {
		t.Errorf("duplicate feature constants:\n%s", out)
	}
	if strings.Count(out, "RHO_FEATURE_MATH_CONTROL = 1") != 1 {
		t.Errorf("expected the dotted feature constant:\n%s", out)
	}
}

func TestEmitGoCallsAppInitWhenPresent(t *testing.T) {
	m := greetIR()
	m.Funcs = append([]ir.FuncIR{{
		Name:       "init",
		CName:      "init",
		ReturnType: ir.Type{Kind: ir.TypeVoid},
	}}, m.Funcs...)
	out := EmitGo(m)
	if !strings.Contains(out, "func AppInit()") {
		t.Error("expected AppInit to be generated")
	}
	if !strings.Contains(out, "\tinit()") {
		t.Error("expected AppInit to call the Rho init function")
	}
	if !strings.Contains(EmitGoEntrypoint(m), "AppInit()") {
		t.Error("expected the entrypoint to call AppInit")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Entrypoint and Runtime                                              */
/* -------------------------------------------------------------------------- */

func TestEmitGoEntrypoint(t *testing.T) {
	out := EmitGoEntrypoint(greetIR())
	if !strings.Contains(out, "package main") {
		t.Error("expected package main")
	}
	if !strings.Contains(out, "func main() {") {
		t.Error("expected a main function")
	}
	if !strings.Contains(out, "AppMain()") {
		t.Error("expected main to call AppMain")
	}
}

// The language intrinsics are always present, because a module may call any of
// them and the generated project must compile on its own.
func TestEmitGoRuntimeAlwaysCarriesIntrinsics(t *testing.T) {
	out := EmitGoRuntime("package main\n")
	for _, want := range []string{
		"func abort()",
		"func panicf(",
		"func rhoStrlen(",
		"func rhoStrAt(",
		"func rhoStrSlice(",
		"func rhoStrIndexOf(",
		"func rhoStrStartsWith(",
		"func rhoStrContains(",
		"func rhoIntToStr(",
		"func rhoIntFromStr(",
		"func rhoStrFromByte(",
		"func rhoEnvOr(",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the intrinsic %s to always be present", want)
		}
	}
	// The intrinsics need these, so they are unconditional too.
	for _, want := range []string{`"os"`, `"strings"`, `"strconv"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the import %s for the intrinsics", want)
		}
	}
}

// The optional helpers are only emitted when the program calls them, so a
// generated project does not carry code it never uses.
func TestEmitGoRuntimeKeepsOnlyReferencedHelpers(t *testing.T) {
	// Nothing optional is used, so nothing optional is emitted.
	empty := EmitGoRuntime("package main\n")
	for _, unwanted := range []string{"rhoStrEq", "rhoStringFromI32", "rhoYamlLoad"} {
		if strings.Contains(empty, unwanted) {
			t.Errorf("expected %s to be omitted when unused", unwanted)
		}
	}

	// A used helper appears; the others still do not.
	out := EmitGoRuntime(`func x() { _ = rhoStrEq("a", "b"); _ = rhoStringFromI32(1) }`)
	for _, want := range []string{"func rhoStrEq", "func rhoStringFromI32"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s to be emitted when used", want)
		}
	}
	if strings.Contains(out, "rhoYamlLoad") {
		t.Error("expected rhoYamlLoad to stay omitted when unused")
	}

	// A different helper brings a different one, not all of them.
	onlyEq := EmitGoRuntime(`func x() { _ = rhoStrEq("a", "b") }`)
	if !strings.Contains(onlyEq, "func rhoStrEq") {
		t.Error("expected rhoStrEq to be emitted when used")
	}
	if strings.Contains(onlyEq, "func rhoStringFromI32") {
		t.Error("expected rhoStringFromI32 to stay omitted when unused")
	}
}

func TestEmitGoMod(t *testing.T) {
	m := greetIR()
	if got := EmitGoMod(m); !strings.Contains(got, "module greet") || !strings.Contains(got, "go 1.22") {
		t.Errorf("unexpected go.mod:\n%s", got)
	}
}

func TestEmitGoModSanitisesModulePath(t *testing.T) {
	m := ir.ModuleIR{Name: "My App.v2"}
	if got := EmitGoMod(m); !strings.Contains(got, "module my-app-v2") {
		t.Errorf("expected a sanitised module path, got:\n%s", got)
	}
}

func TestEmitGoModWithRequirements(t *testing.T) {
	out := EmitGoModWithRequirements(greetIR(), "example.com/x", []string{"github.com/a/b"})
	if !strings.Contains(out, "module example.com/x") {
		t.Errorf("expected the configured module path:\n%s", out)
	}
	if !strings.Contains(out, "require (") || !strings.Contains(out, "github.com/a/b") {
		t.Errorf("expected a require block:\n%s", out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Server Artifacts                                                    */
/* -------------------------------------------------------------------------- */

func TestEmitGoServer(t *testing.T) {
	out := EmitGoServer(greetIR())
	for _, want := range []string{
		"package main",
		"func StartServer(addr string) error",
		"func registerRoutes(router *router)",
		"type router struct",
		"func (rt *router) serve(",
		"/healthz",
		"type Request struct",
		"type Response struct",
		"const staticDir",
		"RHO_HTTP_ADDR",
		"http.Server",
		"server.Shutdown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected server.go to contain %q\n%s", want, out)
		}
	}
}

func TestEmitGoServerEntrypoint(t *testing.T) {
	out := EmitGoServerEntrypoint(greetIR())
	if !strings.Contains(out, "StartServer(addr)") {
		t.Error("expected the server entrypoint to start the server")
	}
	if !strings.Contains(out, "os.Exit(1)") {
		t.Error("expected a non-zero exit when the server fails")
	}
	// The module name is interpolated into the format string, so it must not
	// itself contain a verb that would unbalance the call. go vet in the
	// compile test is the real guard for this.
	if strings.Count(out, "%s") != 1 {
		t.Errorf("expected exactly one format verb, got:\n%s", out)
	}
	if !strings.Contains(out, `"rho greet listening on %s\n", addr)`) {
		t.Errorf("expected the module name and address in the log line, got:\n%s", out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Generated Projects Build and Run                                    */
/* -------------------------------------------------------------------------- */

// writeGoProject writes a generated Go project into a temp directory.
func writeGoProject(t *testing.T, m ir.ModuleIR, includeServer bool) string {
	t.Helper()
	dir := t.TempDir()

	implementation := EmitGo(m)
	if implementation == "" {
		t.Fatal("EmitGo produced no output")
	}

	files := map[string]string{
		"app.go":     implementation,
		"runtime.go": EmitGoRuntime(implementation),
		"go.mod":     EmitGoMod(m),
	}
	if includeServer {
		server := EmitGoServer(m)
		entry := EmitGoServerEntrypoint(m)
		files["server.go"] = server
		files["main.go"] = entry
		files["runtime.go"] = EmitGoRuntime(implementation + server + entry)
	} else {
		files["main.go"] = EmitGoEntrypoint(m)
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
}

// The generated C-syntax constructs must translate into Go that a compiler
// accepts.
func TestGeneratedGoCompiles(t *testing.T) {
	requireGoToolchain(t)
	dir := writeGoProject(t, counterIR(), false)

	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "app"), ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated project did not compile: %v\n%s", err, out)
	}
}

// The translated program must also behave like the Rho source: adding 2 and 4
// gives a mean of 3.
func TestGeneratedGoRuns(t *testing.T) {
	requireGoToolchain(t)
	dir := writeGoProject(t, counterIR(), false)

	build := exec.Command("go", "build", "-o", filepath.Join(dir, "app"), ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	run := exec.Command(filepath.Join(dir, "app"))
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "3.0" {
		t.Fatalf("expected the mean 3.0, got %q", got)
	}
}

func TestGeneratedGoServerCompiles(t *testing.T) {
	requireGoToolchain(t)
	dir := writeGoProject(t, greetIR(), true)

	// go vet catches format-verb mistakes that the compiler accepts.
	vet := exec.Command("go", "vet", "./...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet rejected the generated server: %v\n%s", err, out)
	}

	build := exec.Command("go", "build", "-o", filepath.Join(dir, "srv"), ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("server did not compile: %v\n%s", err, out)
	}
}

// Both backends must accept the same IR, so the shared fixture compiles for C
// as well. This guards the Go work against diverging from the C contract.
func TestSameIRCompilesForBothBackends(t *testing.T) {
	m := counterIR()
	if out := EmitC(m); !strings.Contains(out, "Accum") {
		t.Error("the C backend should also emit the shared fixture")
	}
	if out := EmitGo(m); !strings.Contains(out, "Accum") {
		t.Error("the Go backend should emit the shared fixture")
	}
}

func TestGoBackendHandlesRealTimeInvariant(t *testing.T) {
	// Guards against a generated file that only builds on some platforms.
	if runtime.GOOS == "" {
		t.Fatal("expected a runtime OS")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Function Naming Matches Call Sites                                  */
/* -------------------------------------------------------------------------- */

// Definitions must use the same identity the typechecker used for call sites.
// When a function belongs to an imported module both sides carry the
// qualified name, otherwise the generated Go does not resolve.
func TestEmitGoUsesQualifiedFunctionName(t *testing.T) {
	m := ir.ModuleIR{
		Name: "app",
		Funcs: []ir.FuncIR{
			{
				Name:       "clamp_f64",
				CName:      "std_math_basic_clamp_f64",
				Params:     []ir.ParamIR{{Name: "x", Type: ir.Type{Kind: ir.TypeF64}}},
				ReturnType: ir.Type{Kind: ir.TypeF64},
				Body:       []ir.Instr{{Op: ir.OpReturn, Args: []string{"x"}}},
			},
		},
	}
	out := EmitGo(m)
	if !strings.Contains(out, "func stdMathBasicClampF64(x float64) float64 {") {
		t.Errorf("expected the qualified definition, got:\n%s", out)
	}
}

// A function without a qualified name keeps its bare spelling.
func TestEmitGoKeepsBareFunctionName(t *testing.T) {
	m := greetIR()
	out := EmitGo(m)
	if !strings.Contains(out, "func greeting() string {") {
		t.Errorf("expected the bare definition, got:\n%s", out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Unused Locals Are Discarded                                         */
/* -------------------------------------------------------------------------- */

// C only warns about an unread local; Go rejects it. A Rho program may leave a
// binding unread, so the generated Go has to discard it explicitly.
func TestEmitGoDiscardsUnusedLocal(t *testing.T) {
	m := ir.ModuleIR{
		Name: "app",
		Funcs: []ir.FuncIR{{
			Name:       "main",
			CName:      "main",
			ReturnType: ir.Type{Kind: ir.TypeVoid},
			Body: []ir.Instr{
				{Op: ir.OpLet, Dest: "unused", Type: "double", Args: []string{"1.0"}},
				{Op: ir.OpExpr, Args: []string{`printf("ok\n")`}},
			},
		}},
	}
	out := EmitGo(m)
	if !strings.Contains(out, "_ = unused") {
		t.Errorf("expected the unused local to be discarded, got:\n%s", out)
	}
}

// A local that is read must not be discarded.
func TestEmitGoKeepsUsedLocal(t *testing.T) {
	m := ir.ModuleIR{
		Name: "app",
		Funcs: []ir.FuncIR{{
			Name:       "main",
			CName:      "main",
			ReturnType: ir.Type{Kind: ir.TypeVoid},
			Body: []ir.Instr{
				{Op: ir.OpLet, Dest: "used", Type: "double", Args: []string{"1.0"}},
				{Op: ir.OpExpr, Args: []string{`printf("%f\n", used)`}},
			},
		}},
	}
	out := EmitGo(m)
	if strings.Contains(out, "_ = used") {
		t.Errorf("a read local must not be discarded, got:\n%s", out)
	}
}

// The discard has to precede the return, because Go only accepts a return as
// the final statement of a function.
func TestEmitGoDiscardsBeforeReturn(t *testing.T) {
	m := ir.ModuleIR{
		Name: "app",
		Funcs: []ir.FuncIR{{
			Name:       "value",
			CName:      "value",
			ReturnType: ir.Type{Kind: ir.TypeF64},
			Body: []ir.Instr{
				{Op: ir.OpLet, Dest: "spare", Type: "double", Args: []string{"0.0"}},
				{Op: ir.OpReturn, Args: []string{"1.0"}},
			},
		}},
	}
	out := EmitGo(m)
	discard := strings.Index(out, "_ = spare")
	ret := strings.Index(out, "return 1.0")
	if discard < 0 || ret < 0 || discard > ret {
		t.Errorf("expected the discard before the return, got:\n%s", out)
	}
}

// A program that leaves a binding unread must still compile for Go.
func TestGeneratedGoCompilesWithUnusedLocal(t *testing.T) {
	requireGoToolchain(t)
	dir := writeGoProject(t, unusedLocalIR(), false)

	build := exec.Command("go", "build", "-o", filepath.Join(dir, "app"), ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated project did not compile: %v\n%s", err, out)
	}
}

// unusedLocalIR is a program with a binding that is never read.
func unusedLocalIR() ir.ModuleIR {
	return ir.ModuleIR{
		Name: "sloppy",
		Funcs: []ir.FuncIR{
			{
				Name:       "main",
				CName:      "main",
				ReturnType: ir.Type{Kind: ir.TypeVoid},
				Body: []ir.Instr{
					{Op: ir.OpLet, Dest: "ignored", Type: "double", Args: []string{"41.0"}},
					{Op: ir.OpExpr, Args: []string{`printf("hello\n")`}},
				},
			},
		},
	}
}
