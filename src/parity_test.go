//
// @file parity_test.go
// @brief The C and Go backends are one language: same program, same answer.
//
// The compiler emits C and Go from one tree, and nothing but a test says they mean
// the same thing. A difference between them is the worst kind of bug a compiler
// has — the program is accepted, runs, and does something other than what the other
// backend's build of it does — so this runs a program through both and compares
// what they print.
//
// The C side is built with AddressSanitizer and UndefinedBehaviorSanitizer. That is
// the point of building it here rather than only comparing output: a program that
// reads past an array or overflows a signed integer can print the same thing as the
// Go build on one machine and be undefined behaviour on every machine, and only a
// sanitizer says so.
//
// Programs whose backends are known to differ are listed in `knownDivergences` with
// what differs, rather than left out: a divergence that is written down is one that
// can be fixed and then removed from the list, and the list failing to shrink is
// visible.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const parityManifest = `module:
  name: parity
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - c
    - go
`

// sanitizersWork reports whether this machine's C compiler can build with ASan and
// UBSan, which not every one can.
func sanitizersWork(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "a.c")
	if err := os.WriteFile(source, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "a")
	if err := exec.Command("gcc", "-fsanitize=address,undefined", "-o", out, source).Run(); err != nil {
		return false
	}
	return exec.Command(out).Run() == nil
}

// outcome is what one backend did with a program.
type outcome struct {
	stdout string
	// failed is the text of a failure: a non-zero exit, a sanitizer report, a build
	// error. Empty when the program ran cleanly.
	failed string
}

// runBothBackends builds a Rho program for C and for Go and runs each.
func runBothBackends(t *testing.T, source string) (c outcome, g outcome) {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	cDir := dirs[0]
	cBinary := filepath.Join(t.TempDir(), "parity-c")
	build := exec.Command("gcc", "-std=c11", "-g", "-O0", "-fsanitize=address,undefined",
		"-fno-sanitize-recover=undefined", "-o", cBinary,
		filepath.Join(cDir, "main.c"), filepath.Join(cDir, "app.c"), filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := build.CombinedOutput(); err != nil {
		c.failed = "C build failed: " + string(out)
	} else {
		c = runBinary(cBinary)
	}

	goDir := dirs[1]
	goBinary := filepath.Join(t.TempDir(), "parity-go")
	goBuild := exec.Command("go", "build", "-o", goBinary, ".")
	goBuild.Dir = goDir
	if out, err := goBuild.CombinedOutput(); err != nil {
		g.failed = "Go build failed: " + string(out)
	} else {
		g = runBinary(goBinary)
	}
	return c, g
}

// runBinary runs a built program and records what it said and whether it was clean.
func runBinary(path string) outcome {
	command := exec.Command(path)
	// Leaks are not what this is about, and a short-lived program that exits
	// through a panic path would report them for reasons that are not the
	// program's.
	command.Env = append(os.Environ(), "ASAN_OPTIONS=detect_leaks=0")
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := outcome{stdout: stdout.String()}
	if err != nil {
		result.failed = err.Error() + "\n" + stderr.String()
	} else if strings.Contains(stderr.String(), "runtime error:") || strings.Contains(stderr.String(), "AddressSanitizer") {
		result.failed = stderr.String()
	}
	return result
}

// parityCase is one program, and what is known about it.
type parityCase struct {
	name   string
	source string
	// diverges, when set, says how the backends differ today. The case is then
	// expected to differ, and a case that stops differing fails — so the entry
	// is removed when the divergence is fixed.
	diverges string
}

func TestBackendsAgree(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	if !sanitizersWork(t) {
		t.Skip("this C compiler cannot build with the sanitizers")
	}
	for _, c := range parityCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cOut, gOut := runBothBackends(t, c.source)
			agree := cOut.failed == "" && gOut.failed == "" && cOut.stdout == gOut.stdout
			if c.diverges != "" {
				if agree {
					t.Fatalf("this case is listed as diverging (%s) but the backends now agree; "+
						"remove it from the list of known divergences", c.diverges)
				}
				t.Logf("known divergence (%s):\nC  = %q failed=%q\nGo = %q failed=%q",
					c.diverges, cOut.stdout, cOut.failed, gOut.stdout, gOut.failed)
				return
			}
			if cOut.failed != "" {
				t.Fatalf("C: %s\nstdout so far: %q", cOut.failed, cOut.stdout)
			}
			if gOut.failed != "" {
				t.Fatalf("Go: %s\nstdout so far: %q", gOut.failed, gOut.stdout)
			}
			if cOut.stdout != gOut.stdout {
				t.Fatalf("backends disagree:\nC  = %q\nGo = %q", cOut.stdout, gOut.stdout)
			}
		})
	}
}
