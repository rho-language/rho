package core

import "testing"

/* -------------------------------------------------------------------------- */
/*  Test: Parse Semver                                                         */
/* -------------------------------------------------------------------------- */

func TestParseSemver(t *testing.T) {
	v, err := ParseSemver("1.2.3")
	if err != nil {
		t.Fatalf("ParseSemver failed: %v", err)
	}

	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
		t.Fatalf("wrong semver parsed")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Compare Semver                                                       */
/* -------------------------------------------------------------------------- */

func TestCompareSemver(t *testing.T) {
	a, _ := ParseSemver("1.2.3")
	b, _ := ParseSemver("1.3.0")

	if CompareSemver(a, b) != -1 {
		t.Fatalf("expected a < b")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: SemverSatisfies                                                      */
/* -------------------------------------------------------------------------- */

func TestSemverSatisfies(t *testing.T) {
	ok, err := SemverSatisfies("1.5.0", ">=1.0.0 <2.0.0")
	if err != nil {
		t.Fatalf("SemverSatisfies error: %v", err)
	}
	if !ok {
		t.Fatalf("expected version to satisfy constraint")
	}

	ok, _ = SemverSatisfies("2.0.0", ">=1.0.0 <2.0.0")
	if ok {
		t.Fatalf("expected version NOT to satisfy constraint")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Semver                                                       */
/* -------------------------------------------------------------------------- */

func TestInvalidSemver(t *testing.T) {
	if IsValidSemver("1.2") {
		t.Fatalf("expected invalid semver")
	}
}
