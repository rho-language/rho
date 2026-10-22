//
// @file signing_policy_test.go
// @brief Strict mode, roots, unsigned installs, and the key file itself.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A wildcard root is a signer trusted for every module.
func TestARootIsTrustedForAnyModule(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "1")
	key, public := aKey(t)
	if err := TrustSigner("*", public); err != nil {
		t.Fatal(err)
	}
	digest := digestOf([]byte("x"))
	signature := SignPackage(key, "anything", "1.0.0", digest)
	if err := checkSigner("anything", "1.0.0", digest, &signature); err != nil {
		t.Fatalf("a signature from a root was refused: %v", err)
	}
}

// Strict mode: nothing unsigned and nothing from a signer that is not already
// pinned, so no first use.
func TestStrictModeRefusesUnsignedAndUnpinned(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "1")
	digest := digestOf([]byte("x"))

	if err := checkSigner("math", "1.0.0", digest, nil); err == nil {
		t.Error("an unsigned package was accepted in strict mode")
	}
	key, _ := aKey(t)
	signature := SignPackage(key, "math", "1.0.0", digest)
	if err := checkSigner("math", "1.0.0", digest, &signature); err == nil {
		t.Error("an unpinned signer was accepted in strict mode")
	}
	if trusted, _ := readTrust(); len(trusted["math"]) != 0 {
		t.Error("strict mode pinned a signer on first use")
	}
}

// Outside strict mode an unsigned package still installs, and says so.
func TestAnUnsignedPackageInstallsWithAWarning(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "")
	var said bytes.Buffer
	old := unverifiedNotice
	unverifiedNotice = &said
	t.Cleanup(func() { unverifiedNotice = old })

	if err := checkSigner("math", "1.0.0", digestOf([]byte("x")), nil); err != nil {
		t.Fatalf("an unsigned package was refused outside strict mode: %v", err)
	}
	if !strings.Contains(said.String(), "not signed") {
		t.Errorf("an unsigned install said nothing: %q", said.String())
	}
}

// The key file is created owner-only and is never silently replaced.
func TestTheSigningKeyIsPrivateAndNotReplaced(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	public, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	file, _ := signingKeyFile()
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the signing key is %v, not owner-only", info.Mode().Perm())
	}
	key, found, err := LoadSigningKey()
	if err != nil || !found || PublicKeyOf(key) != public {
		t.Fatalf("the key was not read back: %v %v", found, err)
	}
	if _, err := GenerateSigningKey(); err == nil {
		t.Error("an existing signing key was replaced")
	}
}

// Pinning a key that is not a key is refused rather than written.
func TestTrustRefusesANonKey(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	if err := TrustSigner("math", "not-a-key"); err == nil {
		t.Error("a non-key was pinned")
	}
	if err := TrustSigner("bad name", strings.Repeat("ab", 32)); err == nil {
		t.Error("a name with a space was pinned")
	}
}
