//
// @file helpers_test.go
// @brief What the tests share: a copied fixture workspace and files written by hand.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate keeps the environment from choosing the workspace or the date.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("ROBOHUB_WORKSPACE", "")
	t.Setenv("RHO_WORKSPACE", "")
	t.Setenv("SOURCE_DATE_EPOCH", "1795046400") // 2026-11-19
}

// fixture copies testdata/workspace to a temporary directory and puts the
// gateway's fixture reports where `rho test` writes them. (They are kept outside
// build/ in the tree because build/ is ignored by git.)
func fixture(t *testing.T) string {
	t.Helper()
	isolate(t)
	dst := t.TempDir()
	copyTree(t, filepath.Join("testdata", "workspace"), dst)
	reports := filepath.Join(dst, "gateway", ReportsDir)
	if err := os.MkdirAll(reports, 0755); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(dst, "gateway", "test-reports.fixture"), reports)
	return dst
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// writeFiles writes files under a directory; the text is dedented by its first
// line's indentation, so a test can write a file inline.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func load(t *testing.T, dir string, workspace bool) *Workspace {
	t.Helper()
	w, err := Load(dir, Options{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func codes(issues Issues) string {
	out := []string{}
	for _, issue := range issues {
		out = append(out, issue.Severity+":"+issue.Code)
	}
	return strings.Join(out, " ")
}

func hasCode(issues Issues, severity, code string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Severity == severity {
			return true
		}
	}
	return false
}
