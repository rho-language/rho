//
// @file deps.go
// @brief rho-cimport's own dependency manager for C sources.
//
// This is the embedded side's package manager, and it is deliberately not the
// one rho-deps is. A Rho module dependency is a component or a plugin: Rho
// code, dispatched to at run time, installed into the Rho home. A C dependency
// is a library or a vendor SDK: C sources and headers, compiled into a program,
// and a gigabyte and a half of it when nobody is careful. The two want opposite
// things from a resolver — one wants the newest matching version from a registry,
// the other wants one known revision of one repository, as shallowly as it can be
// had — and a resolver that tries to do both does neither well.
//
// So nothing here shares with rho-deps. A C dependency is declared in a
// project's manifest, fetched into a cache this component owns, and reported as
// the directories and files a build needs. It is not a lockfile entry, is not
// installed into the Rho home, and is never enabled or disabled: a program that
// links a library is not a program that runs a component, and treating it as one
// would put a compiled library among things a person chose to have installed.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CDep is one C dependency a project declares.
//
// The fields are the ones a C dependency actually has. There is no version
// range because a vendor SDK is fetched at a revision rather than resolved
// against a range of releases: there is no registry to resolve a range
// against, and a build that picked the newest HAL would not be the build its
// author tested. The revision is a branch, a tag, or whatever the repository's
// default is.
type CDep struct {
	// Name is how the dependency is referred to in a report. It is not a
	// module path, because nothing resolves it as one.
	Name string
	// Git is the repository the sources are fetched from.
	Git string
	// Branch and Tag pin the revision. At most one is used; a dependency that
	// names both is refused rather than resolved by preferring one, because
	// which one was meant is a question only its author can answer.
	Branch string
	Tag    string
	// Path is a directory the sources are already in, which is how a project
	// works against a checkout rather than a remote. A dependency with a path
	// is never fetched.
	Path string
	// Include names the directories that go on the include path, relative to
	// the dependency's root. A pattern is allowed and each match is taken.
	Include []string
	// Src names the sources: a file is compiled, a directory contributes the C
	// files in it.
	Src []string
	// Where the dependency was read from, for a report.
	Source string
}

// Revision is what a report calls this dependency's revision.
func (d CDep) Revision() string {
	if d.Tag != "" {
		return d.Tag
	}
	if d.Branch != "" {
		return d.Branch
	}
	return "the default branch"
}

// Validate reports whether a dependency says enough to be fetched.
func (d CDep) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("a C dependency names nothing")
	}
	if d.Tag != "" && d.Branch != "" {
		return fmt.Errorf("dependency %s names both a tag and a branch; one of them is meant", d.Name)
	}
	if d.Git == "" && d.Path == "" {
		return fmt.Errorf("dependency %s says where it comes from nowhere: it needs a git URL or a path", d.Name)
	}
	if d.Git != "" && d.Path != "" {
		return fmt.Errorf("dependency %s names both a repository and a path; it is one or the other", d.Name)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

// Resolution is what a project ends up with once its C dependencies are
// fetched: the directories that go on the include path, and the files that are
// compiled.
type Resolution struct {
	// IncludeDirs are absolute, and are what a build puts on the include path.
	IncludeDirs []string
	// SourceFiles are absolute paths of the C files to compile.
	SourceFiles []string
	// Fetched names the dependencies that were fetched, in the order they were
	// resolved, so a report can say what the build spent its time on.
	Fetched []string
	// Missing names what was asked for and was not found. A dependency whose
	// directory is absent is reported rather than fatal: a project that names a
	// path for a family it does not target should still build.
	Missing []string
}

// Resolver fetches and lays out a project's C dependencies.
//
// It is a type rather than a set of functions because the cache directory and
// the project it serves are two things a caller has to choose, and a function
// taking four arguments where two of them are configuration is a function whose
// calls cannot be read.
type Resolver struct {
	// ProjectRoot is the project whose dependencies these are.
	ProjectRoot string
	// Cache is where fetched dependencies are kept. It is a directory this
	// component owns and nothing else reads.
	Cache string
	// Out is where a fetch's own output goes. Nil means this resolver's output
	// is captured and discarded, which is what a check wants.
	Out *os.File
}

// Resolve fetches what is not already fetched and works out what the build
// needs.
//
// Already fetched is not re-fetched: a build that fetched a vendor SDK on every
// run would spend longer on the network than on the compiler, and a dependency
// that had been fetched at a revision is fetched at that revision until it is
// removed. That is the same rule the Rho module resolver uses, and it is
// deliberate there for the same reason.
func (r *Resolver) Resolve(preset Preset) (Resolution, error) {
	resolution := Resolution{}
	seenInclude := map[string]bool{}
	seenSource := map[string]bool{}

	addInclude := func(path string) {
		if !seenInclude[path] {
			seenInclude[path] = true
			resolution.IncludeDirs = append(resolution.IncludeDirs, path)
		}
	}
	addSource := func(path string) {
		if !seenSource[path] {
			seenSource[path] = true
			resolution.SourceFiles = append(resolution.SourceFiles, path)
		}
	}

	if err := preset.Validate(); err != nil {
		return resolution, err
	}

	for _, one := range preset.Dependencies {
		if err := one.Validate(); err != nil {
			return resolution, fmt.Errorf("preset %q: %w", preset.Name, err)
		}
		{
			root, err := r.rootOf(one)
			if err != nil {
				return resolution, err
			}
			if root == "" {
				resolution.Missing = append(resolution.Missing, one.Name)
				continue
			}

			for _, pattern := range one.Include {
				for _, match := range expand(filepath.Join(root, pattern)) {
					if info, statErr := os.Stat(match); statErr == nil && info.IsDir() {
						addInclude(match)
					}
				}
			}
			for _, pattern := range one.Src {
				for _, match := range expand(filepath.Join(root, pattern)) {
					info, statErr := os.Stat(match)
					if statErr != nil {
						continue
					}
					if info.IsDir() {
						for _, file := range cFilesIn(match) {
							addSource(file)
						}
						continue
					}
					if filepath.Ext(match) == ".c" {
						addSource(match)
					}
				}
			}
		}
		if one.Git != "" {
			resolution.Fetched = append(resolution.Fetched, one.Name)
		}
	}

	// Sorted, so that a build which does not otherwise differ produces the same
	// command line. Two builds of the same project should be comparable by their
	// output rather than by the order a map happened to be walked in.
	sort.Strings(resolution.IncludeDirs)
	sort.Strings(resolution.SourceFiles)
	return resolution, nil
}

// rootOf returns where a dependency's sources are, fetching them if it named a
// repository. An empty string means the dependency was not found.
func (r *Resolver) rootOf(dep CDep) (string, error) {
	if dep.Path != "" {
		root := dep.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(r.ProjectRoot, root)
		}
		if _, err := os.Stat(root); err != nil {
			return "", nil
		}
		return root, nil
	}
	if dep.Git == "" {
		return "", nil
	}

	dest := filepath.Join(r.Cache, r.directoryFor(dep))
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return dest, nil
	}
	if err := r.fetch(dep, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// directoryFor is the name a dependency is fetched under.
//
// The name is qualified by where it came from, because two dependencies may
// legitimately have the same name and different repositories — a project that
// uses two vendors' SDKs, each of which calls its own thing `include`. Keyed on
// the host and path, a fetched dependency is also recognisable on sight in a
// cache directory, which is worth something when someone is looking at a
// thousand-megabyte cache wondering what is in it.
func (r *Resolver) directoryFor(dep CDep) string {
	name := dep.Name
	if name == "" {
		name = "unnamed"
	}
	host, path := dep.Git, ""
	if index := strings.Index(dep.Git, "://"); index >= 0 {
		rest := dep.Git[index+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			host, path = rest[:slash], rest[slash+1:]
		}
	} else if index := strings.LastIndex(dep.Git, ":"); index >= 0 {
		host, path = dep.Git[:index], dep.Git[index+1:]
	}
	host = strings.NewReplacer(":", "-", "/", "-", ".", "-").Replace(host)
	path = strings.TrimSuffix(strings.NewReplacer("/", "-", ".", "-").Replace(path), ".git")
	if host == "" || path == "" {
		return name
	}
	return name + "__" + host + "-" + path
}

// fetch clones a dependency as shallowly and as partly as it can be had.
//
// Both matter for the same reason. A vendor SDK is enormous — measured in
// gigabytes, for the ones this is built to fetch — and a build needs a few
// directories of one. `--depth 1` asks for the revision rather than the history, and
// `--filter=blob:none --sparse` asks for the tree rather than the files, with the
// files the dependency actually named fetched afterwards. Together they turn a
// gigabyte and a half into a few tens of megabytes.
//
// Nothing here is skipped for a small repository. The cost of being shallow is
// one commit's worth of history that nobody reads, and the cost of being sparse
// is a second git invocation.
func (r *Resolver) fetch(dep CDep, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("cannot create the cache directory: %w", err)
	}

	ref := dep.Tag
	if ref == "" {
		ref = dep.Branch
	}
	args := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse"}
	if ref != "" {
		args = append(args, "--branch", ref, "--single-branch")
	}
	args = append(args, dep.Git, dest)

	if err := r.git(args, ""); err != nil {
		os.RemoveAll(dest)
		return fmt.Errorf("cannot fetch %s: %w", dep.Name, err)
	}

	// The paths the dependency named, and nothing else. A dependency that names
	// none is left sparse with only the top level, which would be useless, so a
	// dependency with nothing named is materialised whole instead.
	paths := append([]string{}, dep.Include...)
	paths = append(paths, dep.Src...)
	if len(paths) == 0 {
		args := []string{"sparse-checkout", "disable"}
		if err := r.git(args, dest); err != nil {
			os.RemoveAll(dest)
			return fmt.Errorf("cannot fetch %s in full: %w", dep.Name, err)
		}
		return nil
	}

	args = []string{"sparse-checkout", "set", "--no-cone"}
	args = append(args, paths...)
	if err := r.git(args, dest); err != nil {
		// The clone is on disk and the build can use whatever of it arrived, so
		// this is a warning. A dependency that named a path which has since been
		// renamed should surface as a missing include in the build, which says
		// what is wrong far better than a failed fetch does.
		fmt.Fprintf(os.Stderr, "warning: %s: could not fetch the declared paths: %v\n", dep.Name, err)
	}
	return nil
}

// git runs a git command, optionally in a directory.
func (r *Resolver) git(args []string, dir string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if r.Out != nil {
		cmd.Stdout = r.Out
		cmd.Stderr = r.Out
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	return cmd.Run()
}

/* -------------------------------------------------------------------------- */
/*  Paths                                                                      */
/* -------------------------------------------------------------------------- */

// expand resolves one declared path to what is there: the path itself, or each
// match of a pattern in it.
//
// A path with no pattern is returned as written even when nothing is there,
// because a dependency naming a directory that has not been fetched yet is
// saying what it will be, and answering with nothing would make one
// declaration mean two things depending on whether a fetch had happened.
func expand(path string) []string {
	if !strings.ContainsAny(path, "*?[") {
		return []string{path}
	}
	matches, err := filepath.Glob(path)
	if err != nil {
		return nil
	}
	return matches
}

// cFilesIn lists the C files directly in a directory.
func cFilesIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".c" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// CacheDir is where this component keeps fetched C dependencies.
//
// It is under the Rho home because that is where a machine's build state lives,
// and it is this component's own directory because nothing else should be
// reading it: the Rho module cache next to it belongs to rho-deps, and a
// dependency manager that shared a cache with another would eventually find
// something the other had decided was stale.
func CacheDir() (string, error) {
	home, err := RhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "cimport", "pkg"), nil
}

// RhoHome is the Rho home, without the dependency system this component does
// not use.
func RhoHome() (string, error) {
	if home := os.Getenv("RHO_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the home directory: %w", err)
	}
	return filepath.Join(home, ".rho"), nil
}
