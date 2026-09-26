//
// @file auth.go
// @brief The rho auth command: credentials for talking to a registry.
//
// Tokens are created deliberately rather than appearing on their own. A
// registry that hands one to anything that asks provides no authentication, so
// obtaining one is something a person does:
//
//	rho auth login --registry http://localhost:8080
//
// The token is kept in the Rho home, and `rho auth status` reports what is
// stored and how long it has left, so a credential is never a surprise in an
// unrelated command.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rho/internal/paths"
	"rho/src/auth"
)

// authStore returns the credential store under the Rho home.
func authStore() (*auth.Store, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return nil, err
	}
	return auth.Open(auth.DefaultStorePath(home)), nil
}

// authSecretPath returns the signing secret path under the Rho home. A registry
// reads it to sign; a client never needs it.
func authSecretPath() (string, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return "", err
	}
	return auth.DefaultSecretPath(home), nil
}

func authCLI(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: rho auth <login|status|list|revoke> [flags]")
		return 2
	}

	switch args[0] {
	case "login":
		return authLogin(args[1:])
	case "status":
		return authStatus(args[1:])
	case "list":
		return authList(args[1:])
	case "revoke":
		return authRevoke(args[1:])
	case "inspect":
		return authInspect(args[1:])
	case "help", "-h", "--help":
		fmt.Println("Usage: rho auth <login|status|list|revoke> [flags]")
		return 0
	}

	fmt.Fprintf(os.Stderr, "Unknown auth command: %s\n", args[0])
	return 2
}

// authLogin asks a registry for a token and stores it.
//
// A registry issues one through POST /auth/token, answering with the same
// token format the module client presents.
func authLogin(args []string) int {
	fs := flag.NewFlagSet("rho auth login", flag.ExitOnError)
	registry := fs.String("registry", "", "registry base URL")
	subject := fs.String("user", currentUser(), "who the token is for")
	host := fs.String("host", currentHost(), "hostname recorded in the token")
	days := fs.Int("days", 30, "how many days the token should last, at most 90")
	fs.Parse(args)

	if *registry == "" {
		*registry = defaultRegistryURL()
	}
	if *days <= 0 || *days > int(auth.MaxTokenLifetime.Hours()/24) {
		fmt.Fprintf(os.Stderr, "A token may last between 1 and %d days.\n",
			int(auth.MaxTokenLifetime.Hours()/24))
		return 2
	}

	token, claims, err := requestToken(*registry, *subject, *host, time.Duration(*days)*24*time.Hour)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 1
	}

	store, err := authStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 1
	}
	credential := auth.Credential{
		Method:    auth.TokenMethod,
		Registry:  *registry,
		Secret:    token,
		Subject:   claims.Subject,
		Host:      claims.Host,
		Scopes:    claims.Scopes,
		IssuedAt:  time.Unix(claims.IssuedAt, 0),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0),
	}
	if err := store.Put(credential); err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 1
	}

	fmt.Printf("Token for %s stored, valid until %s.\n",
		claims.Subject, credential.ExpiresAt.Format("2006-01-02"))
	return 0
}

// authStatus reports the stored credentials and anything that needs attention,
// which is what a user runs when a command failed for no clear reason.
func authStatus(args []string) int {
	fs := flag.NewFlagSet("rho auth status", flag.ExitOnError)
	registry := fs.String("registry", "", "limit the report to one registry")
	fs.Parse(args)

	store, err := authStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth status:", err)
		return 1
	}
	notices, err := store.Describe(time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth status:", err)
		return 1
	}
	if *registry != "" {
		kept := make([]auth.Notice, 0, len(notices))
		for _, notice := range notices {
			if notice.Registry == *registry {
				kept = append(kept, notice)
			}
		}
		notices = kept
	}

	credentials, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth status:", err)
		return 1
	}
	if len(credentials) == 0 {
		fmt.Println("No credentials stored.")
		fmt.Println("Log in with: rho auth login --registry <url>")
		return 0
	}

	now := time.Now()
	for _, credential := range credentials {
		state := "valid"
		if credential.Expired(now) {
			state = "expired"
		}
		line := fmt.Sprintf("%s  %s  %s", credential.Registry, state, credential.Display())
		fmt.Println(line)
	}
	if len(notices) > 0 {
		fmt.Println()
		fmt.Println(auth.FormatNotices(notices))
	}
	return 0
}

// authList shows the stored credentials without the notices.
func authList(args []string) int {
	store, err := authStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth list:", err)
		return 1
	}
	credentials, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth list:", err)
		return 1
	}
	if len(credentials) == 0 {
		fmt.Println("No credentials stored.")
		return 0
	}
	now := time.Now()
	for _, credential := range credentials {
		state := "valid"
		if credential.Expired(now) {
			state = "expired"
		}
		fmt.Printf("%-40s %-8s %s\n", credential.Registry, state, credential.Display())
	}
	return 0
}

// authInspect decodes a token and reports what it says.
//
// The claims of a signed token are readable by anyone holding it, so this
// shows them without a secret. Whether the signature is intact is reported
// separately, since that is the security question.
func authInspect(args []string) int {
	fs := flag.NewFlagSet("rho auth inspect", flag.ExitOnError)
	token := fs.String("token", "", "the token to inspect")
	from := fs.String("registry", "", "read the stored token for this registry instead")
	fs.Parse(args)

	subject := *token
	if subject == "" && *from != "" {
		stored, err := lookupCredential(*from)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth inspect:", err)
			return 1
		}
		subject = stored.Secret
	}
	if subject == "" {
		fmt.Println("Usage: rho auth inspect --token <token> | --registry <url>")
		return 2
	}

	claims, err := auth.Inspect(subject)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth inspect:", err)
		return 1
	}

	fmt.Printf("subject    %s\n", claims.Subject)
	fmt.Printf("issuer     %s\n", claims.Issuer)
	fmt.Printf("token id   %s\n", claims.TokenID)
	if claims.Audience != "" {
		fmt.Printf("audience   %s\n", claims.Audience)
	}
	if claims.Host != "" {
		fmt.Printf("host       %s\n", claims.Host)
	}
	if len(claims.Scopes) > 0 {
		fmt.Printf("scopes     %s\n", strings.Join(claims.Scopes, ", "))
	}
	fmt.Printf("issued     %s\n", time.Unix(claims.IssuedAt, 0).Format("2006-01-02 15:04"))
	fmt.Printf("expires    %s (%s)\n",
		time.Unix(claims.ExpiresAt, 0).Format("2006-01-02 15:04"),
		expiryDescription(claims.ExpiresAt))

	// The signature can only be checked with the registry's secret, which the
	// client does not have, so this is honest about what it cannot tell.
	fmt.Println()
	fmt.Println("The claims above are readable, not hidden: a signed token is not")
	fmt.Println("encrypted. Only the registry can confirm the signature, and it does")
	fmt.Println("on every request.")
	return 0
}

func expiryDescription(expiry int64) string {
	remaining := time.Until(time.Unix(expiry, 0))
	if remaining <= 0 {
		return "expired"
	}
	days := int(remaining.Hours() / 24)
	if days == 0 {
		return "in under a day"
	}
	return fmt.Sprintf("in %d days", days)
}

// authRevoke removes the credential for a registry. It is a local removal: the
// registry still honours the token until it expires, so a leaked token is
// handled there.
func authRevoke(args []string) int {
	fs := flag.NewFlagSet("rho auth revoke", flag.ExitOnError)
	registry := fs.String("registry", defaultRegistryURL(), "registry to revoke for")
	fs.Parse(args)

	store, err := authStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth revoke:", err)
		return 1
	}
	removed, err := store.Revoke(*registry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth revoke:", err)
		return 1
	}
	if !removed {
		fmt.Printf("No stored credential for %s.\n", *registry)
		return 0
	}
	fmt.Printf("Revoked the stored credential for %s.\n", *registry)
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// currentUser is who a token is issued to when the caller does not say.
func currentUser() string {
	for _, key := range []string{"RHO_USER", "USER", "LOGNAME"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return "unknown"
}

// currentHost is the hostname recorded in a token, for display and auditing.
func currentHost() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "unknown"
	}
	return name
}

// ensureAuthDirs creates the Rho home auth directory, so a first login has
// somewhere to write.
func ensureAuthDirs() error {
	home, err := paths.GetRhoHome()
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(home, "auth"), 0o700)
}
