//
// @file port_test.go
// @brief Tests for the built-in ports a generated project can request.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Registry                                                                  */
/* -------------------------------------------------------------------------- */

// The ports a descriptor may name have to exist, so a manifest cannot ask for
// something the backend would not generate.
func TestBuiltInPortsAreRegistered(t *testing.T) {
	for _, name := range []string{"fsstore", "logemit"} {
		if !HasPort(name) {
			t.Errorf("expected %q to be a built-in port", name)
		}
	}
	if got := RegisteredPorts(); len(got) < 2 {
		t.Errorf("expected the built-in ports to be listed, got %v", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Logging port                                                              */
/* -------------------------------------------------------------------------- */

// The logging port supplies the function the library declares, which is the
// whole point: the project gets it without copying anything.
func TestLogPortSuppliesEmit(t *testing.T) {
	imports, body := emitLogPort(ir.ModuleIR{Name: "app"})
	out := strings.Join(imports, " ") + "\n" + body
	if !strings.Contains(out, "func emit(level uint32, subject string, message string)") {
		t.Errorf("expected emit with the declared signature, got:\n%s", out)
	}
}

// Every level the library can pass has to have a name, or a record would read
// as info when it was not.
func TestLogPortNamesEveryLevel(t *testing.T) {
	_, body := emitLogPort(ir.ModuleIR{Name: "app"})
	for _, level := range []string{"trace", "debug", "info", "warn", "error", "fatal"} {
		if !strings.Contains(body, `"`+level+`"`) {
			t.Errorf("expected a name for the %s level", level)
		}
	}
}

// A record goes to standard error, so a program's own standard output stays
// machine readable.
func TestLogPortWritesToStandardError(t *testing.T) {
	_, body := emitLogPort(ir.ModuleIR{Name: "app"})
	if !strings.Contains(body, "os.Stderr") {
		t.Errorf("expected records on standard error, got:\n%s", body)
	}
}

// The line has to match the formatter's shape, so a record reads the same
// whether it came from the command or from a service.
func TestLogPortMatchesTheFormatterShape(t *testing.T) {
	_, body := emitLogPort(ir.ModuleIR{Name: "app"})
	if !strings.Contains(body, "[%s] [%s] [%s] %s") {
		t.Errorf("expected the formatter's shape, got:\n%s", body)
	}
}

func TestLogPortImportsWhatItUses(t *testing.T) {
	imports, _ := emitLogPort(ir.ModuleIR{Name: "app"})
	joined := strings.Join(imports, " ")
	for _, want := range []string{"fmt", "os", "strings", "time"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected the %q import, got %s", want, joined)
		}
	}
}
