package deps

import (
	"fmt"
	"strconv"
	"strings"
)

type Semver struct {
	Major int
	Minor int
	Patch int
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

func ParseSemver(s string) (Semver, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid semver: %s", s)
	}

	m, err := strconv.Atoi(parts[0])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid major: %w", err)
	}

	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid minor: %w", err)
	}

	p, err := strconv.Atoi(parts[2])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid patch: %w", err)
	}

	return Semver{Major: m, Minor: n, Patch: p}, nil
}

func compareSemver(a, b Semver) int {
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}
	return 0
}

type Constraint interface {
	SatisfiedBy(v Semver) bool
}

type RangeConstraint struct {
	Op string // ">", ">=", "<", "<=", "=="
	V  Semver
}

func (c RangeConstraint) SatisfiedBy(v Semver) bool {
	switch c.Op {
	case ">":
		return compareSemver(v, c.V) > 0
	case ">=":
		return compareSemver(v, c.V) >= 0
	case "<":
		return compareSemver(v, c.V) < 0
	case "<=":
		return compareSemver(v, c.V) <= 0
	case "==":
		return compareSemver(v, c.V) == 0
	default:
		return false
	}
}

func ParseVersionRange(raw string) ([]Constraint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Fields(raw)
	var cs []Constraint

	for _, p := range parts {
		var op string

		switch {
		case strings.HasPrefix(p, ">="):
			op = ">="
			p = strings.TrimPrefix(p, ">=")
		case strings.HasPrefix(p, "<="):
			op = "<="
			p = strings.TrimPrefix(p, "<=")
		case strings.HasPrefix(p, ">"):
			op = ">"
			p = strings.TrimPrefix(p, ">")
		case strings.HasPrefix(p, "<"):
			op = "<"
			p = strings.TrimPrefix(p, "<")
		default:
			op = "=="
		}

		v, err := ParseSemver(p)
		if err != nil {
			return nil, fmt.Errorf("invalid version in range: %w", err)
		}

		cs = append(cs, RangeConstraint{Op: op, V: v})
	}

	return cs, nil
}

func VersionInRange(v Semver, cs []Constraint) bool {
	for _, c := range cs {
		if !c.SatisfiedBy(v) {
			return false
		}
	}
	return true
}
