//
// @file trust.go
// @brief Whom to believe about who published a package.
//
// Trust is a file of `name key` lines that the person owns: `*` as a name is a root
// for any module, and a module that has no line is pinned to the first valid signer
// it shows — trust on first use, which is what makes a registry that later swaps a
// package for an existing module detectable.
//
// Unsigned packages still install, with a warning, because the registry holds
// packages published before this existed and refusing them would stop every install
// at once. RHO_REQUIRE_SIGNATURES=1 is the strict setting: nothing unsigned, and no
// first use.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"bufio"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
)

// trustFile is the pinned signers: one `name key` pair per line.
func trustFile() (string, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config", "trusted-signers"), nil
}

// readTrust is the pinned keys, by module name; `*` holds the roots.
func readTrust() (map[string][]string, error) {
	file, err := trustFile()
	if err != nil {
		return nil, err
	}
	handle, err := os.Open(file)
	if os.IsNotExist(err) {
		return map[string][]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	trusted := map[string][]string{}
	scanner := bufio.NewScanner(io.LimitReader(handle, 1<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		trusted[fields[0]] = append(trusted[fields[0]], strings.ToLower(fields[1]))
	}
	return trusted, scanner.Err()
}

// TrustSigner pins a key for a module, or for every module with `*`.
func TrustSigner(name, key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != ed25519.PublicKeySize {
		return fmt.Errorf("%q is not an ed25519 public key: it is 64 hex digits", key)
	}
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("%q is not a module name", name)
	}
	file, err := trustFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%s %s\n", name, key); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// RequireSignatures reports whether this machine accepts only signed packages from
// a signer it already trusts.
func RequireSignatures() bool { return os.Getenv("RHO_REQUIRE_SIGNATURES") == "1" }

// checkSigner applies the trust policy to what a registry served for a package.
//
// The digest has already been checked against the registry's record; this is the
// part that asks who vouched for it.
func checkSigner(name, version, digest string, signature *Signature) error {
	trusted, err := readTrust()
	if err != nil {
		return fmt.Errorf("cannot read the pinned signers: %w", err)
	}
	pinned := trusted[name]

	if signature == nil || signature.Signature == "" {
		if len(pinned) > 0 {
			return fmt.Errorf("%s@%s is not signed, but %s is pinned to a signer; "+
				"a package that has lost its signature is not the one that was pinned", name, version, name)
		}
		if RequireSignatures() {
			return fmt.Errorf("%s@%s is not signed, and RHO_REQUIRE_SIGNATURES=1", name, version)
		}
		fmt.Fprintf(unverifiedNotice, "warning: %s@%s is not signed; its checksum matches the registry's, "+
			"which says nothing about who published it\n", name, version)
		return nil
	}

	if err := VerifySignature(name, version, digest, *signature); err != nil {
		return fmt.Errorf("%s@%s: %w", name, version, err)
	}
	signer := strings.ToLower(signature.Signer)
	for _, key := range append(append([]string{}, pinned...), trusted["*"]...) {
		if key == signer {
			return nil
		}
	}
	if len(pinned) > 0 {
		return fmt.Errorf("%s@%s is signed by %s, but %s is pinned to a different signer; "+
			"if the publisher changed keys, pin the new one with: rho module trust %s %s",
			name, version, signer, name, name, signer)
	}
	if RequireSignatures() {
		return fmt.Errorf("%s@%s is signed by %s, which is not pinned, and RHO_REQUIRE_SIGNATURES=1; "+
			"pin it with: rho module trust %s %s", name, version, signer, name, signer)
	}
	// First use: this signer is who the module is pinned to from now on.
	if err := TrustSigner(name, signer); err != nil {
		return fmt.Errorf("cannot pin the signer of %s: %w", name, err)
	}
	fmt.Fprintf(unverifiedNotice, "note: %s is now pinned to signer %s\n", name, signer)
	return nil
}
