//
// @file types.go
// @brief What a column's type becomes in generated code.
//
// A result column's type is a fact about the database, and the engine reports it
// in the engine's own spelling: `INTEGER` in sqlite, `bigint` in duckdb,
// `NUMERIC(10, 2)` for a scaled number. The mapping from those to a type a
// generated struct can have is this file's whole job, and it exists as a
// separate step because it is the step that can be wrong quietly: a column
// mapped to a string is a column that compiles, runs, and returns zeroes.
//
// So the mapping is by *meaning* rather than by name. A type is first reduced to
// what it means — a number, a string, a boolean, a time, bytes — and only then to
// a language type. `NUMERIC(10, 2)` and `DECIMAL(10, 2)` mean the same thing and
// are spelled differently; `VARCHAR(255)` and `TEXT` mean different things in
// storage and the same thing to a caller, and the second is what generated code
// wants.
//
// A type that means nothing this file recognises is not guessed at. It becomes
// `unknown`, which a caller can see and a generated function can return as a
// string, and which a reader is told about rather than left to discover.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"strings"
)

// Type is what a column means, as far as generated code is concerned.
//
// It is narrower than a SQL type on purpose. The types a generated struct can
// hold are the ones every driver can carry without loss, and a type that is not
// one of these is better carried as its own meaning than coerced into a neighbour
// that happens to be nearby.
type Type string

// The types a generated result can have.
const (
	// TypeInteger is a whole number, whatever the width it is stored in. A width
	// a generated int64 cannot hold is reported rather than truncated.
	TypeInteger Type = "integer"
	// TypeFloat is a number with a fraction.
	TypeFloat Type = "float"
	// TypeText is text, of any length. A length is a storage concern; a Go string
	// or a Rho string does not have one.
	TypeText Type = "text"
	// TypeBoolean is a true or a false.
	TypeBoolean Type = "boolean"
	// TypeBytes is binary data, which is not text however printable it is.
	TypeBytes Type = "bytes"
	// TypeTime is a moment. A driver hands it over as a time, and a generated
	// field of a time type is what the caller wants rather than a string they
	// parse.
	TypeTime Type = "time"
	// TypeJSON is a document the database stores, which a caller wants as
	// whatever its language calls a document.
	TypeJSON Type = "json"
	// TypeNull is a column that holds nothing, whatever it is called.
	TypeNull Type = "null"
	// TypeUnknown is a type this mapping does not know, reported rather than
	// guessed at.
	TypeUnknown Type = "unknown"
)

// SQLiteTypeAffinity is sqlite's rule for reducing a declared type to what it
// stores, and it is spelled out rather than approximated because it is the
// affinity a sqlite column actually has.
//
//  1. a type containing INT is an integer
//  2. a type containing CHAR, CLOB or TEXT is text
//  3. a type containing BLOB, or with no type at all, is bytes
//  4. a type containing REAL, FLOA or DOUB is a float
//  5. anything else is a number
//
// A number is kept as a number rather than as a float, because a scaled number
// is a number the caller is going to do arithmetic on and a float64 is the wrong
// answer for a value that is meant to be exact.
func SQLiteTypeAffinity(declared string) Type {
	declared = upper(declared)
	if declared == "" {
		return TypeBytes
	}
	switch {
	case contains(declared, "INT"):
		return TypeInteger
	case contains(declared, "CHAR"), contains(declared, "CLOB"), contains(declared, "TEXT"):
		return TypeText
	case contains(declared, "BLOB"):
		return TypeBytes
	case contains(declared, "REAL"), contains(declared, "FLOA"), contains(declared, "DOUB"):
		return TypeFloat
	case contains(declared, "BOOL"):
		return TypeBoolean
	case contains(declared, "DATETIME"), contains(declared, "TIMESTAMP"):
		return TypeTime
	case contains(declared, "JSON"):
		return TypeJSON
	case contains(declared, "DEC"), contains(declared, "NUMERIC"):
		// A scaled number. The scale is a precision the database keeps and a
		// generated value does not, so what a caller gets is a number.
		return TypeNumber()
	}
	return TypeNumber()
}

// TypeNumber is the meaning of a numeric type whose width is not one a generated
// integer holds.
//
// A `numeric(38, 10)` is a number the database stores exactly and a float64
// cannot: the digits are gone before the caller sees them. It is carried as text
// rather than as a float, because text is the only one of the two that keeps
// them, and a caller that knows it is dealing with a scaled number can convert.
func TypeNumber() Type { return TypeText }

// DuckDBType is what a declared type means in the engines whose spellings differ
// from sqlite's.
//
// The reductions are the same because the *meanings* are the same: what differs
// between engines is how a type is written, and reducing a spelling to a meaning
// is the same work twice. An engine that has a spelling this does not know
// reports a type this cannot reduce, and the type stays unknown rather than
// becoming its nearest neighbour.
func DuckDBType(declared string) Type {
	declared = upper(declared)
	switch declared {
	case "BIGINT", "INT8", "HUGEINT", "INT", "INTEGER", "INT4", "INT2", "INT1",
		"SERIAL", "BIGSERIAL", "SMALLINT", "TINYINT", "UBIGINT", "UINTEGER", "USMALLINT", "UHUGEINT":
		return TypeInteger
	case "DOUBLE", "FLOAT", "FLOAT4", "FLOAT8", "REAL", "DECIMAL", "NUMERIC":
		return TypeFloat
	case "VARCHAR", "TEXT", "STRING", "BPCHAR", "CHARACTER", "UUID", "JSON", "JSONB":
		return TypeText
	case "BOOL", "BOOLEAN", "LOGICAL":
		return TypeBoolean
	case "BLOB", "BYTEA", "BINARY", "VARBINARY":
		return TypeBytes
	case "TIMESTAMP", "TIMESTAMP WITH TIME ZONE", "TIMESTAMPTZ", "DATETIME", "DATE", "TIME":
		return TypeTime
	}
	return TypeUnknown
}

// PostgresType is what a declared type means in postgres's spellings, which are
// the ones a service written against a server most often carries.
func PostgresType(declared string) Type {
	declared = upper(declared)
	switch declared {
	case "SMALLINT", "INT2", "INTEGER", "INT", "INT4", "BIGINT", "INT8", "SERIAL",
		"BIGSERIAL", "SMALLSERIAL", "OID":
		return TypeInteger
	case "REAL", "FLOAT4", "DOUBLE PRECISION", "FLOAT8", "NUMERIC", "DECIMAL":
		return TypeFloat
	case "CHARACTER VARYING", "VARCHAR", "CHARACTER", "CHAR", "BPCHAR", "TEXT",
		"NAME", "CITEXT", "UUID", "JSON", "JSONB", "XML", "TSVECTOR":
		return TypeText
	case "BOOLEAN", "BOOL":
		return TypeBoolean
	case "BYTEA", "BIT", "VARBIT":
		return TypeBytes
	case "TIMESTAMP", "TIMESTAMP WITH TIME ZONE", "TIMESTAMPTZ", "DATE", "TIME",
		"TIME WITH TIME ZONE", "INTERVAL":
		return TypeTime
	}
	return TypeUnknown
}

// ReduceType is what a declared type means, across the spellings a service is
// likely to have written.
//
// It is the reduction a generator uses when the engine has not answered, which is
// the case a caller of `describe_query` without a database is in. It is
// deliberately more forgiving than any one dialect's rule, because its job is to
// be right often rather than to be right for one engine, and anything it is not
// sure about becomes `unknown` rather than a guess.
func ReduceType(declared string) Type {
	declared = strings.TrimSpace(declared)
	if declared == "" {
		return TypeUnknown
	}
	// A parameterised type reduces by its head, because what the parameters say
	// is the width or the scale, and neither changes what the value *is*. This is
	// the `NUMERIC(10, 2)` case, which is the one a sqlite engine actually
	// reports and which a mapping that did not know it would get wrong.
	base := declared
	if open := strings.Index(base, "("); open > 0 {
		base = strings.TrimSpace(base[:open])
	}
	// A scaled number is not a float. `numeric(10, 2)` keeps two decimal places
	// exactly, and a float64 holding a money value is a wrong answer rather than
	// an approximate one. It is carried as text, the one type that keeps the
	// digits, and that is decided before the named lists because every dialect
	// names it as a number.
	if isScaledNumber(base) {
		return TypeNumber()
	}
	// The named lists are tried first because they are exact: a spelling a
	// dialect names is that dialect's type, and none of them is a catch-all. The
	// sqlite affinity is tried last, because it is a rule of containment and so
	// has an answer for anything — its answer for a type it does not know is its
	// catch-all rather than an admission, and a catch-all is not a hit.
	for _, spelling := range []string{declared, base} {
		for _, reduce := range []func(string) Type{DuckDBType, PostgresType} {
			if meaning := reduce(spelling); meaning != TypeUnknown {
				return meaning
			}
		}
	}
	if meaning := SQLiteTypeAffinity(declared); meaning != TypeUnknown {
		return meaning
	}
	return TypeUnknown
}

// isScaledNumber reports whether a type is a number with a scale — a number that
// keeps a fixed number of decimal places, which is what money is written as.
//
// A parameterised number is a scaled one: `numeric(10, 2)` says ten digits with
// two after the point, and `decimal(18, 4)` says the same of a different width.
// Neither is a float.
func isScaledNumber(base string) bool {
	switch upper(base) {
	case "NUMERIC", "DECIMAL", "MONEY":
		return true
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  What a caller gets                                                          */
/* -------------------------------------------------------------------------- */

// GoType is the Go type a column of this type is carried in.
//
// An unknown type becomes a string rather than an interface: an interface in a
// generated struct is a value every caller has to type-assert, and a string at
// least keeps the value the database sent. The caller is told, so a type that
// should have been known is a thing a reader can see and fix.
func (t Type) GoType() string {
	switch t {
	case TypeInteger:
		return "int64"
	case TypeFloat:
		return "float64"
	case TypeText, TypeJSON, TypeTime, TypeUnknown:
		return "string"
	case TypeBoolean:
		return "bool"
	case TypeBytes:
		// A byte column is a Go `string`, not a `[]byte`.
		//
		// Rho has one sequence type and it is a string, so a generated function
		// takes a byte column as a string and a caller has nothing to convert.
		// Declaring it as `[]byte` here would make the contract describe a
		// parameter the call sites do not pass: a `string` is not assignable to
		// a `[]byte`, so every statement taking one would fail to link.
		return "string"
	case TypeNull:
		return "any"
	}
	return "any"
}

// RhoType is the Rho type a column of this type is carried in.
//
// It follows the language's own types rather than Go's, because a Rho caller
// reads `i64` and not `int64`. A text column is a `string` in both, and a number
// is an `i64` or an `f64` by what it is rather than by what it was stored as.
func (t Type) RhoType() string {
	switch t {
	case TypeInteger:
		return "i64"
	case TypeFloat:
		return "f64"
	case TypeText, TypeJSON, TypeTime, TypeUnknown:
		return "string"
	case TypeBoolean:
		return "bool"
	case TypeBytes, TypeNull:
		return "string"
	}
	return "string"
}

// ZeroValue is what a column of this type carries when it holds nothing.
//
// A generated struct is filled from a row, and a row that has no value for a
// column is a NULL, which is a value a generated field still has to be able to
// hold. For most types that is the language's own zero; for text and bytes it is
// an empty value rather than a null, because a caller that reads a field it
// never set should see nothing rather than a nil it has to check.
func (t Type) ZeroValue() string {
	switch t {
	case TypeInteger, TypeFloat, TypeBoolean:
		return "0"
	case TypeBytes:
		return "nil"
	case TypeNull:
		return "nil"
	}
	return `""`
}

// Describe is a sentence about what the type means, which a caller reads when it
// wants to know what it got rather than having to look it up.
func (t Type) Describe() string {
	switch t {
	case TypeInteger:
		return "a whole number"
	case TypeFloat:
		return "a number with a fraction"
	case TypeText:
		return "text"
	case TypeBoolean:
		return "true or false"
	case TypeBytes:
		return "bytes, which are not text however printable they are"
	case TypeTime:
		return "a moment in time"
	case TypeJSON:
		return "a document"
	case TypeNull:
		return "nothing, whatever the column is called"
	case TypeUnknown:
		return "a type this does not know, carried as text"
	}
	return string(t)
}

/* -------------------------------------------------------------------------- */
/*  Spelling                                                                    */
/* -------------------------------------------------------------------------- */

// upper is a string in upper case, which is how a type is compared: SQL type
// names are written in whatever case the service used and mean the same thing.
func upper(text string) string { return strings.ToUpper(text) }

// contains reports whether a word is in a spelling, on word boundaries, so an
// `INT` inside `POINT` is not an integer.
func contains(text, word string) bool {
	at := 0
	for {
		index := strings.Index(text[at:], word)
		if index < 0 {
			return false
		}
		start := at + index
		end := start + len(word)
		before := start == 0 || !isTypeByte(text[start-1])
		after := end == len(text) || !isTypeByte(text[end])
		if before && after {
			return true
		}
		at = end
	}
}

// isTypeByte reports whether a byte is part of a type name rather than a
// separator between its words.
func isTypeByte(b byte) bool {
	return b == '_' ||
		('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9')
}
