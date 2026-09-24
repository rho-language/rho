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
    "fmt"
    "io"
    "os"
)

/* -------------------------------------------------------------------------- */
/*  Key Derivation                                                             */
/* -------------------------------------------------------------------------- */

// deriveKey generates a deterministic AES-256 key using machine-id + user-id.
func deriveKey() ([]byte, error) {
    machineID, err := os.ReadFile("/etc/machine-id")
    if err != nil {
        return nil, fmt.Errorf("cannot read machine-id: %w", err)
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
