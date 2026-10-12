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
// SPDX-License-Identifier: Apache-2.0
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

// bindingParams is the signature every typed accessor has: a request and the
// name of the value being read from it. The name is a string rather than an
// identifier because the request carries its values as text, and the
// conversion is what makes them typed.
var bindingParams = []ir.Type{
	{Kind: ir.TypeStruct, Name: RequestTypeName},
	{Kind: ir.TypeString, Name: "string"},
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
	// Typed parameter binding, supplied by the routing port. Each accessor
	// converts one value from one request source, with a statement the
	// generator wrote rather than with a runtime decision.
	//
	//	path_u32(req, "id")
	//	if !bind_ok(req) {
	//	    return error_json(status_bad_request(), bind_error(req))
	//	}
	"path_str":    {params: bindingParams, result: ir.Type{Kind: ir.TypeString, Name: "string"}},
	"path_i32":    {params: bindingParams, result: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
	"path_u32":    {params: bindingParams, result: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	"path_u64":    {params: bindingParams, result: ir.Type{Kind: ir.TypeU64, Name: "u64"}},
	"path_f64":    {params: bindingParams, result: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
	"path_bool":   {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"query_str":   {params: bindingParams, result: ir.Type{Kind: ir.TypeString, Name: "string"}},
	"query_i32":   {params: bindingParams, result: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
	"query_u32":   {params: bindingParams, result: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	"query_u64":   {params: bindingParams, result: ir.Type{Kind: ir.TypeU64, Name: "u64"}},
	"query_f64":   {params: bindingParams, result: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
	"query_bool":  {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"header_str":  {params: bindingParams, result: ir.Type{Kind: ir.TypeString, Name: "string"}},
	"header_i32":  {params: bindingParams, result: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
	"header_u32":  {params: bindingParams, result: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	"header_u64":  {params: bindingParams, result: ir.Type{Kind: ir.TypeU64, Name: "u64"}},
	"header_f64":  {params: bindingParams, result: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
	"header_bool": {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"body_str":    {params: bindingParams, result: ir.Type{Kind: ir.TypeString, Name: "string"}},
	"body_i32":    {params: bindingParams, result: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
	"body_u32":    {params: bindingParams, result: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	"body_u64":    {params: bindingParams, result: ir.Type{Kind: ir.TypeU64, Name: "u64"}},
	"body_f64":    {params: bindingParams, result: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
	"body_bool":   {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"field_str":   {params: bindingParams, result: ir.Type{Kind: ir.TypeString, Name: "string"}},
	"field_i32":   {params: bindingParams, result: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
	"field_u32":   {params: bindingParams, result: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	"field_u64":   {params: bindingParams, result: ir.Type{Kind: ir.TypeU64, Name: "u64"}},
	"field_f64":   {params: bindingParams, result: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
	"field_bool":  {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	// bind_ok reports whether every value the handler read converted, and
	// bind_error says which one did not.
	"bind_ok": {
		params: []ir.Type{{Kind: ir.TypeStruct, Name: RequestTypeName}},
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"bind_error": {
		params: []ir.Type{{Kind: ir.TypeStruct, Name: RequestTypeName}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	// The presence reads and the defaulting read, which ask about a value
	// without converting it and without recording a failure. An optional
	// parameter has to be read through one of these: the typed accessor would
	// record a failure for a request that is perfectly correct, and the failure
	// would sit on the request for the rest of the handler.
	"has_path":   {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"has_query":  {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"has_header": {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"has_field":  {params: bindingParams, result: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
	"query_default": {
		params: []ir.Type{
			{Kind: ir.TypeStruct, Name: RequestTypeName},
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	// openapi_document returns the service's own API description, which the
	// openapiemit port wrote when the project was generated. It is a string
	// because a description is a document: what a service does with it is send
	// it.
	"openapi_document": {
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
	// Authentication, supplied by a server target's generated auth.go.
	"auth_bearer": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"auth_subject": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"auth_is_authenticated": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_required": {
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_open_tokens": {
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_bootstrap": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_allowed": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_list_configured": {
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"auth_issue": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeU32, Name: "u32"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"auth_expires": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	// Listings the fsstore port provides, rendered as tab-separated lines
	// because a Rho handler cannot walk a nested document.
	//
	// Each takes the identity of the caller and whether to narrow to that
	// caller's own modules, because what a listing may contain depends on who is
	// asking: a private module is only in it for its owner.
	"store_list": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeBool, Name: "bool"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"store_meta": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeBool, Name: "bool"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"store_detail": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"store_latest": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"store_data_dir": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_data_dir": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_access": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"config_get": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"clock_year": {
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	"escape_html": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_title": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_version": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_site_url": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	// registry_url is the address the service is published under, which is what
	// the documentation it generates leads back to.
	"registry_url": {
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	"registry_token_days": {
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	"registry_token_max_days": {
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	"registry_open_tokens": {
		result: ir.Type{Kind: ir.TypeBool, Name: "bool"},
	},
	"registry_max_upload_mb": {
		result: ir.Type{Kind: ir.TypeU32, Name: "u32"},
	},
	"template_get": {
		params: []ir.Type{{Kind: ir.TypeString, Name: "string"}},
		result: ir.Type{Kind: ir.TypeString, Name: "string"},
	},
	// template_fill renders a template with up to four name/value pairs. A
	// pair whose name is empty is skipped, so a caller with fewer values
	// passes empty ones rather than reaching for a different arity.
	// template_fill takes the source and one encoded list of values, so a
	// template can gain placeholders without any signature changing. See
	// std/web/template.rho for the encoding.
	"template_fill": {
		params: []ir.Type{
			{Kind: ir.TypeString, Name: "string"},
			{Kind: ir.TypeString, Name: "string"},
		},
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
//
// The handler is recorded under the same identity the call sites use, so a
// function from an imported module resolves on both sides. Several routes may
// share a handler, so the wrapper is emitted once.
func (tc *TypeChecker) routeToIR(decl ast.RouteDecl) ir.RouteIR {
	handler := decl.Handler
	if signature, ok := tc.funcs[decl.Handler]; ok && signature.cName != "" {
		handler = signature.cName
	}
	return ir.RouteIR{
		Method:  decl.Method,
		Path:    decl.Path,
		Handler: handler,
	}
}

// RouteHandlers returns the distinct handlers a module's routes use, in the
// order they first appear.
func RouteHandlers(m ir.ModuleIR) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, route := range m.Routes {
		if seen[route.Handler] {
			continue
		}
		seen[route.Handler] = true
		out = append(out, route.Handler)
	}
	return out
}
