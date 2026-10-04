//
// @file crypto_test.go
// @brief Tests for Rho Install - Credential encryption utilities.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Encrypt / Decrypt                                                          */
/* -------------------------------------------------------------------------- */

func TestEncryptDecrypt(t *testing.T) {
    plaintext := []byte("super-secret-token")

    ciphertext, err := Encrypt(plaintext)
    if err != nil {
        t.Fatalf("Encrypt failed: %v", err)
    }

    decrypted, err := Decrypt(ciphertext)
    if err != nil {
        t.Fatalf("Decrypt failed: %v", err)
    }

    if string(decrypted) != string(plaintext) {
        t.Fatalf("Decrypt mismatch: expected %s, got %s", plaintext, decrypted)
    }
}

/* -------------------------------------------------------------------------- */
/*  Decrypt invalid data                                                       */
/* -------------------------------------------------------------------------- */

func TestDecryptInvalid(t *testing.T) {
    _, err := Decrypt([]byte("invalid"))
    if err == nil {
        t.Fatalf("Decrypt should fail for invalid ciphertext")
    }
}
