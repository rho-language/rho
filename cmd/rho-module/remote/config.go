//
// @file config.go
// @brief Rho Module Remote Registry Configuration - Enterprise v3.
//
// Provides configuration for remote registry access.
// Config file: ~/.rho/config/registry.yaml
//
// Configuration format:
//
// remote:
//   url: https://registry.rho-lang.org
//   token: <auth-token>
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package remote

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/internal/paths"
	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Config Structure                                                           */
/* -------------------------------------------------------------------------- */

// Config holds the remote registry configuration.
type Config struct {
	Remote RemoteConfig
}

// RemoteConfig holds the remote registry connection settings.
type RemoteConfig struct {
	URL   string
	Token string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// LoadConfig loads registry configuration from ~/.rho/config/registry.yaml.
// Returns empty config if file doesn't exist.
func LoadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	file := filepath.Join(configPath, "registry.yaml")
	if _, err := os.Stat(file); os.IsNotExist(err) {
		// No config file, return defaults
		return &Config{
			Remote: RemoteConfig{
				URL:   "https://registry.rho-lang.org",
				Token: "",
			},
		}, nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read registry config: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid registry config: %w", err)
	}

	cfg := &Config{
		Remote: RemoteConfig{
			URL:   "https://registry.rho-lang.org",
			Token: "",
		},
	}

	if remoteNode, ok := root.Map["remote"]; ok {
		if url, ok := remoteNode.Map["url"]; ok {
			cfg.Remote.URL = url.Str
		}
		if token, ok := remoteNode.Map["token"]; ok {
			cfg.Remote.Token = token.Str
		}
	}

	return cfg, nil
}

// SaveConfig writes the registry configuration to ~/.rho/config/registry.yaml.
func SaveConfig(cfg *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configPath, 0o755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	file := filepath.Join(configPath, "registry.yaml")
	out := fmt.Sprintf("remote:\n  url: %s\n  token: %s\n", cfg.Remote.URL, cfg.Remote.Token)

	return os.WriteFile(file, []byte(out), 0o644)
}

// SetRemoteRegistryURL updates the remote registry URL in the config.
func SetRemoteRegistryURL(url string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Remote.URL = url
	return SaveConfig(cfg)
}

// SetRemoteRegistryToken updates the auth token in the config.
func SetRemoteRegistryToken(token string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Remote.Token = token
	return SaveConfig(cfg)
}

/* -------------------------------------------------------------------------- */
/*  Internal Helpers                                                           */
/* -------------------------------------------------------------------------- */

func getConfigPath() (string, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config"), nil
}
