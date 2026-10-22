//
// @file signing.go
// @brief Who published a package, as something that can be checked.
//
// A checksum from the registry says the bytes are the ones the registry holds. It
// cannot say the registry holds what its publisher uploaded, because the registry
// is the thing that says both. A signature can: the publisher signs the package's
// digest with a key the registry never sees, and a client that has pinned that key
// refuses anything else.
//
// The scheme is deliberately small. Keys are ed25519. What is signed is one
// message naming the module, the version and the digest, so a signature for one
// version cannot be replayed onto another. Keys are kept in keys.go and the policy
// about whom to believe is in trust.go.
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
	"encoding/hex"
	"fmt"
	"strings"
)

// signatureContext is the first line of every signed message. It names the scheme
// and its version, so a signature made for something else — another tool, another
// version of this one — is not a signature over a package.
const signatureContext = "rho-package-v1"

// Signature is a publisher's signature over one package, as the registry serves it.
type Signature struct {
	// Signer is the publisher's public key, hex.
	Signer string `json:"signer"`
	// Signature is the ed25519 signature, hex.
	Signature string `json:"signature"`
}

// signedMessage is what a signature is over: the scheme, the module and version,
// and the package's digest. The name and version are in it so a signature cannot be
// moved to another release of the same bytes.
func signedMessage(name, version, digest string) []byte {
	return []byte(signatureContext + "\n" + name + "@" + version + "\n" + strings.ToLower(digest))
}

// SignPackage signs a package's digest with a private key.
func SignPackage(key ed25519.PrivateKey, name, version, digest string) Signature {
	signature := ed25519.Sign(key, signedMessage(name, version, digest))
	public := key.Public().(ed25519.PublicKey)
	return Signature{Signer: hex.EncodeToString(public), Signature: hex.EncodeToString(signature)}
}

// VerifySignature reports whether a signature is a valid one over this package.
// It says nothing about whether the signer is trusted; that is a separate question
// with its own answer.
func VerifySignature(name, version, digest string, signature Signature) error {
	public, err := hex.DecodeString(signature.Signer)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return fmt.Errorf("the signer is not an ed25519 public key")
	}
	raw, err := hex.DecodeString(signature.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("the signature is not an ed25519 signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(public), signedMessage(name, version, digest), raw) {
		return fmt.Errorf("the signature does not match the package")
	}
	return nil
}
