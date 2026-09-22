package deps

import "testing"

func TestParseSemver(t *testing.T) {
	v, err := ParseSemver("1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
		t.Fatalf("parsed semver wrong: %+v", v)
	}
}

func TestParseSemverInvalid(t *testing.T) {
	_, err := ParseSemver("1.2")
	if err == nil {
		t.Fatalf("expected error for invalid semver")
	}
}

func TestCompareSemver(t *testing.T) {
	a := Semver{1, 2, 3}
	b := Semver{1, 2, 4}

	if compareSemver(a, b) != -1 {
		t.Fatalf("expected a < b")
	}
	if compareSemver(b, a) != 1 {
		t.Fatalf("expected b > a")
	}
	if compareSemver(a, a) != 0 {
		t.Fatalf("expected a == a")
	}
}

func TestParseVersionRange(t *testing.T) {
	cs, err := ParseVersionRange(">=1.0.0 <2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cs) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(cs))
	}
}

func TestVersionInRange(t *testing.T) {
	cs, _ := ParseVersionRange(">=1.0.0 <2.0.0")

	if !VersionInRange(Semver{1, 5, 0}, cs) {
		t.Fatalf("expected version in range")
	}

	if VersionInRange(Semver{2, 0, 0}, cs) {
		t.Fatalf("expected version out of range")
	}
}
