//
// @file parity_std_sql_test.go
// @brief std.sql on the backend it is for: Go, through the sql port.
//
// std.sql declares the database surface as extern fn, and the Go backend's sql
// port implements it over database/sql (`@backends go`, `@port sql`). The
// toolchain has no C implementation, so the module is tested on Go only: a
// program that opens a database, applies a schema, writes a row, writes a batch,
// fails on a duplicate key, prepares, binds, walks the rows and reads every kind
// of column, then rewinds and runs again — against a real SQLite, through the
// same port a service declaring `sql:` gets. The refusal of the same program on
// C, and on a Go target without the port, is in backends_test.go.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const sqlStdManifest = `module:
  name: sqlstd
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: go
  targets:
    - go
`

// sqlStdDescriptor gives the go target the sql port, which is how a project asks
// for the port without declaring generated statements.
const sqlStdDescriptor = `version: 1
output_dir: build/generated
build_dir: build
targets:
  - id: go
    emitter: go
    kind: cli
    ports:
      - sql
    files:
      - path: app.go
        role: implementation
      - path: runtime.go
        role: runtime
      - path: main.go
        role: entrypoint
      - path: go.mod
        role: build
`

const sqlStdSource = `module sqlstd

import std.core.ptr
import std.core.result
import std.core.slice
import std.sql

fn main() void {
    let opened: Result[SqlDb, string] = sql_open(getenv("SQL_TEST_DB"))
    if !opened.is_ok {
        print("open failed: " + opened.err)
        return
    }
    let db: SqlDb = opened.ok
    let made: u32 = sql_apply(db, "CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT, score REAL, ok BOOLEAN, note TEXT);")
    print("apply=" + int_to_str(made))

    var row: SqlValue[5] = [int_value(1), text_value("one"), float_value(1.5), bool_value(true), null_value()]
    var affected: u32 = 0
    let status: u32 = sql_exec(db, "insert", "INSERT INTO t (id, name, score, ok, note) VALUES (?, ?, ?, ?, ?)", Slice[SqlValue]{ Ptr[SqlValue](&row[0]), 5 }, &affected)
    print(" exec=" + int_to_str(status) + "/" + int_to_str(affected))

    var rows: SqlValue[4] = [int_value(2), text_value("two"), int_value(3), text_value("three")]
    var many: u32 = 0
    let batch: u32 = sql_exec_many(db, "insert2", "INSERT INTO t (id, name) VALUES (?, ?)", 2, Slice[SqlValue]{ Ptr[SqlValue](&rows[0]), 4 }, &many)
    print(" many=" + int_to_str(batch) + "/" + int_to_str(many))

    // A duplicate key fails, and says why.
    var dup: SqlValue[2] = [int_value(1), text_value("again")]
    var none: u32 = 0
    let refused: u32 = sql_exec(db, "dup", "INSERT INTO t (id, name) VALUES (?, ?)", Slice[SqlValue]{ Ptr[SqlValue](&dup[0]), 2 }, &none)
    print(" dup=" + int_to_str(refused))
    if str_contains(str_lower(sql_last_message(db)), "unique") {
        print(" unique")
    }

    var stmt: SqlStmt = SqlStmt{ 0 }
    let prepared: u32 = sql_prepare(db, "SELECT id, name, score, ok, note FROM t WHERE id >= ? ORDER BY id", &stmt)
    print(" prepare=" + int_to_str(prepared))
    sql_bind(stmt, 0, int_value(1))
    var more: u32 = 0
    sql_run(stmt, &more)
    // The shape is known once the statement has run.
    print(" cols=" + int_to_str(sql_column_count(stmt)) + " " + sql_column_name(stmt, 1) + " " + str_lower(sql_column_type(stmt, 0)))
    while more != 0 {
        var id: SqlValue = null_value()
        var name: SqlValue = null_value()
        var score: SqlValue = null_value()
        var flag: SqlValue = null_value()
        var note: SqlValue = null_value()
        sql_read_column(stmt, 0, &id)
        sql_read_column(stmt, 1, &name)
        sql_read_column(stmt, 2, &score)
        sql_read_column(stmt, 3, &flag)
        sql_read_column(stmt, 4, &note)
        print(" [" + i64_to_str(id.integer) + " " + name.text)
        if !is_null(score) {
            print(" " + i64_to_str(i64(score.real * 10.0)))
        }
        if !is_null(flag) && (flag.boolean || flag.integer == 1) {
            print(" yes")
        }
        if is_null(note) {
            print(" null")
        }
        print("]")
        sql_run(stmt, &more)
    }
    // Reset, bind again and run again.
    sql_reset(stmt)
    sql_bind(stmt, 0, int_value(3))
    sql_run(stmt, &more)
    var last: SqlValue = null_value()
    sql_read_column(stmt, 1, &last)
    print(" again=" + last.text)
    sql_run(stmt, &more)
    print(" more=" + int_to_str(more))
    sql_finalize(stmt)
    print(" kinds=" + i64_to_str(i64(kind_null())) + i64_to_str(i64(kind_integer())) + i64_to_str(i64(kind_float())) + i64_to_str(i64(kind_boolean())) + i64_to_str(i64(kind_text())) + i64_to_str(i64(kind_blob())))
    let r: Result[u32, string] = ok[u32](3, 0, "")
    let f: Result[u32, string] = ok[u32](0, 1, "broke")
    print(" ok=" + int_to_str(r.ok) + " err=" + f.err)
    sql_close(db)
}
`

// sqlStdDriver is the database the generated program opens: the SQLite driver
// the rho-sql-sqlite plugin names, registered as "sqlite".
const sqlStdDriver = "package main\n\nimport _ \"modernc.org/sqlite\"\n"

func TestStdSqlThroughTheSqlPort(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		filepath.Join(root, "module.yml"):  sqlStdManifest,
		filepath.Join(root, "project.yml"): sqlStdDescriptor,
		entry:                              sqlStdSource,
	} {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	goDir := dirs[0]
	if err := os.WriteFile(filepath.Join(goDir, "driver.go"), []byte(sqlStdDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	get := exec.Command("go", "get", "modernc.org/sqlite@v1.60.0")
	get.Dir = goDir
	get.Env = env
	if out, err := get.CombinedOutput(); err != nil {
		t.Skipf("the SQLite driver could not be fetched, so there is no database to run against: %v\n%s", err, out)
	}
	binary := filepath.Join(t.TempDir(), "sqlstd")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = goDir
	build.Env = env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated program with the sql port did not build: %v\n%s", err, out)
	}
	run := exec.Command(binary)
	run.Dir = t.TempDir()
	run.Env = append(os.Environ(), "RHO_SQL_DRIVER=sqlite", "SQL_TEST_DB="+filepath.Join(run.Dir, "std.db"))
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("the program failed: %v\n%s", err, out)
	}
	want := "apply=1 exec=1/1 many=1/2 dup=0 unique prepare=1 cols=5 name integer [1 one 15 yes null] [2 two null] [3 three null] again=three more=0 kinds=012345 ok=3 err=broke"
	if string(out) != want {
		t.Fatalf("std.sql printed\n%s\nwant\n%s", out, want)
	}
}
