//
// @file types_test.go
// @brief Tests for what a column's type means.
//
// The risk this file is about is a mapping that is wrong quietly. A column
// carried as a string compiles, runs, and returns zeroes for a number nobody can
// add up; a blob carried as text survives being printed and loses a zero byte. So
// the cases here are the ones where the wrong answer is still a program.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import "testing"

// A type is carried in the language type that keeps its value, which is not
// always the type it was stored as.
func TestACarriedTypeKeepsTheValue(t *testing.T) {
	cases := []struct {
		declared string
		want     Type
	}{
		// A whole number, whatever width it was stored in.
		{"integer", TypeInteger},
		{"INTEGER", TypeInteger},
		{"int8", TypeInteger},
		{"bigint", TypeInteger},
		{"smallint", TypeInteger},
		{"serial", TypeInteger},
		// A parameterised number is a number by what it is, not by the digits in
		// its scale. This is the form sqlite actually reports, and the one a
		// mapping that did not know it would get wrong.
		{"NUMERIC(10, 2)", TypeText},
		{"numeric(10, 2)", TypeText},
		{"DECIMAL(38, 10)", TypeText},
		{"numeric", TypeText},
		// Text, of any width.
		{"text", TypeText},
		{"VARCHAR(255)", TypeText},
		{"character varying", TypeText},
		{"char(3)", TypeText},
		{"citext", TypeText},
		{"uuid", TypeText},
		// Bytes are not text however printable they are.
		{"blob", TypeBytes},
		{"bytea", TypeBytes},
		{"binary", TypeBytes},
		// A boolean.
		{"boolean", TypeBoolean},
		{"bool", TypeBoolean},
		// A moment.
		{"timestamp", TypeTime},
		{"timestamptz", TypeTime},
		{"date", TypeTime},
		// A number with a fraction.
		{"real", TypeFloat},
		{"double", TypeFloat},
		{"float8", TypeFloat},
		// A document.
		{"json", TypeText},
		{"jsonb", TypeText},
	}
	for _, test := range cases {
		if got := ReduceType(test.declared); got != test.want {
			t.Errorf("ReduceType(%q) = %q, want %q", test.declared, got, test.want)
		}
	}
}

func TestATypeNothingKnowsIsNotGuessedAt(t *testing.T) {
	// A type this does not know becomes unknown, and unknown is a string in a
	// generated struct rather than its nearest neighbour: a number carried as
	// zeroes is a wrong answer, and a string carrying what the database sent is
	// at least the value.
	for _, declared := range []string{"geometry", "hstore", "tsvector", "point", ""} {
		got := ReduceType(declared)
		if declared == "" {
			if got != TypeUnknown {
				t.Errorf("an empty type reduced to %q, want unknown", got)
			}
			continue
		}
		if got == TypeInteger || got == TypeFloat || got == TypeBoolean {
			t.Errorf("ReduceType(%q) = %q, which is a neighbour rather than an admission",
				declared, got)
		}
	}
}

func TestAWordInsideATypeIsNotAWord(t *testing.T) {
	// `POINT` contains `INT`. A reduction that looked for a substring would
	// make a geometry an integer, and a geometry carried as an integer is a
	// wrong answer rather than a missing one.
	if got := ReduceType("POINT"); got == TypeInteger {
		t.Error("POINT was reduced to an integer because it contains INT")
	}
}

func TestTheCarriedTypeIsTheOnesALanguageHas(t *testing.T) {
	cases := map[Type][2]string{
		TypeInteger: {"int64", "i64"},
		TypeFloat:   {"float64", "f64"},
		TypeText:    {"string", "string"},
		TypeBoolean: {"bool", "bool"},
		TypeBytes:   {"[]byte", "string"},
		TypeTime:    {"string", "string"},
		TypeUnknown: {"string", "string"},
	}
	for given, want := range cases {
		if got := given.GoType(); got != want[0] {
			t.Errorf("%q is a Go %q, want %q", given, got, want[0])
		}
		if got := given.RhoType(); got != want[1] {
			t.Errorf("%q is a Rho %q, want %q", given, got, want[1])
		}
	}
}

func TestBytesAreCarriedAsTextBecauseBytesAreNotText(t *testing.T) {
	// A blob with a zero byte in it is not a string, and a generated field that
	// claims it is has lied about the value it holds. It is carried as bytes in
	// Go and as text in Rho, which is what each language has.
	if got := TypeBytes.GoType(); got != "[]byte" {
		t.Errorf("bytes are a Go %q, want []byte", got)
	}
	if got := TypeBytes.RhoType(); got != "string" {
		t.Errorf("bytes are a Rho %q, want string, which is the type Rho has for them", got)
	}
}

func TestEveryTypeSaysWhatItMeans(t *testing.T) {
	// A caller that wants to know what it got should not have to look it up, and
	// a type that says nothing is a type a reader cannot trust.
	for _, given := range []Type{
		TypeInteger, TypeFloat, TypeText, TypeBoolean,
		TypeBytes, TypeTime, TypeJSON, TypeNull, TypeUnknown,
	} {
		if given.Describe() == "" {
			t.Errorf("%q does not say what it means", given)
		}
		if given.GoType() == "" || given.RhoType() == "" || given.ZeroValue() == "" {
			t.Errorf("%q does not say how it is carried", given)
		}
	}
}
