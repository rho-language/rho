//
// @file backends.go
// @brief The built-in functions only some backends supply.
//
// Most built-ins are lowered to a runtime function every backend writes. Some are
// not: reading a whole file is Go's runtime only, a request's path is the
// generated server's, a module store is a port's. Calling one of those in a
// program built for a backend that does not supply it used to compile and then
// fail where the compiler had no say any more — a C link error naming `get_args`,
// a Go "undefined: escapeHtml". The table here is what the build checks a
// program's calls against first, so the refusal names the function and the
// backend instead.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"rho/src/frontend/ast"
	"rho/src/ir"
)

// BackendRule is where a built-in exists: the backends that supply it and,
// when it comes from a port, the port a Go target has to render.
//
// A backend is "c", "go" (any Go target) or "go-server" (a Go target of kind
// server). A std module's `@backends` header line uses the same names.
type BackendRule struct {
	Backends []string
	Port     string
}

var (
	goOnly       = BackendRule{Backends: []string{"go"}}
	goServerOnly = BackendRule{Backends: []string{"go-server"}}
	fsstorePort  = BackendRule{Backends: []string{"go"}, Port: "fsstore"}
)

// builtinBackends lists every built-in that is not supplied by every backend.
// A name missing from it is one both backends lower.
var builtinBackends = map[string]BackendRule{
	// The Go runtime's whole-file, argument, directory and HTTP client
	// functions. The C runtime has none of them.
	"get_args":            goOnly,
	"read_file":           goOnly,
	"write_file":          goOnly,
	"append":              goOnly,
	"rho_arg":             goOnly,
	"rho_path_exists":     goOnly,
	"rho_list_dir":        goOnly,
	"rho_yaml_flat":       goOnly,
	"rho_yaml_write_flat": goOnly,
	"rho_service_token":   goOnly,
	"rho_http_request":    goOnly,
	"rho_http_body":       goOnly,
	"rho_sse_follow":      goOnly,

	// The generated service's functions.
	"escape_html":           goServerOnly,
	"json_string":           goServerOnly,
	"template_get":          goServerOnly,
	"template_has":          goServerOnly,
	"template_fill":         goServerOnly,
	"auth_bearer":           goServerOnly,
	"auth_subject":          goServerOnly,
	"auth_is_authenticated": goServerOnly,
	"auth_expires":          goServerOnly,
	"auth_required":         goServerOnly,
	"auth_open_tokens":      goServerOnly,
	"auth_bootstrap":        goServerOnly,
	"auth_allowed":          goServerOnly,
	"auth_list_configured":  goServerOnly,
	"auth_issue":            goServerOnly,

	// A module store and the settings that come with it are the fsstore port's.
	"store_list":              fsstorePort,
	"store_meta":              fsstorePort,
	"store_detail":            fsstorePort,
	"store_latest":            fsstorePort,
	"store_data_dir":          fsstorePort,
	"store_release":           fsstorePort,
	"config_get":              fsstorePort,
	"clock_year":              fsstorePort,
	"registry_data_dir":       fsstorePort,
	"registry_access":         fsstorePort,
	"registry_title":          fsstorePort,
	"registry_version":        fsstorePort,
	"registry_site_url":       fsstorePort,
	"registry_url":            fsstorePort,
	"registry_token_days":     fsstorePort,
	"registry_token_max_days": fsstorePort,
	"registry_open_tokens":    fsstorePort,
	"registry_max_upload_mb":  fsstorePort,

	"openapi_document": {Backends: []string{"go"}, Port: "openapiemit"},
}

func init() {
	// Everything that reads a request is the server's: the typed accessors, the
	// presence tests and the raw parts.
	for _, name := range []string{"param", "bind_ok", "bind_error", "query_default",
		"request_query_param", "request_body", "request_auth", "request_path",
		"request_method", "request_remote"} {
		builtinBackends[name] = goServerOnly
	}
	for _, source := range []string{"path", "query", "header", "body", "field"} {
		for _, kind := range []string{"str", "u32", "i32", "u64", "f64", "bool"} {
			builtinBackends[source+"_"+kind] = goServerOnly
		}
	}
	for _, source := range []string{"path", "query", "header", "field"} {
		builtinBackends["has_"+source] = goServerOnly
	}
}

// BuiltinBackends answers where a built-in exists, or false when every backend
// supplies it.
func BuiltinBackends(name string) (BackendRule, bool) {
	rule, ok := builtinBackends[name]
	return rule, ok
}

// noteBackendCall records a call to a built-in that only some backends supply,
// so the build can refuse it on the others (ModuleIR.BackendCalls). A function
// the program declares itself under the same name is its own and is not
// recorded; a foreign declaration of a runtime function is the runtime's, and
// is.
func (tc *TypeChecker) noteBackendCall(name string, at ast.Position) {
	if _, restricted := builtinBackends[name]; !restricted {
		return
	}
	if tc.rhoFuncNames[name] {
		return
	}
	if tc.backendCalls == nil {
		tc.backendCalls = map[string]ir.BackendCall{}
	}
	if _, seen := tc.backendCalls[name]; !seen {
		call := ir.BackendCall{Name: name, Module: tc.owner}
		if at.Valid() {
			call.File, call.Line, call.Column = at.File, at.Line, at.Column
		}
		tc.backendCalls[name] = call
		tc.backendCallOrder = append(tc.backendCallOrder, name)
	}
}

// backendCallsIR is the recorded calls in the order they were first made.
func (tc *TypeChecker) backendCallsIR() []ir.BackendCall {
	out := make([]ir.BackendCall, 0, len(tc.backendCallOrder))
	for _, name := range tc.backendCallOrder {
		out = append(out, tc.backendCalls[name])
	}
	return out
}
