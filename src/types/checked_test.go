package types

import (
	"strings"
	"testing"
)

// What a constant makes certain to go wrong is refused at compile time, on both
// backends alike: C would wrap or leave it undefined, and Go would refuse to build
// some of it and panic on the rest.
func TestConstantOperationsThatCannotBeRightAreRefused(t *testing.T) {
	for _, c := range []struct {
		body string
		want string
	}{
		{"print(int_to_str(u32(7 / -2)))", "constant -3 overflows u32"},
		{"let b: u8 = 300", "constant 300 overflows u8"},
		{"let b: i8 = -129", "constant -129 overflows i8"},
		{"var b: u8 = 1\n    b = 256", "constant 256 overflows u8"},
		{"var b: u8 = 1\n    print(int_to_str(u32(b + 300)))", "constant 300 overflows u8"},
		{"var x: u32 = 1\n    print(int_to_str(x << 32))", "shift count 32 overflows type u32"},
		{"var x: u8 = 1\n    print(int_to_str(u32(x >> 8)))", "shift count 8 overflows type u8"},
		{"var x: u32 = 1\n    print(int_to_str(x << -1))", "negative shift count -1"},
		{"var x: u32 = 1\n    print(int_to_str(x / 0))", "division by zero"},
		{"var x: u32 = 1\n    print(int_to_str(x % 0))", "division by zero"},
		{"var x: u32 = 1\n    x /= 0", "division by zero"},
		{"let v: u32[4] = [1, 2, 3, 4]\n    print(int_to_str(v[4]))", "index 4 out of range [0,4)"},
		{"print(int_to_str(u32(300 as u8)))", "constant 300 overflows u8"},
	} {
		source := "module app\n\nfn main() void {\n    " + c.body + "\n}\n"
		err := checkSource(t, source)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, got %v\n%s", c.want, err, source)
		}
	}
}

// The same operations with constants in range, or with values only known when
// the program runs, are accepted: those are checked as they run.
func TestOperationsInRangeOrAtRunTimeAreAccepted(t *testing.T) {
	for _, body := range []string{
		"print(int_to_str(u32(7 / 2)))",
		"let b: u8 = 255",
		"let b: i8 = -128",
		"var x: u32 = 1\n    print(int_to_str(x << 31))",
		"var x: u32 = 1\n    var n: u32 = 40\n    print(int_to_str(x << n))",
		"var x: u32 = 1\n    var z: u32 = 0\n    print(int_to_str(x / z))",
		"let v: u32[4] = [1, 2, 3, 4]\n    var i: u32 = 9\n    print(int_to_str(v[3] + v[i]))",
		"let big: u64 = 1 << 40\n    print(i64_to_str(i64(big)))",
	} {
		source := "module app\n\nfn main() void {\n    " + body + "\n}\n"
		if err := checkSource(t, source); err != nil {
			t.Errorf("refused: %v\n%s", err, source)
		}
	}
}
