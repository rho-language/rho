package ctest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBuildsHostTestsAndFailsOnNone(t *testing.T) {
	root := t.TempDir()
	got := Resolve(DeclaredSuite{Name: "unit", Kind: "ctest", Options: map[string]any{"ctest": map[string]any{}}}, root)
	if got.Skip != "" {
		t.Fatal(got.Skip)
	}
	if len(got.Steps) != 4 {
		t.Fatalf("steps = %d, want configure, build, run, report", len(got.Steps))
	}
	configure := strings.Join(got.Steps[0].Command, " ")
	for _, want := range []string{"cmake -S " + root, "-DBUILD_TESTING=ON", "-DRHO_BUILD_HOST_TESTS=ON", "-DCMAKE_BUILD_TYPE=Debug"} {
		if !strings.Contains(configure, want) {
			t.Errorf("configure %q is missing %q", configure, want)
		}
	}
	run := strings.Join(got.Steps[2].Command, " ")
	for _, want := range []string{"ctest --test-dir", "--no-tests=error", "--output-junit"} {
		if !strings.Contains(run, want) {
			t.Errorf("run %q is missing %q", run, want)
		}
	}
	if got.Steps[3].Produces == "" {
		t.Error("the report step must say what it produces")
	}
}

func TestResolveHonoursTheSuitesOwnSettings(t *testing.T) {
	root := t.TempDir()
	got := Resolve(DeclaredSuite{Name: "u", Kind: "ctest", Options: map[string]any{"ctest": map[string]any{
		"source": "lib", "host_tests": false, "defines": []string{"FOO=1", "-DBAR"}, "build_type": "Release",
	}}}, root)
	configure := strings.Join(got.Steps[0].Command, " ")
	if strings.Contains(configure, "RHO_BUILD_HOST_TESTS") {
		t.Error("host_tests: false must leave the flag out")
	}
	for _, want := range []string{"-S " + filepath.Join(root, "lib"), "-DFOO=1", "-DBAR", "Release"} {
		if !strings.Contains(configure, want) {
			t.Errorf("configure %q is missing %q", configure, want)
		}
	}
}

func TestResolveRefusesPathsOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"../elsewhere", "/etc"} {
		got := Resolve(DeclaredSuite{Name: "u", Kind: "ctest", Options: map[string]any{"ctest": map[string]any{"build_dir": bad}}}, root)
		if got.Skip == "" || len(got.Steps) != 0 {
			t.Errorf("build_dir %q was accepted", bad)
		}
	}
}

func TestResolveAllAnswersOnlyForItsKind(t *testing.T) {
	got := ResolveAll(t.TempDir(), []DeclaredSuite{{Name: "a", Kind: "ctest"}, {Name: "b", Kind: "gtest"}})
	if len(got) != 1 || got[0].Declared != "a" {
		t.Fatalf("answered for %v", got)
	}
}

const junit = `<?xml version="1.0"?>
<testsuite name="(empty)" tests="3" failures="1" disabled="0" skipped="1">
  <testcase name="test_error" classname="test_error" status="run"/>
  <testcase name="test_registry" classname="test_registry" status="run">
    <failure message="exit code 1">assertion failed
line two</failure>
  </testcase>
  <testcase name="test_net" classname="test_net" status="notrun"><skipped message="needs a socket"/></testcase>
</testsuite>`

func TestParseReadsPassFailAndSkip(t *testing.T) {
	outcomes, err := Parse(strings.NewReader(junit))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Join(Lines(outcomes), "\n")
	for _, want := range []string{"[PASS] test_error", "[FAIL] test_registry", "  assertion failed", "[SKIP] test_net  (needs a socket)"} {
		if !strings.Contains(lines, want) {
			t.Errorf("report lines are missing %q:\n%s", want, lines)
		}
	}
}

func TestAnEmptyReportIsNotAPass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.xml")
	os.WriteFile(path, []byte(`<testsuite name="x" tests="0"></testsuite>`), 0o644)
	var out, errOut bytes.Buffer
	if code := Main([]string{"rho-test-ctest", "--report", path}, nil, &out, &errOut); code == 0 {
		t.Fatalf("an empty report passed: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "nothing was tested") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestDiscoverAnswersTheRunnersRequest(t *testing.T) {
	request, _ := json.Marshal(Envelope{Version: ProtocolVersion, Hook: HookDiscover, Component: ComponentName,
		Payload: mustJSON(DiscoverRequest{Root: t.TempDir(), Suites: []DeclaredSuite{{Name: "unit", Kind: "ctest"}}})})
	var out, errOut bytes.Buffer
	if code := Main([]string{"rho-test-ctest"}, bytes.NewReader(request), &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"declared":"unit"`) {
		t.Errorf("answer = %s", out.String())
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
