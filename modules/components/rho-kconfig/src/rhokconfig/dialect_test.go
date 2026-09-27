//
// @file dialect_test.go
// @brief Tests for writing a configuration in a dialect.
//
// The same symbols are spelled differently by the tools that read them, and a
// project is built by one of those tools, so a dialect is a writer and a wrong
// writer is a firmware that asks a build for the wrong thing. These tests check
// what each dialect writes, and where it writes it, because where a header goes
// is as much a part of the contract as what is in it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhokconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFileAt reads a file a test wrote, so a test can ask what is on disk rather
// than what a function returned.
func readFileAt(root, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	return string(data), err
}

// resolved is a configuration with a flag on, a flag off, a number and a string.
func resolved() Configuration {
	return Configuration{Values: []Value{
		{Name: "CONFIG_SPI_MASTER", Type: TypeBool, Value: "y", From: "defconfig"},
		{Name: "CONFIG_SPI_SLAVE", Type: TypeBool, Value: "n", From: "default"},
		{Name: "CONFIG_UART_BAUD", Type: TypeInt, Value: "115200", From: "module.yml"},
		{Name: "CONFIG_UART_TX_PIN", Type: TypeHex, Value: "0x25", From: "default"},
		{Name: "CONFIG_LOG_LEVEL", Type: TypeString, Value: "info", From: "default"},
	}}
}

func fileNamed(t *testing.T, files []File, path string) string {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("expected %s among %+v", path, pathsOf(files))
	return ""
}

func pathsOf(files []File) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Path)
	}
	return out
}

func TestTheLinuxDialectWritesThePairAKernelBuildReads(t *testing.T) {
	files := linuxDialect{}.Files(resolved())

	config := fileNamed(t, files, ".config")
	if !strings.Contains(config, "CONFIG_SPI_MASTER=y") {
		t.Errorf("expected a set flag to be written as y, got:\n%s", config)
	}
	if !strings.Contains(config, "CONFIG_SPI_SLAVE=n") {
		t.Errorf("expected an unset flag to be written as n, got:\n%s", config)
	}
	if !strings.Contains(config, "CONFIG_UART_BAUD=115200") {
		t.Errorf("expected the number, got:\n%s", config)
	}

	// The header goes where a driver includes it from: include/generated.
	header := fileNamed(t, files, "include/generated/autoconf.h")
	if !strings.Contains(header, "#define CONFIG_SPI_MASTER 1") {
		t.Errorf("expected a flag on to be 1, got:\n%s", header)
	}
	if !strings.Contains(header, "#define CONFIG_SPI_SLAVE 0") {
		t.Errorf("expected a flag off to be 0, got:\n%s", header)
	}
	if !strings.Contains(header, "#define CONFIG_UART_BAUD 115200") {
		t.Errorf("expected the number, got:\n%s", header)
	}
	if !strings.Contains(header, "#define CONFIG_LOG_LEVEL \"info\"") {
		t.Errorf("expected a string to be quoted, got:\n%s", header)
	}
	if !strings.Contains(header, "from module.yml") {
		t.Errorf("expected the header to say where a value came from, got:\n%s", header)
	}
}

func TestTheESPIDFDialectWritesThePairESPIDFReads(t *testing.T) {
	files := espIDFDialect{}.Files(resolved())

	config := fileNamed(t, files, "sdkconfig")
	if !strings.Contains(config, "CONFIG_SPI_MASTER=y") {
		t.Errorf("expected the ESP-IDF spelling of a flag, got:\n%s", config)
	}
	// ESP-IDF reads its header from build/config, which is where its build
	// system looks for it.
	header := fileNamed(t, files, "build/config/sdkconfig.h")
	if !strings.Contains(header, "#define CONFIG_SPI_MASTER 1") {
		t.Errorf("expected a flag on to be 1, got:\n%s", header)
	}
	if !strings.Contains(header, "#pragma once") {
		t.Errorf("expected the header to be includable, got:\n%s", header)
	}
}

func TestTheFreeRTOSDialectWritesTheHeaderFreeRTOSReads(t *testing.T) {
	// FreeRTOSConfig.h is a list of defines with no CONFIG_ prefix, and a
	// define of 0 is a different statement from no define, so an unset flag is
	// not written.
	files := freeRTOSDialect{}.Files(resolved())

	header := fileNamed(t, files, "FreeRTOSConfig.h")
	if !strings.Contains(header, "#define SPI_MASTER 1") {
		t.Errorf("expected the unprefixed spelling of a set flag, got:\n%s", header)
	}
	if strings.Contains(header, "SPI_SLAVE") {
		t.Errorf("expected an unset flag not to be written, got:\n%s", header)
	}
	if !strings.Contains(header, "#define UART_BAUD 115200") {
		t.Errorf("expected the number, got:\n%s", header)
	}
	// The .config is the record the header was generated from.
	if config := fileNamed(t, files, ".config"); !strings.Contains(config, "CONFIG_SPI_SLAVE=n") {
		t.Errorf("expected the record to keep every symbol, got:\n%s", config)
	}
}

func TestAnUnknownDialectIsRefusedWithTheOnesThatExist(t *testing.T) {
	_, err := dialectByName("zephyr")
	if err == nil {
		t.Fatal("expected an unknown dialect to be refused")
	}
	for _, name := range DialectNames() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("expected %q to be named as an option, got %v", name, err)
		}
	}
}

func TestAFileThatIsAlreadyThereIsKept(t *testing.T) {
	// A generated header a person edited by hand is a conflict worth reporting,
	// not something to overwrite in silence.
	root := t.TempDir()
	write(t, root, "FreeRTOSConfig.h", "// written by a person\n")

	files := freeRTOSDialect{}.Files(resolved())
	written, diagnostics := Write(root, files, false)
	if len(written) != 1 || written[0] != ".config" {
		t.Fatalf("expected only the record to be written, got %+v", written)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "already there") {
		t.Fatalf("expected the conflict to be reported, got %+v", diagnostics)
	}
	kept, err := readFileAt(root, "FreeRTOSConfig.h")
	if err != nil || !strings.Contains(kept, "written by a person") {
		t.Fatalf("expected the person's file to be kept, got %q", kept)
	}
}

func TestForcingWritesOverAFileThatIsThere(t *testing.T) {
	root := t.TempDir()
	write(t, root, "FreeRTOSConfig.h", "// written by a person\n")

	written, _ := Write(root, freeRTOSDialect{}.Files(resolved()), true)
	if len(written) != 2 {
		t.Fatalf("expected both files written, got %+v", written)
	}
	header, err := readFileAt(root, "FreeRTOSConfig.h")
	if err != nil || strings.Contains(header, "written by a person") {
		t.Fatalf("expected the file to be written over, got %q", header)
	}
}

func TestAGeneratedHeaderIsOnlyDirectivesAndMacros(t *testing.T) {
	// A header a driver includes is read by a preprocessor, not a person: a word
	// outside a comment or a directive is a syntax error, and this one was found
	// by compiling against it.
	for _, dialect := range []Dialect{linuxDialect{}, espIDFDialect{}, freeRTOSDialect{}} {
		for _, file := range dialect.Files(resolved()) {
			if !strings.HasSuffix(file.Path, ".h") {
				continue
			}
			for number, line := range strings.Split(file.Content, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" ||
					strings.HasPrefix(trimmed, "/*") ||
					strings.HasPrefix(trimmed, "*") ||
					strings.HasPrefix(trimmed, "//") ||
					trimmed == "*/" ||
					strings.HasPrefix(trimmed, "#") {
					continue
				}
				t.Errorf("%s: %s line %d is not valid in a header: %q",
					dialect.Name(), file.Path, number+1, trimmed)
			}
		}
	}
}
