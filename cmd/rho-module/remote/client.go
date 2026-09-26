//
// @file client.go
// @brief Rho Module Remote Registry Client - Enterprise v3.
//
// HTTP client for interacting with the remote registry.
// Currently a placeholder for future remote registry implementation.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  HTTP Client                                                                */
/* -------------------------------------------------------------------------- */

// Client provides HTTP operations against the remote registry.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient creates a new remote registry client.
//
// A token issued by `rho auth login` is preferred over the static token in the
// configuration, because it can carry scopes and an expiry. It is sent as a
// bearer credential on every request, the way a web API expects one.
func NewClient(cfg *Config) *Client {
	token := cfg.Remote.Token
	if issued := issuedToken(cfg.Remote.URL); issued != "" {
		token = issued
	}
	return &Client{
		BaseURL: cfg.Remote.URL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TokenSafeFor reports whether a credential may be sent to a registry URL.
//
// A bearer token authenticates a request but does not hide it, so sending one
// over plain HTTP would hand it to anyone on the path. Loopback is exempt
// because nothing else can observe it, and a deployment that terminates TLS
// in front of an http registry can say so.
func TokenSafeFor(registryURL string) bool {
	parsed, err := url.Parse(registryURL)
	if err != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if os.Getenv("RHO_ALLOW_INSECURE_REGISTRY") == "1" {
		return true
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// credentialFor returns the header value to present, or "" when presenting a
// credential would be unsafe. The caller then proceeds unauthenticated, which
// the registry accepts for public reads and refuses for writes.
func (c *Client) credentialFor() string {
	if c.Token == "" {
		return ""
	}
	if !TokenSafeFor(c.BaseURL) {
		return ""
	}
	return "Bearer " + c.Token
}

// ModuleInfo holds metadata about a module on the remote registry.
type ModuleInfo struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Main        string            `json:"main"`
	Command     string            `json:"command"`
	Features    []string          `json:"features"`
	Deps        map[string]string `json:"deps"`
}

// tokenReader is set by the CLI so this package can find an issued credential
// without importing the auth store itself. It keeps the dependency one way.
var tokenReader func(registry string) string

// SetTokenReader registers how an issued token is looked up.
func SetTokenReader(reader func(registry string) string) { tokenReader = reader }

// issuedToken returns a token issued for a registry, if there is a valid one.
func issuedToken(registry string) string {
	if tokenReader == nil || registry == "" {
		return ""
	}
	return tokenReader(registry)
}

/* -------------------------------------------------------------------------- */
/*  Upload Module                                                              */
/* -------------------------------------------------------------------------- */

// UploadModule uploads a module to the remote registry.
// Returns error if remote registry is not configured.
func UploadModule(client *Client, name, version string, pkgData, sourceData []byte, manifest map[string]string) error {
	if client.BaseURL == "" {
		return fmt.Errorf("remote registry not configured")
	}

	// Build upload URL. The manifest fields travel as query parameters so a
	// registry can record what was published without unpacking the archive.
	query := url.Values{}
	for key, value := range manifest {
		if value != "" {
			query.Set(key, value)
		}
	}
	query.Set("version", version)
	uploadURL := fmt.Sprintf("%s/modules/%s/%s/rho-pkg?%s", client.BaseURL, name, version, query.Encode())

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(pkgData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/gzip")
	if credential := client.credentialFor(); credential != "" {
		req.Header.Set("Authorization", credential)
	}

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %d %s", resp.StatusCode, string(body))
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Download Module                                                            */
/* -------------------------------------------------------------------------- */

// DownloadModule downloads a module package from the remote registry.
func DownloadModule(client *Client, name, version string) ([]byte, error) {
	if client.BaseURL == "" {
		return nil, fmt.Errorf("remote registry not configured")
	}

	url := fmt.Sprintf("%s/modules/%s/%s/rho-pkg", client.BaseURL, name, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if credential := client.credentialFor(); credential != "" {
		req.Header.Set("Authorization", credential)
	}

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %d %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return data, nil
}

/* -------------------------------------------------------------------------- */
/*  Search Remote                                                              */
/* -------------------------------------------------------------------------- */

// SearchRemote searches for modules on the remote registry.
func SearchRemote(client *Client, query string) ([]ModuleInfo, error) {
	if client.BaseURL == "" {
		return nil, fmt.Errorf("remote registry not configured")
	}

	url := fmt.Sprintf("%s/search?q=%s", client.BaseURL, query)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if credential := client.credentialFor(); credential != "" {
		req.Header.Set("Authorization", credential)
	}

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: %d", resp.StatusCode)
	}

	// In a future implementation, this would parse the JSON response
	var results []ModuleInfo
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("cannot read the search response: %w", err)
	}
	return results, nil
}

/* -------------------------------------------------------------------------- */
/*  Convenience helpers                                                        */
/*                                                                             */
/*  These resolve the configured remote once, so a caller does not have to     */
/*  repeat the config lookup.                                                  */
/* -------------------------------------------------------------------------- */

// FetchModule downloads a module package from the configured remote.
func FetchModule(name, version string) ([]byte, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Remote.URL == "" {
		return nil, fmt.Errorf("remote registry not configured; set it with: rho config set remote.url <url>")
	}
	return DownloadModule(NewClient(cfg), name, version)
}

// AvailableVersions lists the published versions of a module, lowest first.
// An empty result means the module is not on the remote, or the remote does
// not publish a version list; callers treat that as "nothing to choose".
func AvailableVersions(name string) ([]string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Remote.URL == "" {
		return nil, fmt.Errorf("remote registry not configured; set it with: rho config set remote.url <url>")
	}
	client := NewClient(cfg)

	url := fmt.Sprintf("%s/modules/%s", client.BaseURL, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if credential := client.credentialFor(); credential != "" {
		req.Header.Set("Authorization", credential)
	}
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version list returned %d", resp.StatusCode)
	}

	var document struct {
		Versions []string `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return nil, fmt.Errorf("cannot read the version list: %w", err)
	}
	return document.Versions, nil
}
