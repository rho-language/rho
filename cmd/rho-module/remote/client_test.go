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
// SPDX-License-Identifier: MIT
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