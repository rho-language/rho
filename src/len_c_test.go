//
// @file len_c_test.go
// @brief The built-in len on a dynamic array is refused on a C target.
//
// SPDX-License-Identifier: Apache-2.0
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
    let bytes: [u8] = read_file("input.txt")
    print(int_to_str(len(bytes)))
}
`

// writeLenProject writes a one-file project building for the given targets.
func writeLenProject(t *testing.T, targets ...string) string {
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
	if err := os.WriteFile(entry, []byte(lenDynamicSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

// A dynamic array is a bare pointer in C, so len on one cannot be emitted
// there: the build stops and names the call, instead of writing a `len` the C
// compiler has never heard of.
func TestLenOfDynamicArrayIsRefusedOnC(t *testing.T) {
	entry := writeLenProject(t, "c")
	_, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected the C build to be refused")
	}
	for _, want := range []string{"len(bytes)", "line 5", "[u8]", "fixed-size array", "Slice[T]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q, got:\n%v", want, err)
		}
	}
}

// The same program built only for Go is not refused: Go's slice has a length.
func TestLenOfDynamicArrayBuildsForGo(t *testing.T) {
	entry := writeLenProject(t, "go")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err != nil {
		t.Fatalf("a Go-only build should not be refused: %v", err)
	}
}
