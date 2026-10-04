//
// @file config_test.go
// @brief Tests for Rho Module Remote Registry Configuration - Enterprise v3.
//
// Validates:
//   - Config loading from file
//   - Config saving to file
//   - Default config values
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package remote

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Test: LoadConfig (default)                                                  */
/* -------------------------------------------------------------------------- */

func TestLoadConfigDefault(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := os.MkdirAll(filepath.Join(tmp, "config"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Remote.URL != "https://registry.rho-lang.org" {
		t.Fatalf("expected default URL, got %s", cfg.Remote.URL)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: SaveConfig and LoadConfig                                            */
/* -------------------------------------------------------------------------- */

func TestSaveAndLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	cfg := &Config{
		Remote: RemoteConfig{
			URL:   "https://my-registry.example.com",
			Token: "secret-token",
		},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Remote.URL != cfg.Remote.URL {
		t.Fatalf("URL mismatch: expected %s, got %s", cfg.Remote.URL, loaded.Remote.URL)
	}
	if loaded.Remote.Token != cfg.Remote.Token {
		t.Fatalf("Token mismatch: expected %s, got %s", cfg.Remote.Token, loaded.Remote.Token)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: SetRemoteRegistryURL                                                 */
/* -------------------------------------------------------------------------- */

func TestSetRemoteRegistryURL(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := SetRemoteRegistryURL("https://new-registry.example.com"); err != nil {
		t.Fatalf("SetRemoteRegistryURL failed: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Remote.URL != "https://new-registry.example.com" {
		t.Fatalf("expected new URL, got %s", cfg.Remote.URL)
	}
}