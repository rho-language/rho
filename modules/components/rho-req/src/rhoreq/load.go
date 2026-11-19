//
// @file load.go
// @brief Finding the repositories of a workspace and reading what they declare.
//
// A repository is a directory. Its name is the directory's name, because that is
// what a `parent:` resolves against and what a qualified ADR reference names
// (`robohub:ADR-0003`). A module nested inside a repository — a directory below
// it whose module.yml declares a `requirements:` block — is a member of its own,
// named by its module.yml's `module.name` (or the block's `name:`), because its
// directory's name (`mesh`, `forms`) says nothing outside the repository. Its
// place in a hierarchy comes from the `requirements:` block of its module.yml:
//
//	requirements:
//	  prefix: GW        # the ids this repository allocates start with GW
//	  parent: robohub   # the repository that holds levels.yml and the SYS level
//
// A parent is found as a sibling directory: `$ROBOHUB_WORKSPACE/<parent>`, then
// `$RHO_WORKSPACE/<parent>`, then `../<parent>`. The repositories that share a
// root — the one at the top of the parent chain — are a family, and a family is
// what "the workspace" means: ids are unique in it, a `satisfies` may cross it,
// and its root's levels.yml and MAINTAINERS.yml govern it.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"rho/src/yaml"
)

// Paths inside a repository.
const (
	RequirementsDir = "docs/requirements"
	LevelsFile      = "docs/requirements/levels.yml"
	ADRDir          = "docs/architecture/adr"
	MaintainersFile = "MAINTAINERS.yml"
	ManifestFile    = "module.yml"
)

// ReqFile is one requirements file.
type ReqFile struct {
	Path   string `json:"-"`
	Rel    string `json:"file"`
	Level  string `json:"level"`
	Prefix string `json:"prefix"`
	Title  string `json:"title,omitempty"`
	Reqs   []*Requirement
}

// Repo is one repository and what it declares.
type Repo struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Prefix string `json:"prefix,omitempty"`
	Parent string `json:"parent,omitempty"`
	// Nested is true for a module nested inside another repository, which is
	// named by its module.name rather than its directory.
	Nested bool `json:"nested,omitempty"`

	ParentRepo *Repo   `json:"-"`
	Levels     []Level `json:"levels"`
	// LevelsFrom is where the levels came from: a path, or "default".
	LevelsFrom string     `json:"levels_from"`
	Files      []*ReqFile `json:"-"`
	ADRs       []*ADR     `json:"-"`
	// Maintainers is the MAINTAINERS.yml that governs this repository, or "".
	Maintainers string `json:"maintainers,omitempty"`

	headerOnly bool
	tags       []*Tag
	tagsDone   bool
	results    *Results
}

// Requirements is every requirement the repository declares, in file order.
func (r *Repo) Requirements() []*Requirement {
	out := []*Requirement{}
	for _, file := range r.Files {
		out = append(out, file.Reqs...)
	}
	return out
}

// Options say how much of a workspace a command is about.
type Options struct {
	// Workspace reports on every repository of the family rather than the one
	// the command was run in.
	Workspace bool
}

// Workspace is a family of repositories, and the one a command is about.
type Workspace struct {
	Focus  *Repo
	Root   *Repo
	Repos  []*Repo
	Issues Issues
	Opts   Options

	byID  map[string][]*Requirement
	adrs  map[string]*ADR
	repos map[string]*Repo
}

// Reported are the repositories a command reports on.
func (w *Workspace) Reported() []*Repo {
	if w.Opts.Workspace {
		return w.Repos
	}
	return []*Repo{w.Focus}
}

// IsReported reports whether a repository is one the command reports on.
func (w *Workspace) IsReported(name string) bool {
	if w.Opts.Workspace {
		return true
	}
	return w.Focus.Name == name
}

// Requirement is the requirement with an id, or nil.
func (w *Workspace) Requirement(id string) *Requirement {
	if found := w.byID[id]; len(found) > 0 {
		return found[0]
	}
	return nil
}

// Repo is the family repository with a name, or nil.
func (w *Workspace) Repo(name string) *Repo { return w.repos[name] }

// ADR resolves a reference written in a repository: `ADR-0009` is that
// repository's, `robohub:ADR-0009` robohub's.
func (w *Workspace) ADR(from, ref string) *ADR {
	match := adrPattern.FindStringSubmatch(ref)
	if match == nil {
		return nil
	}
	repo := match[1]
	if repo == "" {
		repo = from
	}
	return w.adrs[repo+":"+match[2]]
}

// ADRs is every ADR of the family, ordered by repository and id.
func (w *Workspace) ADRs() []*ADR {
	out := []*ADR{}
	for _, repo := range w.Repos {
		out = append(out, repo.ADRs...)
	}
	return out
}

// AllRequirements is every requirement of the family.
func (w *Workspace) AllRequirements() []*Requirement {
	out := []*Requirement{}
	for _, repo := range w.Repos {
		out = append(out, repo.Requirements()...)
	}
	return out
}

// LevelsOf are the levels a repository works with.
func (w *Workspace) LevelsOf(repo string) []Level {
	if found := w.repos[repo]; found != nil {
		return found.Levels
	}
	return DefaultLevels
}

// Children are the requirements that satisfy one.
func (w *Workspace) Children(id string) []*Requirement {
	out := []*Requirement{}
	for _, req := range w.AllRequirements() {
		if contains(req.Satisfies, id) {
			out = append(out, req)
		}
	}
	return out
}

// RefFrom is how a repository writes a reference to an ADR.
func RefFrom(from string, adr *ADR) string {
	if adr.Repo == from {
		return adr.ID
	}
	return adr.Ref()
}

/* -------------------------------------------------------------------------- */
/*  Finding repositories                                                      */
/* -------------------------------------------------------------------------- */

// FindRepoRoot is the repository a directory is in: the nearest directory, at or
// above it, that has docs/requirements, docs/architecture/adr, a module.yml with
// a requirements block, or a .git. A directory in none is its own repository.
func FindRepoRoot(dir string) string {
	start, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for current := start; ; {
		if isDir(filepath.Join(current, RequirementsDir)) || isDir(filepath.Join(current, ADRDir)) || isDir(filepath.Join(current, ".git")) || fileExists(filepath.Join(current, ".git")) {
			return current
		}
		if block, _ := readManifestBlock(current); block.declared {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return start
		}
		current = parent
	}
}

// WorkspaceDirs are the directories sibling repositories are looked for in.
func WorkspaceDirs(repoDir string) []string {
	out := []string{}
	for _, name := range []string{"ROBOHUB_WORKSPACE", "RHO_WORKSPACE"} {
		if value := os.Getenv(name); value != "" {
			if abs, err := filepath.Abs(value); err == nil && !contains(out, abs) {
				out = append(out, abs)
			}
		}
	}
	if parent := filepath.Dir(repoDir); !contains(out, parent) {
		out = append(out, parent)
	}
	return out
}

// resolveParent is the directory of a repository's parent, or "": first the
// repositories that enclose it, by their names, then a sibling of the outermost
// one in the workspace's directories.
func resolveParent(name, from string) string {
	for dir := enclosingRepo(from); dir != ""; dir = enclosingRepo(dir) {
		if repoName(dir) == name {
			return dir
		}
	}
	for _, dir := range WorkspaceDirs(outermostRepo(from)) {
		candidate := filepath.Join(dir, name)
		if isDir(candidate) {
			return candidate
		}
	}
	return ""
}

// isMember reports whether a directory takes part in the process on its own:
// it keeps requirements or decisions, or its module.yml declares a
// `requirements:` block.
func isMember(dir string) bool {
	if isDir(filepath.Join(dir, RequirementsDir)) || isDir(filepath.Join(dir, ADRDir)) {
		return true
	}
	block, _ := readManifestBlock(dir)
	return block.declared
}

// declaresRequirements reports whether a directory's module.yml declares a
// `requirements:` block, which is what makes a directory below a repository a
// nested member of its own.
func declaresRequirements(dir string) bool {
	block, _ := readManifestBlock(dir)
	return block.declared
}

// enclosingRepo is the nearest directory above dir that takes part in the
// process (see isMember), or "" when dir is a repository of the workspace's top
// level. The workspace directories named by the environment, and what is above
// them, never enclose anything, and neither does a directory reached through
// build/, vendor/, node_modules/, testdata/ or a hidden directory: what is
// there is generated, vendored or a fixture (a documentation tree assembled in
// build/ is a workspace of its own), as the search for nested members skips it.
func enclosingRepo(dir string) string {
	stops := map[string]bool{}
	for _, name := range []string{"ROBOHUB_WORKSPACE", "RHO_WORKSPACE"} {
		if value := os.Getenv(name); value != "" {
			if abs, err := filepath.Abs(value); err == nil {
				stops[abs] = true
			}
		}
	}
	for child := dir; ; {
		if name := filepath.Base(child); skippedDirs[name] || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
			return ""
		}
		current := filepath.Dir(child)
		if current == child || stops[current] {
			return ""
		}
		if isMember(current) {
			return current
		}
		child = current
	}
}

// outermostRepo is the repository of the workspace's top level a directory is
// in: dir itself, or the outermost repository that encloses it.
func outermostRepo(dir string) string {
	for enclosing := enclosingRepo(dir); enclosing != ""; enclosing = enclosingRepo(enclosing) {
		dir = enclosing
	}
	return dir
}

// repoName is a repository's name: its directory's name at the top level; for a
// nested module the block's `name:`, else its module.name, else its directory's
// name.
func repoName(dir string) string {
	if enclosingRepo(dir) == "" {
		return filepath.Base(dir)
	}
	block, _ := readManifestBlock(dir)
	switch {
	case block.name != "":
		return block.name
	case block.module != "":
		return block.module
	}
	return filepath.Base(dir)
}

// nestedMembers are the directories below a repository whose module.yml
// declares a `requirements:` block, at any depth; build/, vendor/,
// node_modules/, testdata/ and hidden directories are not looked in.
func nestedMembers(top string) []string {
	out := []string{}
	filepath.WalkDir(top, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == top {
			return nil
		}
		if skippedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if declaresRequirements(path) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

type manifestBlock struct {
	declared       bool
	prefix, parent string
	// name is the block's `name:`, which names a nested module in place of its
	// module.name; module is the manifest's `module.name`.
	name, module string
	unknown      []string
	line         int
	err          error
}

// readManifestBlock reads the `requirements:` block of a repository's module.yml.
func readManifestBlock(dir string) (manifestBlock, bool) {
	path := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return manifestBlock{}, false
	}
	block := manifestBlock{}
	value, err := yaml.Load(string(data))
	if err != nil {
		block.err = err
		return block, true
	}
	if module, ok := value.Map["module"]; ok && module.Kind == yaml.KindMap {
		if name, ok := module.Map["name"]; ok {
			block.module = strings.TrimSpace(name.Str)
		}
	}
	section, ok := value.Map["requirements"]
	if !ok {
		return block, true
	}
	block.declared = true
	for index, line := range splitLines(string(data)) {
		if strings.HasPrefix(line, "requirements:") {
			block.line = index + 1
			break
		}
	}
	if section.Kind == yaml.KindMap {
		for key, entry := range section.Map {
			switch key {
			case "prefix":
				block.prefix = entry.Str
			case "parent":
				block.parent = entry.Str
			case "name":
				block.name = strings.TrimSpace(entry.Str)
			default:
				block.unknown = append(block.unknown, key)
			}
		}
		sort.Strings(block.unknown)
	}
	return block, true
}

// Load reads the family of repositories the directory is in.
func Load(dir string, opts Options) (*Workspace, error) {
	if !isDir(dir) {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	// Absolute, so the repository a command runs in and the same repository
	// found again among the workspace's directories are one repository.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	focusDir := FindRepoRoot(dir)
	w := &Workspace{Opts: opts, byID: map[string][]*Requirement{}, adrs: map[string]*ADR{}, repos: map[string]*Repo{}}
	headers := map[string]*Repo{}
	header := func(dir string) *Repo {
		if found := headers[dir]; found != nil {
			return found
		}
		repo := &Repo{Name: repoName(dir), Dir: dir, headerOnly: true}
		block, _ := readManifestBlock(dir)
		repo.Prefix, repo.Parent = block.prefix, block.parent
		repo.Nested = enclosingRepo(dir) != ""
		manifestIssues(w, repo, block)
		// A nested module copied out to the top level (a documentation tree)
		// keeps its block; the name only matters when it disagrees.
		if block.name != "" && !repo.Nested && block.name != repo.Name {
			w.Issues = append(w.Issues, Issue{Severity: SeverityWarning, Code: "manifest.name", Repo: repo.Name, File: ManifestFile, Line: block.line,
				Message: fmt.Sprintf("requirements.name %s names a module nested in a repository; a repository of the workspace's top level is named by its directory, %s", block.name, repo.Name)})
		}
		headers[dir] = repo
		return repo
	}
	// rootOf follows a repository's parents to the top.
	rootOf := func(repo *Repo, report bool) *Repo {
		seen := map[string]bool{repo.Dir: true}
		current := repo
		for current.Parent != "" {
			parentDir := resolveParent(current.Parent, current.Dir)
			if parentDir == "" {
				if report {
					w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "parent.unresolved", Repo: current.Name, File: ManifestFile,
						Message: fmt.Sprintf("the parent repository %q is not found (looked in the repositories enclosing it and in %s)", current.Parent, strings.Join(WorkspaceDirs(outermostRepo(current.Dir)), ", "))})
				}
				return current
			}
			parent := header(parentDir)
			current.ParentRepo = parent
			if seen[parent.Dir] {
				if report {
					w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "parent.cycle", Repo: current.Name, File: ManifestFile,
						Message: fmt.Sprintf("the parent chain returns to %s", parent.Name)})
				}
				return parent
			}
			seen[parent.Dir] = true
			current = parent
		}
		return current
	}

	focus := header(focusDir)
	root := rootOf(focus, true)
	members := map[string]*Repo{focus.Dir: focus, root.Dir: root}
	for walk := focus; walk != nil && walk != root; walk = walk.ParentRepo {
		members[walk.Dir] = walk
	}
	for _, wsDir := range WorkspaceDirs(outermostRepo(focusDir)) {
		entries, err := os.ReadDir(wsDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			candidateDir := filepath.Join(wsDir, entry.Name())
			if members[candidateDir] != nil {
				continue
			}
			if block, ok := readManifestBlock(candidateDir); !ok || !block.declared {
				continue
			}
			candidate := header(candidateDir)
			if candidate.Parent == "" {
				continue
			}
			if rootOf(candidate, false).Dir == root.Dir {
				members[candidateDir] = candidate
			}
		}
	}

	// The modules nested in every repository of the family's top level that
	// reach the same root are members too.
	tops := map[string]bool{}
	for dir := range members {
		tops[outermostRepo(dir)] = true
	}
	topDirs := []string{}
	for dir := range tops {
		topDirs = append(topDirs, dir)
	}
	sort.Strings(topDirs)
	for _, top := range topDirs {
		for _, nestedDir := range nestedMembers(top) {
			if members[nestedDir] != nil {
				continue
			}
			candidate := header(nestedDir)
			if candidate.Parent == "" {
				continue
			}
			if rootOf(candidate, false).Dir == root.Dir {
				members[nestedDir] = candidate
			}
		}
	}

	w.Focus, w.Root = focus, root
	for _, repo := range members {
		w.Repos = append(w.Repos, repo)
	}
	sort.Slice(w.Repos, func(a, b int) bool {
		if (w.Repos[a] == root) != (w.Repos[b] == root) {
			return w.Repos[a] == root
		}
		return w.Repos[a].Name < w.Repos[b].Name
	})
	for _, repo := range w.Repos {
		if other := w.repos[repo.Name]; other != nil {
			w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "repo.duplicate", Repo: repo.Name,
				Message: fmt.Sprintf("two repositories of the family are called %s: %s and %s (a nested module can be named apart with name: in its requirements block)", repo.Name, other.Dir, repo.Dir)})
			continue
		}
		w.repos[repo.Name] = repo
	}
	for _, repo := range w.Repos {
		w.loadRepo(repo)
	}
	for _, repo := range w.Repos {
		for _, req := range repo.Requirements() {
			w.byID[req.ID] = append(w.byID[req.ID], req)
		}
		for _, adr := range repo.ADRs {
			if _, taken := w.adrs[adr.Ref()]; !taken {
				w.adrs[adr.Ref()] = adr
			}
		}
	}
	return w, nil
}

func manifestIssues(w *Workspace, repo *Repo, block manifestBlock) {
	if block.err != nil {
		w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "manifest.yaml", Repo: repo.Name, File: ManifestFile, Message: block.err.Error()})
	}
	for _, key := range block.unknown {
		w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "schema.unknown-key", Repo: repo.Name, File: ManifestFile, Line: block.line,
			Message: fmt.Sprintf("requirements.%s is not a known key (prefix, parent, name)", key)})
	}
	if block.prefix != "" && !prefixPattern.MatchString(block.prefix) {
		w.Issues = append(w.Issues, Issue{Severity: SeverityError, Code: "id.prefix", Repo: repo.Name, File: ManifestFile, Line: block.line,
			Message: fmt.Sprintf("the prefix %q is not upper-case words joined by dashes", block.prefix)})
	}
}

// loadRepo reads a repository's levels, requirement files, ADRs and the
// maintainers file that governs it.
func (w *Workspace) loadRepo(repo *Repo) {
	repo.headerOnly = false
	repo.Levels, repo.LevelsFrom = w.levelsFor(repo, map[string]bool{})
	for governing := repo; governing != nil; governing = governing.ParentRepo {
		if path := filepath.Join(governing.Dir, MaintainersFile); fileExists(path) {
			repo.Maintainers = path
			break
		}
		if governing.ParentRepo == governing {
			break
		}
	}
	base := filepath.Join(repo.Dir, RequirementsDir)
	paths := []string{}
	filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if (ext == ".yml" || ext == ".yaml") && path != filepath.Join(repo.Dir, LevelsFile) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	for _, path := range paths {
		repo.Files = append(repo.Files, w.loadReqFile(repo, path))
	}
	w.loadADRs(repo)
}

func (w *Workspace) levelsFor(repo *Repo, seen map[string]bool) ([]Level, string) {
	if seen[repo.Dir] {
		return DefaultLevels, "default"
	}
	seen[repo.Dir] = true
	path := filepath.Join(repo.Dir, LevelsFile)
	if fileExists(path) {
		levels := w.loadLevels(repo, path)
		if len(levels) > 0 {
			return levels, path
		}
		return DefaultLevels, "default"
	}
	if repo.ParentRepo != nil && repo.ParentRepo != repo {
		return w.levelsFor(repo.ParentRepo, seen)
	}
	return DefaultLevels, "default"
}

func (w *Workspace) issue(severity, code string, repo *Repo, file string, line int, id string, format string, args ...any) {
	w.Issues = append(w.Issues, Issue{Severity: severity, Code: code, Repo: repo.Name, File: file, Line: line, ID: id, Message: fmt.Sprintf(format, args...)})
}

func (w *Workspace) rel(repo *Repo, path string) string {
	rel, err := filepath.Rel(repo.Dir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func (w *Workspace) loadLevels(repo *Repo, path string) []Level {
	rel := w.rel(repo, path)
	data, err := os.ReadFile(path)
	if err != nil {
		w.issue(SeverityError, "levels.read", repo, rel, 0, "", "%v", err)
		return nil
	}
	root, err := ParseYAML(string(data))
	if err != nil {
		w.yamlIssue(repo, rel, err)
		return nil
	}
	ctx := schemaContext{w: w, repo: repo, file: rel}
	ctx.check(root, root.Line, "", levelsFileSchema)
	levels := []Level{}
	entry := root.Get("levels")
	if entry == nil || entry.Value.Kind != List {
		return nil
	}
	seen := map[string]bool{}
	for _, item := range entry.Value.Items {
		if item.Kind != Map {
			w.issue(SeverityError, "schema.type", repo, rel, item.Line, "", "a level is a mapping with a name and a title")
			continue
		}
		ctx.check(item, item.Line, "", levelSchema)
		name := item.Str("name")
		if name == "" {
			continue
		}
		if !prefixPattern.MatchString(name) || strings.Contains(name, "-") {
			w.issue(SeverityError, "levels.name", repo, rel, item.Line, "", "the level name %q is not one upper-case word", name)
		}
		if seen[name] {
			w.issue(SeverityError, "levels.duplicate", repo, rel, item.Line, "", "the level %s is declared twice", name)
			continue
		}
		seen[name] = true
		levels = append(levels, Level{Name: name, Title: item.Str("title")})
	}
	return levels
}

func (w *Workspace) yamlIssue(repo *Repo, rel string, err error) {
	line := 0
	if parseErr, ok := err.(*ParseError); ok {
		line = parseErr.Line
		err = fmt.Errorf("%s", parseErr.Message)
	}
	w.issue(SeverityError, "yaml", repo, rel, line, "", "%v", err)
}

func (w *Workspace) loadReqFile(repo *Repo, path string) *ReqFile {
	rel := w.rel(repo, path)
	file := &ReqFile{Path: path, Rel: rel}
	data, err := os.ReadFile(path)
	if err != nil {
		w.issue(SeverityError, "file.read", repo, rel, 0, "", "%v", err)
		return file
	}
	root, err := ParseYAML(string(data))
	if err != nil {
		w.yamlIssue(repo, rel, err)
		return file
	}
	if root.Kind != Map {
		w.issue(SeverityError, "schema.type", repo, rel, root.Line, "", "a requirements file is a mapping with level, prefix and requirements")
		return file
	}
	ctx := schemaContext{w: w, repo: repo, file: rel}
	ctx.check(root, 1, "", fileSchema)
	file.Level, file.Prefix, file.Title = root.Str("level"), root.Str("prefix"), root.Str("title")
	if file.Level != "" && LevelIndex(repo.Levels, file.Level) < 0 {
		w.issue(SeverityError, "file.level", repo, rel, lineOf(root, "level"), "", "the level %s is not one of %s (from %s)", file.Level, levelNames(repo.Levels), repo.LevelsFrom)
	}
	if file.Prefix != "" {
		switch {
		case !prefixPattern.MatchString(file.Prefix):
			w.issue(SeverityError, "file.prefix", repo, rel, lineOf(root, "prefix"), "", "the prefix %q is not upper-case words joined by dashes", file.Prefix)
		case len(repo.Levels) > 0 && file.Level == repo.Levels[0].Name:
			if file.Prefix != repo.Levels[0].Name {
				w.issue(SeverityError, "file.prefix", repo, rel, lineOf(root, "prefix"), "", "a %s-level file uses the prefix %s, not %s", file.Level, repo.Levels[0].Name, file.Prefix)
			}
		case repo.Prefix != "" && file.Prefix != repo.Prefix && !strings.HasPrefix(file.Prefix, repo.Prefix+"-"):
			w.issue(SeverityError, "file.prefix", repo, rel, lineOf(root, "prefix"), "", "the prefix %s is neither the repository's prefix %s nor a sub-prefix of it (%s-...)", file.Prefix, repo.Prefix, repo.Prefix)
		}
	}
	entry := root.Get("requirements")
	if entry == nil || entry.Value.Kind != List {
		return file
	}
	for _, item := range entry.Value.Items {
		if item.Kind != Map {
			w.issue(SeverityError, "schema.type", repo, rel, item.Line, "", "a requirement is a mapping, found %s", item.Kind)
			continue
		}
		id := item.Str("id")
		ctx.check(item, item.Line, id, requirementSchema)
		req := &Requirement{
			ID: id, Title: item.Str("title"), Statement: strings.TrimSpace(item.Str("statement")), Rationale: strings.TrimSpace(item.Str("rationale")),
			Satisfies: nonNil(item.Strings("satisfies")), Status: item.Str("status"), Verification: item.Str("verification"),
			DecidedBy: nonNil(item.Strings("decided_by")), Owner: item.Str("owner"), Changed: item.Str("changed"), Tags: nonNil(item.Strings("tags")),
			Level: file.Level, Repo: repo.Name, File: rel, Line: item.Line,
		}
		if id != "" {
			prefix, _, ok := SplitID(id)
			switch {
			case !ok:
				w.issue(SeverityError, "id.format", repo, rel, item.Line, id, "the id %q is not <PREFIX>-NNNN (four digits)", id)
			case file.Prefix != "" && prefix != file.Prefix:
				w.issue(SeverityError, "id.prefix", repo, rel, item.Line, id, "the id %s does not use this file's prefix %s", id, file.Prefix)
			}
		}
		file.Reqs = append(file.Reqs, req)
	}
	return file
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

var adrFileName = regexp.MustCompile(`^([0-9]{4})-[A-Za-z0-9._-]+\.md$`)

func (w *Workspace) loadADRs(repo *Repo) {
	base := filepath.Join(repo.Dir, ADRDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(ADRDir, name))
		match := adrFileName.FindStringSubmatch(name)
		if match == nil {
			if lower := strings.ToLower(name); lower != "readme.md" && lower != "index.md" {
				w.issue(SeverityWarning, "adr.filename", repo, rel, 0, "", "not named NNNN-slug.md, so not read as an ADR")
			}
			continue
		}
		if match[1] == "0000" {
			continue // the template
		}
		adr := w.loadADR(repo, filepath.Join(base, name), rel)
		if adr == nil {
			continue
		}
		if want := "ADR-" + match[1]; adr.ID != "" && adr.ID != want {
			w.issue(SeverityError, "adr.id", repo, rel, 2, adr.ID, "the file is numbered %s but its id is %s", match[1], adr.ID)
		}
		repo.ADRs = append(repo.ADRs, adr)
	}
}

// frontMatter is where a Markdown file's YAML header is: the lines between its
// opening and closing `---`, as indexes into its lines.
// ADRBody is an ADR's text after its front matter: the Context, Decision and
// Consequences a reader reads.
func ADRBody(w *Workspace, adr *ADR) string {
	repo := w.Repo(adr.Repo)
	if repo == nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repo.Dir, filepath.FromSlash(adr.File)))
	if err != nil {
		return ""
	}
	lines := splitLines(string(data))
	if _, end, ok := frontMatter(lines); ok {
		lines = lines[end+1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func frontMatter(lines []string) (start, end int, ok bool) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0, 0, false
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			return 1, index, true
		}
	}
	return 0, 0, false
}

func (w *Workspace) loadADR(repo *Repo, path, rel string) *ADR {
	data, err := os.ReadFile(path)
	if err != nil {
		w.issue(SeverityError, "file.read", repo, rel, 0, "", "%v", err)
		return nil
	}
	lines := splitLines(string(data))
	start, end, ok := frontMatter(lines)
	if !ok {
		w.issue(SeverityError, "adr.front-matter", repo, rel, 1, "", "an ADR starts with a YAML front matter between two --- lines")
		return nil
	}
	root, err := parseLines(lines[start:end], start)
	if err != nil {
		w.yamlIssue(repo, rel, err)
		return nil
	}
	ctx := schemaContext{w: w, repo: repo, file: rel}
	ctx.check(root, 1, root.Str("id"), adrSchema)
	adr := &ADR{
		ID: root.Str("id"), Title: root.Str("title"), Status: root.Str("status"), Date: root.Str("date"),
		Requirements: nonNil(root.Strings("requirements")), Supersedes: nonNil(root.Strings("supersedes")), SupersededBy: nonNil(root.Strings("superseded_by")),
		Repo: repo.Name, File: rel,
	}
	if adr.ID != "" && !regexp.MustCompile(`^ADR-[0-9]{4}$`).MatchString(adr.ID) {
		w.issue(SeverityError, "adr.id", repo, rel, lineOf(root, "id"), adr.ID, "the id %q is not ADR-NNNN", adr.ID)
	}
	return adr
}

/* -------------------------------------------------------------------------- */
/*  The schema                                                                */
/* -------------------------------------------------------------------------- */

// field is one key a mapping may carry.
type field struct {
	name     string
	kind     Kind
	required bool
	enum     []string
	date     bool
}

var fileSchema = []field{
	{name: "level", kind: Scalar, required: true},
	{name: "prefix", kind: Scalar, required: true},
	{name: "title", kind: Scalar},
	{name: "requirements", kind: List, required: true},
}

// RequirementKeys are a requirement's keys, in the order a new one is written
// and a missing one is inserted.
var RequirementKeys = []string{"id", "title", "statement", "rationale", "satisfies", "status", "verification", "decided_by", "owner", "changed", "tags"}

var requirementSchema = []field{
	{name: "id", kind: Scalar, required: true},
	{name: "title", kind: Scalar, required: true},
	{name: "statement", kind: Scalar, required: true},
	{name: "rationale", kind: Scalar},
	{name: "satisfies", kind: List},
	{name: "status", kind: Scalar, required: true, enum: Statuses},
	{name: "verification", kind: Scalar, required: true, enum: Verifications},
	{name: "decided_by", kind: List},
	{name: "owner", kind: Scalar},
	{name: "changed", kind: Scalar, date: true},
	{name: "tags", kind: List},
}

// ADRKeys are an ADR front matter's keys, in order.
var ADRKeys = []string{"id", "title", "status", "date", "requirements", "supersedes", "superseded_by"}

var adrSchema = []field{
	{name: "id", kind: Scalar, required: true},
	{name: "title", kind: Scalar, required: true},
	{name: "status", kind: Scalar, required: true, enum: ADRStatuses},
	{name: "date", kind: Scalar, required: true, date: true},
	{name: "requirements", kind: List},
	{name: "supersedes", kind: List},
	{name: "superseded_by", kind: List},
}

var levelsFileSchema = []field{{name: "levels", kind: List, required: true}}

var levelSchema = []field{
	{name: "name", kind: Scalar, required: true},
	{name: "title", kind: Scalar, required: true},
}

var maintainersFileSchema = []field{{name: "maintainers", kind: List, required: true}}

var maintainerSchema = []field{
	{name: "name", kind: Scalar, required: true},
	{name: "github", kind: Scalar},
	{name: "email", kind: Scalar},
}

type schemaContext struct {
	w    *Workspace
	repo *Repo
	file string
}

// check holds a mapping to a schema: no unknown keys, every required key, the
// right shape for each, and an allowed value where the values are a set.
func (c schemaContext) check(node *Node, line int, id string, fields []field) {
	known := map[string]field{}
	names := []string{}
	for _, f := range fields {
		known[f.name] = f
		names = append(names, f.name)
	}
	for _, entry := range node.Entries {
		f, ok := known[entry.Key]
		if !ok {
			c.w.issue(SeverityError, "schema.unknown-key", c.repo, c.file, entry.Line, id, "%q is not a known key (%s)", entry.Key, strings.Join(names, ", "))
			continue
		}
		value := entry.Value
		if value.Kind == Null {
			if f.required {
				c.w.issue(SeverityError, "schema.missing-key", c.repo, c.file, entry.Line, id, "%s has no value", entry.Key)
			}
			continue
		}
		if value.Kind != f.kind {
			c.w.issue(SeverityError, "schema.type", c.repo, c.file, entry.Line, id, "%s is %s, expected %s", entry.Key, value.Kind, f.kind)
			continue
		}
		if f.kind == List {
			for _, item := range value.Items {
				if item.Kind != Scalar && f.name != "requirements" && f.name != "levels" && f.name != "maintainers" {
					c.w.issue(SeverityError, "schema.type", c.repo, c.file, entry.Line, id, "%s holds %s, expected scalars", entry.Key, item.Kind)
				}
			}
		}
		if len(f.enum) > 0 && !contains(f.enum, value.Value) {
			c.w.issue(SeverityError, "schema.enum", c.repo, c.file, entry.Line, id, "%s %q is not one of %s", entry.Key, value.Value, strings.Join(f.enum, ", "))
		}
		if f.date && !validDate(value.Value) {
			c.w.issue(SeverityError, "schema.date", c.repo, c.file, entry.Line, id, "%s %q is not a date written YYYY-MM-DD", entry.Key, value.Value)
		}
		if f.kind == Scalar && strings.TrimSpace(value.Value) == "" && f.required {
			c.w.issue(SeverityError, "schema.missing-key", c.repo, c.file, entry.Line, id, "%s is empty", entry.Key)
		}
	}
	for _, f := range fields {
		if f.required && node.Get(f.name) == nil {
			c.w.issue(SeverityError, "schema.missing-key", c.repo, c.file, line, id, "the required key %s is missing", f.name)
		}
	}
}

func validDate(text string) bool {
	if !datePattern.MatchString(text) {
		return false
	}
	_, err := time.Parse("2006-01-02", text)
	return err == nil
}

func lineOf(node *Node, key string) int {
	if entry := node.Get(key); entry != nil {
		return entry.Line
	}
	return node.Line
}

func levelNames(levels []Level) string {
	names := []string{}
	for _, level := range levels {
		names = append(names, level.Name)
	}
	return strings.Join(names, ", ")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
