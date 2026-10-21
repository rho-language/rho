//
// @file manifest_test.go
// @brief Tests for the manifest that says which tests a project has.
//
// A manifest is a claim, and the tests here are mostly about what the runner does
// with a claim it cannot act on: an empty matrix, a suite with no name, two suites
// with one name. Each of those is a manifest that says something untrue, and each
// is caught here rather than becoming a run that quietly did less than it claimed.
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
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                   */
/* -------------------------------------------------------------------------- */

func TestAManifestIsRead(t *testing.T) {
	manifest, err := LoadManifest(writeManifest(t, `name: platform
suites:
  - name: error
    kind: gtest
    tags: [unit, platform]
  - name: sw-smoke
    kind: build
    tags: [smoke]
`))
	if err != nil {
		t.Fatalf("expected a manifest, got %v", err)
	}
	if manifest.Name != "platform" {
		t.Errorf("expected the project name, got %q", manifest.Name)
	}
	if len(manifest.Declared) != 2 {
		t.Fatalf("expected two suites, got %+v", manifest.Declared)
	}
	if manifest.Declared[0].Name != "error" || manifest.Declared[0].Kind != "gtest" {
		t.Errorf("expected the first suite, got %+v", manifest.Declared[0])
	}
	if strings.Join(manifest.Declared[0].Tags, ",") != "unit,platform" {
		t.Errorf("expected the suite's tags, got %v", manifest.Declared[0].Tags)
	}
}

func TestAManifestIsFoundBesideTheProject(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "tests.yml"), []byte("name: p\nsuites:\n  - name: a\n    kind: rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := FindManifest(root)
	if !ok || filepath.Base(path) != "tests.yml" {
		t.Fatalf("expected the manifest under tests/, got %q ok=%v", path, ok)
	}
	if _, err := LoadProjectManifest(root); err != nil {
		t.Fatalf("expected it to load, got %v", err)
	}
}

// Two manifests in one project is a project whose tests depend on which file was
// read. The runner refuses rather than picking one, because picking one silently
// would mean the other is a file nobody maintains.
func TestTwoManifestsAreARefusalNotAPick(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"tests.yml", filepath.Join("tests", "tests.yml")} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("name: p\nsuites:\n  - name: a\n    kind: rho\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := FindManifest(root); ok {
		t.Error("expected two manifests to be refused rather than one of them chosen")
	}
}

/* -------------------------------------------------------------------------- */
/*  What a manifest may not say                                                */
/* -------------------------------------------------------------------------- */

func TestAManifestWithNoSuitesIsAnError(t *testing.T) {
	// A manifest that declares nothing has not said the project has no tests; it
	// has said the file is empty, and a run that reported "nothing to do" would be
	// reporting a fact the file does not contain.
	if _, err := LoadManifest(writeManifest(t, "name: p\nsuites:\n")); err == nil {
		t.Error("expected an empty suite list to be an error")
	}
}

func TestASuiteWithNoNameIsAnError(t *testing.T) {
	// A report names suites by their names, so an unnamed one cannot be told apart
	// from any other, nor filtered for.
	if _, err := LoadManifest(writeManifest(t, "name: p\nsuites:\n  - kind: rho\n")); err == nil {
		t.Error("expected a suite with no name to be an error")
	}
}

func TestASuiteWithNoKindIsAnError(t *testing.T) {
	// The kind is what decides who knows how to run the suite. A suite with none is
	// a claim nobody can act on.
	if _, err := LoadManifest(writeManifest(t, "name: p\nsuites:\n  - name: a\n")); err == nil {
		t.Error("expected a suite with no kind to be an error")
	}
}

func TestTwoSuitesWithOneNameAreAnError(t *testing.T) {
	// A filter for one would run both, and a report could not say which failed.
	_, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: error
    kind: rho
  - name: error
    kind: gtest
`))
	if err == nil || !strings.Contains(err.Error(), "two suites") {
		t.Errorf("expected duplicate names to be an error, got %v", err)
	}
}

func TestAMatrixWithNoValuesIsAnError(t *testing.T) {
	// A smoke test over a list of boards that is currently empty is a test that
	// passes without testing anything. That is the one outcome a smoke suite must
	// never have, so it is caught in the manifest rather than in the run.
	_, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: sw-smoke
    kind: build
    matrix:
      board: []
`))
	if err == nil || !strings.Contains(err.Error(), "no values") {
		t.Errorf("expected an empty axis to be an error, got %v", err)
	}
}

func TestAMatrixAxisMustBeAList(t *testing.T) {
	_, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: sw-smoke
    kind: build
    matrix:
      board: nucleo-h753zi
`))
	if err == nil || !strings.Contains(err.Error(), "list of values") {
		t.Errorf("expected an axis that is not a list to be an error, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  The two words that describe a suite                                        */
/* -------------------------------------------------------------------------- */

func TestASuiteDeclaredAsNotRunCarriesItsReason(t *testing.T) {
	// A suite somebody wanted to write down before it could be written. Saying so
	// is what lets a project record the hardware test it has not written yet — and
	// it is kept out of the pass column, because a skipped suite is not a pass.
	manifest, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: hw-smoke
    kind: hardware
    tags: [smoke]
    status: skip
    reason: needs a board on a bench and a person to watch it
`))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Declared[0].Skip == "" {
		t.Fatal("expected the suite to carry a reason")
	}
	if !strings.Contains(manifest.Declared[0].Skip, "bench") {
		t.Errorf("expected the reason the manifest gave, got %q", manifest.Declared[0].Skip)
	}
}

func TestASuiteWithNoReasonStillSaysWhyItIsNotRun(t *testing.T) {
	// "Not run" with no reason is still a claim, and a run that reported it as
	// nothing at all would let the gap disappear.
	manifest, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: hw-smoke
    kind: hardware
    status: skip
`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest.Declared[0].Skip, "hw-smoke") {
		t.Errorf("expected the fallback to name the suite, got %q", manifest.Declared[0].Skip)
	}
}

/* -------------------------------------------------------------------------- */
/*  Commands                                                                  */
/* -------------------------------------------------------------------------- */

// A command is written as a list of words or as one string. Both are read
// because a manifest is read by people, and the two forms a person reaches for
// are those.
func TestACommandIsReadBothWays(t *testing.T) {
	manifest, err := LoadManifest(writeManifest(t, `name: p
suites:
  - name: listed
    kind: command
    command: ["./smoke.sh", "--fast"]
  - name: written
    kind: command
    command: ./smoke.sh --slow
`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(manifest.Declared[0].Command, " ") != "./smoke.sh --fast" {
		t.Errorf("expected a listed command, got %v", manifest.Declared[0].Command)
	}
	if strings.Join(manifest.Declared[1].Command, " ") != "./smoke.sh --slow" {
		t.Errorf("expected a written command, got %v", manifest.Declared[1].Command)
	}
}

/* -------------------------------------------------------------------------- */
/*  Matrices                                                                  */
/* -------------------------------------------------------------------------- */

func TestAMatrixExpandsToOneNamePerCell(t *testing.T) {
	matrix := Matrix{
		Axes: []string{"example", "board"},
		Values: map[string][]string{
			"example": {"examples/c", "examples/rho"},
			"board":   {"nucleo-h753zi", "nucleo-f746zg"},
		},
	}
	if matrix.Cells() != 4 {
		t.Errorf("expected four cells, got %d", matrix.Cells())
	}
	names := matrix.Expand("sw-smoke")
	if len(names) != 4 {
		t.Fatalf("expected four names, got %v", names)
	}
	// Sorted, so two runs of the same manifest report in the same order.
	if names[0] != "sw-smoke!example=examples/c!board=nucleo-f746zg" {
		t.Errorf("expected a stable order, got %v", names)
	}
	if got := AxisValue(names[0], "board"); got != "nucleo-f746zg" {
		t.Errorf("expected the board to be readable back out of the name, got %q", got)
	}
}

// A cell's values are paths, and a path contains the character a cell name joins
// its axes with. Reading the first `example=` off
// `sw-smoke/example=examples/c/hello_world/board=nucleo-h753zi` gives `examples`,
// and a smoke matrix would then build whatever directory happened to share the
// first segment — which is how a whole matrix can quietly build the wrong thing and
// report it as built.
func TestAnAxisValueMayContainSlashes(t *testing.T) {
	name := "sw-smoke!board=nucleo-h753zi!example=examples/c/hello_world"
	if got := AxisValue(name, "example"); got != "examples/c/hello_world" {
		t.Errorf("expected the whole path, got %q", got)
	}
	if got := AxisValue(name, "board"); got != "nucleo-h753zi" {
		t.Errorf("expected the board, got %q", got)
	}
	// Either order of the axes gives the same answer, because which came first is
	// not something a manifest should have to get right.
	reversed := "sw-smoke!example=examples/c/hello_world!board=nucleo-h753zi"
	if got := AxisValue(reversed, "example"); got != "examples/c/hello_world" {
		t.Errorf("expected the whole path, got %q", got)
	}
	if got := AxisValue(reversed, "board"); got != "nucleo-h753zi" {
		t.Errorf("expected the board, got %q", got)
	}
	if got := AxisValue(name, "nosuchaxis"); got != "" {
		t.Errorf("expected an axis the cell was not expanded for to be empty, got %q", got)
	}
}

// A manifest may include another, which is how a project keeps its tests in
// folders — the unit tests in one, the smoke tests in another — and says so in
// one place. The suites of the included manifest are the project's suites: they
// run in the project's own root, not in the folder they were written in.
func TestAManifestMayIncludeAnother(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests", "unit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests", "smoke"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "unit", "tests.yml"), []byte(
		"name: unit\nsuites:\n  - name: the unit tests\n    kind: command\n    command: [\"/bin/true\"]\n    tags: [unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "smoke", "tests.yml"), []byte(
		"name: smoke\nsuites:\n  - name: the smoke tests\n    kind: build\n    tags: [smoke]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "tests.yml"), []byte(
		"name: p\ninclude:\n  - unit/tests.yml\n  - smoke/tests.yml\nsuites: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := LoadProjectManifest(root)
	if err != nil {
		t.Fatalf("expected the manifest to load, got %v", err)
	}
	if len(manifest.Declared) != 2 {
		t.Fatalf("expected the two included suites, got %+v", manifest.Declared)
	}
	if manifest.Declared[0].Name != "the unit tests" || manifest.Declared[1].Name != "the smoke tests" {
		t.Errorf("expected the included suites in order, got %+v", manifest.Declared)
	}
	// The suites belong to the project, so a path in one of them is the
	// project's path rather than the folder's.
	built, err := BuiltinResolvers()[2].Resolve(manifest.Declared[1], root)
	if err != nil {
		t.Fatalf("expected the included suite to resolve, got %v", err)
	}
	if built.Root != root {
		t.Errorf("expected the included suite to run in the project, got %q", built.Root)
	}
}
