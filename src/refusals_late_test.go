//
// @file refusals_late_test.go
// @brief What used to pass the checker and fail in the C compiler is refused
// where it is written.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"strings"
	"testing"
)

func TestWhatCCannotCompileIsRefusedByTheChecker(t *testing.T) {
	for _, c := range []struct {
		name, source string
		want         []string
	}{
		{"a function named like a C type word", `module app

fn double(n: u32) u32 {
    return n * 2
}

fn main() void {
    print(int_to_str(double(2)))
}
`, []string{":3:1:", `function "double"`, "C type name"}},
		{"comparing two structs", `module app

struct P {
    x: u32
}

fn main() void {
    var a: P
    var b: P
    a.x = 1
    if a == b {
        print("same")
    }
}
`, []string{":11:8:", "structs cannot be compared with ==", "compare the fields"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, target := range []string{"c", "go"} {
				err := buildOneFile(t, c.source, target)
				if err == nil {
					t.Fatalf("expected the %s build to be refused", target)
				}
				for _, want := range c.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("%s: the refusal should mention %q, got:\n%v", target, want, err)
					}
				}
			}
		})
	}
}
