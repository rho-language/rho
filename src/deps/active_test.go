//
// @file active_test.go
// @brief Which of the installed modules is the one that runs.
//
// These are the rules a machine relies on to build the same program twice: a
// component or plugin is not used until somebody says so, and when several
// versions are installed exactly one of them is the one a build links.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installed writes an installed module of a kind and version.
func installed(t *testing.T, kind, name, version, moduleType string) string {
	t.Helper()
	root := filepath.Join(RhoHome(), "modules", "installed", kind, name+"@"+version)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: " + name + "\n  type: " + moduleType +
		"\n  version: " + version + "\n\nmain: " + name + "\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Installing makes a module available. It does not make it used, because a
// program that runs a component nobody chose is running code its author never
// asked for.
func TestAnInstalledModuleIsNotUsedUntilItIsEnabled(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindComponent, "rho-log", "0.1.0", "component")

	if got := InstalledComponentRoot("rho-log"); got != "" {
		t.Fatalf("a module that was only installed was used: %s", got)
	}
	// It is still there, which is the difference between not enabled and not
	// installed.
	if got := InstalledVersions(KindComponent, "rho-log"); len(got) != 1 {
		t.Fatalf("expected the version to be installed, got %v", got)
	}

	if err := Enable(KindComponent, "rho-log"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledComponentRoot("rho-log"); got == "" {
		t.Fatal("an enabled module did not resolve")
	}
}

// Several versions may be installed at once, and exactly one of them is the one
// a build links. Choosing the largest number would make a build depend on what
// was installed last rather than on what anybody decided to use.
func TestOneVersionIsEnabledAndItIsTheOneThatRuns(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	old := installed(t, KindComponent, "rho-log", "0.1.0", "component")
	newer := installed(t, KindComponent, "rho-log", "0.2.0", "component")

	if err := SetVersionEnabled(KindComponent, "rho-log", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledComponentRoot("rho-log"); got != old {
		t.Fatalf("expected the enabled version %s, got %s", old, got)
	}

	// Turning it onto the newer one moves it, rather than adding to it: a
	// module has one enabled version, not a set.
	if err := Enable(KindComponent, "rho-log"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledComponentRoot("rho-log"); got != newer {
		t.Fatalf("expected the newly enabled version %s, got %s", newer, got)
	}
}

// A module that is turned off is not used, and is still installed: turning it
// back on must not mean installing it again.
func TestDisablingKeepsTheModuleInstalled(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindPlugin, "rho-doc-toc", "0.1.0", "plugin")

	if err := Enable(KindPlugin, "rho-doc-toc"); err != nil {
		t.Fatal(err)
	}
	if err := Disable(KindPlugin, "rho-doc-toc"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledPluginRoot("rho-doc-toc"); got != "" {
		t.Fatalf("a disabled plugin was still used: %s", got)
	}
	if got := InstalledVersions(KindPlugin, "rho-doc-toc"); len(got) != 1 {
		t.Fatalf("disabling removed the plugin: %v", got)
	}
	if err := Enable(KindPlugin, "rho-doc-toc"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledPluginRoot("rho-doc-toc"); got == "" {
		t.Fatal("a disabled plugin could not be turned back on")
	}
}

// A name that is installed and not enabled has to be told apart from one that
// was never installed: the reader has the first and needs to turn it on, and
// does not have the second and needs to install it.
func TestTheRefusalSaysWhichOfTheTwoItIs(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindComponent, "rho-sql", "0.1.0", "component")

	err := &NotEnabledError{Kind: KindComponent, Name: "rho-sql", Installed: InstalledVersions(KindComponent, "rho-sql")}
	message := err.Error()
	if !strings.Contains(message, "installed but not enabled") {
		t.Errorf("the refusal does not say the module is installed: %v", err)
	}
	if !strings.Contains(message, "rho module enable rho-sql@0.1.0") {
		t.Errorf("the refusal does not say how to fix it: %v", err)
	}

	// One that was never installed is a different message, because the fix is
	// different.
	missing := &NotEnabledError{Kind: KindComponent, Name: "rho-fmt"}
	if !strings.Contains(missing.Error(), "not installed") {
		t.Errorf("a module that was never installed is reported as disabled: %v", missing)
	}
}

// A version that is not installed cannot be enabled: a record naming one would
// make every later run fail about a module the reader believes they have.
func TestAVersionThatIsNotInstalledIsRefused(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindComponent, "rho-log", "0.1.0", "component")

	if err := SetVersionEnabled(KindComponent, "rho-log", "0.9.0"); err == nil {
		t.Fatal("a version that is not installed was enabled")
	}
	if err := Enable(KindComponent, "rho-absent"); err == nil {
		t.Fatal("a module that is not installed was enabled")
	}
}

// The record is what makes the decision outlive the command that made it.
func TestTheRecordSurvivesAndReadsBack(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindComponent, "rho-log", "0.1.0", "component")
	if err := Enable(KindComponent, "rho-log"); err != nil {
		t.Fatal(err)
	}

	state, err := LoadEnabled()
	if err != nil {
		t.Fatal(err)
	}
	version, decided := state.Recorded(KindComponent, "rho-log")
	if !decided || version != "0.1.0" {
		t.Fatalf("the record says %q (decided=%v), want 0.1.0", version, decided)
	}
	if !state.IsEnabled(KindComponent, "rho-log", "0.1.0") {
		t.Fatal("the recorded version is not the one that runs")
	}
	// A version nobody chose is not the one that runs, and that is the whole
	// point of the record.
	if state.IsEnabled(KindComponent, "rho-log", "0.2.0") {
		t.Fatal("a version that was never enabled reported itself enabled")
	}
}

// A record that cannot be read is an error rather than an empty one. Read as
// empty it would say nothing is enabled, which is a statement about the
// installation that nobody made.
func TestAnUnreadableRecordIsNotAnEmptyOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EnabledPath(), []byte("component:\n  - not a map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := LoadEnabled()
	if err == nil {
		t.Fatalf("a malformed record was read as an empty one: %+v", state)
	}
}

// A component and a plugin of the same name are different programs, and are
// enabled separately.
func TestTheSameNameInTwoKindsIsEnabledSeparately(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, KindComponent, "shared", "0.1.0", "component")
	installed(t, KindPlugin, "shared", "0.2.0", "plugin")

	if err := Enable(KindComponent, "shared"); err != nil {
		t.Fatal(err)
	}
	if got := InstalledPluginRoot("shared"); got != "" {
		t.Fatalf("enabling a component also enabled a plugin of the same name: %s", got)
	}
}
