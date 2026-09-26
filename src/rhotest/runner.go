//
// @file runner.go
// @brief Executes Rho test files - Enterprise v1.
//
// Workflow:
//   1. rho build file.rho
//   2. gcc file.c -o file
//   3. ./file
//
// Notes:
//   - Minimal execution pipeline
//   - No parallelism yet
//   - No sandboxing
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  RunTestFile                                                                */
/* -------------------------------------------------------------------------- */

// DefaultRhoBinary is used when a caller does not say which rho to build
// with. The environment still wins, so an explicit binary can be pointed at a
// build in progress.
const DefaultRhoBinary = "rho"

func RunTestFile(tf TestFile) ([]TestResult, error) {
	return RunTestFileForTarget(tf, DefaultTarget, "")
}

// DefaultTarget is the backend a test runs on unless another is requested.
const DefaultTarget = "c"

// RunTestFileForTarget runs one test file on a chosen backend. A test is an
// ordinary program, so "c" compiles it natively while a Go target generates a
// Go project and runs the binary the Go toolchain produces.
//
// A server target cannot host a test as a process, so a project whose manifest
// asks only for one should be tested on a plain Go target instead.
func RunTestFileForTarget(tf TestFile, target, rhoBinary string) ([]TestResult, error) {
	if target == "" {
		target = DefaultTarget
	}
	if target != "c" {
		return runGoTestFile(tf, target, rhoBinary)
	}
	return runCTestFile(tf, rhoBinary)
}

// resolveRhoBinary picks the rho to build tests with: the one the caller
// named, then RHO_BIN, then whatever is on PATH.
func resolveRhoBinary(named string) string {
	if named != "" {
		return named
	}
	if fromEnv := os.Getenv("RHO_BIN"); fromEnv != "" {
		return fromEnv
	}
	return DefaultRhoBinary
}

func runCTestFile(tf TestFile, rhoBinary string) ([]TestResult, error) {
	source, err := os.ReadFile(tf.Path)
	if err != nil {
		return nil, fmt.Errorf("read test file: %w", err)
	}
	compiledSource := strings.ReplaceAll(string(source), "test fn ", "fn ")
	buildDir := testBuildDir(tf.Path)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, err
	}
	tempPath := filepath.Join(filepath.Dir(tf.Path), fmt.Sprintf(".rho-test-%d.rho", time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, []byte(compiledSource), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tempPath)
	base := strings.TrimSuffix(tempPath, ".rho")
	name := filepath.Base(base)
	cfile := filepath.Join(buildDir, name+".c")
	exe := filepath.Join(buildDir, name)
	defer os.Remove(cfile)
	defer os.Remove(exe)

	/* --------------------------- Step 1: rho build -------------------------- */
	rhoCommand := os.Getenv("RHO_BIN")
	if rhoCommand == "" {
		rhoCommand = "rho"
	}
	// A test is an ordinary program, so it builds the C target alone: the
	// project's own server or extra targets are not what the test runs.
	cmdBuild := exec.Command(rhoCommand, "build", "-target", DefaultTarget, "-o", cfile, tempPath)
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("rho build: %w: %s", err, out)
	}

	/* --------------------------- Step 2: gcc compile ------------------------ */
	cmdGcc := exec.Command("gcc", cfile, "-o", exe)
	if out, err := cmdGcc.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("gcc: %w: %s", err, out)
	}

	/* --------------------------- Step 3: run executable --------------------- */
	cmdRun := exec.Command(exe)
	out, err := cmdRun.CombinedOutput()

	passed := (err == nil)

	/* --------------------------- Build results ------------------------------ */
	var results []TestResult
	for _, tc := range tf.Tests {
		results = append(results, TestResult{
			Case:   tc,
			Passed: passed,
			Output: string(out) + func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
		})
	}

	return results, nil
}

func testBuildDir(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Dir(path)
	}
	current := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(current, "module.yml")); err == nil {
			return filepath.Join(current, "build")
		}
		if _, err := os.Stat(filepath.Join(current, "rho.mod")); err == nil {
			return filepath.Join(current, "build")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Join(filepath.Dir(abs), ".rho-test-build")
		}
		current = parent
	}
}

// runGoTestFile runs a test on a Go target: rho generates the Go project, the
// Go toolchain builds it, and the resulting binary runs the test functions.
func runGoTestFile(tf TestFile, target, rhoBinary string) ([]TestResult, error) {
	source, err := os.ReadFile(tf.Path)
	if err != nil {
		return nil, fmt.Errorf("read test file: %w", err)
	}
	compiledSource := strings.ReplaceAll(string(source), "test fn ", "fn ")

	projectRoot := projectRootFor(tf.Path)
	if err := os.MkdirAll(filepath.Join(projectRoot, "build"), 0755); err != nil {
		return nil, err
	}
	tempPath := filepath.Join(filepath.Dir(tf.Path), fmt.Sprintf(".rho-test-%d.rho", time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, []byte(compiledSource), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tempPath)

	build := exec.Command(resolveRhoBinary(rhoBinary), "build", "-target", target, tempPath)
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("rho build: %w: %s", err, out)
	}

	// The generated project lives beside the manifest, under the descriptor's
	// output root.
	generatedDir := filepath.Join(projectRoot, "build", "generated", target)
	if _, err := os.Stat(generatedDir); err != nil {
		return nil, fmt.Errorf("generated project %s is missing: %w", generatedDir, err)
	}

	exe := filepath.Join(generatedDir, filepath.Base(strings.TrimSuffix(tempPath, ".rho")))
	cmdGo := exec.Command("go", "build", "-o", exe, ".")
	cmdGo.Dir = generatedDir
	if out, err := cmdGo.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build: %w: %s", err, out)
	}

	run := exec.Command(exe)
	run.Dir = generatedDir
	out, runErr := run.CombinedOutput()
	passed := runErr == nil

	results := make([]TestResult, 0, len(tf.Tests))
	for _, tc := range tf.Tests {
		output := string(out)
		if runErr != nil {
			output += runErr.Error()
		}
		results = append(results, TestResult{Case: tc, Passed: passed, Output: output})
	}
	return results, nil
}

// projectRootFor walks up from a test file to the project that owns it.
func projectRootFor(path string) string {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		dir = filepath.Dir(path)
	}
	for {
		for _, name := range []string{"module.yml", "rho.mod"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

/* -------------------------------------------------------------------------- */
/*  Suite                                                                     */
/* -------------------------------------------------------------------------- */

// Options configure a test run.
type Options struct {
	// Path is a test file or a directory searched for them.
	Path string
	// Target is the backend the tests run on: "c" or a Go target id.
	Target string
	// RhoBinary is the rho to build with. Empty falls back to RHO_BIN and
	// then to whatever is on PATH.
	RhoBinary string
}

// Run discovers, runs and reports a test suite, returning a process exit code:
// 0 when every test passed.
func Run(options Options) int {
	if options.Target == "" {
		options.Target = DefaultTarget
	}
	files, err := Discover(options.Path)
	if err != nil {
		fmt.Println("Error:", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Println("No test files found.")
		return 0
	}

	all := []TestResult{}
	for _, file := range files {
		loaded, loadErr := LoadTests(file)
		if loadErr != nil {
			fmt.Printf("Error loading %s: %v\n", file, loadErr)
			return 1
		}
		results, runErr := RunTestFileForTarget(loaded, options.Target, options.RhoBinary)
		if runErr != nil {
			fmt.Printf("Error running %s: %v\n", file, runErr)
			return 1
		}
		all = append(all, results...)
	}

	Report(all)
	for _, result := range all {
		if !result.Passed {
			return 1
		}
	}
	return 0
}
