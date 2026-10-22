//
// @file keys.go
// @brief The publisher's signing key, kept where only they can read it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
)

// signingKeyFile is where the publisher's private key is kept.
func signingKeyFile() (string, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config", "signing.key"), nil
}

// GenerateSigningKey makes a key and writes the private half, owner-only, and
// answers the public half. It refuses to replace one that exists: a key overwritten
// is a publisher who can no longer be recognised by the people who pinned the old
// one.
func GenerateSigningKey() (string, error) {
	file, err := signingKeyFile()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(file); err == nil {
		return "", fmt.Errorf("a signing key already exists at %s; remove it first if you mean to replace it, "+
			"knowing that everyone who pinned it will refuse what you sign next", file)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", err
	}
	// Exclusive and 0600: the file is created by this call or not at all, and is
	// never readable by another user between creation and a later chmod.
	out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := out.WriteString(hex.EncodeToString(private) + "\n"); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(public), nil
}

// LoadSigningKey reads the publisher's key. A missing key is not an error: it is a
// publisher who does not sign, and the caller says so.
func LoadSigningKey() (ed25519.PrivateKey, bool, error) {
	file, err := signingKeyFile()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, false, fmt.Errorf("%s is not an ed25519 private key", file)
	}
	return ed25519.PrivateKey(raw), true, nil
}

// PublicKeyOf is the hex public half of a private key.
func PublicKeyOf(key ed25519.PrivateKey) string {
	return hex.EncodeToString(key.Public().(ed25519.PublicKey))
}
