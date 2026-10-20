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
