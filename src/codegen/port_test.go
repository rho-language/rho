//
// @file port_test.go
// @brief Tests for the built-in ports a generated project can request.
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

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Registry                                                                  */
/* -------------------------------------------------------------------------- */

// The ports a descriptor may name have to exist, so a manifest cannot ask for
// something the backend would not generate.
func TestBuiltInPortsAreRegistered(t *testing.T) {
	for _, name := range []string{"docemit", "fsstore", "logemit"} {
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
	imports, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	out := strings.Join(imports, " ") + "\n" + body
	if !strings.Contains(out, "func emit(level uint32, subject string, message string)") {
		t.Errorf("expected emit with the declared signature, got:\n%s", out)
	}
}

// Every level the library can pass has to have a name, or a record would read
// as info when it was not.
func TestLogPortNamesEveryLevel(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	for _, level := range []string{"trace", "debug", "info", "warn", "error", "fatal"} {
		if !strings.Contains(body, `"`+level+`"`) {
			t.Errorf("expected a name for the %s level", level)
		}
	}
}

// A record goes to standard error, so a program's own standard output stays
// machine readable.
func TestLogPortWritesToStandardError(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	if !strings.Contains(body, "os.Stderr") {
		t.Errorf("expected records on standard error, got:\n%s", body)
	}
}

// The line has to match the formatter's shape, so a record reads the same
// whether it came from the command or from a service.
func TestLogPortMatchesTheFormatterShape(t *testing.T) {
	_, body := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	if !strings.Contains(body, "[%s] [%s] [%s] %s") {
		t.Errorf("expected the formatter's shape, got:\n%s", body)
	}
}

func TestLogPortImportsWhatItUses(t *testing.T) {
	imports, _ := emitLogPort(PortContext{IR: ir.ModuleIR{Name: "app"}})
	joined := strings.Join(imports, " ")
	for _, want := range []string{"fmt", "os", "strings", "time"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected the %q import, got %s", want, joined)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Documentation port                                                        */
/* -------------------------------------------------------------------------- */

// A project that imports the documentation library has to reach the generator,
// so the port has to carry the entry points the library declares.
func TestDocPortCarriesTheLibraryInterface(t *testing.T) {
	body := EmitGoPorts(PortContext{IR: ir.ModuleIR{Name: "app", Version: "0.1.0"}}, []string{"docemit"})
	// The port supplies the names the Go backend calls: a library's externs are
	// wrapped as <library><name>, and the wrapper around them is generated.
	for _, want := range []string{
		"func rhoDocRender(source string, path string) string",
		"func rhoDocBuildSite(root string, title string) string",
		"func rhoDocGenerateApi(root string) string",
		"func rhoDocCheck(root string) string",
		"func rhoDocWriteSite(root string, out string, title string) uint32",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the port to carry %s", want)
		}
	}
	// The generator is a component, so the port reaches it as a process rather
	// than carrying a copy of it.
	if !strings.Contains(body, `"--fragment"`) && !strings.Contains(body, `doc", "--fragment"`) {
		t.Errorf("expected the port to reach the component as a process")
	}
}

/* -------------------------------------------------------------------------- */
/*  Documentation side of the store                                            */
/* -------------------------------------------------------------------------- */

// The registry documents a module from what it received, so the store has to
// hand the generator a source tree and read the pages back.
func TestStorePortCarriesTheDocumentationOperations(t *testing.T) {
	body := EmitGoPorts(PortContext{IR: ir.ModuleIR{Name: "registry", Version: "0.1.0"}}, []string{"fsstore"})

	for _, want := range []string{
		"func storeUnpack(name, version string) string",
		"func storeDocPage(name, version, page string) string",
		"func storeHasDocs(name, version string) bool",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the store port to carry %s", want)
		}
	}
	// Unpacking a package is a filesystem read of an archive, so the archive
	// packages have to be imported by the generated file.
	for _, want := range []string{`"archive/tar"`, `"compress/gzip"`, `"io"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the store port to import %s", want)
		}
	}
}

func TestStorePortRefusesAPackageEntryThatLeavesTheDirectory(t *testing.T) {
	// A package is a file list, and a file list that climbs out of the directory
	// it is unpacked into would write wherever it names.
	body := EmitGoPorts(PortContext{IR: ir.ModuleIR{Name: "registry", Version: "0.1.0"}}, []string{"fsstore"})

	if !strings.Contains(body, "leaves the package directory") {
		t.Error("expected the unpack to refuse an entry outside the package directory")
	}
	if !strings.Contains(body, "func withinDir(dir, path string) bool") {
		t.Error("expected the containment check to be present")
	}
}
