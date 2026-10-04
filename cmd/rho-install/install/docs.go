//
// @file docs.go
// @brief Putting the documentation where an installed tool can reach it.
//
// The documentation is about the project rather than about any one component, so
// it is not part of a component's package and does not travel with one. That
// leaves a tool that shows it — the dashboard reads the project's own pages —
// unable to find it on a machine that has the toolchain and not the source.
//
// So `rho install` puts it beside everything else, and a tool looks there. It is
// a copy, and a copy can drift; that is why it is written on install rather than
// assumed, and why `rho install` says it wrote it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// docsDirName is the directory the documentation is copied into, inside the Rho
// home. It is named "docs" because that is what it is, and a tool looks for it
// under that name rather than being told.
const docsDirName = "docs"

// InstallDocs copies the project's documentation into the Rho home.
//
// It is a copy rather than a link because the source is a checkout: a machine
// that installs the toolchain and then removes the checkout would otherwise lose
// the documentation, and a link to a directory that is no longer there is worse
// than a copy that is a version old.
//
// A source that is not there is not a failure. A toolchain installed from a
// package has no source beside it, and refusing to install because of that would
// make the toolchain un-installable from a package.
func InstallDocs(sourceRoot string) error {
	if strings.TrimSpace(sourceRoot) == "" {
		return nil
	}
	source := filepath.Join(sourceRoot, docsDirName)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return nil
	}
	home, err := GetRhoHome()
	if err != nil {
		return err
	}
	return copyTree(source, filepath.Join(home, docsDirName))
}

// copyTree copies a directory, replacing what is there.
//
// Replacing rather than merging is deliberate: a page that was renamed would
// otherwise be copied forever, and a documentation tree that grows stale copies
// is a documentation tree nobody can trust to be current.
func copyTree(from, to string) error {
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
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("cannot copy %s: %w", from, err)
	}
	return nil
}
