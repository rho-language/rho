//
// @file version_test.go
// @brief Tests for which version is wanted, and whether this is it.
//
// A version constraint is the difference between "the debugger is installed" and
// "the debugger that is installed can drive this session", so the rules here are
// the difference between a working board and one that fails in ways that name the
// adapter. Every rule in the grammar is here, because a rule nobody tested is a rule
// nobody has.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package sysdep

import (
	"strings"
	"testing"
)

func TestAVersionIsReadOutOfWhatAProgramPrinted(t *testing.T) {
	// The real shapes, from real programs, in the order they are easiest to get
	// wrong: a bare number, the version last after a package name and an epoch, and
	// a pre-release that has to sort below the release it leads to.
	for _, testCase := range []struct {
		output string
		want   string
	}{
		{"15", "15"},
		{"GNU gdb (Ubuntu 12.1-3ubuntu1~22.04) 12.1", "12.1"},
		{"openocd 0.11.0", "0.11.0"},
		{"cmake version 4.4.3", "4.4.3"},
		{"arm-none-eabi-gcc (15:13.2.rel1-2) 13.2.1 20221008", "15"},
		{"1.2.3-rc1", "1.2.3-rc1"},
		{"v9.0.1", "9.0.1"},
	} {
		version, ok := ParseVersion(testCase.output)
		if !ok {
			t.Errorf("%q: expected a version to be found", testCase.output)
			continue
		}
		if version.String() != testCase.want {
			t.Errorf("%q: expected %q, got %q", testCase.output, testCase.want, version.String())
		}
	}

	// And output with no version in it is not a version, which is a different answer
	// from a version of zero.
	for _, output := range []string{"", "command not found", "no version here"} {
		if _, ok := ParseVersion(output); ok {
			t.Errorf("%q: expected no version to be found", output)
		}
	}
}

// A pre-release is older than the release it leads to, and that is the one thing
// everybody assumes and nobody checks. A debugger that is a release candidate and a
// debugger that is a release are not the same thing to drive.
func TestAPreReleaseIsOlderThanItsRelease(t *testing.T) {
	release := Version{Major: 12, Minor: 1}
	candidate := Version{Major: 12, Minor: 1, Pre: "rc1"}
	alpha := Version{Major: 12, Minor: 1, Pre: "alpha"}

	if candidate.Compare(release) >= 0 {
		t.Error("expected a release candidate to be older than the release")
	}
	if release.Compare(candidate) <= 0 {
		t.Error("expected the release to be newer than its candidate")
	}
	if alpha.Compare(candidate) >= 0 {
		t.Error("expected an alpha to be older than a release candidate")
	}
	if alpha.Compare(alpha) != 0 {
		t.Error("expected two identical pre-releases to be equal")
	}

	// So a constraint written against the release refuses the candidate, which is the
	// whole reason this is here: accepting it would be a session that works by luck.
	constraint, err := ParseConstraint(">=12.1")
	if err != nil {
		t.Fatal(err)
	}
	if !constraint.Accepts(release) {
		t.Error("expected the release to satisfy >=12.1")
	}
	if constraint.Accepts(candidate) {
		t.Error("expected a release candidate not to satisfy >=12.1")
	}
}

func TestAConstraintSaysWhichVersionsItTakes(t *testing.T) {
	for _, testCase := range []struct {
		want string
		take []string
		skip []string
	}{
		{want: ">=10.0", take: []string{"10.0", "10.1", "14.2", "15"}, skip: []string{"9.3", "9.9.9"}},
		{want: ">=10", take: []string{"10", "10.0.1", "99"}, skip: []string{"9.99"}},
		{want: ">=0.11.0", take: []string{"0.11.0", "0.12"}, skip: []string{"0.10.2"}},
		{want: ">=0.11 <0.13", take: []string{"0.11.0", "0.12.9"}, skip: []string{"0.10", "0.13", "1.0"}},
		{want: ">=10 <11 || >=12 <14", take: []string{"10.5", "12.1", "13.9"},
			skip: []string{"9.9", "11.0", "14.0"}},
		// The alternative that catches a person out: `>=10 || >=12` takes
		// everything above ten, so the second half is doing nothing at all.
		{want: ">=10 || >=12", take: []string{"10.0", "11.0", "13.0"},
			skip: []string{"9.0"}},
		{want: "~1.2.3", take: []string{"1.2.3", "1.2.9"}, skip: []string{"1.3.0", "1.2.2"}},
		{want: "^1.2", take: []string{"1.2.0", "1.9.9"}, skip: []string{"2.0.0", "1.1.9"}},
		{want: "=0.12.0", take: []string{"0.12.0"}, skip: []string{"0.12.1", "0.11.0"}},
		{want: "!=0.10", take: []string{"0.10.1", "0.11"}, skip: []string{"0.10"}},
		{want: "*", take: []string{"0.0.1", "99"}, skip: nil},
		{want: "", take: []string{"0.0.1", "99"}, skip: nil},
	} {
		constraint, err := ParseConstraint(testCase.want)
		if err != nil {
			t.Errorf("%q: %v", testCase.want, err)
			continue
		}
		for _, taken := range testCase.take {
			version, _ := ParseVersion(taken)
			if !constraint.Accepts(version) {
				t.Errorf("%q should take %s", testCase.want, taken)
			}
		}
		for _, skipped := range testCase.skip {
			version, _ := ParseVersion(skipped)
			if constraint.Accepts(version) {
				t.Errorf("%q should not take %s", testCase.want, skipped)
			}
		}
	}
}

// A requirement nobody can enforce is not a requirement to enforce: refusing every
// version because a bound is misspelled would make the report useless, and the
// spellings that matter are all read.
func TestAConstraintNobodyCanReadIsAcceptedRatherThanEnforced(t *testing.T) {
	for _, spoken := range []string{"newer than 10", ">=ten", "latest", ">=1.x"} {
		satisfied, why := Satisfies(spoken, Version{Major: 12})
		if !satisfied {
			t.Errorf("%q: expected any version to be accepted, got %q", spoken, why)
		}
		if !strings.Contains(why, "cannot be read") {
			t.Errorf("%q: expected the failure to say the requirement is unreadable, got %q", spoken, why)
		}
	}
}

// The explanation is the point: a report that says OUTDATED and leaves a person to
// work out that 12.1 is not >=13 has not done the work.
func TestTheRefusalSaysWhichSideOfWhichBoundItFellOn(t *testing.T) {
	satisfied, why := Satisfies(">=13.0", Version{Major: 12, Minor: 1})
	if satisfied {
		t.Fatal("expected 12.1 not to satisfy >=13.0")
	}
	if !strings.Contains(why, "12.1") || !strings.Contains(why, ">=13.0") {
		t.Errorf("expected both versions named, got %q", why)
	}
	satisfied, why = Satisfies(">=10.0", Version{Major: 12, Minor: 1})
	if !satisfied || why != "" {
		t.Errorf("expected a satisfied version to say nothing, got ok=%v why=%q", satisfied, why)
	}
}

// A pattern is how a module says which of several version-shaped runs in a
// program's output it means, and `gdb` is why: four of them, and it means the last.
func TestAPatternSaysWhichVersionIsMeant(t *testing.T) {
	const pattern = `[^
]*?\)\s*([0-9][^\s(]*)`
	version, ok := ExtractVersion("GNU gdb (Ubuntu 12.1-3ubuntu1~22.04) 12.1", pattern)
	if !ok || version.String() != "12.1" {
		t.Errorf("expected the version after the parenthesis, got %q (%v)", version.String(), ok)
	}
	// Without one, the first version-shaped run is taken — which is right most of
	// the time and why a module says so when it is not.
	version, ok = ExtractVersion("cmake version 4.4.3", "")
	if !ok || version.String() != "4.4.3" {
		t.Errorf("expected the only version there is, got %q (%v)", version.String(), ok)
	}
	// A pattern that does not compile is a module's mistake, and the best thing to do
	// about it is read the version the way it would have been without one — so the
	// requirement is still checked rather than skipped.
	version, ok = ExtractVersion("openocd 0.11.0", "[unclosed")
	if !ok || version.String() != "0.11.0" {
		t.Errorf("expected the version to be read anyway, got %q (%v)", version.String(), ok)
	}
	if err := PatternCompiles("[unclosed"); err == nil {
		t.Error("expected a pattern that does not compile to be reported")
	}
	if err := PatternCompiles(""); err != nil {
		t.Errorf("expected no pattern to be fine, got %v", err)
	}
}
