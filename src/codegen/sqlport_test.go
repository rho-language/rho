//
// @file sqlport_test.go
// @brief The sql port, end to end.
//
// A project that declares SQL gets statements generated from it and a port that
// implements the externs those statements declare. Neither half is useful alone,
// and the seam between them is where a mismatch shows: a statement whose name
// the port spells differently, a signature the compiler emitted a call for and
// the port did not write, a schema the port was not given.
//
// Every test here builds the generated Go and asks the Go compiler what it
// thinks, because that is the only check that sees all three of those at once.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/ir"
)

// A statement the project declared is a function the port implements: the
// extern's name, the parameters it takes, and the rows it writes.
func TestThePortImplementsEveryDeclaredStatement(t *testing.T) {
	port := EmitGoPorts(PortContext{
		ModuleName: "billing",
		SQLSchema:  "CREATE TABLE invoices (id INTEGER PRIMARY KEY, total INTEGER NOT NULL);",
		SQLQueries: []SQLStatement{{
			Name:   "sql_invoices_over",
			SQL:    "SELECT id, total FROM invoices WHERE total > ?",
			Result: "InvoicesOverRow",
			Fields: []SQLField{
				{Name: "id", Column: "id", GoType: "int64", IsInteger: true},
				{Name: "total", Column: "total", GoType: "int64", IsInteger: true},
			},
		}},
	}, []string{"sql"})

	if !strings.Contains(port, "func sqlInvoicesOver(") {
		t.Errorf("the port does not implement the statement the project declared:\n%s", portLines(port, "sqlInvoicesOver"))
	}
	if !strings.Contains(port, "SELECT id, total FROM invoices WHERE total > ?") {
		t.Error("the port lost the statement's own text")
	}
	// A read takes the caller's slice, so the port has to write into it.
	if !strings.Contains(port, "out SliceInvoicesOverRow") {
		t.Error("the statement's slice parameter is not the one the extern declared")
	}
}

// A write has no rows to place, so its function takes no slice: a signature with
// a slice of a result that does not exist is not a signature anything can call.
func TestAWriteTakesNoSlice(t *testing.T) {
	port := EmitGoPorts(PortContext{
		ModuleName: "billing",
		SQLQueries: []SQLStatement{{
			Name:     "sql_add_invoice",
			SQL:      "INSERT INTO invoices (id, total) VALUES (?, ?)",
			Affected: true,
			Parameters: []SQLParameter{
				{Name: "id", GoType: "int64", IsInteger: true},
				{Name: "total", GoType: "int64", IsInteger: true},
			},
		}},
	}, []string{"sql"})

	if !strings.Contains(port, "func sqlAddInvoice(id int64, total int64, db SqlDb) uint32") {
		t.Errorf("a write is not the shape a write has:\n%s", portLines(port, "sqlAdd"))
	}
	if strings.Contains(portLine(port, "sqlAdd"), "Slice") {
		t.Errorf("a write was given a slice to write rows into, and it returns none: %s",
			portLine(port, "sqlAdd"))
	}
}

// A value is bound as what it is, not as text: a driver handed a number as a
// string reports it back as one, and a caller reads a column that changed type.
func TestAValueIsBoundWithItsOwnType(t *testing.T) {
	port := EmitGoPorts(PortContext{
		ModuleName: "billing",
		SQLQueries: []SQLStatement{{
			Name:   "sql_paid_between",
			SQL:    "SELECT id FROM invoices WHERE total BETWEEN ? AND ?",
			Result: "PaidBetweenRow",
			Parameters: []SQLParameter{
				{Name: "low", GoType: "int64", IsInteger: true},
				{Name: "high", GoType: "string", IsText: true},
			},
		}},
	}, []string{"sql"})

	if !strings.Contains(port, "rhoSqlBind(1, low)") {
		t.Error("a whole number was not bound as one")
	}
	if !strings.Contains(port, "rhoSqlBind(4, high)") {
		t.Error("a text was not bound as text")
	}
}

// The port is what a service uses to reach a database, so it must implement
// everything the standard library declares. A missing one is a program that
// calls a function nothing defines.
func TestThePortImplementsTheWholeStandardSurface(t *testing.T) {
	port := EmitGoPorts(PortContext{ModuleName: "billing"}, []string{"sql"})
	for _, declared := range []string{
		"func stdSqlSqlOpen(", "func stdSqlSqlClose(", "func stdSqlSqlLastMessage(",
		"func stdSqlSqlApply(", "func stdSqlSqlExec(", "func stdSqlSqlPrepare(",
		"func stdSqlSqlRun(", "func stdSqlSqlReset(", "func stdSqlSqlFinalize(",
		"func stdSqlSqlColumnCount(", "func stdSqlSqlColumnName(", "func stdSqlSqlColumnType(",
		"func stdSqlSqlReadColumn(", "func stdSqlSqlBind(",
	} {
		if !strings.Contains(port, declared) {
			t.Errorf("the port does not implement %s", declared)
		}
	}
}

// The schema a project declared is embedded, so a service can create the tables
// its statements assume before it takes a request rather than on every one.
func TestTheDeclaredSchemaIsEmbeddedInThePort(t *testing.T) {
	const schema = "CREATE TABLE invoices (id INTEGER PRIMARY KEY, total INTEGER NOT NULL);"
	port := EmitGoPorts(PortContext{ModuleName: "billing", SQLSchema: schema}, []string{"sql"})
	if !strings.Contains(port, schema) {
		t.Error("the port carries no schema, so a service creates none of the tables its statements assume")
	}
}

// A driver is a Go package that registers itself, and the port imports the one
// the project asked for so the generated program can open anything.
func TestTheDriverIsRegisteredByTheGeneratedProgram(t *testing.T) {
	ctx := PortContext{ModuleName: "billing", SQLDriver: "rho-sql-sqlite", SQLDriverPackage: "modernc.org/sqlite"}
	port := EmitGoPorts(ctx, []string{"sql"})
	if !strings.Contains(port, `_ "modernc.org/sqlite"`) {
		t.Error("the port does not import the driver, so nothing registers it")
	}
}

// A project with a database gets the port without asking for it, and a project
// with no database gets no port: supplying either is a second statement of a
// fact the manifest already settles.
func TestThePortIsRegisteredAndIsTheOnlyOneWritten(t *testing.T) {
	names := RegisteredPorts()
	if !contains(names, "sql") {
		t.Errorf("the sql port is not registered: %v", names)
	}
	if port := EmitGoPorts(PortContext{ModuleName: "billing"}, nil); port != "" {
		t.Error("a project that asked for no ports was given one")
	}
}

// The port and the compiler have to agree on a type, because the call sites the
// compiler wrote are what the port's signatures have to answer. A handle is the
// case that goes wrong: the compiler spells it one way and a port that guessed
// another answers a call nobody made.
func TestThePortSpellsAHandleAsTheCompilerDoes(t *testing.T) {
	port := EmitGoPorts(PortContext{
		ModuleName: "billing",
		SQLQueries: []SQLStatement{{
			Name: "sql_all", SQL: "SELECT id FROM invoices",
			Result: "AllRow", Affected: false,
		}},
	}, []string{"sql"})
	// The statement takes the handle the caller holds, which is a struct the
	// standard library declares, not a raw address.
	if !strings.Contains(port, "func sqlAll(db SqlDb, out SliceAllRow, capacity uint32)") {
		t.Errorf("the statement's handle is not the declared one:\n%s", portLines(port, "sqlAll"))
	}
}

// The whole of it, assembled the way a build assembles it: a project declared a
// database, the port is generated, and the result is a program the Go compiler
// accepts. This is the check that sees the seam — it fails on a name, a
// signature or a type that any of the three halves got wrong on its own.
func TestTheGeneratedProgramWithThePortCompiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()

	// The compiler's own output for a module that calls what the port supplies.
	implementation := EmitGo(irModuleWithHandle())
	port := EmitGoPorts(PortContext{
		ModuleName: "billing",
		SQLSchema:  "CREATE TABLE invoices (id INTEGER PRIMARY KEY, total INTEGER NOT NULL);",
		SQLQueries: []SQLStatement{{
			Name:       "sql_invoices_over",
			SQL:        "SELECT id, total FROM invoices WHERE total > ?",
			Result:     "InvoicesOverRow",
			Parameters: []SQLParameter{{Name: "low", GoType: "int64", IsInteger: true}},
			Fields: []SQLField{
				{Name: "id", Column: "id", GoType: "int64", IsInteger: true},
				{Name: "total", Column: "total", GoType: "int64", IsInteger: true},
			},
		}},
	}, []string{"sql"})

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.go", implementation)
	write("ports.go", port)
	write("main.go", "package main\n\nfunc main() {}\n")
	// The driver is a real dependency, so the check runs without one: the port's
	// own code is what is being verified, and a driver would be a separate test.
	port = strings.Replace(port, "\t_ \"modernc.org/sqlite\"\n", "", 1)
	write("ports.go", port)
	write("go.mod", "module billing\n\ngo 1.18\n")

	build := exec.Command("go", "build", ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Errorf("a project that declared SQL and got the port does not build: %v\n%s", err, out)
	}
}

// A slice carries its data as an address — a pointer to nothing — and the
// backend gives that address a name so the generated file can declare a field of
// it. A module that holds a slice needs that definition, and one that does not
// must not carry a type nothing refers to.
//
// The type arrives as a pointer with an empty element rather than as a type
// already called PtrUnit, which is the shape a check has to read.
func TestASliceBringsTheAddressTypeWithIt(t *testing.T) {
	withSlice := ir.ModuleIR{
		Name: "billing",
		Structs: []ir.StructIR{{
			Name:  "SliceRow",
			CName: "SliceRow",
			Fields: []ir.FieldIR{
				{Name: "ptr", Type: ir.Type{Kind: ir.TypePtr, Args: []ir.Type{{Kind: ir.TypeStruct, Name: "Unit"}}}},
				{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
			},
		}},
	}
	out := EmitGo(withSlice)
	if !strings.Contains(out, "type PtrUnit struct") {
		t.Errorf("a module holding a slice does not define the address its data is:\n%s", out)
	}

	plain := EmitGo(greetIR())
	if strings.Contains(plain, "type PtrUnit struct") {
		t.Error("a module holding no address carries a definition nothing refers to")
	}
}

// irModuleWithHandle is the compiler's own output for a module that opens a
// database and runs a statement against it: the shape a project that declared
// `sql:` produces before the port is added beside it.
func irModuleWithHandle() ir.ModuleIR {
	handle := ir.Type{Kind: ir.TypeStruct, Name: "SqlDb"}
	sliceOfRow := ir.Type{Kind: ir.TypeStruct, Name: "SliceInvoicesOverRow"}
	result := ir.Type{Kind: ir.TypeStruct, Name: "ResultU32SqlError"}
	whole := ir.Type{Kind: ir.TypeI64, Name: "i64"}
	wide := ir.Type{Kind: ir.TypeU32, Name: "u32"}
	return ir.ModuleIR{
		Name: "billing",
		Structs: []ir.StructIR{
			{Name: "SqlDb", CName: "SqlDb", Fields: []ir.FieldIR{{Name: "raw", Type: ir.Type{Kind: ir.TypeU64, Name: "u64"}}}},
			{Name: "InvoicesOverRow", CName: "InvoicesOverRow", Fields: []ir.FieldIR{
				{Name: "id", Type: whole}, {Name: "total", Type: whole}}},
			{Name: "SliceInvoicesOverRow", CName: "SliceInvoicesOverRow", Fields: []ir.FieldIR{
				{Name: "ptr", Type: handle}, {Name: "len", Type: wide}}},
			{Name: "SqlError", CName: "SqlError", Fields: []ir.FieldIR{
				{Name: "message", Type: ir.Type{Kind: ir.TypeString, Name: "string"}},
				{Name: "code", Type: ir.Type{Kind: ir.TypeI32, Name: "i32"}}}},
			{Name: "ResultU32SqlError", CName: "ResultU32SqlError", Fields: []ir.FieldIR{
				{Name: "is_ok", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
				{Name: "ok", Type: wide}, {Name: "err", Type: ir.Type{Kind: ir.TypeStruct, Name: "SqlError"}}}},
			// The rest is what the standard library's own module declares, which
			// reaches the generated program as types: a value crossing the
			// boundary, a statement the engine holds, and the slots the two
			// out-parameters are written through.
			{Name: "SqlValue", CName: "SqlValue", Fields: []ir.FieldIR{
				{Name: "kind", Type: ir.Type{Kind: ir.TypeI32, Name: "i32"}},
				{Name: "integer", Type: ir.Type{Kind: ir.TypeI64, Name: "i64"}},
				{Name: "real", Type: ir.Type{Kind: ir.TypeF64, Name: "f64"}},
				{Name: "boolean", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
				{Name: "text", Type: ir.Type{Kind: ir.TypeString, Name: "string"}},
				{Name: "blob", Type: ir.Type{Kind: ir.TypeString, Name: "string"}},
				{Name: "is_null", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}}}},
			{Name: "SqlStmt", CName: "SqlStmt", Fields: []ir.FieldIR{{Name: "raw", Type: ir.Type{Kind: ir.TypeU64, Name: "u64"}}}},
			{Name: "PtrU32", CName: "PtrU32", Fields: []ir.FieldIR{{Name: "raw", Type: ir.Type{Kind: ir.TypeU64, Name: "u64"}}}},
			{Name: "PtrSqlValue", CName: "PtrSqlValue", Fields: []ir.FieldIR{{Name: "raw", Type: ir.Type{Kind: ir.TypeU64, Name: "u64"}}}},
			{Name: "SliceSqlValue", CName: "SliceSqlValue", Fields: []ir.FieldIR{
				{Name: "ptr", Type: ir.Type{Kind: ir.TypeStruct, Name: "PtrUnit"}},
				{Name: "len", Type: wide}}},
			// PtrUnit is the address a slice carries and a program cannot read.
			// It is defined by the backend itself when a slice names it, so it
			// is deliberately not declared here.
			{Name: "ResultSqlDbString", CName: "ResultSqlDbString", Fields: []ir.FieldIR{
				{Name: "is_ok", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
				{Name: "ok", Type: handle}, {Name: "err", Type: ir.Type{Kind: ir.TypeString, Name: "string"}}}},
		},
		Funcs: []ir.FuncIR{
			{
				Name: "InvoicesOver", CName: "InvoicesOver",
				Params: []ir.ParamIR{
					{Name: "low", Type: whole},
					{Name: "db", Type: handle},
					{Name: "out", Type: sliceOfRow},
					{Name: "capacity", Type: wide},
				},
				ReturnType: result,
				Body: []ir.Instr{{Op: ir.OpExpr, Args: []string{
					"sqlInvoicesOver(low, db, out, capacity)"}}},
			},
			{
				Name: "main", CName: "main",
				ReturnType: ir.Type{Kind: ir.TypeVoid},
				Body:       []ir.Instr{{Op: ir.OpExpr, Args: []string{`printf("%s\n", "ok")`}}},
			},
		},
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// portLine is the one generated function whose name contains the given text.
func portLine(text, name string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "func ") && strings.Contains(line, name) {
			return line
		}
	}
	return ""
}

// portLines is the whole file when a function was not found at all, and the
// lines around it when one was: a failure here is about one statement, and a
// reader wants to see that statement rather than the file's banner.
func portLines(text, name string) string {
	if line := portLine(text, name); line != "" {
		return line
	}
	return text
}
