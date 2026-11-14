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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// A statement forgets its handle's last failure, records its own, and the
	// generated wrapper reads it back: zero rows and a failure are told apart.
	body := portLines(port, "sqlInvoicesOver")
	for _, want := range []string{"rhoSqlStatementBegin(db)", "return rhoSqlStatementFail(db, err)", "rows.Err()"} {
		if !strings.Contains(port, want) {
			t.Errorf("the statement does not %s:\n%s", want, body)
		}
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

	if !strings.Contains(port, "func sqlAddInvoice(argId int64, argTotal int64, db SqlDb) uint32") {
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

	if !strings.Contains(port, "rhoSqlBind(1, argLow)") {
		t.Error("a whole number was not bound as one")
	}
	if !strings.Contains(port, "rhoSqlBind(4, argHigh)") {
		t.Error("a text was not bound as text")
	}
}

// The port is what a service uses to reach a database, so it must implement
// everything the standard library declares. A missing one is a program that
// calls a function nothing defines.
func TestThePortImplementsTheWholeStandardSurface(t *testing.T) {
	port := EmitGoPorts(PortContext{ModuleName: "billing"}, []string{"sql"})
	for _, declared := range []string{
		"func stdSqlSqlOpen(", "func stdSqlSqlClose(", "func stdSqlSqlLastMessage(", "func stdSqlSqlFailed(",
		"func stdSqlSqlApply(", "func stdSqlSqlExec(", "func stdSqlSqlExecMany(",
		"func stdSqlSqlPrepare(", "func stdSqlSqlRun(", "func stdSqlSqlReset(", "func stdSqlSqlFinalize(",
		"func stdSqlSqlColumnCount(", "func stdSqlSqlColumnName(", "func stdSqlSqlColumnType(",
		"func stdSqlSqlReadColumn(", "func stdSqlSqlBind(",
	} {
		if !strings.Contains(port, declared) {
			t.Errorf("the port does not implement %s", declared)
		}
	}
}

// A batch is one statement prepared once and executed per row inside one
// transaction. The port must expose the whole of that, not a per-row call the
// caller would have to wrap itself.
func TestThePortImplementsBatchExecution(t *testing.T) {
	port := EmitGoPorts(PortContext{ModuleName: "billing"}, []string{"sql"})
	if !strings.Contains(port, "func stdSqlSqlExecMany(db SqlDb, name string, query string, columns uint32, values SliceSqlValue, affected unsafe.Pointer) uint32") {
		t.Error("the port does not implement sql_exec_many")
	}
	for _, part := range []string{"handle.Begin()", "transaction.Prepare(", "transaction.Commit()", "transaction.Rollback()"} {
		if !strings.Contains(port, part) {
			t.Errorf("the batch path does not contain %s", part)
		}
	}
}

// Looking a handle up already takes the lock, so a caller that takes it first
// would wait on itself forever. The generated port is checked at the source
// level because the mistake is a deadlock rather than a wrong value, and a
// test that ran it would time out instead of reporting what is wrong.
func TestAStatementDoesNotLockAroundTheHandleLookup(t *testing.T) {
	port := EmitGoPorts(PortContext{ModuleName: "billing"}, []string{"sql"})
	if strings.Contains(port, "rhoSqlMu.Lock()\n\thandle, ok := rhoSqlHandle(") {
		t.Error("a statement takes the handle-table lock and then asks for the handle, which locks it again")
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
			// A parameter named like one of the generated function's own
			// locals: the port's names must not collide with a column's.
			Parameters: []SQLParameter{{Name: "handle", GoType: "int64", IsInteger: true}},
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

// The batch path is exercised against a driver that can refuse a row, because
// the property that matters is not that the call compiles: it is that a failure
// in the middle leaves none of the earlier rows committed.
func TestABatchIsOneTransactionAndRollsBackOnFailure(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("ports.go", EmitGoPorts(PortContext{ModuleName: "billing"}, []string{"sql"}))
	write("types.go", `package main

import "unsafe"

type SqlDb struct{ raw uint64 }
type SqlStmt struct{ raw uint64 }
type SqlValue struct {
	kind    int32
	integer int64
	real    float64
	boolean bool
	text    string
	blob    string
	isNull  bool
}
type SliceSqlValue struct {
	ptr unsafe.Pointer
	len uint32
}
type ResultSqlDbString struct {
	isOk bool
	ok   SqlDb
	err  string
}
`)
	// A driver that records what committed and refuses the sentinel 99. It is
	// enough of database/sql to exercise the port's own transaction handling,
	// and it keeps the test free of a database dependency.
	write("driver.go", `package main

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

type fakeDriver struct{}
type fakeConn struct {
	pending   int
	committed int
	executed  []string
}
type fakeStmt struct {
	conn  *fakeConn
	query string
}
type fakeTx struct{ conn *fakeConn }

var fake *fakeConn

func (fakeDriver) Open(string) (driver.Conn, error) { fake = &fakeConn{}; return fake, nil }
func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{conn: c, query: query}, nil
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return &fakeTx{conn: c}, nil }
func (s *fakeStmt) Close() error { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.conn.executed = append(s.conn.executed, s.query)
	for _, arg := range args {
		if n, ok := arg.(int64); ok && n == 99 {
			return nil, errors.New("refused")
		}
	}
	s.conn.pending++
	return driver.RowsAffected(1), nil
}
func (s *fakeStmt) Query([]driver.Value) (driver.Rows, error) { return nil, errors.New("no rows") }
func (t *fakeTx) Commit() error {
	t.conn.committed += t.conn.pending
	t.conn.pending = 0
	return nil
}
func (t *fakeTx) Rollback() error {
	t.conn.pending = 0
	return nil
}

func init() { sql.Register("fake", fakeDriver{}) }
`)
	write("main.go", `package main

import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

func sliceOf(values []SqlValue) SliceSqlValue {
	if len(values) == 0 {
		return SliceSqlValue{}
	}
	return SliceSqlValue{
		ptr: unsafe.Pointer(&values[0]),
		len: uint32(len(values)),
	}
}

func main() {
	opened := stdSqlSqlOpen("fake.db")
	if !opened.isOk {
		fmt.Println("open failed:", opened.err)
		os.Exit(1)
	}
	db := opened.ok

	schema := "PRAGMA journal_mode=WAL; CREATE TABLE t (id INTEGER); CREATE INDEX i ON t(id);"
	if applied := stdSqlSqlApply(db, schema); applied != 3 {
		fmt.Println("schema apply did not run every statement:", applied)
		os.Exit(1)
	}
	if len(fake.executed) < 3 ||
		!strings.Contains(fake.executed[0], "PRAGMA") ||
		!strings.Contains(fake.executed[1], "CREATE TABLE") ||
		!strings.Contains(fake.executed[2], "CREATE INDEX") {
		fmt.Println("schema apply did not pass every statement through:", fake.executed)
		os.Exit(1)
	}
	fake.executed = nil
	fake.pending = 0
	fake.committed = 0

	good := []SqlValue{
		{kind: 1, integer: 1}, {kind: 4, text: "a"},
		{kind: 1, integer: 2}, {kind: 4, text: "b"},
	}
	var affectedCount uint32
	affected := unsafe.Pointer(&affectedCount)
	ok := stdSqlSqlExecMany(db, "insert", "INSERT INTO t (id, name) VALUES (?, ?)", 2, sliceOf(good), affected)
	if ok != 1 || affectedCount != 2 || fake.committed != 2 {
		fmt.Printf("batch failed: ok=%d affected=%d committed=%d err=%s\n",
			ok, affectedCount, fake.committed, rhoSqlLastErrMsg)
		os.Exit(1)
	}

	before := fake.committed
	bad := []SqlValue{
		{kind: 1, integer: 3}, {kind: 4, text: "c"},
		{kind: 1, integer: 99}, {kind: 4, text: "d"},
	}
	var rollbackCount uint32
	rollbackAffected := unsafe.Pointer(&rollbackCount)
	ok = stdSqlSqlExecMany(db, "insert", "INSERT INTO t (id, name) VALUES (?, ?)", 2, sliceOf(bad), rollbackAffected)
	if ok != 0 || rollbackCount != 0 || fake.committed != before {
		fmt.Printf("rollback failed: ok=%d affected=%d committed=%d err=%s\n",
			ok, rollbackCount, fake.committed, rhoSqlLastErrMsg)
		os.Exit(1)
	}
	fmt.Println("PASS")
}
`)
	write("go.mod", "module batchtest\n\ngo 1.18\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := exec.CommandContext(ctx, "go", "run", ".")
	run.Dir = root
	run.Env = append(os.Environ(), "GOWORK=off", "RHO_SQL_DRIVER=fake")
	out, err := run.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("the batch program did not finish: %v\n%s", ctx.Err(), out)
	}
	if err != nil {
		t.Fatalf("the batch program failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("the batch program did not pass:\n%s", out)
	}
}

// A slice carries its data as a pointer, and every pointer is an
// unsafe.Pointer on the Go backend: a module holding a slice imports the package
// its field names, and defines no address type of its own.
func TestASliceCarriesItsDataAsAnUnsafePointer(t *testing.T) {
	withSlice := ir.ModuleIR{
		Name: "billing",
		Structs: []ir.StructIR{{
			Name:  "SliceRow",
			CName: "SliceRow",
			Fields: []ir.FieldIR{
				{Name: "ptr", Type: pointerTo(ir.Type{Kind: ir.TypeStruct, Name: "Row"})},
				{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
			},
		}},
	}
	out := EmitGo(withSlice)
	if !strings.Contains(out, "ptr unsafe.Pointer") || !strings.Contains(out, `"unsafe"`) {
		t.Errorf("a slice's data is not an unsafe.Pointer with its import:\n%s", out)
	}
	if strings.Contains(out, "PtrUnit") {
		t.Errorf("a pointer has one spelling, and it is not a struct:\n%s", out)
	}
}

// pointerTo is the one pointer type, `Ptr[T]`, at an element.
func pointerTo(element ir.Type) ir.Type {
	return ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + element.Name + "]", Args: []ir.Type{element}}
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
				{Name: "ptr", Type: pointerTo(ir.Type{Kind: ir.TypeStruct, Name: "InvoicesOverRow"})}, {Name: "len", Type: wide}}},
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
			{Name: "SliceSqlValue", CName: "SliceSqlValue", Fields: []ir.FieldIR{
				{Name: "ptr", Type: pointerTo(ir.Type{Kind: ir.TypeStruct, Name: "SqlValue"})},
				{Name: "len", Type: wide}}},
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

// The telemetry port is written against the sql port, so the two have to be
// generated together and the result has to compile. This is the seam a project
// with a `telemetry` port and a `sql:` block gets.
func TestTheTelemetryPortCompilesWithTheSQLPort(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("types.go", `package main

import "unsafe"

type SqlDb struct{ raw uint64 }
type SqlStmt struct{ raw uint64 }
type SqlValue struct {
	kind    int32
	integer int64
	real    float64
	boolean bool
	text    string
	blob    string
	isNull  bool
}
type SliceSqlValue struct {
	ptr unsafe.Pointer
	len uint32
}
type ResultSqlDbString struct {
	isOk bool
	ok   SqlDb
	err  string
}
`)
	write("ports.go", EmitGoPorts(PortContext{
		ModuleName: "mesh",
		SQLSchema:  "CREATE TABLE telemetry_reading (project TEXT, area_id INTEGER, node_id INTEGER, field TEXT, value REAL, status INTEGER, received_ms INTEGER);",
	}, []string{"sql", "telemetry"}))
	write("main.go", "package main\n\nfunc main() {}\n")
	write("go.mod", "module telemetrytest\n\ngo 1.18\n")

	build := exec.Command("go", "build", ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the telemetry port does not compile with the sql port: %v\n%s", err, out)
	}
}
