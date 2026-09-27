//
// @file main.go
// @brief Rho auth component.
//
// Manages the credentials a client uses to talk to a registry: logging in,
// showing what is stored, and revoking it.
//
// Usage:
//
//	rho auth login   --registry <url> --user <name> [--days 30]
//	rho auth status  [--registry <url>]
//	rho auth list
//	rho auth inspect --token <token> | --registry <url>
//	rho auth revoke  --registry <url>
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rho/rho-auth/src/auth"
	rholog "github.com/rho/rho-log/src/logger"
)

// log identifies this component in a shared log.
var log = rholog.Default("rho-auth")

// registry is where a client finds a registry, defaulting to the configured one.
const defaultRegistry = "https://registry.rho-lang.org"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "login":
		return login(args[1:])
	case "status":
		return status(args[1:])
	case "list":
		return list(args[1:])
	case "inspect":
		return inspect(args[1:])
	case "revoke":
		return revoke(args[1:])
	case "help", "-h", "--help":
		usage()
		return 0
	}

	fmt.Fprintf(os.Stderr, "rho auth: unknown command %q\n", args[0])
	usage()
	return 2
}

func usage() {
	fmt.Println("Usage: rho auth <login|status|list|inspect|revoke> [flags]")
}

// openStore opens the credential store in the user's Rho home.
func openStore() (*auth.Store, error) {
	return auth.StoreForHost()
}

// flags is the small set of options every subcommand shares, so the parsing
// stays in one place.
type flags struct {
	registry string
	user     string
	host     string
	days     int
	token    string
	asJSON   bool
}

func parseFlags(args []string) (flags, []string, error) {
	out := flags{registry: defaultRegistry, days: 30}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		value := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch args[i] {
		case "--registry", "-r":
			out.registry, err = value("--registry")
		case "--user", "-u":
			out.user, err = value("--user")
		case "--host":
			out.host, err = value("--host")
		case "--token":
			out.token, err = value("--token")
		case "--days":
			var text string
			text, err = value("--days")
			if err == nil {
				var days int
				if _, scanErr := fmt.Sscanf(text, "%d", &days); scanErr != nil {
					err = fmt.Errorf("--days must be a number")
				} else {
					out.days = days
				}
			}
		case "--json":
			out.asJSON = true
		default:
			rest = append(rest, args[i])
		}
		if err != nil {
			return out, nil, err
		}
	}
	return out, rest, nil
}

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

func login(args []string) int {
	options, _, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 2
	}
	user := options.user
	if user == "" {
		user = currentUser()
	}
	host := options.host
	if host == "" {
		host = currentHost()
	}
	if options.days <= 0 || options.days > int(auth.MaxTokenLifetime.Hours()/24) {
		fmt.Fprintf(os.Stderr, "a token may last between 1 and %d days\n", int(auth.MaxTokenLifetime.Hours()/24))
		return 2
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 1
	}
	token, claims, err := auth.IssueTokenFor(store, options.registry, user, host, options.days)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth login:", err)
		return 1
	}

	credential := auth.Credential{
		Method:    auth.TokenMethod,
		Registry:  options.registry,
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
	log.Info("credential stored", "registry", options.registry, "subject", claims.Subject)
	fmt.Printf("Token for %s stored, valid until %s.\n", claims.Subject, credential.ExpiresAt.Format("2006-01-02"))
	return 0
}

func status(args []string) int {
	options, _, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth status:", err)
		return 2
	}
	store, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth status:", err)
		return 1
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
		if options.registry != defaultRegistry && credential.Registry != options.registry {
			continue
		}
		state := "valid"
		if credential.Expired(now) {
			state = "expired"
		}
		fmt.Printf("%-40s %-8s %s\n", credential.Registry, state, credential.Display())
	}
	// Anything about to lapse is worth saying, so a failure is not a surprise.
	notices, err := store.Describe(now)
	if err == nil && len(notices) > 0 {
		fmt.Println()
		fmt.Println(auth.FormatNotices(notices))
	}
	return 0
}

func list(args []string) int {
	return status(args)
}

// inspect decodes a token and reports what it says. The claims of a signed
// token are readable, so this needs no secret; whether the signature is intact
// is a question only the registry can answer.
func inspect(args []string) int {
	options, _, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth inspect:", err)
		return 2
	}
	subject := options.token
	if subject == "" {
		store, err := openStore()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth inspect:", err)
			return 1
		}
		stored, err := store.Get(options.registry, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth inspect:", err)
			return 1
		}
		subject = stored.Secret
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
		auth.Remaining(time.Unix(claims.ExpiresAt, 0)))
	return 0
}

func revoke(args []string) int {
	options, _, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth revoke:", err)
		return 2
	}
	store, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth revoke:", err)
		return 1
	}
	removed, err := store.Revoke(options.registry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth revoke:", err)
		return 1
	}
	if !removed {
		fmt.Printf("No stored credential for %s.\n", options.registry)
		return 0
	}
	log.Info("credential revoked", "registry", options.registry)
	fmt.Printf("Revoked the stored credential for %s.\n", options.registry)
	return 0
}
