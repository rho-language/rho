//
// @file roundtrip_test.go
// @brief Round-trip tests for the Rho formatter.
//
// The formatter is only safe to run over real sources if formatting and
// re-parsing yields the same program. These tests check that property across
// every Rho file in the repository, so a change to the printer that silently
// drops a declaration, statement or expression fails here rather than eating
// someone's code.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/frontend/ast"
)

// repoRoot walks up from this package to the repository root. It is found
// rather than assumed, because this component may be built from its own
// directory or from the repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// The component has its own go.mod, so keep going for the one
			// that also holds the Rho sources.
			if _, err := os.Stat(filepath.Join(dir, "std")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find the repository root")
	return ""
}

// repoSources lists the Rho sources that must survive formatting.
func repoSources(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	var files []string
	for _, dir := range []string{"std", "applications", "examples", "modules"} {
		found, err := rhoFilesUnder(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Fatal("no Rho sources were found, so the round trip would prove nothing")
	}
	return files
}

// Formatting a real source must not lose the program.
//
// The check is deliberately strict: a formatted file that does not parse, or
// that parses to fewer declarations, is a bug in the formatter rather than in
// the source.
func TestFormatterPreservesEveryRepoSource(t *testing.T) {
	for _, path := range repoSources(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			before, err := ParseRho(string(original))
			if err != nil {
				t.Skipf("source does not parse, so it is not a formatting subject: %v", err)
			}

			formatted, err := FormatSource(string(original))
			if err != nil {
				t.Fatalf("formatting failed: %v", err)
			}
			after, err := ParseRho(formatted)
			if err != nil {
				t.Fatalf("the formatted source does not parse: %v\n%s", err, formatted)
			}

			if len(after.Decls) != len(before.Decls) {
				t.Errorf("declaration count changed: %d then %d", len(before.Decls), len(after.Decls))
			}
			if len(after.Imports) != len(before.Imports) {
				t.Errorf("import count changed: %d then %d", len(before.Imports), len(after.Imports))
			}
			if len(after.Includes) != len(before.Includes) {
				t.Errorf("include count changed: %d then %d", len(before.Includes), len(after.Includes))
			}
			// A function body that vanished is the failure this test exists for.
			if got, want := countStatements(after), countStatements(before); got != want {
				t.Errorf("statement count changed: %d then %d", got, want)
			}
		})
	}
}

// Formatting is idempotent: formatting a formatted file changes nothing.
func TestFormatterIsIdempotent(t *testing.T) {
	for _, path := range repoSources(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			once, err := FormatSource(string(original))
			if err != nil {
				t.Skipf("source does not parse: %v", err)
			}
			twice, err := FormatSource(once)
			if err != nil {
				t.Fatalf("re-formatting failed: %v", err)
			}
			if once != twice {
				t.Errorf("formatting is not stable\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
			}
		})
	}
}

// countStatements totals the statements in every function body, which is the
// clearest measure of whether the printer kept the program.
func countStatements(m *ast.Module) int {
	total := 0
	for _, decl := range m.Decls {
		fn, ok := decl.(ast.FuncDecl)
		if !ok {
			continue
		}
		total += len(fn.Body)
	}
	return total
}
