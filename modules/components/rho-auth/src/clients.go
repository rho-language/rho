package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"crypto/rand"

	"github.com/rho/rho-auth/src/auth"
)

// clientsPath is the registry a command reads: --store, or the installation's.
func clientsPath(options map[string]string) (string, error) {
	if path := options["store"]; path != "" {
		return path, nil
	}
	home, err := auth.RhoHome()
	if err != nil {
		return "", err
	}
	return auth.DefaultClientsPath(home), nil
}

// client implements `rho auth client add|remove|list`.
//
//	rho auth client add --id aquarium --audiences mesh --scopes mesh:home,mesh:home:actuate
//
// add prints the secret once; the registry keeps only its hash.
func client(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: rho auth client <add|remove|list> [--store FILE] [--id ID] [--audiences A,B] [--scopes S,T] [--secret-out FILE]")
		return 2
	}
	options := authOptions(args[1:])
	path, err := clientsPath(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth client:", err)
		return 1
	}
	switch args[0] {
	case "add":
		secret, err := auth.AddClient(path, options["id"], splitOption(options["audiences"]), splitOption(options["scopes"]))
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth client add:", err)
			return 1
		}
		// --secret-out hands the secret to the service that will use it without
		// it passing through anyone's terminal or a log: the file is created
		// readable by its owner only.
		if out := options["secret-out"]; out != "" {
			if err := auth.WriteSecretFile(out, secret+"\n"); err != nil {
				fmt.Fprintln(os.Stderr, "rho auth client add:", err)
				return 1
			}
			return 0
		}
		fmt.Println(secret)
		return 0
	case "remove":
		if err := auth.RemoveClient(path, options["id"]); err != nil {
			fmt.Fprintln(os.Stderr, "rho auth client remove:", err)
			return 1
		}
		return 0
	case "list":
		clients, err := auth.LoadClients(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth client list:", err)
			return 1
		}
		for id, c := range clients {
			fmt.Printf("%s\taudiences=%s\tscopes=%s\n", id, strings.Join(c.Audiences, ","), strings.Join(c.Scopes, ","))
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho auth client: unknown command %q\n", args[0])
	return 2
}

// serviceToken implements `rho auth service-token`: a registered client's
// credential exchanged for a short-lived token for one audience.
//
//	rho auth service-token --id aquarium --secret-stdin --audience mesh \
//	    --scopes mesh:home --private keys/private.pem --issuer keeper --ttl 10m
//
// The secret is read from standard input or from a file, never from the command
// line, where every user of the machine can read it.
func serviceToken(args []string, stdin io.Reader) int {
	options := authOptions(args)
	path, err := clientsPath(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	secret := ""
	switch {
	case options["secret-file"] != "":
		data, err := os.ReadFile(options["secret-file"])
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
			return 1
		}
		secret = strings.TrimSpace(string(data))
	case options["secret-stdin"] == "true":
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		secret = strings.TrimSpace(line)
	default:
		fmt.Fprintln(os.Stderr, "rho auth service-token: give the secret with --secret-stdin or --secret-file")
		return 2
	}
	registered, err := auth.Authenticate(path, options["id"], secret)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	scopes, err := registered.Grant(options["audience"], splitOption(options["scopes"]))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	// A service token is short by construction: ten minutes unless asked for
	// less, and never more, so a leaked one is a short problem.
	ttl := 10 * time.Minute
	if options["ttl"] != "" {
		asked, err := time.ParseDuration(options["ttl"])
		if err != nil || asked <= 0 {
			fmt.Fprintln(os.Stderr, "rho auth service-token: --ttl must be a positive duration")
			return 2
		}
		if asked < ttl {
			ttl = asked
		}
	}
	private, err := auth.LoadPrivateKey(options["private"])
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	now := time.Now()
	token, err := auth.SignTicket(private, auth.TicketClaims{
		Subject: "client:" + registered.ID, Issuer: options["issuer"], Audience: options["audience"],
		TokenID: hex.EncodeToString(raw), Scopes: scopes,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho auth service-token:", err)
		return 1
	}
	fmt.Printf("{\"token\":%q,\"expires_in\":%d}\n", token, int(ttl.Seconds()))
	return 0
}

func splitOption(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
