//
// @file contract.go
// @brief The machine-readable half of what was generated.
//
// The generated Rho declares a statement as an `extern fn`, and something has
// to implement it. That something is the compiler's `sql` port, which is
// generated into the project being built — and it needs to know the statements'
// names, parameters and result shapes.
//
// Those are written here, once, by the same reading that wrote the Rho. The port
// reads this file rather than re-reading the SQL, because the Rho that names a
// statement and the Go that implements it have to agree exactly, and two
// independent readings of the same SQL are two chances not to.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"encoding/json"
)

// ContractFile is what the generated statements are described in, beside the
// files they were generated into.
//
// It sits with the generated code rather than in the build directory because it
// is generated: deleting the output removes it, and regenerating rewrites it. A
// contract kept elsewhere would outlive the statements it describes and leave a
// port implementing functions no file declares.
const ContractFile = "statements.json"

// Contract is what a project's statements are, in the form the compiler reads.
type Contract struct {
	// Package is the Rho module the generated files belong to.
	Package string `json:"package"`
	// Statements are the ones declared, in the order they were written.
	Statements []ContractStatement `json:"statements"`
}

// ContractStatement is one statement, as the port implements it.
type ContractStatement struct {
	// Name is the extern's name, exactly as the generated Rho spells it.
	Name string `json:"name"`
	// Comment is what the service wrote about the statement.
	Comment string `json:"comment,omitempty"`
	// SQL is the statement with its markers already rendered into the ones the
	// engine takes.
	SQL string `json:"sql"`
	// Parameters are what it takes, in order.
	Parameters []ContractParameter `json:"parameters,omitempty"`
	// Result is the Rho type of the rows it returns, or "" for a write.
	Result string `json:"result,omitempty"`
	// Fields are the result's columns, in the order the database returns them.
	Fields []ContractField `json:"fields,omitempty"`
	// Affected is whether it reports how many rows it changed.
	Affected bool `json:"affected,omitempty"`
}

// ContractParameter is one value a statement takes.
type ContractParameter struct {
	Name string `json:"name"`
	// GoType is how the generated Go receives it, which is the Rho type in Go's
	// spelling rather than a second choice.
	GoType string `json:"goType"`
	// The kind flags say what the value carries, because a driver takes a typed
	// argument and a Go type is not one a switch can read.
	IsInteger bool `json:"integer,omitempty"`
	IsFloat   bool `json:"float,omitempty"`
	IsText    bool `json:"text,omitempty"`
	IsBoolean bool `json:"boolean,omitempty"`
}

// ContractField is one column of a result.
type ContractField struct {
	// Name is the Go struct field's name.
	Name string `json:"name"`
	// Column is the column as the database calls it, which is what a row is
	// read by.
	Column string `json:"column"`
	// GoType is how the value is read.
	GoType   string `json:"goType"`
	IsInteger bool   `json:"integer,omitempty"`
	IsFloat   bool   `json:"float,omitempty"`
	IsText    bool   `json:"text,omitempty"`
	IsBoolean bool   `json:"boolean,omitempty"`
	IsBlob    bool   `json:"blob,omitempty"`
}

// GoContract is the contract as JSON, for the build to read.
func (r Result) GoContract() (string, error) {
	contract := Contract{
		Package:    r.Package,
		Statements: make([]ContractStatement, 0, len(r.Queries)),
	}
	for _, query := range r.Queries {
		statement := ContractStatement{
			Name:     query.Extern,
			Comment:  query.Comment,
			SQL:      query.SQL,
			Result:   query.Result,
			Affected: query.Affected,
		}
		for _, parameter := range query.Parameters {
			statement.Parameters = append(statement.Parameters, ContractParameter{
				// The Rho name, not a Go one: the generated Rho declares the
				// parameter under its own spelling, and the compiler is what
				// lowers that to Go. Spelling it a second time here would give
				// the port a parameter name no call site uses.
				Name:     rhoFieldName(parameter.Name),
				GoType:   parameter.Type.GoType(),
				IsInteger: parameter.Type == TypeInteger,
				IsFloat:   parameter.Type == TypeFloat,
				IsText:    isTextType(parameter.Type),
				IsBoolean: parameter.Type == TypeBoolean,
			})
		}
		for _, field := range r.fieldsFor(query.Result) {
			statement.Fields = append(statement.Fields, ContractField{
				// The Rho name, for the same reason as a parameter's: the
				// compiler lowers it, so a second spelling would be a field the
				// generated struct does not have.
				Name:      rhoFieldName(field.Name),
				Column:    field.Column,
				GoType:    field.Type.GoType(),
				IsInteger: field.Type == TypeInteger,
				IsFloat:   field.Type == TypeFloat,
				IsText:    isTextType(field.Type),
				IsBoolean: field.Type == TypeBoolean,
				IsBlob:    field.Type == TypeBytes,
			})
		}
		contract.Statements = append(contract.Statements, statement)
	}
	encoded, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

// fieldsFor are the columns of one result shape, or none when the statement
// returns no rows.
func (r Result) fieldsFor(name string) []Field {
	if name == "" {
		return nil
	}
	for _, model := range r.Models {
		if model.Name == name {
			return model.Fields
		}
	}
	return nil
}

// isTextType reports whether a column of this type is carried as text.
//
// Everything that is not a number or a boolean is: a date, a JSON document and a
// blob all arrive as text, and a caller that declared one of them wants the
// driver's own spelling rather than a conversion this component invented.
func isTextType(t Type) bool {
	switch t {
	case TypeText, TypeJSON, TypeTime, TypeUnknown, TypeNull, TypeBytes:
		return true
	}
	return false
}
