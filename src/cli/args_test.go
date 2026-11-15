package cli

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

func newSet() (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("target", "", "")
	clean := fs.Bool("clean", false, "")
	return fs, target, clean
}

func TestFlagsAfterThePathAreRead(t *testing.T) {
	cases := []struct {
		args       []string
		positional []string
		target     string
		clean      bool
	}{
		{[]string{".", "--target", "go"}, []string{"."}, "go", false},
		{[]string{".", "-target", "pico"}, []string{"."}, "pico", false},
		{[]string{"--target", "go", "."}, []string{"."}, "go", false},
		{[]string{".", "--target=go", "--clean"}, []string{"."}, "go", true},
		{[]string{"-clean", "a", "-target", "c", "b"}, []string{"a", "b"}, "c", true},
		{[]string{}, nil, "", false},
		{[]string{".", "--", "--target", "x"}, []string{"."}, "", false},
	}
	for _, c := range cases {
		fs, target, clean := newSet()
		positional, _, err := ParseInterspersed(fs, c.args)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if !reflect.DeepEqual(positional, c.positional) || *target != c.target || *clean != c.clean {
			t.Errorf("%v: got %v target=%q clean=%v; want %v %q %v", c.args, positional, *target, *clean, c.positional, c.target, c.clean)
		}
	}
}

func TestTheTerminatorPassesTheRestThrough(t *testing.T) {
	fs, target, _ := newSet()
	positional, passthrough, err := ParseInterspersed(fs, []string{".", "--target", "go", "--", "-v", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(positional, []string{"."}) || !reflect.DeepEqual(passthrough, []string{"-v", "x"}) || *target != "go" {
		t.Fatalf("got %v %v %q", positional, passthrough, *target)
	}
}

func TestADoubleDashThatIsAValueIsNotTheEnd(t *testing.T) {
	fs, target, _ := newSet()
	positional, passthrough, err := ParseInterspersed(fs, []string{"--target", "--", "."})
	if err != nil {
		t.Fatal(err)
	}
	if *target != "--" || !reflect.DeepEqual(positional, []string{"."}) || passthrough != nil {
		t.Fatalf("got target=%q %v %v", *target, positional, passthrough)
	}
}

func TestAnUnknownFlagAfterThePathIsAnError(t *testing.T) {
	fs, _, _ := newSet()
	if _, _, err := ParseInterspersed(fs, []string{".", "--nope"}); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}

func TestParseUntilFirstLeavesTheProgramsArguments(t *testing.T) {
	fs, target, _ := newSet()
	first, rest, ok, err := ParseUntilFirst(fs, []string{"--target", "go", ".", "sample.csv", "-v", "--", "x"})
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if first != "." || *target != "go" || !reflect.DeepEqual(rest, []string{"sample.csv", "-v", "--", "x"}) {
		t.Fatalf("got %q %q %v", first, *target, rest)
	}

	fs, _, _ = newSet()
	first, rest, ok, err = ParseUntilFirst(fs, []string{"--", "-v", "a"})
	if err != nil || ok || first != "" || !reflect.DeepEqual(rest, []string{"-v", "a"}) {
		t.Fatalf("got %q %v %v %v", first, rest, ok, err)
	}

	fs, _, _ = newSet()
	first, rest, ok, err = ParseUntilFirst(fs, []string{".", "--", "-v"})
	if err != nil || !ok || first != "." || !reflect.DeepEqual(rest, []string{"-v"}) {
		// A `--` right after the path is the separator, and is dropped.
		t.Fatalf("got %q %v %v %v", first, rest, ok, err)
	}

	fs, _, _ = newSet()
	first, rest, ok, err = ParseUntilFirst(fs, []string{".", "a", "--", "-v"})
	if err != nil || !ok || first != "." || !reflect.DeepEqual(rest, []string{"a", "--", "-v"}) {
		// A later one is the program's own.
		t.Fatalf("got %q %v %v %v", first, rest, ok, err)
	}
}

func TestWantsHelp(t *testing.T) {
	if !WantsHelp([]string{".", "--help"}) || !WantsHelp([]string{"-h"}) || WantsHelp([]string{"--", "-h"}) || WantsHelp(nil) {
		t.Fatal("WantsHelp is wrong")
	}
}
