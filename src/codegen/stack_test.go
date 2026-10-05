//
// @file stack_test.go
// @brief How much stack a program needs, before anybody runs it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"
)

// A call chain's worst case is the sum of the frames along it, which is the whole
// of what a stack bound is.
func TestTheStackIsTheLongestPathNotTheSum(t *testing.T) {
	emitted := `
int leaf(int a) { char buffer[100]; return a + buffer[0]; }
int middle(int a) { return leaf(a) + 1; }
int main(void) { return middle(1); }
`
	analysis := ComputeStack(emitted, "main", StackBinding{})
	// main's own frame, plus middle's, plus leaf's hundred bytes.
	if analysis.Bytes < 100 {
		t.Fatalf("expected at least the leaf's 100 bytes, got %d (%s)", analysis.Bytes, analysis.Describe())
	}
	if analysis.Bytes > 200 {
		t.Errorf("expected the longest path, not a sum of every function: %d", analysis.Bytes)
	}
}

// Two branches, one deeper. The bound is the deeper one, which is what a stack
// has to hold.
func TestTheDeepestBranchIsTheBound(t *testing.T) {
	emitted := `
void shallow(void) { }
void deep_a(void) { char a[200]; (void)a; }
void deep_b(void) { char b[400]; (void)b; }
void branch(int n) { if (n) deep_a(); else deep_b(); }
int main(void) { branch(1); return 0; }
`
	analysis := ComputeStack(emitted, "main", StackBinding{})
	if analysis.Bytes < 400 {
		t.Errorf("expected the deeper branch's 400 bytes, got %d", analysis.Bytes)
	}
}

// A call into code the analysis has not read is charged a declared frame, because
// a bound that omitted the deepest thing in the program would be a fiction.
func TestAnUnknownCallIsChargedAndReported(t *testing.T) {
	emitted := `
int main(void) { return vendor_library_call(1, 2); }
`
	analysis := ComputeStack(emitted, "main", StackBinding{Bytes: 128})
	if analysis.Bytes < 128 {
		t.Errorf("expected the declared 128-byte frame, got %d", analysis.Bytes)
	}
	if len(analysis.Unknown) != 1 || analysis.Unknown[0] != "vendor_library_call" {
		t.Errorf("expected the unknown call reported, got %v", analysis.Unknown)
	}
}

// Recursion has no worst case, and a program containing one must be told that
// rather than given a number that looks computed.
func TestRecursionIsReportedRatherThanFollowed(t *testing.T) {
	emitted := `
int down(int n) { if (n <= 0) return 0; return down(n - 1) + 1; }
int main(void) { return down(10); }
`
	analysis := ComputeStack(emitted, "main", StackBinding{})
	if len(analysis.Recursive) != 1 || analysis.Recursive[0] != "down" {
		t.Fatalf("expected the recursion reported, got %v", analysis.Recursive)
	}
	if !strings.Contains(analysis.Describe(), "not a bound") {
		t.Errorf("expected the report to say this is not a bound: %s", analysis.Describe())
	}
}

// The program Rho emits for a firmware like the hello-world: one function, a
// runtime call, an ffi shim. The bound has to come out small, because that is the
// whole point of computing it instead of reserving a kilobyte.
func TestAFirmwareSizedProgramGetsASmallBound(t *testing.T) {
	emitted := `
char* rho_string_from_i32(int32_t value) { char buffer[12]; return buffer; }
void rho_print(const char* s) { (void)s; }
void app_main(void) {
    rho_print("[");
    rho_print(rho_string_from_i32(1));
    rho_print(" s] hello, in Rho\n");
}
`
	analysis := ComputeStack(emitted, "app_main", StackBinding{})
	if analysis.Bytes == 0 || analysis.Bytes > 256 {
		t.Errorf("expected a small bound, got %d (%s)", analysis.Bytes, analysis.Describe())
	}
	// And the deepest path is the one that reaches the twelve-byte buffer.
	joined := strings.Join(analysis.Path, "→")
	if !strings.Contains(joined, "app_main") || !strings.Contains(joined, "rho_string_from_i32") {
		t.Errorf("expected the path through the buffer's function, got %s", joined)
	}
}

// A prototype and a definition share a shape apart from the brace, and counting the
// prototype would give half the functions an empty frame.
func TestAPrototypeIsNotAFunction(t *testing.T) {
	emitted := `
int helper(int a);
int helper(int a) { char pad[40]; return a + pad[0]; }
int main(void) { return helper(1); }
`
	analysis := ComputeStack(emitted, "main", StackBinding{})
	if analysis.Bytes < 40 {
		t.Errorf("expected the definition's frame counted once, got %d", analysis.Bytes)
	}
}
