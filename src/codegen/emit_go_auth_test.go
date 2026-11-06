//
// @file emit_go_auth_test.go
// @brief Tests for the generated SSO middleware security surface.
//
// SCOPE ENFORCEMENT
// A service can declare the scopes its tokens must carry; the middleware
// injects the verified subject, scopes and audience into headers so the
// application can enforce per-project and per-endpoint access, and refuses a
// Bearer token that lacks a required scope.
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
)

func TestAuthMiddlewareRequiresScopes(t *testing.T) {
	body := emitAuthMiddleware("auth", MiddlewareSpec{
		Options: map[string]string{
			"issuer":  "keeper",
			"audience": "mesh",
			"key":     "pub.pem",
			"scopes":  "mesh:default,telemetry:read",
		},
	})
	for _, part := range []string{
		"authSsoRequireScopes",
		"authSsoHoldsScopes",
		"authSsoDenyScope",
		"authSsoRevoked",
		"insufficient scope",
		"X-Rho-Subject",
		"X-Rho-Scopes",
		"X-Rho-Aud",
	} {
		if !strings.Contains(body, part) {
			t.Errorf("generated middleware is missing %q", part)
		}
	}
	if !strings.Contains(body, "mesh:default,telemetry:read") {
		t.Error("the required scopes were not written into the generated source")
	}
	if strings.Contains(body, "authSsoSplitPaths(\"\\n\")") {
		t.Error("an empty scope list was emitted")
	}
}

func TestAuthMiddlewareWithoutScopesStaysOpen(t *testing.T) {
	body := emitAuthMiddleware("auth", MiddlewareSpec{
		Options: map[string]string{"issuer": "keeper", "audience": "mesh"},
	})
	if !strings.Contains(body, "var authSsoRequireScopes = authSsoSplitPaths(\"\")") {
		t.Errorf("no required scopes should be emitted as an empty list: %s", body)
	}
	// With an empty requirement the helper must accept any token; the deny path
	// stays compiled for the deployment that later turns scopes on.
	if !strings.Contains(body, "func authSsoHoldsScopes(held []string) bool") {
		t.Error("the scope helper was not emitted")
	}
}
