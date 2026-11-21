//
// @file toolchain_missing_test.go
// @brief A board whose compiler is not installed is a build that was skipped.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const absentCompiler = "rho-test-absent-elf-gcc"

// missingToolchainTree writes a board whose compiler is on no machine, and a
// project that declares it in its system_deps.
func missingToolchainTree(t *testing.T) (root, project string) {
	t.Helper()
	root = t.TempDir()
	specs := map[string]string{
		"targets/arch.json": `{"kind": "arch", "name": "absent-arch",
		  "toolchain": {"compiler": "` + absentCompiler + `"}}`,
		"targets/board.json": `{"kind": "board", "name": "absent-board", "arch": "absent-arch",
		  "memory": {"flash": {"origin": "0x0", "length": "0x1000"},
		             "ram": {"origin": "0x20000000", "length": "0x1000"}}}`,
		"project/module.yml": `module:
  name: absent-platform
  version: 0.1.0
  type: library
system_deps:
  executables:
    - name: ` + absentCompiler + `
      install:
        apt: gcc-absent-elf
        brew: absent-elf-gcc
`,
	}
	for name, content := range specs {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, root)
	return root, filepath.Join(root, "project")
}

// The compiler is not installed: nothing about the project is wrong, so the build
// exits 77 — skipped — and writes nothing into the project.
func TestABuildWhoseCompilerIsMissingIsSkipped(t *testing.T) {
	_, project := missingToolchainTree(t)
	t.Setenv("PATH", t.TempDir())

	code := run([]string{"build", "--target", "absent-board", "--project", project})
	if code != ExitSkipped {
		t.Fatalf("expected a missing compiler to exit %d, got %d", ExitSkipped, code)
	}
	if _, err := os.Stat(filepath.Join(project, "cmake")); err == nil {
		t.Errorf("a skipped build wrote the project's cmake/ directory")
	}
}

// Any other failure keeps its own code: an unknown target is not a skip.
func TestABuildForAnUnknownTargetIsNotSkipped(t *testing.T) {
	_, project := missingToolchainTree(t)
	if code := run([]string{"build", "--target", "no-such-board", "--project", project}); code == 0 || code == ExitSkipped {
		t.Fatalf("expected an unknown target to fail, not exit %d", code)
	}
}

// The message says how to get the compiler, from the system_deps that declare it.
func TestTheInstallHintComesFromSystemDeps(t *testing.T) {
	_, project := missingToolchainTree(t)
	hint := installHint(absentCompiler, project)
	for _, want := range []string{"rho keeper install absent-platform " + absentCompiler, "apt install gcc-absent-elf", "brew install absent-elf-gcc"} {
		if !strings.Contains(hint, want) {
			t.Errorf("expected %q in the hint, got %q", want, hint)
		}
	}
	if other := installHint("some-other-gcc", project); !strings.Contains(other, "rho doctor") {
		t.Errorf("a compiler nothing declares should point at rho doctor, got %q", other)
	}
}
