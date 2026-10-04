//
// @file version.go
// @brief Which version is wanted, and whether this is it.
//
// A version constraint is what separates "the debugger is installed" from "the
// debugger that is installed can drive this session", and the second is the one
// that matters: a GDB from 2016 speaks a protocol that does not have the commands
// this uses, and a session against one fails in ways that name the adapter rather
// than the debugger.
//
// So a requirement carries a constraint, and this evaluates it. It is written here
// rather than taken from a library for one reason that is worth more than the
// hundred lines it saves: the toolchain has no dependencies outside itself, and a
// release of a debugging tool that cannot be built on a machine with no network is
// not a tool anybody should depend on to find out whether they can debug. The
// grammar is small and every rule in it is tested.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package sysdep

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a version, as a program prints one.
type Version struct {
	Major, Minor, Patch int
	// Pre is a pre-release marker: `beta1`, `rc2`. Compared before the numbers it
	// belongs to, and only against itself — a pre-release is older than the release
	// it leads to, which is the one thing everybody assumes and nobody checks.
	Pre string
	// Text is what was read, kept so a report can repeat it exactly as the program
	// printed it rather than as a version this normalised into something plausible.
	Text string
}

// String is the version as it was found.
func (v Version) String() string {
	if v.Text != "" {
		return v.Text
	}
	out := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		out += "-" + v.Pre
	}
	return out
}

// versionIn matches one version inside a word: the digits and dots, and the
// pre-release marker that follows them.
//
// Written as one pattern because the parts belong together: `1.2.3-rc1` is one
// version, and reading it as `1.2.3` and then a stray `1` is how a release candidate
// comes to look like the release it leads to.
//
// The leading `[^0-9]*` is what makes this work on real output: `v9.0.1`, `arm64-3.1`
// and `openocd0.11.0` all start with something that is not a digit.
var versionIn = regexp.MustCompile(`^[^0-9]*([0-9]+(?:\.[0-9]+){0,2})(?:[-+]([A-Za-z][A-Za-z0-9.]*))?`)

// ParseVersion reads a version out of what a program printed.
//
// Deliberately forgiving, because this is text scraped from somebody else's output:
// the first version in the line is taken, and anything around it is ignored. A
// program that prints `gdb (Ubuntu 12.1-3ubuntu1~22.04) 12.1` has four versions in it
// and means the last, which is why a module that cares says so with a pattern.
func ParseVersion(text string) (Version, bool) {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '(' || r == ')'
	}) {
		match := versionIn.FindStringSubmatch(word)
		if match == nil {
			continue
		}
		// A version-shaped thing that is not a version: `1.x`, which several
		// programs print for "any 1". A requirement written against one of those
		// would otherwise be silently enforced as the lowest of the range.
		if rest := word[len(match[0]):]; strings.HasPrefix(rest, ".") {
			continue
		}
		return toVersion(match[1], match[2]), true
	}
	return Version{}, false
}

// toVersion is the numbers and the marker of one match, as a version.
//
// `Text` is the version and not the word it was found in, because a report shows
// what was read: `v9.0.1` is version 9.0.1, and repeating the `v` in every line of a
// table is noise.
func toVersion(numbers string, pre string) Version {
	out := Version{Pre: pre, Text: numbers}
	if pre != "" {
		out.Text = numbers + "-" + pre
	}
	parts := strings.Split(numbers, ".")
	into := []*int{&out.Major, &out.Minor, &out.Patch}
	for index, part := range parts {
		if index >= len(into) {
			break
		}
		// A part that is not a number is not a version, and a field like `1.2.x`
		// — which several programs print for "any 1.2" — is a shape, not a version.
		number, err := strconv.Atoi(part)
		if err != nil {
			return Version{}
		}
		*into[index] = number
	}
	return out
}

// Compare is this version against that one: -1, 0 or 1.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{
		{v.Major, other.Major},
		{v.Minor, other.Minor},
		{v.Patch, other.Patch},
	} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	// Equal numbers, and the pre-release decides: a beta of a version is older than
	// the version itself, which is what makes `>=12.1` refuse a `12.1-rc1` rather
	// than quietly accepting a debugger that is a release candidate.
	switch {
	case v.Pre == "" && other.Pre == "":
		return 0
	case v.Pre == "":
		return 1
	case other.Pre == "":
		return -1
	case v.Pre < other.Pre:
		return -1
	case v.Pre > other.Pre:
		return 1
	default:
		return 0
	}
}

/* -------------------------------------------------------------------------- */
/*  Constraints                                                                */
/* -------------------------------------------------------------------------- */

// Constraint is what a requirement accepts.
//
// Read as a set of ranges separated by `||`, each of which is a set of comparisons
// joined by a space: `>=0.11 <0.13`, or `>=10 <11 || >=12 <14`. An empty constraint
// accepts anything, which is what a program whose output format has been stable for
// a decade should say by saying nothing.
type Constraint struct {
	text string
	// ranges are the alternatives, each a list of comparisons that must all hold.
	ranges [][]comparison
}

// comparison is one test of a version against another.
type comparison struct {
	operator string
	version  Version
}

// ParseConstraint reads a constraint, and says whether it was one.
//
// The operators are `>=`, `>`, `<=`, `<`, `=`, `==` and `!=`, plus `~` (the same
// major and minor, and a later patch) and `^` (the same major, and a later minor) —
// the two that appear in almost every version requirement and are the two most
// often implemented wrongly. `~1.2.3` and `^1.2.3` are the two forms npm and cargo
// established, and a machine that reads both the same way is a machine whose
// requirement means the same thing on every toolchain.
func ParseConstraint(text string) (Constraint, error) {
	trimmed := strings.TrimSpace(text)
	constraint := Constraint{text: trimmed}
	if trimmed == "" || trimmed == "*" {
		return constraint, nil
	}

	for _, alternative := range strings.Split(trimmed, "||") {
		parts := strings.Fields(alternative)
		if len(parts) == 0 {
			continue
		}
		comparisons := []comparison{}
		for _, part := range parts {
			operator, versionText := splitOperator(part)
			version, ok := ParseVersion(versionText)
			if !ok {
				return Constraint{}, fmt.Errorf("%q in the version requirement %q is not a version", part, trimmed)
			}
			comparisons = append(comparisons, comparison{operator: operator, version: version})
		}
		constraint.ranges = append(constraint.ranges, comparisons)
	}
	if len(constraint.ranges) == 0 {
		return Constraint{}, fmt.Errorf("%q is not a version requirement", trimmed)
	}
	return constraint, nil
}

// splitOperator is the comparison a term asks for, and the version it is against.
func splitOperator(term string) (string, string) {
	for _, operator := range []string{">=", "<=", "==", "!=", ">", "<", "=", "~", "^"} {
		if strings.HasPrefix(term, operator) {
			return operator, strings.TrimSpace(term[len(operator):])
		}
	}
	return "=", term
}

// String is the constraint as it was written.
func (c Constraint) String() string { return c.text }

// Accepts whether a version satisfies the constraint.
func (c Constraint) Accepts(version Version) bool {
	if len(c.ranges) == 0 {
		return true
	}
	for _, comparisons := range c.ranges {
		if holdsFor(version, comparisons) {
			return true
		}
	}
	return false
}

// holdsFor is one alternative: every comparison in it, which is what a space
// between two terms means.
func holdsFor(version Version, comparisons []comparison) bool {
	for _, one := range comparisons {
		if !one.holds(version) {
			return false
		}
	}
	return true
}

// holds is one comparison.
func (c comparison) holds(version Version) bool {
	found := version.Compare(c.version)
	switch c.operator {
	case ">=":
		return found >= 0
	case ">":
		return found > 0
	case "<=":
		return found <= 0
	case "<":
		return found < 0
	case "!=":
		return found != 0
	case "~":
		// The same major and minor, and any later patch: `~1.2` is `>=1.2.0 <1.3.0`.
		return version.Major == c.version.Major &&
			version.Minor == c.version.Minor &&
			found >= 0
	case "^":
		// The same major, and any later minor: `^1.2` is `>=1.2.0 <2.0.0`.
		return version.Major == c.version.Major && found >= 0
	default:
		return found == 0
	}
}

// Satisfies is whether a version meets a requirement written as text, and says what
// is wrong with it when it does not.
//
// The explanation is the point. A report that says `OUTDATED` and leaves a person to
// work out that `12.1` is not `>=13` has not done the work; this says which side of
// which bound it fell on.
func Satisfies(want string, found Version) (bool, string) {
	constraint, err := ParseConstraint(want)
	if err != nil {
		// A requirement this cannot read is not a requirement to enforce: refusing
		// every version because the bound is misspelled would make the report
		// useless, and the spellings that matter are all read.
		return true, fmt.Sprintf("the version requirement %q cannot be read, so any version is accepted", want)
	}
	if strings.TrimSpace(want) == "" || strings.TrimSpace(want) == "*" {
		return true, ""
	}
	if constraint.Accepts(found) {
		return true, ""
	}
	return false, fmt.Sprintf("%s does not satisfy %s", found, want)
}
