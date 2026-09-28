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
	"strings"
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
// HasCredential reports whether this client can present an identity to a
// registry: either the configured token or one `rho auth login` stored.
//
// A publish is only attempted when this is true, because a registry that
// records an owner cannot be sent a publish it cannot attribute. Gating on the
// configured token alone would skip the upload for every user who logged in,
// which is exactly the user the credential is for.
func (c *Client) HasCredential() bool { return c.credentialFor() != "" }

// Identity is who this client is publishing as, as the registry will see it.
func (c *Client) Identity() string {
	credential := c.credentialFor()
	if credential == "" {
		return ""
	}
	if after, found := cutPrefix(credential, "Bearer "); found {
		return after
	}
	return ""
}

func (c *Client) credentialFor() string {
	if c.Token == "" {
		return ""
	}
	if !TokenSafeFor(c.BaseURL) {
		return ""
	}
	return "Bearer " + c.Token
}

// cutPrefix is strings.CutPrefix, spelled here because this file is read by a
// toolchain that builds with Go 1.18, where that function arrived later.
func cutPrefix(text, prefix string) (string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return text[len(prefix):], true
}

// UnpublishModule withdraws a published version from a remote registry.
//
// Only the identity that published the module may do this, and the registry
// decides that from the credential it is sent. The refusal it answers with is
// returned as the error, because a caller who is not the owner needs to read who
// is.
func UnpublishModule(client *Client, name, version string) error {
	if client.BaseURL == "" {
		return fmt.Errorf("remote registry not configured")
	}
	request, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/modules/%s/%s", client.BaseURL, name, version), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if credential := client.credentialFor(); credential != "" {
		request.Header.Set("Authorization", credential)
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("unpublish request: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("%s", remoteMessage(body,
			"the registry requires a credential to unpublish"))
	case http.StatusForbidden:
		return fmt.Errorf("%s", remoteMessage(body,
			"only the module's owner may unpublish it"))
	case http.StatusNotFound:
		return fmt.Errorf("%s@%s is not published on this registry", name, version)
	default:
		return fmt.Errorf("unpublish failed: %d %s", response.StatusCode,
			remoteMessage(body, ""))
	}
}

// remoteMessage reads the reason out of a registry's answer, which is JSON with
// an `error` field, falling back to the text it was given.
func remoteMessage(body []byte, fallback string) string {
	answer := struct {
		Error string `json:"error"`
	}{}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&answer); err == nil && answer.Error != "" {
		return answer.Error
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return fallback
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
	// Author is the module's own claim about who wrote it, from its manifest.
	Author string `json:"author,omitempty"`
	// Owner is the identity the registry recorded from the credential that
	// published the module, which is who may change or withdraw it. It is only
	// in the answer for a module the caller owns.
	Owner string `json:"owner,omitempty"`
	// Visibility is whether anybody may install the module or only its owner.
	// An empty value means public.
	Visibility string `json:"visibility,omitempty"`
	// Mine is set when the module was published by the caller, which is what
	// makes an owner's own private modules findable.
	Mine bool `json:"mine,omitempty"`
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

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	// A refusal from the registry says what was wrong and who holds the name,
	// and that is more useful than the status it came with. It is the difference
	// between "409" and "the module is published by miraur", which are the same
	// event and only one of which a user can act on.
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s", remoteMessage(body,
			"the registry requires a token to publish; log in with 'rho auth login'"))
	case http.StatusConflict:
		return fmt.Errorf("%s", remoteMessage(body,
			"the module is published by another publisher"))
	case http.StatusForbidden:
		return fmt.Errorf("%s", remoteMessage(body, "the registry refused the publish"))
	default:
		return fmt.Errorf("upload failed: %d %s", resp.StatusCode, remoteMessage(body, ""))
	}
}

// SetVisibility changes whether a module is anybody's to install or its owner's
// alone.
//
// It is a statement about the name rather than about a version, so it is one
// call and not one per release: a module that is made private is private
// whatever versions it gains afterwards.
func SetVisibility(client *Client, name, visibility string) error {
	if client.BaseURL == "" {
		return fmt.Errorf("remote registry not configured")
	}
	query := url.Values{}
	query.Set("visibility", visibility)
	endpoint := fmt.Sprintf("%s/modules/%s/visibility?%s",
		client.BaseURL, name, query.Encode())

	request, err := http.NewRequest("PUT", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if credential := client.credentialFor(); credential != "" {
		request.Header.Set("Authorization", credential)
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("visibility request: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)

	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("%s", remoteMessage(body,
			"the registry requires a token; log in with 'rho auth login'"))
	case http.StatusForbidden:
		return fmt.Errorf("%s", remoteMessage(body,
			"only the module's owner may change its visibility"))
	case http.StatusNotFound:
		return fmt.Errorf("%s is not published on this registry", name)
	default:
		return fmt.Errorf("visibility change failed: %d %s",
			response.StatusCode, remoteMessage(body, ""))
	}
}

// SearchMine lists the modules the caller published, whatever their visibility.
//
// It is a separate call from a search because the two mean different things: a
// search asks what is available, and this asks what is yours. An owner needs the
// second to find a private module at all, since a private module is deliberately
// absent from what everybody else can see.
func SearchMine(client *Client, query string) ([]ModuleInfo, error) {
	if client.BaseURL == "" {
		return nil, fmt.Errorf("remote registry not configured")
	}
	if !client.HasCredential() {
		return nil, fmt.Errorf("a token is required to list your own modules")
	}
	values := url.Values{}
	values.Set("q", query)
	values.Set("mine", "1")
	request, err := http.NewRequest("GET",
		fmt.Sprintf("%s/api/search?%s", client.BaseURL, values.Encode()), nil)
	if err != nil {
		return nil, err
	}
	if credential := client.credentialFor(); credential != "" {
		request.Header.Set("Authorization", credential)
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("search failed: %d %s", response.StatusCode, remoteMessage(body, ""))
	}
	var results []ModuleInfo
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("cannot read the search response: %w", err)
	}
	return results, nil
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

	url := fmt.Sprintf("%s/api/search?q=%s", client.BaseURL, query)

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
	return ClientVersions(NewClient(cfg), name)
}

// ClientVersions lists the versions a registry holds for a module.
//
// It takes the client rather than loading the configuration, because a caller
// that has already built a client — because it needed to know whether it had a
// credential — should not build a second one that may not carry the same
// token. The credential is what a private module is fetched with, so the two
// clients would not be interchangeable.
func ClientVersions(client *Client, name string) ([]string, error) {
	if client.BaseURL == "" {
		return nil, fmt.Errorf("remote registry not configured")
	}

	url := fmt.Sprintf("%s/api/modules/%s", client.BaseURL, name)
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

// ResolveRemoteVersion picks the version to install: the one named, or the
// newest the registry holds.
//
// The newest is the last of the list rather than the greatest of it, because the
// registry answers lowest first; comparing them properly would need a version
// comparison in a place that has no business having one, and a registry that
// answered in another order would be the thing to fix.
func ResolveRemoteVersion(client *Client, name, version string) (string, error) {
	if version != "" {
		return version, nil
	}
	versions, err := ClientVersions(client, name)
	if err != nil {
		return "", fmt.Errorf("cannot list versions of %s: %w", name, err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("the registry has no published version of %s", name)
	}
	return versions[len(versions)-1], nil
}
