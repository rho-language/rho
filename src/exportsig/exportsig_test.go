package exportsig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rho/internal/paths"
)

// A module whose manifest moved to a new version is compared with the module as
// it was at the version before, in git; removing an export under a minor
// version of a 1.x module is refused, with the consumer that uses it.
func TestARemovedExportNeedsAMajorVersionAndNamesItsConsumers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the path")
	}
	paths.SetTestRhoHome(t.TempDir())
	workspace := t.TempDir()
	t.Setenv("RHO_WORKSPACE", workspace)
	write := func(path, text string) {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(dir string, args ...string) {
		command := exec.Command("git", args...)
		command.Dir = filepath.Join(workspace, dir)
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	manifest := func(version string) string {
		return "module:\n  name: acme-shape\n  type: module\n  version: " + version + "\nmain: src/shape.rho\nprovides:\n  import: acme.shape\n  exports:\n    - area\n    - Point\n    - ORIGIN\n"
	}
	write("acme/modules/shape/module.yml", manifest("1.0.0"))
	write("acme/modules/shape/src/shape.rho", "module shape\ninclude \"point.rho\"\nconst ORIGIN: u32 = 0\nfn area(w: u32, h: u32) u32 { return w * h }\n")
	write("acme/modules/shape/src/point.rho", "module shape.point\nstruct Point { x: u32, y: u32 }\n")
	run("acme", "init", "-q")
	run("acme", "add", ".")
	run("acme", "commit", "-q", "-m", "1.0.0")
	write("app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndeps:\n  acme:\n    acme-shape: \">=1.0.0\"\n")
	write("app/src/main.rho", "module main\nimport acme.shape\nfn main() void {\n    // area is not called here\n    print(int_to_str(shape.area(2, 3)))\n}\n")

	// A later commit of the same version is still 1.0.0.
	write("acme/modules/shape/src/shape.rho", "module shape\ninclude \"point.rho\"\nconst ORIGIN: u32 = 0\n// doubled\nfn area(w: u32, h: u32) u32 { return w * h }\n")
	run("acme", "commit", "-q", "-am", "comment")

	// 1.1.0 renames a parameter, which promises nothing, adds a field, which
	// does, and removes nothing yet.
	write("acme/modules/shape/module.yml", manifest("1.1.0"))
	write("acme/modules/shape/src/point.rho", "module shape.point\nstruct Point { x: u32, y: u32, z: u32 }\n")
	write("acme/modules/shape/src/shape.rho", "module shape\ninclude \"point.rho\"\nconst ORIGIN: u32 = 0\nfn area(width: u32, height: u32) u32 { return width * height }\n")
	root := filepath.Join(workspace, "acme", "modules", "shape")
	current, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	baseline, found, err := FindBaseline(root, current)
	if err != nil || !found {
		t.Fatalf("baseline: found %v, %v", found, err)
	}
	if baseline.Set.Version != "1.0.0" || !strings.Contains(baseline.Source, "in git") {
		t.Errorf("baseline = %s from %s", baseline.Set.Version, baseline.Source)
	}
	diff := Compare(baseline.Set, current)
	if len(diff.Changed) != 1 || diff.Changed[0].Name != "Point" || len(diff.Removed) != 0 {
		t.Errorf("diff = %+v", diff)
	}
	if err := CheckVersion("1.0.0", "1.1.0", diff); err == nil || !strings.Contains(err.Error(), "needs 2.0.0") {
		t.Errorf("a changed struct under a minor version: %v", err)
	}

	// Removing the function: the consumer's call is found, not its comment.
	write("acme/modules/shape/src/point.rho", "module shape.point\nstruct Point { x: u32, y: u32 }\n")
	write("acme/modules/shape/src/shape.rho", "module shape\ninclude \"point.rho\"\nconst ORIGIN: u32 = 0\n")
	current, _ = Read(root)
	diff = Compare(baseline.Set, current)
	if len(diff.Removed) != 1 || diff.Removed[0].Name != "area" {
		t.Fatalf("diff = %+v", diff)
	}
	consumers := FindConsumers(root, current, diff.Names())
	if len(consumers) != 1 || consumers[0].Module != "app" || len(consumers[0].Uses) != 1 || consumers[0].Uses[0].Line != 5 {
		t.Errorf("consumers = %+v", consumers)
	}
	if err := CheckVersion("1.0.0", "2.0.0", diff); err != nil {
		t.Errorf("a major version: %v", err)
	}
	if err := CheckVersion("0.3.1", "0.3.2", diff); err == nil || !strings.Contains(err.Error(), "needs at least 0.4.0") {
		t.Errorf("a 0.x patch: %v", err)
	}
}

// A published version's record is the baseline in preference to git.
func TestAPublishedRecordIsTheBaseline(t *testing.T) {
	home := t.TempDir()
	paths.SetTestRhoHome(home)
	installed, _ := paths.GetModulesInstalledPath()
	for _, version := range []string{"0.1.0", "0.2.0", "0.9.0"} {
		dir := filepath.Join(installed, "library", "acme-shape@"+version)
		os.MkdirAll(dir, 0o755)
		Write(dir, Set{Module: "acme-shape", Version: version, Entries: map[string]Entry{"area": {Name: "area", Kind: "fn", Signature: "fn(u32) u32"}}})
	}
	baseline, found, _ := FindBaseline(t.TempDir(), Set{Module: "acme-shape", Version: "0.3.0"})
	if !found || baseline.Set.Version != "0.2.0" || !strings.HasPrefix(baseline.Source, "published") {
		t.Errorf("baseline = %+v, %v", baseline, found)
	}
}
