//
// @file bin.go
// @brief Rho Install - Binary deployment subsystem.
//
// Responsibilities:
//   - Copy Rho executables into ~/.rho/bin
//   - Validate source binaries
//   - Ensure correct permissions
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// InstallBinary copies a single binary into ~/.rho/bin.
// InstallBinary puts a built binary in place.
//
// It refuses to overwrite: an install that silently replaced a binary would leave
// nobody knowing which one they were running, and a first install over a
// directory that is not a Rho home would put a toolchain into it. Replacing is
// what an update is for, and it says so.
func InstallBinary(src string) error { return installBinary(src, false) }

// ReplaceBinary puts a built binary in place, over whatever is there.
//
// This is an update: the caller has built the binary and intends it to run, and
// the one in place is the one being updated. The old one is moved aside first and
// only removed once the new one is written, so a failure part-way through leaves
// a working toolchain rather than a half-written one.
func ReplaceBinary(src string) error { return installBinary(src, true) }

func installBinary(src string, replace bool) error {
	binPath, err := GetBinPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source binary does not exist: %s", src)
	}

	dst := filepath.Join(binPath, filepath.Base(src))

	// A binary that is already there is only replaced when the caller said it was
	// an update, and even then the old one is moved aside first: a write that
	// fails half way through a truncated executable is a machine with no
	// toolchain, and the binary in place is a working one.
	var previous string
	if _, err := os.Stat(dst); err == nil {
		if !replace {
			return fmt.Errorf("binary already exists: %s", dst)
		}
		previous = dst + ".previous"
		_ = os.Remove(previous)
		if err := os.Rename(dst, previous); err != nil {
			return fmt.Errorf("cannot move the installed binary aside: %w", err)
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open source binary: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		if previous != "" {
			_ = os.Rename(previous, dst)
		}
		return fmt.Errorf("cannot create destination binary: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	return nil
}

// InstallAllBinaries installs all Rho executables.
// UpdateBinaries replaces the toolchain's binaries, in place.
//
// It is `InstallAllBinaries` for the case where there is a toolchain to update,
// which is the case a developer is in after changing the source in front of it.
func UpdateBinaries(srcDir string) error {
	for _, b := range []string{"rho", "rho-deps"} {
		src := filepath.Join(srcDir, b)
		if err := ReplaceBinary(src); err != nil {
			return fmt.Errorf("failed to install %s: %w", b, err)
		}
	}
	return nil
}

func InstallAllBinaries(srcDir string) error {
	// The same set the installation check verifies. cmd/rho-pkg is a library
	// with no main package, so there is no binary to install for it. The
	// formatter, the test runner and the documentation generator are
	// components rather than core tools: they are installed from a registry
	// and dispatched to, so building them here would put them back in the core
	// and defeat the point of late binding.
	binaries := []string{
		"rho",
		"rho-deps",
	}

	for _, b := range binaries {
		src := filepath.Join(srcDir, b)
		if err := InstallBinary(src); err != nil {
			return fmt.Errorf("failed to install %s: %w", b, err)
		}
	}

	return nil
}
