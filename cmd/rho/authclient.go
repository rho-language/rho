//
// @file authclient.go
// @brief Talking to a registry's token endpoint, and using a stored token.
//
// A token is obtained from the registry rather than minted locally: the
// signing secret stays on the server, so a stolen client configuration cannot
// be used to mint credentials.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"rho/cmd/rho-module/remote"
	"rho/src/auth"
)

// defaultRegistryURL is the registry a command uses when none is configured.
func defaultRegistryURL() string {
	if cfg, err := remote.LoadConfig(); err == nil && cfg.Remote.URL != "" {
		return cfg.Remote.URL
	}
	return "https://registry.rho-lang.org"
}

// requestToken asks a registry to issue a token.
//
// The endpoint answers with the same token format the module client presents,
// so there is one scheme on the wire rather than a login exchange and an API
// exchange that disagree.
func requestToken(registry, subject, host string, lifetime time.Duration) (string, auth.TokenClaims, error) {
	var claims auth.TokenClaims

	// The parameters travel in the query string, which is where a Rho handler
	// reads them from, so one side does not have to parse a form body.
	query := url.Values{}
	query.Set("user", subject)
	query.Set("host", host)
	query.Set("days", fmt.Sprintf("%d", int(lifetime.Hours()/24)))
	query.Set("scopes", strings.Join([]string{auth.ScopePull, auth.ScopePush}, " "))
	endpoint := strings.TrimRight(registry, "/") + "/auth/token?" + query.Encode()

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return "", claims, err
	}
	// An already-stored credential doubles as proof of identity, so a registry
	// that has closed issuance still recognises an existing client.
	if existing, lookupErr := lookupCredential(registry); lookupErr == nil && existing.Secret != "" {
		req.Header.Set("Authorization", auth.Authorization(existing.Secret))
	}
	return readTokenResponse(req, claims)
}

// readTokenResponse performs a token request and decodes the answer.
func readTokenResponse(req *http.Request, claims auth.TokenClaims) (string, auth.TokenClaims, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", claims, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", claims, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", claims, fmt.Errorf("the registry answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var answer struct {
		Token string `json:"token"`
		auth.TokenClaims
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", claims, fmt.Errorf("cannot read the token response: %w", err)
	}
	if answer.Token == "" {
		return "", claims, fmt.Errorf("the registry returned no token")
	}
	return answer.Token, answer.TokenClaims, nil
}

// lookupCredential returns the stored credential for a registry.
func lookupCredential(registry string) (auth.Credential, error) {
	store, err := authStore()
	if err != nil {
		return auth.Credential{}, err
	}
	return store.Get(registry, "")
}

// authHeaderForRegistry returns the Authorization header value to use, or "".
//
// A stored token is used when there is one. Otherwise the failure says how to
// get one, because creating a token is a deliberate act rather than something
// that happens on its own.
func authHeaderForRegistry(registry string) (string, error) {
	store, err := authStore()
	if err != nil {
		return "", nil
	}
	credential, err := store.Get(registry, "")
	if err != nil {
		return "", nil
	}
	if credential.Expired(time.Now()) {
		return "", nil
	}
	return auth.Authorization(credential.Secret), nil
}

// authNotices renders the credential warnings a command should show, so an
// expiring token is noticed during ordinary work rather than at the moment it
// fails.
func authNotices() string {
	store, err := authStore()
	if err != nil {
		return ""
	}
	notices, err := store.Describe(time.Now())
	if err != nil || len(notices) == 0 {
		return ""
	}
	return auth.FormatNotices(notices)
}

// printAuthNotices shows the warnings, if any, ahead of a command's output.
func printAuthNotices() {
	if text := authNotices(); text != "" {
		fmt.Fprintln(os.Stderr, text)
	}
}
