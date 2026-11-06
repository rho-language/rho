//
// @file revocation.go
// @brief Revocation of issued SSO tickets.
//
// A ticket has an expiry, but an administrator may want one no longer valid
// before then. The revocation list is a file of token ids, one per line, that
// a verifier reads when it authenticates a ticket: a ticket whose id is in the
// list is refused even though its signature is intact.
//
// The file is owner-only and lives beside the signing key, because it is the
// other half of the same trust: whoever can edit it can decide who is allowed
// in.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultRevokedPath is where the revocation list lives under a Rho home.
func DefaultRevokedPath(rhoHome string) string {
	return filepath.Join(rhoHome, "auth", "revoked.txt")
}

// IsRevoked reports whether a ticket id is in a revocation list.
//
// An empty list or an unreadable file is treated as an empty list: a missing
// revocation file must not break a service that has never revoked anything.
// An empty token id is never revoked.
func IsRevoked(tokenID, file string) bool {
	if tokenID == "" || file == "" {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == tokenID {
			return true
		}
	}
	return false
}

// AppendRevoked records a ticket id as revoked.
func AppendRevoked(tokenID, file string) error {
	if tokenID == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	handle, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(handle, "%s\n", tokenID); err != nil {
		_ = handle.Close()
		return err
	}
	return handle.Close()
}