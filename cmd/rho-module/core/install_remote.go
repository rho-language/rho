//
// @file install_remote.go
// @brief Installing a module from a remote registry into the local one.
//
// The local registry is `~/.rho`, and it is the only registry the toolchain
// uses: a component is dispatched to because its directory is under
// `modules/installed`, a Rho import resolves because the module is there, and
// nothing else is consulted. Installing is therefore the act that turns a module
// someone else published into a module this machine can use, and it is the one
// place the two registries meet.
//
// What arrives is unpacked into the installed tree and recorded in the index
// with an origin of "remote", so a listing can say where a module came from.
// That origin is the only trace of the remote registry kept locally: the local
// registry does not mirror the remote's contents, it holds the modules this
// machine chose to install.
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
	"os"
	"path/filepath"
	"strings"

	"rho/cmd/rho-module/remote"
	pkgcore "rho/cmd/rho-pkg/core"
	"rho/internal/paths"
)

// InstallResult is what an install produced, so a caller can report it.
type InstallResult struct {
	Name    string
	Version string
	// Kind is the directory the module was installed under, which is what the
	// toolchain resolves against.
	Kind string
	// Path is where it was installed.
	Path string
}

// InstallFromRemote fetches a module from a remote registry and installs it
// into the local one.
//
// A module that is already installed is replaced rather than merged, because a
// package whose file list changed must not leave the previous form's files
// behind. Reinstalling is therefore also how a user repairs a damaged
// installation, which is a thing they should not have to uninstall first to do.
func InstallFromRemote(client *remote.Client, name, version string) (InstallResult, error) {
	result := InstallResult{Name: name, Version: version}

	archive, err := remote.DownloadModule(client, name, version)
	if err != nil {
		return result, fmt.Errorf("cannot fetch %s@%s: %w", name, version, err)
	}
	if len(archive) == 0 {
		return result, fmt.Errorf("%s@%s is empty on the registry", name, version)
	}

	// What was downloaded is checked against the digest the registry recorded
	// when it was published, before anything is written. The digest is the
	// publisher's bytes, so a package altered where it is kept does not agree
	// with it — which is the difference between installing a module and
	// installing whatever is at that address. A registry that records no digest
	// answers "", and the check is skipped, which is how it was before.
	expected, err := remote.ModuleSHA256(client, name, version)
	if err != nil {
		return result, fmt.Errorf("cannot read the registry's checksum for %s@%s: %w", name, version, err)
	}
	if expected != "" {
		sum := sha256.Sum256(archive)
		if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, expected) {
			return result, fmt.Errorf(
				"%s@%s does not match the registry's checksum: got %s, want %s",
				name, version, got, expected)
		}
	}

	// The archive is read to find out what it is before anything is written: a
	// package that cannot be read is a package this machine should not have
	// half of.
	kind, err := archiveKind(archive)
	if err != nil {
		return result, fmt.Errorf("%s@%s cannot be installed: %w", name, version, err)
	}
	result.Kind = kind

	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return result, err
	}
	target := filepath.Join(modulesDir, kind, fmt.Sprintf("%s@%s", name, version))
	result.Path = target

	if err := os.RemoveAll(target); err != nil {
		return result, fmt.Errorf("cannot clear the previous installation: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return result, err
	}
	if err := pkgcore.UnpackBytes(archive, target); err != nil {
		return result, fmt.Errorf("cannot unpack %s@%s: %w", name, version, err)
	}

	// The index is reconciled rather than written directly, so the new
	// directory is recorded by the same code that records everything else and
	// the origin is the only thing this has to add.
	registry, err := Reconcile()
	if err != nil {
		return result, err
	}
	recordFetched(registry, name, version, kind)
	registryFile, err := registryFilePath()
	if err != nil {
		return result, err
	}
	if err := writeIndex(registryFile, registry); err != nil {
		return result, err
	}
	return result, nil
}

// recordFetched marks an installed module as having come from a remote registry.
//
// The tree already has it, because it was just unpacked there, and reconciliation
// has already recorded it. What this adds is where it came from, which is the one
// thing a directory cannot say.
func recordFetched(registry *Registry, name, version, kind string) {
	versions := versionBucketOf(registry, kind)
	for i := range *versions {
		if (*versions)[i].Name == name && (*versions)[i].Version == version {
			(*versions)[i].Origin = originRemote
			return
		}
	}
	*versions = append(*versions, VersionEntry{
		Name: name, Version: version, Origin: originRemote,
	})
}

// archiveKind reads a package to find out which kind of module it holds.
//
// The kind is the directory a module is installed under and the toolchain
// resolves against, so it has to come from the package rather than be assumed.
// The manifest inside says it; a package without one is not installable.
func archiveKind(archive []byte) (string, error) {
	manifest, err := pkgcore.ReadArchiveManifest(archive)
	if err != nil {
		return "", err
	}
	kind := manifest.Module.Type
	if kind == "" {
		return "", fmt.Errorf("the package's manifest names no type")
	}
	// The toolchain installs under these four, and a type outside them would be
	// a directory nothing resolves against: a module that is installed and
	// unusable.
	for _, known := range IndexKinds {
		if kind == known {
			return kind, nil
		}
	}
	switch kind {
	case "rho", "application":
		// A Rho application is installed as a module, which is what the
		// toolchain resolves a dependency against.
		return KindModule, nil
	case "interfaces", "platform", "platform-board", "driver", "driver-board", "runtime", "middleware":
		// All of them are native code a project links rather than something it
		// runs, so all of them install as a library.
		return KindLibrary, nil
	}
	return "", fmt.Errorf("a module of type %q has nowhere to install to", kind)
}
