//
// @file suites.go
// @brief Turning a declared ctest suite into steps that run it.
//
// A C library's tests are C, registered with CMake and run with CTest. A project
// declares that once, in its suite manifest, and this turns it into the steps a
// run takes — configure, build, run, report — so the one entry point is
// `rho test`, whatever the host tests are built with underneath.
//
//	configure   cmake, asked for the host tests a build normally leaves out
//	build       cmake --build
//	run         ctest, told to write a JUnit report and to fail if it found none
//	report      this plugin, reading that report and saying what it says
//
// "No tests were found" is a pass as far as CTest's exit status goes, and a suite
// that verified nothing while reporting success is read as coverage. So the run
// step is told `--no-tests=error`, and the report step refuses an empty report.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package ctest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// settings is a suite's `ctest:` mapping as this plugin reads it. Every key is
// optional: a project whose CMakeLists is at its root and whose tests are guarded
// by RHO_BUILD_HOST_TESTS declares `ctest: {}` and nothing else.
type settings struct {
	// Source is the directory holding the CMakeLists.txt, relative to the project
	// root. The root itself when left out.
	Source string `json:"source"`
	// BuildDir is where the tests are built, relative to the project root. It is
	// an output, so it goes under the project's build directory.
	BuildDir string `json:"build_dir"`
	// Defines are extra configure definitions, written NAME or NAME=VALUE.
	Defines []string `json:"defines"`
	// HostTests, false, leaves out the -DRHO_BUILD_HOST_TESTS=ON the host tests of
	// a Rho platform are guarded by.
	HostTests *bool `json:"host_tests"`
	// BuildType is the CMake build type; Debug when left out.
	BuildType string `json:"build_type"`
}

// Resolve turns a declared ctest suite into the steps that run it.
func Resolve(declared DeclaredSuite, root string) ResolvedSuite {
	resolved := ResolvedSuite{Declared: declared.Name, Name: declared.Name}

	var given settings
	if raw, ok := declared.Options["ctest"]; ok && raw != nil {
		if err := decodeSettings(raw, &given); err != nil {
			resolved.Skip = fmt.Sprintf("%s: %v", declared.Name, err)
			return resolved
		}
	}
	source, err := within(root, given.Source, ".")
	if err != nil {
		resolved.Skip = fmt.Sprintf("%s: ctest.source %v", declared.Name, err)
		return resolved
	}
	buildDir, err := within(root, given.BuildDir, filepath.Join("build", "tests", declared.Name))
	if err != nil {
		resolved.Skip = fmt.Sprintf("%s: ctest.build_dir %v", declared.Name, err)
		return resolved
	}
	buildType := given.BuildType
	if buildType == "" {
		buildType = "Debug"
	}

	configure := []string{
		"cmake", "-S", source, "-B", buildDir,
		"-DCMAKE_BUILD_TYPE=" + buildType, "-DBUILD_TESTING=ON",
	}
	if given.HostTests == nil || *given.HostTests {
		configure = append(configure, "-DRHO_BUILD_HOST_TESTS=ON")
	}
	for _, define := range given.Defines {
		if define != "" {
			configure = append(configure, "-D"+strings.TrimPrefix(define, "-D"))
		}
	}
	report := filepath.Join(buildDir, "ctest-report.xml")

	resolved.Steps = []Step{
		{Name: "configure " + declared.Name, Dir: root, Command: configure},
		{Name: "build " + declared.Name, Dir: root, Command: []string{"cmake", "--build", buildDir, "--parallel"}},
		{Name: "run " + declared.Name, Dir: root, Command: []string{
			"ctest", "--test-dir", buildDir, "--output-on-failure", "--no-tests=error",
			"--output-junit", report,
		}},
		{Name: "read the ctest report", Dir: root, Command: []string{selfPath(), "--report", report}, Produces: report},
	}
	return resolved
}

// within resolves a path the manifest gave against the project root and refuses
// one that leaves it: a suite's build tree is the project's, and a manifest that
// named somewhere else would be a manifest that wrote there.
func within(root, given, fallback string) (string, error) {
	if given == "" {
		given = fallback
	}
	if filepath.IsAbs(given) {
		return "", fmt.Errorf("%q must be inside the project, not an absolute path", given)
	}
	joined := filepath.Join(root, filepath.FromSlash(given))
	relative, err := filepath.Rel(root, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q reaches outside the project", given)
	}
	return joined, nil
}

// selfPath is this plugin's own executable, so the report step can call back into
// it, resolved from the process rather than written into a manifest.
func selfPath() string {
	if self, err := executablePath(); err == nil {
		return self
	}
	return "rho-test-ctest"
}

// ResolveAll turns every declared suite of a kind this plugin knows into steps,
// and leaves every other kind alone.
func ResolveAll(root string, suites []DeclaredSuite) []ResolvedSuite {
	out := []ResolvedSuite{}
	for _, declared := range suites {
		if strings.EqualFold(declared.Kind, KindCTest) {
			out = append(out, Resolve(declared, root))
		}
	}
	return out
}

func decodeSettings(raw any, into *settings) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("the `ctest:` settings are not readable: %w", err)
	}
	if err := json.Unmarshal(encoded, into); err != nil {
		return fmt.Errorf("the `ctest:` settings do not fit this plugin: %w", err)
	}
	return nil
}
