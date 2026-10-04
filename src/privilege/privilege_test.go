//
// @file privilege_test.go
// @brief Tests for when a root run is worth a word.
//
// Three situations and a silence, and the awkward one in the middle is a container:
// a warning about root there teaches people to ignore the warning that matters. So
// these are about which sentence comes out, and about never failing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package privilege

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyRootIsEverToldAnything(t *testing.T) {
	out := &bytes.Buffer{}
	if Check(out, Situation{Root: false, RhoHome: "/home/a/.rho"}) {
		t.Fatalf("expected nothing said to somebody who is not root, got %q", out.String())
	}
	if out.Len() != 0 {
		t.Errorf("expected no output at all, got %q", out.String())
	}
}

// A container runs as root and always will. A paragraph about being careful there
// would be noise, and noise is what makes people stop reading warnings.
func TestAContainerIsToldOnceAndBriefly(t *testing.T) {
	for name, situation := range map[string]Situation{
		"a container": {Root: true, Container: true, RhoHome: "/root/.rho"},
		"a pipeline":  {Root: true, CI: true, RhoHome: "/root/.rho"},
	} {
		out := &bytes.Buffer{}
		if !Check(out, situation) {
			t.Errorf("%s: expected to be told something", name)
		}
		text := out.String()
		if !strings.Contains(text, "container") {
			t.Errorf("%s: expected one line naming the situation, got %q", name, text)
		}
		if strings.Count(text, "\n") > 1 {
			t.Errorf("%s: expected one line, got %d: %q", name, strings.Count(text, "\n"), text)
		}
		if strings.Contains(text, "belongs to root") {
			t.Errorf("%s: expected no lecture about ownership in a container, got %q", name, text)
		}
	}
}

// The case that matters: somebody ran `sudo rho`, and the sentence has to say what
// that costs and who should run it instead. A warning that does not name the
// consequence is not a warning.
func TestASudoInvocationIsToldWhatItCosts(t *testing.T) {
	out := &bytes.Buffer{}
	if !Check(out, Situation{Root: true, SudoUser: "mira", RhoHome: "/home/mira/.rho"}) {
		t.Fatal("expected to be told")
	}
	text := out.String()
	for _, wanted := range []string{"sudo", "root", "/home/mira/.rho", "mira"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("expected %q in the warning:\n%s", wanted, text)
		}
	}
	// And the way out, because a warning without one is just a complaint.
	if !strings.Contains(text, "as mira instead") {
		t.Errorf("expected to be told who should run it instead:\n%s", text)
	}
}

// A root shell is somebody setting a machine up, which is a legitimate thing to do
// with this toolchain. Said once, without accusing anybody of anything.
func TestARootShellIsToldOnceWithoutAccusation(t *testing.T) {
	out := &bytes.Buffer{}
	Check(out, Situation{Root: true, RhoHome: "/root/.rho"})
	text := out.String()
	if strings.Contains(text, "SudoUser") || strings.Contains(text, "chown") {
		t.Errorf("expected the sudo case's advice not to appear here:\n%s", text)
	}
	if !strings.Contains(text, "belong to root") {
		t.Errorf("expected to be told what it costs:\n%s", text)
	}
}

// The part that is actually broken: files this toolchain owns have already been
// created by root, and nobody finds that out for an afternoon. Said with the
// concrete thing, because `chown -R` advice that turns out to be irrelevant costs
// more than the warning was worth — so a directory nobody owns is not reported.
func TestRootOwnedStateIsCalledOutAndAnUnownedOneIsNot(t *testing.T) {
	root := t.TempDir()
	if os.Geteuid() != 0 {
		t.Skip("this test needs to create a directory owned by root")
	}
	// Written as root, so it is root-owned in a way the test can rely on.
	owned := filepath.Join(root, "rho-home")
	if err := os.MkdirAll(owned, 0o755); err != nil {
		t.Fatal(err)
	}
	unowned := filepath.Join(root, "project")
	if err := os.MkdirAll(unowned, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(unowned, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	Check(out, Situation{Root: true, RhoHome: owned, WorkDir: unowned})
	text := out.String()
	if !strings.Contains(text, owned) {
		t.Errorf("expected the root-owned state to be named:\n%s", text)
	}
	if strings.Contains(text, unowned) {
		t.Errorf("expected no mention of a directory that is not root's:\n%s", text)
	}
	if !strings.Contains(text, "chown") {
		t.Errorf("expected the way out to be given:\n%s", text)
	}
}

// A pipeline that has read the warning and has decided, with no way to pass a flag,
// has one environment setting to say so.
func TestTheWarningCanBeSilenced(t *testing.T) {
	t.Setenv("RHO_NO_PRIVILEGE_WARNING", "1")
	if !Silenced() {
		t.Error("expected the warning to be silenced")
	}
	t.Setenv("RHO_NO_PRIVILEGE_WARNING", "false")
	if Silenced() {
		t.Error("expected `false` not to silence anything")
	}
	t.Setenv("RHO_NO_PRIVILEGE_WARNING", "")
	t.Setenv("CI", "true")
	if Silenced() {
		t.Error("expected the absence of the setting to leave the warning on in a pipeline")
	}
}

// `CI=false` is a pipeline that says it is not one, and must be believed: some
// tools set the variable to that value rather than removing it.
func TestAPipelineThatSaysItIsNotOneIsBelieved(t *testing.T) {
	for name, value := range map[string]string{
		"CI":             "false",
		"GITHUB_ACTIONS": "0",
		"GITLAB_CI":      "no",
	} {
		t.Setenv(name, value)
	}
	if isCI() {
		t.Error("expected a pipeline that says it is not one not to count as one")
	}
	t.Setenv("CI", "true")
	if !isCI() {
		t.Error("expected CI to count")
	}
}

// Nothing here may ever fail a run. The check returns whether it spoke, and that is
// all a caller can do with it — a container cannot be told it may not build.
func TestTheCheckHasNoWayToFailARun(t *testing.T) {
	for _, situation := range []Situation{
		{Root: true},
		{Root: true, Container: true},
		{Root: true, CI: true, SudoUser: "a"},
		{Root: true, SudoUser: "a", RhoHome: "/nonexistent", WorkDir: "/nonexistent"},
	} {
		out := &bytes.Buffer{}
		Check(out, situation) // must not panic on paths that are not there
	}
}
