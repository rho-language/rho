//
// @file verify.go
// @brief Checking a downloaded package against what the registry recorded.
//
// Two paths fetch a package — `rho module install` and a dependency download — and
// each used to carry its own copy of the check, and each copy skipped it when the
// registry recorded no digest. That is the wrong default: a package with nothing to
// compare it against is a package nothing has vouched for, and "the registry said
// nothing" is exactly what a registry that wanted a package accepted would say.
//
// So there is one function, and it refuses. A person who trusts a registry that
// records no digests says so in the environment (RHO_ALLOW_UNVERIFIED_PACKAGES=1),
// and the install says out loud that it went ahead unverified.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"rho/cmd/rho-module/remote"
)

// unverifiedNotice is where the note about an unverified install goes. A variable
// so a test can read what was said.
var unverifiedNotice io.Writer = os.Stderr

// digestOf is the SHA-256 of a package, as lower-case hex.
func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// verifyPackage checks a package against the digest the registry recorded for it,
// and answers the digest it hashed to.
//
// A digest that does not agree is refused. So is no digest at all, unless this
// machine has said it accepts that — in which case the package is installed and the
// fact is written to standard error, because an unverified install that is silent
// is indistinguishable from a verified one afterwards.
func verifyPackage(name, version string, data []byte, record remote.VersionRecord) (string, error) {
	got := digestOf(data)
	recorded := strings.TrimSpace(record.SHA256)
	if recorded == "" {
		if !remote.AllowUnverified() {
			return "", fmt.Errorf(
				"the registry recorded no checksum for %s@%s, so what was downloaded cannot be checked; "+
					"set RHO_ALLOW_UNVERIFIED_PACKAGES=1 to install it anyway", name, version)
		}
		fmt.Fprintf(unverifiedNotice,
			"warning: installing %s@%s without a checksum from the registry (sha256 %s)\n", name, version, got)
	} else if !strings.EqualFold(got, recorded) {
		return "", fmt.Errorf("%s@%s does not match the registry's checksum: got %s, want %s",
			name, version, got, recorded)
	}

	// The checksum says the bytes are the registry's. Who published them is a
	// separate question, and it is asked of the digest that was actually hashed,
	// not of the one the registry claims: a registry that served a different
	// package with a different digest would otherwise be asked to vouch for itself.
	var signed *Signature
	if record.Signature != "" {
		signed = &Signature{Signer: record.Signer, Signature: record.Signature}
	}
	if err := checkSigner(name, version, got, signed); err != nil {
		return "", err
	}
	return got, nil
}
