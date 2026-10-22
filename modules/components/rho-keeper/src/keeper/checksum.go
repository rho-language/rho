//
// @file checksum.go
// @brief A downloaded toolchain is the vendor's file, or it is not installed.
//
// A toolchain is fetched from the internet, unpacked under the Rho home and then
// run. The URL says where it came from and nothing about what answered: a mirror
// that was compromised, a redirect, a proxy that rewrote the body would each hand
// over a program that is then executed with the person's own authority. The recipe
// therefore carries the digest the vendor publishes, and the archive is hashed
// while it is written and compared before anything is unpacked.
//
// A recipe with no digest is refused rather than fetched on trust. Saying so costs
// a line in a manifest; not saying so costs a machine.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// validSHA256 reports whether a string is sixty-four hex digits.
func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// requireDigest says whether a recipe may be fetched. It is the check made before
// anything is downloaded, so a person is told what is wrong with the recipe and not
// after a gigabyte has arrived.
func requireDigest(program string, digest string) error {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if digest == "" {
		return fmt.Errorf("%s has a prebuilt archive but no sha256 for it, so what is downloaded could not be "+
			"checked; the module that declares it needs a `sha256:` in its `fetch` block "+
			"(the digest the vendor publishes)", program)
	}
	if !validSHA256(digest) {
		return fmt.Errorf("the sha256 declared for %s is not a SHA-256 digest: it is sixty-four hex digits, got %q",
			program, digest)
	}
	return nil
}

// fileDigest is the SHA-256 of a file on disk, as lower-case hex.
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// checkDigest compares a downloaded file with the digest its recipe declares.
func checkDigest(program string, path string, want string) error {
	got, err := fileDigest(path)
	if err != nil {
		return fmt.Errorf("could not read the download of %s to check it: %w", program, err)
	}
	if got != strings.ToLower(strings.TrimSpace(want)) {
		return fmt.Errorf("what was downloaded for %s is not the file its recipe names: sha256 is %s, the recipe says %s; "+
			"it was not unpacked", program, got, want)
	}
	return nil
}
