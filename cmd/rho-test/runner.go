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

package main

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

func RunTestFile(tf TestFile) ([]TestResult, error) {
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
	cmdBuild := exec.Command(rhoCommand, "build", "-o", cfile, tempPath)
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
