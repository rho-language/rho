//
// @file crypto.go
// @brief Rho Install - Credential encryption utilities.
//
// Responsibilities:
//   - AES-256-GCM encryption/decryption
//   - Deterministic key generation based on machine-id + user-id
//   - Secure storage for registry plugin credentials
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  Key Derivation                                                             */
/* -------------------------------------------------------------------------- */

// deriveKey generates a deterministic AES-256 key using machine-id + user-id.
//
// The machine identity is looked for where it is, and where it is not there is
// something to say rather than a reason to fail. A machine with no
// `/etc/machine-id` is not a broken machine: a container image is one, an
// appliance filesystem is one, and a build that refuses to install on it because
// of it reports a file missing rather than the thing the reader cares about,
// which is that this installation is not the same one as another machine's.
//
// The second path is dbus's copy of the same identifier, which is what a system
// that does not use systemd writes instead. The third is a generated identifier,
// kept beside the tool home, which is the only one that is any use for secrecy —
// and which is a different answer from one machine to the next by construction,
// because a machine with no identity of its own has to be given one.
func deriveKey() ([]byte, error) {
	machineID, err := machineIdentity()
	if err != nil {
		return nil, err
	}

	userID := os.Getenv("USER")
	if userID == "" {
		userID = "unknown"
	}

	h := sha256.New()
	h.Write(machineID)
	h.Write([]byte(userID))

	return h.Sum(nil), nil
}

/* -------------------------------------------------------------------------- */
/*  Encryption                                                                 */
/* -------------------------------------------------------------------------- */

// Encrypt encrypts plaintext using AES-256-GCM.
func Encrypt(plaintext []byte) ([]byte, error) {
	key, err := deriveKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cannot create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cannot create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("cannot generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

/* -------------------------------------------------------------------------- */
/*  Decryption                                                                 */
/* -------------------------------------------------------------------------- */

// Decrypt decrypts ciphertext using AES-256-GCM.
func Decrypt(ciphertext []byte) ([]byte, error) {
	key, err := deriveKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cannot create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cannot create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// machineIdentityPaths are where a machine says what it is, in the order to ask.
var machineIdentityPaths = []string{
	"/etc/machine-id",
	"/var/lib/dbus/machine-id",
}

// generatedIdentityFile is where a machine with no identity of its own is given
// one. Inside the tool home rather than beside the machine's, because writing to
// /etc is not something an installer may do and because a tool home it already
// creates is somewhere the answer belongs.
func generatedIdentityFile() string {
	return filepath.Join(deps.RhoHome(), "machine-id")
}

// machineIdentity is this machine's identifier.
func machineIdentity() ([]byte, error) {
	for _, path := range machineIdentityPaths {
		if id, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(id))) > 0 {
			return []byte(strings.TrimSpace(string(id))), nil
		}
	}

	// None of them. Read what was generated for this installation last time, or
	// generate one and keep it: a key derived from a fresh random value on every
	// run would decrypt nothing at all, which is the one outcome a cipher must not
	// have.
	home := generatedIdentityFile()
	if id, err := os.ReadFile(home); err == nil && len(strings.TrimSpace(string(id))) > 0 {
		return []byte(strings.TrimSpace(string(id))), nil
	}

	generated := make([]byte, 32)
	if _, err := rand.Read(generated); err != nil {
		return nil, fmt.Errorf("no machine identity to read and none could be generated: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(home), 0o755); err != nil {
		// A tool home it cannot write to is a machine where the identity cannot be
		// kept, and an identity that changes is worse than none.
		return nil, fmt.Errorf("cannot keep a generated machine identity at %s: %w", home, err)
	}
	// The identity is the hex of what was generated, and it is that string which is
	// both written and derived from. Writing the hex and returning the bytes would
	// mean this call and the next one — which reads the file back — derive different
	// keys from the same identity, and the symptom is a cipher that encrypts what it
	// cannot then decrypt.
	identity := hex.EncodeToString(generated)
	if err := os.WriteFile(home, []byte(identity+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("cannot keep a generated machine identity at %s: %w", home, err)
	}
	return []byte(identity), nil
}
