//
// @file discover.go
// @brief Finding the Rho test files in a project.
//
// Only the files this runner knows how to run itself. Everything else — a gtest
// suite, a build, a board — comes from the manifest, because the runner cannot
// tell what those are without being told.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"os"
	"path/filepath"
	"sort"
)

/* -------------------------------------------------------------------------- */
/*  Discover                                                                   */
/* -------------------------------------------------------------------------- */

// Discover returns the Rho test files under a path, or the file itself.
//
// A path that is not there is an error rather than an empty list, for the same
// reason a manifest that cannot be read is: a run that found nothing because the
// thing it was pointed at is missing and a run that found nothing because there
// is nothing there are the same event from outside, and only one of them is true.
func Discover(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if IsRhoTestFile(path) {
			return []string{path}, nil
		}
		return []string{}, nil
	}

	found := []string{}
	err = filepath.Walk(path, func(current string, entry os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// A build directory holds what a build produced, and a test a build
			// generated is not a test the project has: walking into one finds the
			// project's own tests copied next to their own output, twice.
			if entry.Name() == "build" || entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if IsRhoTestFile(current) {
			found = append(found, current)
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

// DescribeFiles turns paths into what the discover hook is asked about them: a
// path, and the cases each file holds.
//
// A file whose cases cannot be read is still passed on, with none: the file is
// still a file, and a plugin that knows the format may well know how to read what
// this runner cannot.
func DescribeFiles(paths []string) []DiscoveredFile {
	files := make([]DiscoveredFile, 0, len(paths))
	for _, path := range paths {
		file := DiscoveredFile{Path: path}
		if cases, err := LoadTests(path); err == nil {
			file.Tests = cases
		}
		files = append(files, file)
	}
	return files
}
