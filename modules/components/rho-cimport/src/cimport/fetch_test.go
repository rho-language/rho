//
// @file fetch_test.go
// @brief A vendor repository fetched for one board gains the paths the next board needs.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package cimport

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs git in a directory and fails the test if it fails.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// makeVendorRepo makes a repository laid out like a vendor's device package: one
// include directory every board shares, and a startup file per board.
func makeVendorRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	work := t.TempDir()
	files := map[string]string{
		"Include/device.h":                 "#define DEVICE 1\n",
		"Source/gcc/startup_board_a.s":     "Reset_Handler_a:\n",
		"Source/gcc/startup_board_b.s":     "Reset_Handler_b:\n",
		"Source/gcc/startup_board_c.s":     "Reset_Handler_c:\n",
		"Documentation/enormous-manual.md": "nobody builds this\n",
	}
	for name, body := range files {
		path := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, work, "init", "-q", "-b", "main")
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-q", "-m", "vendor drop")

	bare := filepath.Join(t.TempDir(), "device.git")
	runGit(t, work, "clone", "-q", "--bare", work, bare)
	runGit(t, bare, "config", "uploadpack.allowFilter", "true")
	return "file://" + bare
}

// A repository shared by two boards of a family is fetched once, sparsely, with
// what the first board named. The second board names its own startup file, and
// before this was fixed it found the directory there and used it as it was — so
// on a fresh machine the second board linked without an entry point.
func TestFetchedRepositoryGainsTheNextBoardsPaths(t *testing.T) {
	url := makeVendorRepo(t)
	resolver := &Resolver{ProjectRoot: t.TempDir(), Cache: t.TempDir()}

	boardA := CDep{Name: "device", Git: url, Branch: "main",
		Include: []string{"Include"}, Src: []string{"Source/gcc/startup_board_a.s"}}
	root, err := resolver.rootOf(boardA)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !exists(filepath.Join(root, "Source/gcc/startup_board_a.s")) {
		t.Fatalf("board A's startup file was not checked out")
	}
	if exists(filepath.Join(root, "Source/gcc/startup_board_b.s")) {
		t.Fatalf("the first fetch was not sparse: board B's startup file is there already")
	}

	boardB := boardA
	boardB.Src = []string{"Source/gcc/startup_board_b.s"}
	again, err := resolver.rootOf(boardB)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if again != root {
		t.Fatalf("the second board resolved to %s, not the shared checkout %s", again, root)
	}
	for _, want := range []string{"Include/device.h", "Source/gcc/startup_board_a.s", "Source/gcc/startup_board_b.s"} {
		if !exists(filepath.Join(root, want)) {
			t.Errorf("%s is missing after the second board's build", want)
		}
	}
	if exists(filepath.Join(root, "Source/gcc/startup_board_c.s")) || exists(filepath.Join(root, "Documentation/enormous-manual.md")) {
		t.Errorf("the checkout was widened past what the two boards named")
	}
}

// A board whose paths are all checked out already runs nothing, so a build that
// needs nothing new works without the repository it came from.
func TestFetchedRepositoryNeedsNoNetworkWhenNothingIsNew(t *testing.T) {
	url := makeVendorRepo(t)
	resolver := &Resolver{ProjectRoot: t.TempDir(), Cache: t.TempDir()}

	board := CDep{Name: "device", Git: url, Branch: "main",
		Include: []string{"Include"}, Src: []string{"Source/gcc/startup_board_a.s"}}
	root, err := resolver.rootOf(board)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// The origin goes away.
	if err := os.RemoveAll(strings.TrimPrefix(url, "file://")); err != nil {
		t.Fatal(err)
	}
	before := runGit(t, root, "sparse-checkout", "list")
	if _, err := resolver.rootOf(board); err != nil {
		t.Fatalf("resolve offline: %v", err)
	}
	if after := runGit(t, root, "sparse-checkout", "list"); after != before {
		t.Errorf("the checkout changed although nothing was new:\nbefore %q\nafter  %q", before, after)
	}
	if !exists(filepath.Join(root, "Source/gcc/startup_board_a.s")) {
		t.Errorf("the startup file went missing")
	}
}

// A checkout that is not sparse has everything already, and is not made sparse.
func TestFullCheckoutIsLeftAsItIs(t *testing.T) {
	url := makeVendorRepo(t)
	cache := t.TempDir()
	resolver := &Resolver{ProjectRoot: t.TempDir(), Cache: cache}
	board := CDep{Name: "device", Git: url, Branch: "main",
		Include: []string{"Include"}, Src: []string{"Source/gcc/startup_board_a.s"}}

	dest := filepath.Join(cache, resolver.directoryFor(board))
	runGit(t, cache, "clone", "-q", url, dest)

	if _, err := resolver.rootOf(board); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !exists(filepath.Join(dest, "Documentation/enormous-manual.md")) {
		t.Errorf("a full checkout was narrowed")
	}
	if out := runGit(t, dest, "config", "--default", "false", "--get", "core.sparseCheckout"); strings.TrimSpace(out) != "false" {
		t.Errorf("a full checkout was made sparse: core.sparseCheckout=%s", out)
	}
}
