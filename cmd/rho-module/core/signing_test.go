//
// @file signing_test.go
// @brief A package signed by someone, and what that does and does not prove.
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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// aKey is a fresh key pair for a test.
func aKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return private, hex.EncodeToString(public)
}

// A signature verifies for the package it was made over, and for nothing else.
func TestASignatureIsOverOneVersionOfOnePackage(t *testing.T) {
	key, _ := aKey(t)
	digest := digestOf([]byte("package"))
	signature := SignPackage(key, "math", "1.0.0", digest)

	if err := VerifySignature("math", "1.0.0", digest, signature); err != nil {
		t.Fatalf("a valid signature was refused: %v", err)
	}
	if err := VerifySignature("math", "1.0.1", digest, signature); err == nil {
		t.Error("a signature for 1.0.0 verified for 1.0.1")
	}
	if err := VerifySignature("other", "1.0.0", digest, signature); err == nil {
		t.Error("a signature for math verified for another module")
	}
	if err := VerifySignature("math", "1.0.0", digestOf([]byte("changed")), signature); err == nil {
		t.Error("a signature verified for different bytes")
	}
	if err := VerifySignature("math", "1.0.0", digest, Signature{Signer: "zz", Signature: "zz"}); err == nil {
		t.Error("a malformed signature verified")
	}
}

// Trust on first use: the first signer a module shows is who it is pinned to.
func TestFirstUsePinsTheSigner(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "")
	var said bytes.Buffer
	old := unverifiedNotice
	unverifiedNotice = &said
	t.Cleanup(func() { unverifiedNotice = old })

	key, public := aKey(t)
	digest := digestOf([]byte("package"))
	signature := SignPackage(key, "math", "1.0.0", digest)
	if err := checkSigner("math", "1.0.0", digest, &signature); err != nil {
		t.Fatalf("a first signed install was refused: %v", err)
	}
	if !strings.Contains(said.String(), "pinned to signer "+public) {
		t.Errorf("the pin was not announced: %q", said.String())
	}
	trusted, _ := readTrust()
	if len(trusted["math"]) != 1 || trusted["math"][0] != public {
		t.Fatalf("the signer was not recorded: %v", trusted)
	}
}

// After that, a package from anyone else is refused: this is the registry that
// swaps a package for an existing module, and it is what a signature is for.
func TestAPinnedModuleRefusesAnotherSigner(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "")
	original, public := aKey(t)
	attacker, _ := aKey(t)
	if err := TrustSigner("math", public); err != nil {
		t.Fatal(err)
	}

	digest := digestOf([]byte("malicious package"))
	forged := SignPackage(attacker, "math", "1.0.1", digest)
	err := checkSigner("math", "1.0.1", digest, &forged)
	if err == nil || !strings.Contains(err.Error(), "pinned to a different signer") {
		t.Fatalf("a package signed by someone else was accepted: %v", err)
	}

	genuine := SignPackage(original, "math", "1.0.1", digest)
	if err := checkSigner("math", "1.0.1", digest, &genuine); err != nil {
		t.Fatalf("the pinned signer was refused: %v", err)
	}
}

// A package that has lost its signature is not the one that was pinned. Stripping
// the signature is the obvious next move once signing is expected.
func TestAPinnedModuleRefusesAPackageWithNoSignature(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	_, public := aKey(t)
	if err := TrustSigner("math", public); err != nil {
		t.Fatal(err)
	}
	err := checkSigner("math", "1.0.1", digestOf([]byte("x")), nil)
	if err == nil || !strings.Contains(err.Error(), "lost its signature") {
		t.Fatalf("an unsigned package was accepted for a pinned module: %v", err)
	}
}
