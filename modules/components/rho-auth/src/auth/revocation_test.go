//
// @file revocation_test.go
// @brief Tests for the revocation list.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevokeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "auth", "revoked.txt")

	if err := AppendRevoked("abc123", file); err != nil {
		t.Fatalf("AppendRevoked: %v", err)
	}
	if !IsRevoked("abc123", file) {
		t.Fatal("a revoked id was not refused")
	}
	if IsRevoked("other", file) {
		t.Fatal("an id that was never revoked was refused")
	}
	if IsRevoked("", file) {
		t.Fatal("an empty id must not be revoked")
	}
}

func TestRevokedMissingFile(t *testing.T) {
	if IsRevoked("abc", filepath.Join(t.TempDir(), "none")) {
		t.Fatal("a missing revocation list refused a token")
	}
}

func TestRevokedFileIsOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "auth", "revoked.txt")
	if err := AppendRevoked("x", file); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("revoked file mode = %o, want 600", stat.Mode().Perm())
	}
}