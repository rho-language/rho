//
// @file std.go
// @brief Putting the standard library where an installed toolchain can find it.
//
// Rho's standard library is written in Rho, and a program that imports any of
// it needs the files that declare it. They are found by walking up from the
// project, which works in a checkout and nowhere else: a machine that installed
// the toolchain has the binary and the documentation, and without this step it
// would have neither the declarations nor a way to reach them.
//
// So the library travels with the toolchain, into the same place the binary
// goes. It is copied rather than linked, because the machine that runs Rho is
// often not the machine it was built on, and a link to a checkout that is not
// there is worse than a copy that is a version behind.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
	"os"
	"path/filepath"
	"strings"
)

// StdDirName is the directory the standard library is installed into, inside the
// Rho home. It is named "std" because that is what an import names: the resolver
// looks for std/core/result.rho, and a tool that looked for anything else would
// be a tool that could not read a module it ships.
const StdDirName = "std"

// InstallStd copies the standard library into the Rho home.
//
// A source tree that has no `std/` beside it is not a failure. A toolchain built
// from a package carries the library with it, and refusing to install because
// the source it was built from did not happen to be checked out would make the
// toolchain un-installable from the thing it was built from.
func InstallStd(sourceRoot string) error {
	if strings.TrimSpace(sourceRoot) == "" {
		return nil
	}
	source := filepath.Join(sourceRoot, StdDirName)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return nil
	}
	home, err := GetRhoHome()
	if err != nil {
		return err
	}
	return copyDirectory(source, filepath.Join(home, StdDirName))
}

// copyDirectory replaces a directory with a copy of another.
//
// It replaces rather than merges, because a module that was renamed or removed
// would otherwise be left behind by every install, and a standard library that
// grew stale files is one that reports types the compiler no longer has.
func copyDirectory(from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Only the sources travel. A build artefact or a file that is not Rho
		// belongs to the tree it was built in, not to an installation.
		if !strings.HasSuffix(path, ".rho") {
			return nil
		}
		return copyFile(path, target, info.Mode())
	})
}
