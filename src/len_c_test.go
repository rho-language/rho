//
// @file len_c_test.go
// @brief The built-in len on a bare pointer is refused on every target.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lenDynamicSource = `module app

fn main() void {
    let bytes: [u8] = [1, 2, 3]
    print(int_to_str(len(bytes)))
}
`

const lenSliceSource = `module app

fn main() void {
    let bytes: Slice[u8] = read_file("input.txt")
    print(int_to_str(len(bytes)))
    write_file("copy.txt", bytes)
}
`

// writeLenProject writes a one-file project building for the given targets.
func writeLenProject(t *testing.T, source string, targets ...string) string {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: lenc\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n"
	for _, target := range targets {
		manifest += "    - " + target + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

// A `[T]` is a bare pointer, so len on one is refused on every target: the
// build stops and names the call, and a program means the same on each.
func TestLenOfDynamicArrayIsRefusedOnEveryTarget(t *testing.T) {
	for _, target := range []string{"c", "go"} {
		entry := writeLenProject(t, lenDynamicSource, target)
		_, err := GenerateAllTargets(entry, "", nil, nil, nil)
		if err == nil {
			t.Fatalf("expected the %s build to be refused", target)
		}
		for _, want := range []string{"len(bytes)", ":5:", "[u8]", "fixed-size array", "Slice[T]"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the %s refusal should mention %q, got:\n%v", target, want, err)
			}
		}
	}
}

// read_file gives a Slice[u8] and write_file takes one, which has a length.
func TestFileBuiltinsSpeakSliceOnGo(t *testing.T) {
	entry := writeLenProject(t, lenSliceSource, "go")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err != nil {
		t.Fatalf("a Go build of the file built-ins should not be refused: %v", err)
	}
}
