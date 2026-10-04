//
// @file client_test.go
// @brief Tests for Rho Module Remote Registry Client - Enterprise v3.
//
// Validates:
//   - NewClient creates client correctly
//   - Config URL is used
//   - ModuleInfo structure
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package remote

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: NewClient                                                            */
/* -------------------------------------------------------------------------- */

func TestNewClient(t *testing.T) {
	cfg := &Config{
		Remote: RemoteConfig{
			URL:   "https://registry.example.com",
			Token: "test-token",
		},
	}

	client := NewClient(cfg)
	if client.BaseURL != cfg.Remote.URL {
		t.Fatalf("expected BaseURL %s, got %s", cfg.Remote.URL, client.BaseURL)
	}
	if client.Token != cfg.Remote.Token {
		t.Fatalf("expected Token %s, got %s", cfg.Remote.Token, client.Token)
	}
	if client.HTTP == nil {
		t.Fatalf("expected HTTP client to be initialized")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: NewClient Default Config                                            */
/* -------------------------------------------------------------------------- */

func TestNewClientDefault(t *testing.T) {
	cfg := &Config{
		Remote: RemoteConfig{
			URL:   "https://registry.rho-lang.org",
			Token: "",
		},
	}

	client := NewClient(cfg)
	if client.BaseURL != "https://registry.rho-lang.org" {
		t.Fatalf("unexpected URL: %s", client.BaseURL)
	}
	if client.Token != "" {
		t.Fatalf("expected empty token for default config")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: ModuleInfo Structure                                                 */
/* -------------------------------------------------------------------------- */

func TestModuleInfoStructure(t *testing.T) {
	info := ModuleInfo{
		Name:        "test-module",
		Version:     "1.0.0",
		Type:        "component",
		Description: "Test module",
		Main:        "src/main.go",
		Command:     "test",
		Features:    []string{"feature1", "feature2"},
		Deps:        map[string]string{"math": ">=1.0.0"},
	}

	if info.Name != "test-module" {
		t.Fatalf("unexpected name: %s", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if len(info.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(info.Features))
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Credentials On The Wire                                              */
/* -------------------------------------------------------------------------- */

// A token is presented as a bearer credential on every exchange, the way a web
// API expects one, rather than being passed as a parameter.
func TestClientPresentsABearerToken(t *testing.T) {
	client := NewClient(&Config{Remote: RemoteConfig{URL: "http://127.0.0.1:8080", Token: "a-token"}})
	if got := client.credentialFor(); got != "Bearer a-token" {
		t.Fatalf("got %q, want a bearer credential", got)
	}
}

// An issued token takes precedence over a static one, because it can carry
// scopes and an expiry.
func TestIssuedTokenWinsOverConfigured(t *testing.T) {
	SetTokenReader(func(registry string) string {
		if registry == "http://127.0.0.1:8080" {
			return "issued-token"
		}
		return ""
	})
	t.Cleanup(func() { SetTokenReader(nil) })

	client := NewClient(&Config{Remote: RemoteConfig{URL: "http://127.0.0.1:8080", Token: "configured"}})
	if client.Token != "issued-token" {
		t.Fatalf("got %q, want the issued token", client.Token)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Transport Safety                                                    */
/* -------------------------------------------------------------------------- */

// A bearer token authenticates a request but does not hide it, so it is not
// sent over plain HTTP to a remote host.
func TestTokenIsNotSentOverPlainHTTP(t *testing.T) {
	cases := map[string]bool{
		"https://registry.example.com": true,
		"http://localhost:8080":        true,
		"http://127.0.0.1:8080":        true,
		"http://[::1]:8080":            true,
		"http://registry.example.com":  false,
		"http://10.0.0.5:8080":         false,
	}
	for registry, want := range cases {
		if got := TokenSafeFor(registry); got != want {
			t.Errorf("TokenSafeFor(%q) = %v, want %v", registry, got, want)
		}
	}
}

func TestClientWithholdsAnUnsafeCredential(t *testing.T) {
	client := NewClient(&Config{Remote: RemoteConfig{URL: "http://registry.example.com", Token: "a-token"}})
	if got := client.credentialFor(); got != "" {
		t.Fatalf("a credential must not go out over plain HTTP, got %q", got)
	}
}

// A deployment that terminates TLS in front of an http registry can say so.
func TestInsecureTransportCanBeAllowed(t *testing.T) {
	t.Setenv("RHO_ALLOW_INSECURE_REGISTRY", "1")
	client := NewClient(&Config{Remote: RemoteConfig{URL: "http://registry.example.com", Token: "a-token"}})
	if got := client.credentialFor(); got != "Bearer a-token" {
		t.Fatalf("expected the credential to be sent once allowed, got %q", got)
	}
}

func TestNoCredentialWhenNoneIsConfigured(t *testing.T) {
	client := NewClient(&Config{Remote: RemoteConfig{URL: "https://registry.example.com"}})
	if got := client.credentialFor(); got != "" {
		t.Fatalf("got %q, want no credential", got)
	}
}
