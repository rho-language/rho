//
// @file build_test.go
// @brief Tests for `rho module build` command.
// @project Rho Programming Language
// @version 0.1
// @date 2026
// SPDX-License-Identifier: MIT
//

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildModule(t *testing.T) {
	// Create a temporary directory for the test module
	tmpDir, err := os.MkdirTemp("", "rho-module-build-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module.yml
	modYaml := `module:
  name: test-build
  type: component
  version: 0.1.0
  command: test-cmd

main: test-build

features:
  - test
`
	if err := os.WriteFile(filepath.Join(tmpDir, "module.yml"), []byte(modYaml), 0644); err != nil {
		t.Fatalf("failed to write module.yml: %v", err)
	}

	// Create src/ directory with a simple Go program
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	mainGo := `package main

import "fmt"

func main() {
    fmt.Println("Hello from test-build")
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Create go.mod
	goMod := `module github.com/test/test-build

go 1.18
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Run build command - should build binary to current dir
	err = Build([]string{tmpDir})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// The binary is a build artefact, so it lands in the project's build
	// directory rather than beside the sources.
	binaryPath := filepath.Join(tmpDir, "build", "test-build")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("binary not built at %s", binaryPath)
	}

	// Run with -o flag to specify output directory
	outputDir := filepath.Join(tmpDir, "out")
	err = Build([]string{tmpDir, "-o", outputDir})
	if err != nil {
		t.Fatalf("Build with -o failed: %v", err)
	}

	binaryPath2 := filepath.Join(outputDir, "test-build")
	if _, err := os.Stat(binaryPath2); os.IsNotExist(err) {
		t.Fatalf("binary not built at %s", binaryPath2)
	}
}

func TestBuildModuleMissingGoMod(t *testing.T) {
	// Create a temporary directory without go.mod
	tmpDir, err := os.MkdirTemp("", "rho-module-build-test-no-gomod")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module.yml
	modYaml := `module:
  name: test-no-gomod
  type: component
  version: 0.1.0
  command: test-cmd

main: test-no-gomod
`
	if err := os.WriteFile(filepath.Join(tmpDir, "module.yml"), []byte(modYaml), 0644); err != nil {
		t.Fatalf("failed to write module.yml: %v", err)
	}

	// Create src/main.go
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	mainGo := `package main

func main() {}
`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Run build command - should fail because no go.mod
	err = Build([]string{tmpDir})
	if err == nil {
		t.Fatalf("Build should have failed without go.mod")
	}
}
