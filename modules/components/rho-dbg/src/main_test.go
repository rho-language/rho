package main

import "testing"

// A flag is spelled both ways in the world — `--target` in documentation, `-target`
// on a command line somebody typed — and the help says the double one. Reading only
// one of them makes the documented form the one that does not work.
func TestAFlagIsReadSpelledEitherWay(t *testing.T) {
	for _, testCase := range []struct {
		args []string
		want string
	}{
		{[]string{"--target", "pico"}, "pico"},
		{[]string{"-target", "pico"}, "pico"},
		{[]string{"--target=pico"}, "pico"},
		{[]string{"-target=pico"}, "pico"},
		{[]string{"--other", "x", "--target", "pico"}, "pico"},
		{[]string{"target", "pico"}, ""},
		{[]string{"--targets", "pico"}, ""},
		{nil, ""},
	} {
		if got := namedArg(testCase.args, "-target"); got != testCase.want {
			t.Errorf("%v: expected %q, got %q", testCase.args, testCase.want, got)
		}
	}
}
