//
// @file consumers.go
// @brief The modules a change to another module's exports reaches.
//
// A module that declares another under its deps uses some of what it exports;
// a provider implements its interface's functions. Each is found in the
// workspace by its manifest, and each use of a changed name by its source line,
// so a person deciding on a release sees which of those it would break.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package exportsig

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"rho/internal/paths"
	"rho/src/deps"
)

// Use is one line of a consumer that names a changed export.
type Use struct {
	File string
	Line int
	Name string
	Text string
}

// Consumer is a module that depends on the changed one, or implements it.
type Consumer struct {
	Module string
	Root   string
	// Implements is set for a provider of the changed interface.
	Implements bool
	Uses       []Use
}

// skipped are the directories a workspace walk does not enter.
var skipped = map[string]bool{".git": true, "build": true, "node_modules": true, "vendor": true, ".kilo": true}

// FindConsumers lists the modules in the workspace around root that depend on
// the module or implement its interface, with their uses of names.
func FindConsumers(root string, set Set, names []string) []Consumer {
	workspace, _ := paths.Workspace(root)
	if workspace == "" {
		return nil
	}
	self, _ := filepath.Abs(root)
	consumers := []Consumer{}
	filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipped[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != "module.yml" {
			return nil
		}
		dir := filepath.Dir(path)
		if dir == self {
			return nil
		}
		manifest, err := deps.LoadManifest(path)
		if err != nil || manifest == nil {
			return nil
		}
		depends := false
		for _, dep := range manifest.Deps {
			if dep.Name == set.Module {
				depends = true
			}
		}
		implements := false
		if set.Import != "" {
			for _, iface := range manifest.Implements {
				if iface == set.Import {
					implements = true
				}
			}
		}
		if !depends && !implements {
			return nil
		}
		consumer := Consumer{Module: manifest.ModuleName, Root: dir, Implements: implements}
		consumer.Uses = uses(dir, set, names)
		consumers = append(consumers, consumer)
		return nil
	})
	sort.Slice(consumers, func(a, b int) bool { return consumers[a].Module < consumers[b].Module })
	return consumers
}

// uses finds the lines of a module's Rho sources that name one of the changed
// exports: `alias.name`, or the bare name, which is what an importer may write
// when only one module declares it. An interface function is found where a
// provider defines it.
func uses(root string, set Set, names []string) []Use {
	if len(names) == 0 {
		return nil
	}
	alias := ""
	if set.Import != "" {
		alias = set.Import[strings.LastIndex(set.Import, ".")+1:]
	}
	patterns := map[string]*regexp.Regexp{}
	for _, name := range names {
		if function, isInterface := strings.CutPrefix(name, "interface "); isInterface {
			patterns[name] = regexp.MustCompile(`\bfn\s+` + regexp.QuoteMeta(function) + `\s*\(`)
			continue
		}
		pattern := `(^|[^.\w])` + regexp.QuoteMeta(name) + `\b`
		if alias != "" {
			pattern = `\b` + regexp.QuoteMeta(alias) + `\.` + regexp.QuoteMeta(name) + `\b|` + pattern
		}
		patterns[name] = regexp.MustCompile(pattern)
	}
	found := []Use{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipped[info.Name()] {
				return filepath.SkipDir
			}
			// A module inside this one is a consumer of its own.
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "module.yml")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".rho" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(text), "//") {
				continue
			}
			for _, name := range names {
				if patterns[name].MatchString(text) {
					rel, _ := filepath.Rel(root, path)
					found = append(found, Use{File: rel, Line: line, Name: name, Text: strings.TrimSpace(text)})
				}
			}
		}
		return nil
	})
	return found
}
