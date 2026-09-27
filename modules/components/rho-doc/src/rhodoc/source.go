//
// @file source.go
// @brief Finding the documentation a repository carries.
//
// Doc-as-code lives with the thing it documents: a module names the sources
// under its own root in its manifest, and those files are part of the module, so
// they travel with it when it is published. This file is the one place that
// knows how to find them.
//
// Three sources are read, in this order:
//
//   - a docs: section in a module manifest, which names files or a glob
//   - a docs directory beside that manifest, which is the convention
//   - a docs directory at the root of a repository, which is how a repository
//     documents the language and the system rather than one module
//
// Everything is sorted, so a site built twice from the same tree is identical.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  Modules                                                                   */
/* -------------------------------------------------------------------------- */

// ModuleDoc is the documentation one module contributes to a site.
type ModuleDoc struct {
	// Name is the module name from its manifest.
	Name string
	// Version is the version it was read at, so a page can report it.
	Version string
	// Type is the module type: component, plugin or an application.
	Type string
	// Description is the manifest's own one-line description.
	Description string
	// DocsTitle is the title the module declares for its documentation, which
	// the page it leads a section with carries.
	DocsTitle string
	// Root is the module root, which every source path is relative to.
	Root string
	// Section is where the pages appear in the sidebar.
	Section string
	// Order sorts the module against its peers in a section.
	Order int
	// Sources are the doc-as-code files, absolute.
	Sources []string
	// Index is the page the section leads with, relative to the root.
	Index string
	// Manifest is the parsed manifest, for the fields a page reports.
	Manifest *deps.Manifest
}

// DiscoverModules reads every module in a tree, so a site can be built from a
// repository or from a directory of modules.
//
// The walk stops at a nested module root, because a component that carries its
// own go.mod is a module in its own right and is visited on its own account.
func DiscoverModules(root string) ([]ModuleDoc, []string) {
	return DiscoverModulesWith(root, SourceOptions{})
}

// SourceOptions says how a tree's documentation is gathered, because what a
// tree is decides what it can be documented from.
type SourceOptions struct {
	// Readme reads a module's root README.md as the page its section leads
	// with, when the module carries neither a docs: section nor a docs
	// directory.
	//
	// A published package is what a registry documents a module from, and the
	// README is the one piece of prose such a package always carries: the
	// prose a module wrote about itself travels with it, and anything else
	// would have to be sent separately.
	Readme bool
}

// DiscoverModulesWith reads every module in a tree with the given options.
func DiscoverModulesWith(root string, options SourceOptions) ([]ModuleDoc, []string) {
	modules := []ModuleDoc{}
	missing := []string{}
	seen := map[string]bool{}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".kilo", "node_modules", "build", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != "module.yml" {
			return nil
		}
		moduleRoot := filepath.Dir(path)
		if seen[moduleRoot] {
			return nil
		}
		seen[moduleRoot] = true
		module, ok := readModule(moduleRoot, options)
		if !ok {
			missing = append(missing, moduleRoot)
			return nil
		}
		modules = append(modules, module)
		return nil
	})

	sortModules(modules)
	// A repository documents itself apart from its modules: the language and the
	// handbook are properties of the repository, not of any one module. A tree
	// that is itself a module is not a repository, so the convention does not
	// apply to it: a published package carries one module's docs/ directory, and
	// that is the module's own documentation rather than the repository's.
	if !isModuleRoot(root) {
		modules = append(modules, discoverRepositoryDocs(root)...)
	}
	return modules, missing
}

// RepositorySection is the sidebar group the pages written directly in a
// repository's docs directory appear under.
const RepositorySection = "documentation"

// discoverRepositoryDocs reads a repository's own documentation.
//
// A repository is not a module — it has no manifest and no entry point — so its
// pages are found by the same convention a module uses, one section per
// subdirectory: `docs/handbook/` is the handbook, and the pages written
// directly in `docs/` are the overview.
func discoverRepositoryDocs(root string) []ModuleDoc {
	docs := filepath.Join(root, "docs")
	if info, err := os.Stat(docs); err != nil || !info.IsDir() {
		return nil
	}
	modules := []ModuleDoc{}
	for _, source := range markdownTree(docs) {
		relative, err := filepath.Rel(docs, source)
		if err != nil {
			continue
		}
		module := ModuleDoc{
			Root:    root,
			Section: RepositorySection,
			Sources: []string{source},
		}
		// A page in a subdirectory belongs to the section that directory names,
		// so a repository can grow a section by adding a directory.
		if directory := firstSegment(relative); directory != "" {
			module.Section = directory
		}
		modules = append(modules, module)
	}
	return modules
}

// firstSegment is the leading directory of a relative path, or "" when the path
// has none.
func firstSegment(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// readModule reads one module and the documentation it carries.
func readModule(root string, options SourceOptions) (ModuleDoc, bool) {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return ModuleDoc{}, false
	}
	module := ModuleDoc{
		Name:        manifest.ModuleName,
		Version:     manifest.Version,
		Type:        manifest.ModuleType,
		Root:        root,
		Manifest:    manifest,
		Description: manifest.Description,
		Section:     "modules",
	}
	if module.Name == "" {
		return ModuleDoc{}, false
	}
	if manifest.Docs != nil {
		if manifest.Docs.Section != "" {
			module.Section = manifest.Docs.Section
		}
		module.DocsTitle = manifest.Docs.Title
		if manifest.Docs.Index != "" {
			module.Index = manifest.Docs.Index
		}
		if manifest.Docs.Order != "" {
			// An order that is not a number is no order at all, rather than
			// a build that fails over a sort key.
			if value, err := strconv.Atoi(manifest.Docs.Order); err == nil {
				module.Order = value
			}
		}
		module.Sources = expandSources(root, manifest.Docs.Sources)
	}
	// The convention applies whether or not the manifest says anything, so a
	// module that drops a file into docs/ gets it documented.
	if len(module.Sources) == 0 {
		module.Sources = expandSources(root, []string{"docs"})
	}
	// A module whose prose did not travel still has the README it published.
	if len(module.Sources) == 0 && options.Readme {
		if readme := filepath.Join(root, "README.md"); isDocSource(readme) && exists(readme) {
			module.Sources = []string{readme}
			module.Index = "README.md"
		}
	}
	return module, true
}

// isModuleRoot reports whether a directory is itself a module, which is what
// separates a working tree of modules from a single published package.
func isModuleRoot(root string) bool {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	return err == nil && manifest != nil && manifest.ModuleName != ""
}

// expandSources turns the source patterns of a manifest into files.
func expandSources(root string, patterns []string) []string {
	found := []string{}
	seen := map[string]bool{}
	for _, pattern := range patterns {
		for _, path := range expand(root, pattern) {
			if seen[path] {
				continue
			}
			seen[path] = true
			found = append(found, path)
		}
	}
	sort.Strings(found)
	return found
}

// expand resolves one source pattern to the doc-as-code files it names.
//
// A pattern may be a file, a directory or a glob. A directory is read as a
// tree of Markdown, and nothing else: a repository is full of files that are
// not documentation, and a documentation tool that guessed would publish them.
func expand(root, pattern string) []string {
	base := filepath.Join(root, filepath.FromSlash(pattern))
	// A glob is resolved against the module root by path walking, because
	// filepath.Glob cannot cross a directory boundary on its own.
	if strings.ContainsAny(pattern, "*?[") {
		return expandGlob(root, pattern)
	}
	info, err := os.Stat(base)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if isDocSource(base) {
			return []string{base}
		}
		return nil
	}
	return markdownTree(base)
}

// markdownTree reads every doc-as-code file in a tree, sorted.
func markdownTree(root string) []string {
	found := []string{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if isDocSource(path) {
			found = append(found, path)
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// expandGlob resolves a pattern such as docs/*.md or docs/**/*.md.
func expandGlob(root, pattern string) []string {
	found := []string{}
	// The pattern is walked from its static prefix, which is everything before
	// the first wildcard.
	prefix := root
	rest := filepath.FromSlash(pattern)
	if cut := strings.IndexAny(rest, "*?["); cut > 0 {
		prefix = filepath.Join(root, rest[:cut])
		rest = rest[cut:]
	}
	// filepath.Match needs a pattern relative to the directory it is matched
	// in, so the walk carries the pattern down as it descends.
	filepath.Walk(prefix, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		matches, _ := filepath.Glob(filepath.Join(path, rest))
		for _, match := range matches {
			if !isDocSource(match) {
				continue
			}
			if info, err := os.Stat(match); err == nil && !info.IsDir() {
				found = append(found, match)
			}
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// exists reports whether a path is there.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isDocSource reports whether a file is a doc-as-code source.
func isDocSource(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

// sortModules orders modules the way a sidebar lists them: by section, then by
// the order each module declared, then by name so the result is stable.
func sortModules(modules []ModuleDoc) {
	sort.SliceStable(modules, func(i, j int) bool {
		left, right := modules[i], modules[j]
		if left.Section != right.Section {
			return left.Section < right.Section
		}
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.Name < right.Name
	})
}
