//
// @file routes_test.go
// @brief Tests for HTTP route declarations.
//
// Validates:
//   - the parser captures method, path and handler
//   - a handler must exist and use the Request/Response signature
//   - routes reach the IR and the Go backend
//   - a generated server answers real requests
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

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	"rho/src/ir"
)

// The helpers std/web/http provides are declared locally here, because a unit
// test has no project root to resolve an import from.
const routedSource = `module app

route GET "/health"          => handle_health
route POST "/items/{id}"     => handle_item

fn text(status: u32, body: string) Response {
    return Response{ status, body, "text/plain" }
}

fn json(status: u32, body: string) Response {
    return Response{ status, body, "application/json" }
}

fn handle_health(req: Request) Response {
    return text(200, "ok")
}

fn handle_item(req: Request) Response {
    let id: string = param(req, "id")
    return json(200, "{\"id\":\"" + id + "\"}")
}

fn main() void {
    print("app")
}
`

/* -------------------------------------------------------------------------- */
/*  Test: Parsing                                                              */
/* -------------------------------------------------------------------------- */

func TestRouteParsing(t *testing.T) {
	tokens, err := lexer.Lex(routedSource)
	if err != nil {
		t.Fatalf("lexing failed: %v", err)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}

	routes := []ast.RouteDecl{}
	for _, decl := range module.Decls {
		if route, ok := decl.(ast.RouteDecl); ok {
			routes = append(routes, route)
		}
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if routes[0].Method != "GET" || routes[0].Path != "/health" || routes[0].Handler != "handle_health" {
		t.Fatalf("unexpected first route: %+v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/items/{id}" {
		t.Fatalf("unexpected second route: %+v", routes[1])
	}
}

// A lowercase method is accepted and normalised.
func TestRouteMethodIsUpperCased(t *testing.T) {
	tokens, _ := lexer.Lex("module app\n\nroute get \"/x\" => h\n\nfn main() void {\n}\n")
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}
	for _, decl := range module.Decls {
		if route, ok := decl.(ast.RouteDecl); ok && route.Method != "GET" {
			t.Fatalf("expected GET, got %q", route.Method)
		}
	}
}

func TestPathWildcards(t *testing.T) {
	got := pathWildcards("/modules/{name}/{version}/rho-pkg")
	if len(got) != 2 || got[0] != "name" || got[1] != "version" {
		t.Fatalf("unexpected wildcards: %v", got)
	}
	if len(pathWildcards("/search")) != 0 {
		t.Fatal("expected no wildcards in a static path")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Validation                                                           */
/* -------------------------------------------------------------------------- */

func compileRoutes(t *testing.T, source string) (ir.ModuleIR, error) {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		return ir.ModuleIR{}, err
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		return ir.ModuleIR{}, err
	}
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		return ir.ModuleIR{}, err
	}
	return tc.ToIR(*module), nil
}

func TestRoutesReachIR(t *testing.T) {
	out, err := compileRoutes(t, routedSource)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(out.Routes) != 2 {
		t.Fatalf("expected 2 routes in IR, got %d", len(out.Routes))
	}
	if out.Routes[0].Path != "/health" || out.Routes[0].Handler != "handle_health" {
		t.Fatalf("unexpected route: %+v", out.Routes[0])
	}
}

// A route must name a method the service supports.
func TestRouteRejectsUnknownMethod(t *testing.T) {
	_, err := compileRoutes(t, "module app\n\nroute FETCH \"/x\" => h\n\nfn main() void {\n}\n")
	if err == nil {
		t.Fatal("expected an error for an unknown method")
	}
	if !strings.Contains(err.Error(), "FETCH") {
		t.Fatalf("expected the method named, got %v", err)
	}
}

func TestRouteRejectsRelativePath(t *testing.T) {
	_, err := compileRoutes(t, "module app\n\nroute GET \"health\" => h\n\nfn main() void {\n}\n")
	if err == nil {
		t.Fatal("expected an error for a path without a leading slash")
	}
}

func TestRouteRejectsMissingHandler(t *testing.T) {
	_, err := compileRoutes(t, "module app\n\nroute GET \"/x\" => nowhere\n\nfn main() void {\n}\n")
	if err == nil {
		t.Fatal("expected an error for an undefined handler")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("expected the handler named, got %v", err)
	}
}

// A handler must take a single Request.
func TestRouteRejectsWrongParameterCount(t *testing.T) {
	_, err := compileRoutes(t, `module app

route GET "/x" => h

fn h(a: string, b: string) Response {
    return text(200, "x")
}

fn main() void {
}
`)
	if err == nil {
		t.Fatal("expected an error for a handler with two parameters")
	}
}

// A handler must return Response.
func TestRouteRejectsWrongReturnType(t *testing.T) {
	_, err := compileRoutes(t, `module app

route GET "/x" => h

fn h(req: Request) string {
    return "x"
}

fn main() void {
}
`)
	if err == nil {
		t.Fatal("expected an error for a handler that does not return Response")
	}
	if !strings.Contains(err.Error(), "Response") {
		t.Fatalf("expected the expected type named, got %v", err)
	}
}

// The protocol types are usable in a signature without an import.
func TestRequestAndResponseNeedNoImport(t *testing.T) {
	out, err := compileRoutes(t, `module app

route GET "/x" => h

fn h(req: Request) Response {
    return Response{ 200, "ok", "text/plain" }
}

fn main() void {
}
`)
	if err != nil {
		t.Fatalf("Request and Response should resolve without an import: %v", err)
	}
	// A target with no server still needs the types to compile.
	if !structNames(out)["Request"] || !structNames(out)["Response"] {
		t.Fatalf("expected synthesised protocol types, got %v", structNames(out))
	}
}

// A module that declares its own Response keeps it.
func TestUserDefinedResponseIsNotSynthesised(t *testing.T) {
	out, err := compileRoutes(t, `module app

struct Response {
    status: u32
    body: string
    content_type: string
}

route GET "/x" => h

fn h(req: Request) Response {
    return Response{ 200, "ok", "text/plain" }
}

fn main() void {
}
`)
	if err != nil {
		t.Fatalf("a module-defined Response should be used: %v", err)
	}
	count := 0
	for _, s := range out.Structs {
		if s.Name == "Response" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one Response definition, got %d", count)
	}
}

// A module with no routes emits no protocol types.
func TestNoRoutesMeansNoProtocolTypes(t *testing.T) {
	out, err := compileRoutes(t, "module app\n\nfn main() void {\n    print(\"x\")\n}\n")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(out.Routes) != 0 {
		t.Fatal("expected no routes")
	}
	if structNames(out)["Request"] {
		t.Fatal("a module with no routes needs no protocol types")
	}
}
