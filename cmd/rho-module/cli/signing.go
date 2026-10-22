//
// @file signing.go
// @brief `rho module keygen` and `rho module trust` — who may publish what you install.
//
// Usage:
//   rho module keygen                    make a signing key, and print its public half
//   rho module trust <name> <key>        believe this key for that module
//   rho module trust '*' <key>           believe this key for any module
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"

	"rho/cmd/rho-module/core"
)

// Keygen makes the publisher's signing key.
func Keygen(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: rho module keygen")
	}
	public, err := core.GenerateSigningKey()
	if err != nil {
		return fmt.Errorf("rho module keygen: %w", err)
	}
	fmt.Printf("signing key created. Your public key, which installers pin with `rho module trust`:\n\n  %s\n\n", public)
	fmt.Println("The private half is in ~/.rho/config/signing.key, readable by you alone. Keep it: a package you")
	fmt.Println("sign is attributed to whoever holds this key.")
	return nil
}

// Trust pins a signer's key for a module.
func Trust(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: rho module trust <name|*> <public-key>")
	}
	if err := core.TrustSigner(args[0], args[1]); err != nil {
		return fmt.Errorf("rho module trust: %w", err)
	}
	if args[0] == "*" {
		fmt.Printf("any module signed by %s is now trusted\n", args[1])
		return nil
	}
	fmt.Printf("%s is now pinned to signer %s\n", args[0], args[1])
	return nil
}
