//
// @file baseline.go
// @brief The release a module's working tree is compared with.
//
// A published version's exports were recorded when it was published, in the
// local registry beside the version. A module that is used from its repository
// and never published has its releases in git instead: the version a manifest
// declared is a release, and the last commit before the manifest moved past it
// is what that release was. The registry's record is preferred, because it is
// what was actually handed to other people.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package exportsig

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"rho/internal/paths"
	"rho/src/deps"
)

// Baseline is the release a tree is compared with, and where it was found.
type Baseline struct {
	Set    Set
	Source string
}

// FindBaseline finds the release before current — or current itself, when that
// version was already published — for the module in root. It answers false when
// the module has no earlier release.
func FindBaseline(root string, current Set) (Baseline, bool, error) {
	if baseline, found := registryBaseline(current); found {
		return baseline, true, nil
	}
	return gitBaseline(root, current)
}

// registryBaseline is the highest version the local registry recorded that is
// not newer than current.
func registryBaseline(current Set) (Baseline, bool) {
	installed, err := paths.GetModulesInstalledPath()
	if err != nil || current.Module == "" {
		return Baseline{}, false
	}
	now, err := deps.ParseSemver(current.Version)
	if err != nil {
		return Baseline{}, false
	}
	matches, _ := filepath.Glob(filepath.Join(installed, "*", current.Module+"@*", FileName))
	var best Baseline
	var bestVersion deps.Semver
	found := false
	for _, record := range matches {
		dir := filepath.Dir(record)
		version, err := deps.ParseSemver(strings.TrimPrefix(filepath.Base(dir), current.Module+"@"))
		if err != nil || newer(version, now) {
			continue
		}
		if found && !newer(version, bestVersion) {
			continue
		}
		set, err := Load(dir)
		if err != nil {
			continue
		}
		best = Baseline{Set: set, Source: "published " + current.Module + "@" + version.String()}
		bestVersion = version
		found = true
	}
	return best, found
}

func newer(a, b deps.Semver) bool {
	if a.Major != b.Major {
		return a.Major > b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	return a.Patch > b.Patch
}

var manifestVersion = regexp.MustCompile(`(?m)^  version:[ \t]*["']?([^"'\s#]+)`)

// gitBaseline is the module as it was at the end of its previous version: the
// commit before the one that moved the manifest to the current version, or the
// last commit when the move is not committed yet.
func gitBaseline(root string, current Set) (Baseline, bool, error) {
	top, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return Baseline{}, false, nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Baseline{}, false, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(strings.TrimSpace(top), abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return Baseline{}, false, nil
	}
	rel = filepath.ToSlash(rel)
	manifest := "module.yml"
	if rel != "." {
		manifest = rel + "/module.yml"
	}
	log, err := git(root, "log", "--format=%H", "--", "module.yml")
	if err != nil {
		return Baseline{}, false, nil
	}
	newest := ""
	for _, commit := range strings.Fields(log) {
		text, err := git(root, "show", commit+":"+manifest)
		if err != nil {
			continue
		}
		match := manifestVersion.FindStringSubmatch(text)
		if match == nil || match[1] == current.Version {
			newest = commit
			continue
		}
		revision := "HEAD"
		if newest != "" {
			revision = newest + "^"
		}
		set, err := treeAt(root, top, rel, revision)
		if err != nil {
			return Baseline{}, false, err
		}
		short := revision
		if len(newest) >= 12 {
			short = newest[:12] + "^"
		}
		return Baseline{Set: set, Source: fmt.Sprintf("%s %s in git (%s)", current.Module, set.Version, short)}, true, nil
	}
	return Baseline{}, false, nil
}

// treeAt reads the module's exports as they were at one revision.
func treeAt(root, top, rel, revision string) (Set, error) {
	scratch, err := os.MkdirTemp("", "rho-exports-")
	if err != nil {
		return Set{}, err
	}
	defer os.RemoveAll(scratch)
	path := rel
	if path == "." {
		path = ""
	}
	archive := exec.Command("git", "archive", "--format=tar", revision, "--", path)
	if path == "" {
		archive = exec.Command("git", "archive", "--format=tar", revision)
	}
	archive.Dir = strings.TrimSpace(top)
	var tarball bytes.Buffer
	archive.Stdout = &tarball
	if err := archive.Run(); err != nil {
		return Set{}, fmt.Errorf("git archive %s: %w", revision, err)
	}
	extract := exec.Command("tar", "-x", "-C", scratch)
	extract.Stdin = &tarball
	if out, err := extract.CombinedOutput(); err != nil {
		return Set{}, fmt.Errorf("extract %s: %v: %s", revision, err, out)
	}
	return Read(filepath.Join(scratch, filepath.FromSlash(rel)))
}

func git(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	out, err := command.Output()
	return string(out), err
}
