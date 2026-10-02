//
// @file suites_test.go
// @brief Tests for turning a declared gtest suite into steps that run it.
//
// The plugin's whole claim on this is that it does not assume how a project builds
// its tests. So the tests here are mostly about what it passes through, and about
// the one thing it refuses to do: invent a path.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package gtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// declared is a suite as the runner asks about it.
func declared(name string, options map[string]any) DeclaredSuite {
	return DeclaredSuite{Name: name, Kind: KindGTest, Options: options}
}

func gtestOptions(binary string, extra map[string]any) map[string]any {
	settings := map[string]any{"binary": binary}
	for key, value := range extra {
		settings[key] = value
	}
	return map[string]any{"gtest": settings}
}

/* -------------------------------------------------------------------------- */
/*  The steps                                                                 */
/* -------------------------------------------------------------------------- */

// A suite is run in three steps: the build the project declared, the binary
// itself told to write a report, and this plugin reading that report. The third is
// what makes the second useful — the framework's XML is the only place it states
// its results unambiguously, so that is what is read.
func TestASuiteIsRunAsBuildThenRunThenReport(t *testing.T) {
	root := t.TempDir()
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", map[string]any{
		"configure": []any{"cmake", "-S", ".", "-B", "build"},
		"build":     []any{"cmake", "--build", "build"},
	})), root)

	// fetch, configure, build, run, report.
	if len(resolved.Steps) != 5 {
		t.Fatalf("expected fetch, configure, build, run and report, got %+v", resolved.Steps)
	}
	run := resolved.Steps[3]
	if run.Command[len(run.Command)-1] == "" {
		t.Error("expected the run to ask for a report")
	}
	joined := strings.Join(run.Command, " ")
	if !strings.Contains(joined, "--gtest_output=xml:") {
		t.Errorf("expected the run to write a report, got %q", joined)
	}
	if !strings.HasSuffix(run.Command[0], filepath.Join("build", "tests", "error")) {
		t.Errorf("expected the declared binary, got %q", run.Command[0])
	}
	if !strings.Contains(strings.Join(resolved.Steps[4].Command, " "), "--report") {
		t.Errorf("expected the report to be read by this plugin, got %v", resolved.Steps[4].Command)
	}
	if resolved.Steps[4].Produces == "" {
		t.Error("a step that leaves a report behind should say where")
	}
}

// A project that declares a GoogleTest suite has said it wants GoogleTest. A
// runner that needs a manual `git clone` first is a step between the declaration
// and the result, and it is the kind that a person forgets on the machine that is
// not theirs.
func TestGoogleTestIsFetchedRatherThanAsked(t *testing.T) {
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", map[string]any{
		"configure": []any{"cmake", "-S", ".", "-B", "build"},
		"build":     []any{"cmake", "--build", "build"},
	})), t.TempDir())

	if len(resolved.Steps) == 0 {
		t.Fatal("expected the fetch to be among the steps")
	}
	fetch := resolved.Steps[0]
	if !strings.Contains(strings.Join(fetch.Command, " "), googletestRepository) {
		t.Errorf("expected the fetch to name where GoogleTest comes from, got %v", fetch.Command)
	}
	// The build is told where it went, because a project cannot hardcode a path it
	// did not fetch and a plugin cannot write the answer into the project's files.
	for _, step := range resolved.Steps {
		if len(step.Env) == 0 {
			continue
		}
		found := false
		for _, entry := range step.Env {
			if strings.HasPrefix(entry, "GTEST_ROOT=") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q to be told where GoogleTest is, got %v", step.Name, step.Env)
		}
	}
}

// A suite whose binary is already built does not need a framework fetched to run
// it, and a run must not reach for the network because a project declared a suite
// it has already compiled.
func TestNoFrameworkIsFetchedForASuiteThatIsAlreadyBuilt(t *testing.T) {
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", nil)), t.TempDir())

	if len(resolved.Steps) == 0 {
		t.Fatal("expected a run and a report")
	}
	if strings.Contains(strings.Join(resolved.Steps[0].Command, " "), googletestRepository) {
		t.Errorf("expected no fetch for a suite that is already built, got %v", resolved.Steps[0].Command)
	}
}

// A person who has already built their tests should not have them rebuilt: the
// steps to build them are the project's to declare, and a plugin that always
// configured and built would take over a build it knows nothing about.
func TestAProjectThatBuildsItsOwnTestsDeclaresNoBuildSteps(t *testing.T) {
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", nil)), t.TempDir())

	if len(resolved.Steps) != 2 {
		t.Fatalf("expected only the run and the report, got %+v", resolved.Steps)
	}
	if resolved.Steps[0].Name == "configure" || resolved.Steps[0].Name == "build" {
		t.Errorf("expected no build steps, got %q", resolved.Steps[0].Name)
	}
}

// A plugin that guessed where a project's test binary was would be a plugin that
// ran somebody else's tests. So a suite that does not say is skipped, and says so.
func TestASuiteWithNoBinaryIsSkippedRatherThanGuessed(t *testing.T) {
	resolved := Resolve(declared("error", map[string]any{"gtest": map[string]any{}}), t.TempDir())

	if resolved.Skip == "" {
		t.Fatal("expected a suite with no binary to be skipped")
	}
	if len(resolved.Steps) != 0 {
		t.Errorf("expected no steps for a suite that cannot run, got %+v", resolved.Steps)
	}
	if !strings.Contains(resolved.Skip, "binary") {
		t.Errorf("expected the reason to say what is missing, got %q", resolved.Skip)
	}
}

func TestASuiteWithNoGtestMappingIsSkipped(t *testing.T) {
	resolved := Resolve(declared("error", nil), t.TempDir())
	if resolved.Skip == "" {
		t.Error("expected a suite with no settings to be skipped rather than resolved")
	}
}

// A report is an output, so it goes under the build directory rather than beside
// the sources, unless the manifest said otherwise.
func TestAReportGoesUnderTheBuildDirectory(t *testing.T) {
	root := t.TempDir()
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", nil)), root)

	if !strings.Contains(resolved.Steps[1].Produces, filepath.Join("build", "test-reports")) {
		t.Errorf("expected the report under the build directory, got %q", resolved.Steps[1].Produces)
	}
	if !strings.HasPrefix(resolved.Steps[1].Produces, root) {
		t.Errorf("expected an absolute path, so a step's working directory does not change where it lands: %q", resolved.Steps[1].Produces)
	}
}

func TestAManifestCanSayWhereTheReportGoes(t *testing.T) {
	root := t.TempDir()
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", map[string]any{
		"xml": "build/reports/error.xml",
	})), root)

	if !strings.HasSuffix(resolved.Steps[1].Produces, filepath.Join("reports", "error.xml")) {
		t.Errorf("expected the declared path, got %q", resolved.Steps[1].Produces)
	}
}

// A filter is for a developer running one case. It is passed through, and it does
// not change anything else about how the suite runs.
func TestAFilterIsPassedToTheBinary(t *testing.T) {
	resolved := Resolve(declared("error", gtestOptions("build/tests/error", map[string]any{
		"filter": "arithmetic.adds",
	})), t.TempDir())

	if !strings.Contains(strings.Join(resolved.Steps[0].Command, " "), "--gtest_filter=arithmetic.adds") {
		t.Errorf("expected the filter to be passed, got %v", resolved.Steps[0].Command)
	}
}

/* -------------------------------------------------------------------------- */
/*  Which suites this plugin answers for                                       */
/* -------------------------------------------------------------------------- */

// A run asks every plugin about every declaration at once, so this one answers
// only for its own kinds and leaves the rest alone.
func TestOnlyThisPluginsKindsAreAnsweredFor(t *testing.T) {
	suites := []DeclaredSuite{
		declared("mine", gtestOptions("build/tests/mine", nil)),
		{Name: "rho", Kind: "rho"},
		{Name: "smoke", Kind: "command"},
	}
	resolved := ResolveAll(t.TempDir(), suites)

	if len(resolved) != 1 {
		t.Fatalf("expected only the gtest suite, got %+v", resolved)
	}
	if resolved[0].Declared != "mine" {
		t.Errorf("expected the gtest suite, got %q", resolved[0].Declared)
	}
}

// The spelling is matched without regard to case, because the two a person
// reaches for are both reasonable and neither is worth failing a build over.
func TestTheKindIsMatchedWithoutRegardToCase(t *testing.T) {
	resolved := ResolveAll(t.TempDir(), []DeclaredSuite{
		{Name: "a", Kind: "GTest", Options: gtestOptions("build/a", nil)},
		{Name: "b", Kind: "gtest", Options: gtestOptions("build/b", nil)},
	})
	if len(resolved) != 2 {
		t.Errorf("expected both spellings answered, got %+v", resolved)
	}
}

/* -------------------------------------------------------------------------- */
/*  Commands written both ways                                                */
/* -------------------------------------------------------------------------- */

// A manifest is read by people, and both of these are what a person writes.
func TestABuildCommandIsReadAsAListOrAsAString(t *testing.T) {
	root := t.TempDir()
	listed := Resolve(declared("a", gtestOptions("build/a", map[string]any{
		"build": []any{"cmake", "--build", "build"},
	})), root)
	written := Resolve(declared("b", gtestOptions("build/b", map[string]any{
		"build": "cmake --build build",
	})), root)

	for _, resolved := range []ResolvedSuite{listed, written} {
		// fetch, build, run, report.
		if len(resolved.Steps) != 4 {
			t.Fatalf("expected fetch, build, run and report, got %+v", resolved.Steps)
		}
		if strings.Join(resolved.Steps[1].Command, " ") != "cmake --build build" {
			t.Errorf("expected the build command, got %v", resolved.Steps[1].Command)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The whole protocol                                                         */
/* -------------------------------------------------------------------------- */

// The plugin answers over the envelope, in the version it was written against, and
// answers nothing when asked in another. A run sends every plugin every hook, so
// the answer to a version it cannot read has to be "nothing" rather than a refusal
// that would be read as an error.
func TestThePluginAnswersInItsOwnProtocolVersion(t *testing.T) {
	var out, errOut strings.Builder
	code := Main(nil, strings.NewReader(`{"version":1,"hook":"discover","component":"rho-test","payload":{}}`), &out, &errOut)
	if code != 0 {
		t.Fatalf("expected a clean answer, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"version":1`) {
		t.Errorf("expected the protocol version in the answer, got %s", out.String())
	}
}

func TestThePluginContributesNothingToAnotherVersion(t *testing.T) {
	var out, errOut strings.Builder
	code := Main(nil, strings.NewReader(`{"version":99,"hook":"discover","component":"rho-test","payload":{}}`), &out, &errOut)
	if code != 0 {
		t.Fatalf("expected a clean answer rather than a failure, got %d", code)
	}
	if !strings.Contains(out.String(), `"suites":null`) && !strings.Contains(out.String(), `"suites":[]`) {
		t.Errorf("expected no suites, got %s", out.String())
	}
	if !strings.Contains(errOut.String(), "protocol") {
		t.Errorf("expected the mismatch to be said on stderr, got %q", errOut.String())
	}
}

func TestThePluginIgnoresAnotherComponentsRequest(t *testing.T) {
	var out, errOut strings.Builder
	Main(nil, strings.NewReader(`{"version":1,"hook":"discover","component":"rho-doc","payload":{}}`), &out, &errOut)
	if strings.Contains(out.String(), "steps") {
		t.Errorf("expected nothing for another component, got %s", out.String())
	}
}

// The report step is this program again, asked what a report says. A report that is
// not there is a failure, because a binary that produced no report did not pass.
func TestTheReportStepFailsWhenThereIsNoReport(t *testing.T) {
	var out, errOut strings.Builder
	code := Main([]string{"rho-test-gtest", "--report", filepath.Join(t.TempDir(), "absent.xml")}, nil, &out, &errOut)
	if code == 0 {
		t.Fatal("expected a missing report to fail the step")
	}
	if out.String() != "" {
		t.Errorf("expected nothing on standard output, got %q", out.String())
	}
}

func TestTheReportStepFailsOnAReportWithNoTests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xml")
	if err := os.WriteFile(path, []byte(`<testsuites tests="0"><testsuite name="a" tests="0"/></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if code := Main([]string{"rho-test-gtest", "--report", path}, nil, &out, &errOut); code == 0 {
		t.Error("expected a suite with no tests to fail: it tested nothing")
	}
	if !strings.Contains(errOut.String(), "no tests") {
		t.Errorf("expected the reason to say so, got %q", errOut.String())
	}
}

func TestTheReportStepPrintsWhatTheReportSays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.xml")
	if err := os.WriteFile(path, []byte(googleTestReport), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if code := Main([]string{"rho-test-gtest", "--report", path}, nil, &out, &errOut); code != 0 {
		t.Fatalf("expected a clean report, got %d: %s", code, errOut.String())
	}
	for _, want := range []string{"[PASS] arithmetic.adds", "[FAIL] arithmetic.fails", "[SKIP] arithmetic.notyet"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in:\n%s", want, out.String())
		}
	}
}
