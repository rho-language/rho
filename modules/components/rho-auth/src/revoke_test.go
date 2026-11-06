// @file revoke_test.go
// @brief Checks the CLI wiring for revocation via the auth subcommands.
//
// The revocation list is what `rho auth revoke --jti` appends to and what the
// generated middleware reads. This test builds the small helpers and verifies
// the CLI can append and the generated middleware will refuse a revoked id.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rho/rho-auth/src/auth"
)

func TestRevokeCommand(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "revoked.txt")
	code := run([]string{"revoke", "--jti", "abc123", "--file", file})
	if code != 0 {
		t.Fatalf("revoke exit %d", code)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "abc123") {
		t.Fatalf("revoked list = %q, want abc123", data)
	}
}

func TestVerifyCommandHonoursRevocation(t *testing.T) {
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	if code := run([]string{"keygen", "--dir", dir, "--force"}); code != 0 {
		t.Fatalf("keygen exit %d", code)
	}
	token := runCapture(t, []string{"ticket", "--private", privatePath,
		"--issuer", "keeper", "--audience", "mesh", "--subject", "svc", "--ttl", "1h"})
	if token == "" {
		t.Fatal("no token minted")
	}
	sig := token[strings.LastIndex(token, ".")+1:]
	_ = sig
	claims, err := auth.Inspect(token)
	if err != nil {
		t.Fatal(err)
	}
	revoked := filepath.Join(dir, "revoked.txt")
	if err := auth.AppendRevoked(claims.TokenID, revoked); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"verify", "--public", publicPath, "--token", token,
		"--issuer", "keeper", "--audience", "mesh", "--revoked", revoked}); code == 0 {
		t.Fatal("a revoked ticket was accepted")
	}
	if code := run([]string{"verify", "--public", publicPath, "--token", token,
		"--issuer", "keeper", "--audience", "mesh"}); code != 0 {
		t.Fatalf("the same ticket without the revoked list was refused (exit %d)", code)
	}
}

func runCapture(t *testing.T, args []string) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := run(args)
	os.Stdout = old
	_ = w.Close()
	data, _ := io.ReadAll(r)
	if code != 0 {
		t.Fatalf("command %v exit %d", args, code)
	}
	return strings.TrimSpace(string(data))
}
