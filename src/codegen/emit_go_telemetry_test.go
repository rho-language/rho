//
// @file emit_go_telemetry_test.go
// @brief Tests for the WAL, retention and fault features of the telemetry port.
//
// The port is emitted as source; the generated project's own tests exercise the
// SQLite behaviour. This test keeps the contract the Rho side depends on from
// disappearing: bounded WAL, batched pruning, vacuum, and the fault table.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"
)

func TestTelemetryPortMaintenanceAndFaultsAreEmitted(t *testing.T) {
	for _, want := range []string{
		"func telemetryCheckpoint(mode string) string",
		"PRAGMA wal_checkpoint(",
		"DELETE FROM \"+table+\" WHERE rowid IN (",
		"LIMIT 5000",
		"func telemetryVacuum(mode string) string",
		"PRAGMA incremental_vacuum",
		"func telemetryStorageStatus(project string) string",
		"telemetry_fault",
		"func telemetryFaultMaxId(project string) uint32",
		"func telemetryFaultNew(project string, afterID uint32, limit uint32) string",
		"func telemetryFaultList(project string, sinceText string, limit uint32) string",
		"func telemetryFaultClear(project string, id uint32) string",
		"func telemetryFaultPrune(project string, days uint32) string",
		"func telemetryFaultRows(project string, limit uint32) string",
	} {
		if !strings.Contains(telemetryPortBody, want) {
			t.Errorf("the telemetry port does not contain %q", want)
		}
	}
}
