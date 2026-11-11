//
// @file ids.go
// @brief `rho module ids` — the identities a tree declares, and whether they hold.
//
// A module's id is what a registry, a lock file and a saved plan refer to it by,
// so two modules with one id, or an id that is also another's previous id, is a
// reference that resolves to two things. This reads every module.yml under the
// given directories (the working directory by default), prints each module's id,
// and exits non-zero when the set cannot all be true or an id is not well formed.
//
//	rho module ids [dir...]
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"rho/src/schema"
	"rho/src/yaml"
)

// skippedDirs are not part of a tree's identity: they hold generated output,
// fetched code and history.
var skippedDirs = map[string]bool{".git": true, "build": true, "node_modules": true, "vendor": true, ".rho-deps": true}

// Ids implements `rho module ids`.
func Ids(args []string) error {
	roots := args
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var entries []schema.IDEntry
	var malformed []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if skippedDirs[entry.Name()] && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "module.yml" {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			document, loadErr := yaml.Load(string(data))
			if loadErr != nil || document.Kind != yaml.KindMap {
				return nil
			}
			module := document.Map["module"]
			if module.Kind != yaml.KindMap || module.Map["id"].Str == "" {
				return nil
			}
			found := schema.IDEntry{Path: path, Name: yaml.Unquote(module.Map["name"].Str), ID: yaml.Unquote(module.Map["id"].Str)}
			for _, previous := range module.Map["previous_ids"].List {
				found.Previous = append(found.Previous, yaml.Unquote(previous.Str))
			}
			if idErr := schema.CheckModuleID(found.ID); idErr != nil {
				malformed = append(malformed, fmt.Sprintf("%s: %v", path, idErr))
			}
			for _, previous := range found.Previous {
				if idErr := schema.CheckModuleID(previous); idErr != nil {
					malformed = append(malformed, fmt.Sprintf("%s: previous id: %v", path, idErr))
				}
			}
			entries = append(entries, found)
			return nil
		})
		if err != nil {
			return fmt.Errorf("rho module ids: %w", err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	for _, entry := range entries {
		fmt.Printf("%-48s %s\n", entry.ID, entry.Path)
	}
	problems := append(malformed, schema.CheckIDs(entries)...)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "rho module ids:", problem)
		}
		return fmt.Errorf("rho module ids: %d problem(s) in %d module(s)", len(problems), len(entries))
	}
	return nil
}
