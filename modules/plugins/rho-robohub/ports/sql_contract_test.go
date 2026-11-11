package main

import "database/sql"

// What the telemetry port needs from the sql port.
//
// The telemetry port is written against the sql port, which the generator merges
// into the same file and which defines these. They are declared here, in a test
// file, so the port's source compiles and is type-checked where it lives and an
// editor can follow it. They are never generated: the generator reads only the
// non-test files in this directory, and the real definitions are the sql port's.
// If the sql port changes one of these signatures, the real build breaks and so
// does this file's copy once it is brought in line.

type SqlDb struct{ raw uint64 }

type ResultSqlDbString struct {
	isOk bool
	ok   SqlDb
}

type rhoSqlStatementBatch struct {
	SQL  string
	Rows [][]any
}

var rhoSqlLastErrMsg string

func stdSqlSqlOpen(path string) ResultSqlDbString                                     { return ResultSqlDbString{} }
func stdSqlSqlApply(db SqlDb, schema string) uint32                                   { return 0 }
func rhoSqlExecBatch(db SqlDb, name string, statements []rhoSqlStatementBatch) uint32 { return 0 }
func rhoSqlHandle(raw uint64) (*sql.DB, bool)                                         { return nil, false }
