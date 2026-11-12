/*
 * csrc_test.go — the C runtime's own rules.
 *
 * The runtime is a set of C files emitted into a build, and what they say to
 * each other has to hold for a program that calls them. These are the rules
 * that are easy to break and hard to see: a linkage that is right in the file
 * that defines a symbol and wrong in the file that calls it is a program that
 * compiles and does not link.
 *
 * @project Rho Programming Language
 * @license Apache-2.0
 */

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A fragment's symbols are called from the program's own translation unit, and
// the fragment itself is emitted into the runtime's. A `static` prototype in
// the program declares a symbol that unit does not define: the call is
// compiled as an internal one, and the link fails on a name that was declared
// right there.
func TestRuntimeDeclarationsAreNeverStatic(t *testing.T) {
	for _, fragment := range runtimeFragments {
		for _, decl := range fragment.Decls {
			if strings.HasPrefix(strings.TrimSpace(decl), "static ") {
				t.Errorf("%s: %q is static, and the program that calls it is a different translation unit",
					fragment.Name, decl)
			}
		}
	}
}

// The other half of the same rule: a symbol the program calls is defined
// without internal linkage, or the program's call names a symbol that exists
// nowhere.
func TestTheRuntimeDefinesWhatItDeclaresToTheProgram(t *testing.T) {
	for _, fragment := range runtimeFragments {
		for _, symbol := range fragment.Provides {
			if !fragmentDeclares(fragment, symbol) {
				continue
			}
			if definesStatically(fragment.Source, symbol) {
				t.Errorf("%s: %s is declared to the program and defined statically, so the program's call names a symbol that does not exist",
					fragment.Name, symbol)
			}
		}
	}
}

// fragmentDeclares reports whether a fragment declares a symbol to the program.
func fragmentDeclares(fragment RuntimeFragment, symbol string) bool {
	for _, decl := range fragment.Decls {
		if strings.Contains(decl, symbol+"(") {
			return true
		}
	}
	return false
}

// definesStatically reports whether a C source defines a symbol with internal
// linkage.
func definesStatically(source, symbol string) bool {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "static ") {
			continue
		}
		if strings.Contains(line, symbol+"(") {
			return true
		}
	}
	return false
}

// A platform whose console is its own says which function writes to it, and a
// platform whose part the runtime has never heard of says how its program
// stops. Deciding both by architecture is how a firmware on such a part links
// against a C library's stdout and stops in an `exit` it has no use for.
func TestTheRuntimeLetsThePlatformOwnTheConsoleAndTheStop(t *testing.T) {
	if !strings.Contains(printSource, "RHO_PROGRAM_WRITE") {
		t.Error("expected the runtime's console to be the platform's to name")
	}
	if !strings.Contains(panicSource, "RHO_PROGRAM_HALT") {
		t.Error("expected the runtime's stop to be the platform's to name")
	}
}

// Each number written as text is a string of its own: two in one expression
// are both read after both were made, and a shared buffer made the first read
// as the second. A negative number's sign goes in front.
func TestTheRuntimeWritesEachNumberIntoItsOwnString(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler on the path")
	}
	source := ""
	for _, fragment := range runtimeFragments {
		if fragment.Name == "strings" {
			source = fragment.Source
		}
	}
	if source == "" {
		t.Fatal("the runtime has no strings fragment")
	}
	dir := t.TempDir()
	program := "#include <stddef.h>\n#include <stdint.h>\n#include <stdbool.h>\n#include <string.h>\n#include <stdlib.h>\n#include <stdio.h>\n" +
		source + `
int main(void) {
    printf("%s|%s|%s|%s", rho_concat(rho_int_to_str(7), rho_int_to_str(4)), rho_string_from_i32(-5),
        rho_string_from_i32(-2147483647 - 1), rho_int_to_str(4294967295u));
    return 0;
}
`
	path := filepath.Join(dir, "numbers.c")
	if err := os.WriteFile(path, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "numbers")
	if out, err := exec.Command(compiler, "-o", binary, path).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "74|-5|-2147483648|4294967295"; string(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
