//
// @file logport_test.go
// @brief The logging port's configuration and formatting contract.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// An application has to be able to change its own level, format and file at
// run time; the settings page calls this entry point.
func TestLogPortSuppliesConfigure(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	for _, want := range []string{
		"func log_configure(level uint32, format string, path string, maxBytes int64,",
		"rhoLogState.min = level",
		"rhoLogState.format = value",
		"rhoLogState.stderr = stderr != 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in the log port, got:\n%s", want, body)
		}
	}
}

// The environment is the operator and test override, and every one of its
// names has to be read.
func TestLogPortReadsTheEnvironment(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	for _, want := range []string{
		"RHO_LOG_LEVEL", "RHO_LOG_FORMAT", "RHO_LOG_FILE",
		"RHO_LOG_MAX_BYTES", "RHO_LOG_KEEP", "RHO_LOG_STDERR",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s in the log port", want)
		}
	}
}

// Text and JSON are both supported, and the file rotates itself so an operator
// does not need an external logrotate rule.
func TestLogPortSupportsJsonAndRotation(t *testing.T) {
	imports, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	joined := strings.Join(imports, " ")
	if !strings.Contains(joined, "encoding/json") {
		t.Errorf("expected encoding/json, got %s", joined)
	}
	for _, want := range []string{
		`json:"time"`, `json:"level"`, `json:"subject"`, `json:"message"`,
		"rhoLogRotateLocked",
		`rhoLogState.path+".1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in the log port", want)
		}
	}
}

// A level below the configured minimum is dropped before any sink sees it.
func TestLogPortFiltersBelowTheMinimum(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	if !strings.Contains(body, "if level < rhoLogState.min") {
		t.Errorf("expected level filtering, got:\n%s", body)
	}
}
