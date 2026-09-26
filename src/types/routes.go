//
// @file routes.go
// @brief Type checking and IR generation for HTTP routes.
//
// A route binds an HTTP method and a ServeMux path pattern to a Rho function:
//
//	route GET "/api/v1/modules/{name}" => get_module
//
// Handlers take a single Request and return a Response, both declared in the
// generated server. The path wildcards are read from the Request, so a handler
// signature never has to change when a path does.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package types

import (
	"fmt"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// RequestTypeName and ResponseTypeName are the handler signature types. A
// handler takes a Request and returns a Response.
//
// The Go backend defines both for a server target, because it has to decode a
// net/http request into them. A target without a server, such as C, still needs
// a definition for the code to compile, so one is synthesised.
const (
	RequestTypeName  = "Request"
	ResponseTypeName = "Response"
)

// protocolFields returns the field tables for the synthesised Request and
// Response. The field names match the Go definitions the server emits.
func (tc *TypeChecker) protocolFields() map[string]map[string]ir.Type {
	return map[string]map[string]ir.Type{
		RequestTypeName: {
			"method": {Kind: ir.TypeString, Name: "string"},
			"path":   {Kind: ir.TypeString, Name: "string"},
			"query":  {Kind: ir.TypeString, Name: "string"},
			"body":   {Kind: ir.TypeString, Name: "string"},
			"auth":   {Kind: ir.TypeString, Name: "string"},
		},
		ResponseTypeName: {
			"status":       {Kind: ir.TypeU32, Name: "u32"},
			"body":         {Kind: ir.TypeString, Name: "string"},
			"content_type": {Kind: ir.TypeString, Name: "string"},
		},
	}
}

// registerProtocolTypes makes Request and Response usable in handler
// signatures. A module that declares either itself keeps its own definition.
func (tc *TypeChecker) registerProtocolTypes() {
	fields := tc.protocolFields()
	for name, members := range fields {
		if _, declared := tc.types[name]; declared {
			continue
		}
		tc.types[name] = ir.Type{Kind: ir.TypeStruct, Name: name}
		tc.fields[name] = members
	}
}

// synthesiseProtocolTypes emits definitions for a target that has no server
// backend, so a module with routes still compiles.
func (tc *TypeChecker) synthesiseProtocolTypes(out *ir.ModuleIR) {
	fields := tc.protocolFields()
	// Declaration order matters: a Rho struct literal is positional, so the
	// fields have to be laid out in the same order the type declares them.
	order := map[string][]string{
		RequestTypeName:  {"method", "path", "query", "body", "auth"},
		ResponseTypeName: {"status", "body", "content_type"},
	}
	// Deterministic order: Request then Response.
	for _, name := range []string{RequestTypeName, ResponseTypeName} {
		if _, userDefined := tc.userStructs[name]; userDefined {
			continue
		}
		irStruct := ir.StructIR{Name: name, CName: name}
		for _, field := range order[name] {
			fieldType, ok := fields[name][field]
			if !ok {
				continue
			}
			irStruct.Fields = append(irStruct.Fields, ir.FieldIR{Name: field, Type: fieldType})
		}
		out.Structs = append(out.Structs, irStruct)
	}
}

// routeMethods are the HTTP methods a route may use.
var routeMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true, "HEAD": true,
}

// protocolFuncs are the helper functions the server backend provides, so a
// handler can read a captured path wildcard.
//
//	param(req, "name")
var protocolFuncs = map[string]functionType{
	"param": {
		params: []ir.Type{
			{Kind: ir.TypeStruct, Name: RequestTypeName},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"request_body": {
		params: []ir.Type{{Kind: ir.TypeStruct, Name: RequestTypeName}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"request_auth": {
		params: []ir.Type{{Kind: ir.TypeStruct, Name: RequestTypeName}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"request_query_param": {
		params: []ir.Type{
			{Kind: ir.TypeStruct, Name: RequestTypeName},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},

	// A few string and environment helpers a service needs. Rho's string
	// library is still limited, so these fill the gap a server target needs
	// without pulling in host-specific code.
	"str_starts_with": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"str_trim_prefix": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"strlen": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	"env_or": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
}

// pathWildcards returns the {name} placeholders in a ServeMux pattern, in order.
func pathWildcards(pattern string) []string {
	out := []string{}
	for i := 0; i < len(pattern); {
		if pattern[i] != '{' {
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			break
		}
		out = append(out, pattern[i+1:i+end])
		i += end + 1
	}
	return out
}

// checkRoute validates a route binding: the method is known, the path is a
// ServeMux pattern, and the handler exists with the expected signature.
func (tc *TypeChecker) checkRoute(decl ast.RouteDecl) error {
	if !routeMethods[decl.Method] {
		return fmt.Errorf("route %s %s: unsupported method %q", decl.Method, decl.Path, decl.Method)
	}
	if !strings.HasPrefix(decl.Path, "/") {
		return fmt.Errorf("route %s: path must start with '/'", decl.Path)
	}
	if decl.Handler == "" {
		return fmt.Errorf("route %s: no handler named", decl.Path)
	}

	entry, ok := tc.funcs[decl.Handler]
	if !ok {
		return fmt.Errorf("route %s: handler %q is not defined", decl.Path, decl.Handler)
	}
	if len(entry.params) != 1 {
		return fmt.Errorf("route %s: handler %q must take a single %s", decl.Path, decl.Handler, RequestTypeName)
	}
	if entry.result.Name != ResponseTypeName {
		return fmt.Errorf("route %s: handler %q must return %s, not %s",
			decl.Path, decl.Handler, ResponseTypeName, typeName(entry.result))
	}
	return nil
}

// routeToIR converts a route declaration for the backends.
func (tc *TypeChecker) routeToIR(decl ast.RouteDecl) ir.RouteIR {
	return ir.RouteIR{
		Method:  decl.Method,
		Path:    decl.Path,
		Handler: decl.Handler,
	}
}
