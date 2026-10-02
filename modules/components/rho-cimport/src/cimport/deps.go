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

	"rho/src/deps"
	"rho/src/yaml"
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
	// Name is what the dependency is called, and what an override narrows it by.
	Name string `json:"name" yaml:"name"`
	// Git is where it is fetched from when it is not already checked out.
	Git string `json:"git,omitempty" yaml:"git,omitempty"`
	// Branch and Tag are the revision, and a build takes whichever is declared.
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
	Tag    string `json:"tag,omitempty" yaml:"tag,omitempty"`
	// Include are the directories of this repository that are headers, and Src
	// the ones that are code. They are separate because a build adds one to the
	// include path and compiles the other, and a directory that is both would be
	// added and compiled wholesale.
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Src     []string `json:"src,omitempty" yaml:"src,omitempty"`
	// Path is where the dependency is checked out, for a build that is told rather
	// than asked. It is how a vendor source is resolved inside a tree the platform
	// already has, rather than fetched.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// Source is which repository of a platform this came from — which preset
	// directory, in practice — so a report can say where a dependency was found
	// without walking the tree again.
	Source string `json:"-" yaml:"-"`
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
	// A dependency need not name a git URL or a path of its own. A board declares
	// where each of its vendor repositories lives — a folder in the project's
	// vendor tree, used when it is there and fetched when it is not — and that is
	// where most of them come from. Requiring the name here as well would make the
	// same fact be written twice, and the copy in the preset is the one that
	// cannot know where the project keeps its vendor tree.
	//
	// Whether it can actually be resolved is decided where it is used, because
	// only there is the board that might say.
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
	// BoardName is the board the selected preset names. It is how the board's own
	// contract — including where its vendor repositories live — is reached, since
	// a board is not a dependency of the project that builds for it.
	BoardName string

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
				// A dependency nothing placed. Reported rather than skipped: a
				// build that silently omits the HAL a preset names produces a
				// program that fails later with a missing header, which says
				// nothing about the decision that left it out.
				resolution.Missing = append(resolution.Missing, one.Name)
				continue
			}

			// What the dependency's own manifest says a consumer needs on its
			// include path.
			//
			// A preset says where to fetch the vendor SDKs; the library in front
			// of them is a Rho module with a manifest, and that manifest already
			// declares its include directories so a consumer reads them rather
			// than being told them. Without this a program restates them, and a
			// restated path is one that only means something on the machine that
			// wrote it — which is the whole of what a dependency exists to avoid.
			for _, dir := range manifestIncludeDirs(root) {
				for _, match := range expand(filepath.Join(root, dir)) {
					if info, statErr := os.Stat(match); statErr == nil && info.IsDir() {
						addInclude(match)
					}
				}
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
						for _, file := range sourceFilesIn(match) {
							addSource(file)
						}
						continue
					}
					// Assembly as well as C, because the one source a board
					// preset most often exists to declare is the startup file —
					// and a build that dropped it would link without an entry
					// point, and say so by defaulting the entry to the base
					// address rather than by naming the file it wanted.
					switch strings.ToLower(filepath.Ext(match)) {
					case ".c", ".s", ".S":
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
	if len(resolution.Missing) > 0 {
		return resolution, fmt.Errorf("nothing says where %s is: a preset names it as a dependency, and neither it nor the board it belongs to gives a git URL or a folder",
			strings.Join(resolution.Missing, ", "))
	}
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
	// Where the module said its vendor repositories are, if it said. That is the
	// contract: a repository already in that folder is used as it is, and one that
	// is absent is fetched into it — so a vendor tree somebody patched, or fetched
	// once to work offline, is kept rather than fetched over, and it is beside the
	// project that builds against it rather than in a shared cache where two
	// projects on two revisions of the same SDK could not both be right.
	dest := filepath.Join(r.Cache, r.directoryFor(dep))

	// What the board or the platform said about this repository, if it said
	// anything.
	//
	// A preset names what a board needs and where inside a repository each thing
	// is. It does not say where the repository comes from, because a preset is
	// about a family and one repository is shared by every board that uses the
	// family. That is the board's manifest, which declares `vendor.repos` with a
	// path, a URL and a branch each.
	//
	// Reading all of it here rather than only its path is the whole of this change.
	// Before, the declared path was used when the checkout was already there, and
	// the preset's own — always empty, because a preset has no reason to know a
	// URL — `git` decided whether it could be fetched otherwise. So the URL sitting
	// in the board's manifest was never read at all, and every board whose vendor
	// tree had not already been fetched by hand reported its dependencies as
	// missing, naming a repository the platform had in fact declared.
	declared := r.declaredVendorRepo(dep.Name)
	if declared.path != "" {
		dest = declared.path
	}

	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return dest, nil
	}

	// Where it comes from: the preset's own URL if it gave one, and the board's
	// otherwise.
	source := dep
	if source.Git == "" && declared.git != "" {
		source.Git = declared.git
		if source.Branch == "" {
			source.Branch = declared.branch
		}
	}
	if source.Git == "" {
		// Nothing here and nowhere to fetch it from. Left to the caller to
		// report, which can name the dependency; a fetch with no URL would fail
		// less clearly.
		return "", nil
	}
	if err := r.fetch(source, dest); err != nil {
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

// manifestIncludeDirs reads the include directories a dependency's own manifest
// declares for its consumers.
//
// A path carrying a build-tool variable is left out rather than half-resolved. A
// platform declares things like `${CMAKE_BINARY_DIR}/generated/`, which is
// correct in a CMake build and unresolvable here; passing the unexpanded form to
// the compiler produces an include path that does not exist and a diagnostic
// naming a path nobody can find, whereas leaving it out produces a build that
// either works or fails on a header that is genuinely missing.
func manifestIncludeDirs(root string) []string {
	for _, name := range []string{"module.yml", "rho.mod"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		document, err := yaml.Load(string(data))
		if err != nil || document.Kind != yaml.KindMap {
			continue
		}
		build, ok := document.Map["build"]
		if !ok || build.Kind != yaml.KindMap {
			continue
		}
		dirs, ok := build.Map["include_dirs"]
		if !ok {
			continue
		}
		kept := make([]string, 0, len(dirs.List))
		for _, dir := range readStrings(dirs) {
			if !strings.Contains(dir, "${") {
				kept = append(kept, dir)
			}
		}
		return kept
	}
	return nil
}

// sourceFilesIn lists the C and assembly a directory holds.
//
// Assembly counts, because a directory of startup files is what a board preset
// narrows to, and a reader that only understood C would find a directory full of
// sources it had no name for and report nothing about it.
func sourceFilesIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// ST ships some of its drivers as templates: a file that shows how to
		// use a peripheral rather than implementing it. The vendor names those
		// `*_template.c` precisely so they are not compiled, and compiling one
		// fails on the type the template was asking the reader to supply.
		//
		// That is the vendor's convention rather than one of ours, which is why
		// it is named here rather than left to a preset to list around: a preset
		// that said `src: [Src]` and also had to remember which files in there are
		// examples would be a second place for the vendor's layout to be wrong.
		if strings.HasSuffix(entry.Name(), "_template.c") {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".c", ".s":
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
// CacheDir returns where vendor sources are kept, given the module that
// declared where they go.
//
// A module that declares a `vendor.dir` gets that directory, resolved against its
// own root. That is the case that matters: a board's vendor tree is part of the
// project, beside the code that builds against it, so it is versioned with it,
// patched with it, and deleted with it.
//
// The Rho home is only the fallback, for a module that says nothing. Sharing one
// cache across every project on a machine means two projects on two revisions of
// the same SDK cannot both be right, and means a checkout carries no record of
// what it was built against — which is the thing a vendor tree in the project is
// for.
func CacheDir(declaringRoot string) (string, error) {
	if declaringRoot != "" {
		if manifest, err := deps.LoadManifest(deps.ManifestPath(declaringRoot)); err == nil && manifest != nil && manifest.VendorDir != "" {
			directory := manifest.VendorDir
			if !filepath.IsAbs(directory) {
				directory = filepath.Join(declaringRoot, directory)
			}
			return filepath.Join(directory, "pkg"), nil
		}
	}
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

// declaredVendorPath returns where a module said a named vendor repository is, or
// "" when it said nothing about that one.
//
// It asks every module in the project's dependency chain, because the repository
// a build needs is usually declared by the board and the vendor directory that
// holds it belongs to the platform — so the name is looked for in both rather than
// in whichever happened to be nearer.
func (r *Resolver) declaredVendorPath(name string) string {
	repo := r.declaredVendorRepo(name)
	return repo.path
}

// declaredVendorRepo is everything a module said about a named repository: where
// it lives, and where it comes from when it is not already there.
func (r *Resolver) declaredVendorRepo(name string) vendorRepo {
	for _, base := range r.platformRoots() {
		for _, boardRoot := range boardRootsFor(base, r.BoardName) {
			if repo, ok := vendorRepoIn(boardRoot, name); ok {
				return repo
			}
		}
	}

	for _, base := range append([]string{r.ProjectRoot}, r.platformRoots()...) {
		if repo, ok := vendorRepoIn(base, name); ok {
			return repo
		}
	}
	return vendorRepo{}
}

// vendorRepo is a repository a module declared.
type vendorRepo struct {
	// path is where it lives, absolute once resolved against the module that
	// declared it.
	path string
	// git and branch are where it comes from and at which revision. Empty on a
	// declaration that says only where a checkout is, which is a tree somebody
	// fetched for themselves and a build keeps rather than fetching over.
	git    string
	branch string
}

// vendorRepoIn returns what a module said about a named repository, or false.
func vendorRepoIn(base, name string) (vendorRepo, bool) {
	module, err := deps.LoadManifest(deps.ManifestPath(base))
	if err != nil || module == nil || module.VendorDir == "" {
		return vendorRepo{}, false
	}
	for _, repo := range module.VendorRepos {
		if repo.Name != name || repo.Path == "" {
			continue
		}
		directory := module.VendorDir
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(base, directory)
		}
		return vendorRepo{
			path:   filepath.Join(directory, repo.Path),
			git:    repo.Git,
			branch: repo.Branch,
		}, true
	}
	return vendorRepo{}, false
}

// boardRootsFor returns the module roots of the boards a name may refer to.
//
// A board is named by its designation in a preset and by its module name in a
// dependency, and the two are written in different cases by the two people who
// wrote them. Both are tried rather than requiring one spelling of a thing that
// exists in two.
func boardRootsFor(platformRoot, board string) []string {
	if platformRoot == "" || board == "" {
		return nil
	}
	roots := []string{}
	for _, candidate := range []string{board, strings.ToLower(board)} {
		root := filepath.Join(platformRoot, "boards", candidate)
		if _, err := deps.LoadManifest(deps.ManifestPath(root)); err == nil {
			roots = append(roots, root)
		}
	}
	return roots
}

// platformRoots returns the roots of the C dependencies a project declares.
func (r *Resolver) platformRoots() []string {
	return r.projectDependencies()
}

// projectDependencies returns the roots of this project's C dependencies.
func (r *Resolver) projectDependencies() []string {
	roots := []string{}
	seen := map[string]bool{}
	// Read from the document rather than through `deps.Manifest`.
	//
	// The two read `depends:` differently, and a manifest writes the list form:
	// a dependency carries a type and a path, not a version constraint. Asking the
	// other reader would find nothing here, and "no dependencies" looks exactly
	// like "this project depends on nothing".
	var walk func(base string, depth int)
	walk = func(base string, depth int) {
		if depth > 8 {
			return
		}
		document, err := loadDocument(base)
		if err != nil {
			return
		}
		for _, item := range document.Map["depends"].List {
			if item.Kind != yaml.KindMap {
				continue
			}
			if readString(item.Map["type"]) != string(deps.DepTypeC) {
				continue
			}
			path := readString(item.Map["path"])
			if path == "" {
				continue
			}
			root := path
			if !filepath.IsAbs(root) {
				root = filepath.Join(base, root)
			}
			if seen[root] {
				continue
			}
			seen[root] = true
			roots = append(roots, root)
			walk(root, depth+1)
		}
	}
	walk(r.ProjectRoot, 0)
	return roots
}
