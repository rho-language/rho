//
// @file rhokconfig_test.go
// @brief Tests for the configuration bridge.
//
// The bridge is read by a build and read by a person, so the tests are about
// what it says: which symbols a fragment declares, which value wins, what is
// refused, and what each dialect writes. A configuration that is silently wrong
// is a firmware that boots with a peripheral it does not have.
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

// fragment is a Kconfig file a test writes, in the spelling a project uses.
const fragment = `
menu "Peripherals"

config CONFIG_SPI_MASTER
	bool "SPI master"
	default y
	help "Drive the SPI bus"

config CONFIG_UART_BAUD
	int "UART baud rate"
	range 9600..921600
	default 115200

config CONFIG_LOG_LEVEL
	string "Log level"
	default "info"

config CONFIG_SPI_SLAVE
	bool
	default n

config CONFIG_UART_TX_PIN
	hex "UART TX pin"
	default 0x25

endmenu
`

// write writes a file below a root and returns the path.
func write(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

/* -------------------------------------------------------------------------- */
/*  Reading fragments                                                        */
/* -------------------------------------------------------------------------- */

func TestAFragmentDeclaresItsSymbols(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)

	read, diagnostics := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	if len(diagnostics) != 0 {
		t.Fatalf("expected a fragment to be read cleanly, got %+v", diagnostics)
	}
	if len(read.Symbols) != 5 {
		t.Fatalf("expected five symbols, got %d: %+v", len(read.Symbols), read.Symbols)
	}
	if symbol, found := read.Fragment("CONFIG_SPI_MASTER"); !found {
		t.Fatal("expected the symbol to be declared")
	} else {
		if symbol.Type != TypeBool {
			t.Errorf("expected a bool, got %q", symbol.Type)
		}
		if symbol.Default != "y" {
			t.Errorf("expected the default to be kept, got %q", symbol.Default)
		}
		if !strings.Contains(symbol.Prompt, "SPI master") {
			t.Errorf("expected the prompt, got %q", symbol.Prompt)
		}
		if !strings.Contains(symbol.Help, "Drive the SPI bus") {
			t.Errorf("expected the help, got %q", symbol.Help)
		}
	}
	if symbol, _ := read.Fragment("CONFIG_UART_BAUD"); symbol.Range != "9600..921600" {
		t.Errorf("expected the range to be kept, got %q", symbol.Range)
	}
	if symbol, _ := read.Fragment("CONFIG_UART_TX_PIN"); symbol.Type != TypeHex || symbol.Default != "0x25" {
		t.Errorf("expected a hex symbol, got %+v", symbol)
	}
}

func TestACommentIsNotPartOfAValue(t *testing.T) {
	// A `#` starts a comment, so a fragment written by a person stays readable
	// and a value never carries one.
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", "config CONFIG_LOG_LEVEL\n\tstring\n\tdefault \"info\"  # the level\n")

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	symbol, found := read.Fragment("CONFIG_LOG_LEVEL")
	if !found {
		t.Fatal("expected the symbol")
	}
	if symbol.Default != "info" {
		t.Fatalf("expected the comment to be dropped, got %q", symbol.Default)
	}
}

func TestAFragmentIsSourced(t *testing.T) {
	// A fragment that sources another is how a target's fragments and a project's
	// own compose without either knowing the other's file names.
	root := t.TempDir()
	write(t, root, "kconfig/project.Kconfig", "source \"board.Kconfig\"\n")
	write(t, root, "kconfig/board.Kconfig", "config CONFIG_SPI_MASTER\n\tbool\n\tdefault y\n")

	read, diagnostics := ReadFragments(root, []string{"kconfig/project.Kconfig"})
	if len(diagnostics) != 0 {
		t.Fatalf("expected the sourced fragment to be read, got %+v", diagnostics)
	}
	if _, found := read.Fragment("CONFIG_SPI_MASTER"); !found {
		t.Fatalf("expected the sourced symbol, got %+v", read.Symbols)
	}
}

func TestAFragmentSourcedTwiceIsReadOnce(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kconfig/one.Kconfig", "source \"shared.Kconfig\"\n")
	write(t, root, "kconfig/two.Kconfig", "source \"shared.Kconfig\"\n")
	write(t, root, "kconfig/shared.Kconfig", "config CONFIG_ONE\n\tbool\n\tdefault y\n")

	read, _ := ReadFragments(root, []string{"kconfig/one.Kconfig", "kconfig/two.Kconfig"})
	count := 0
	for _, symbol := range read.Symbols {
		if symbol.Name == "CONFIG_ONE" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected the shared fragment once, got %d", len(read.Symbols))
	}
}

func TestAFragmentThatCannotBeReadIsReported(t *testing.T) {
	root := t.TempDir()
	read, diagnostics := ReadFragments(root, []string{"kconfig/absent.Kconfig"})
	if len(read.Symbols) != 0 {
		t.Fatalf("expected nothing read, got %+v", read.Symbols)
	}
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "cannot read") {
		t.Fatalf("expected the missing fragment to be reported, got %+v", diagnostics)
	}
}

func TestAnUnknownKeywordIsReportedWithItsLine(t *testing.T) {
	// A fragment the reader does not understand is a fragment a build should stop
	// for: half a configuration is a configuration with a hole in it.
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", "config CONFIG_ONE\n\tbool\n\tdefault y\n\nwobble CONFIG_TWO\n")

	_, diagnostics := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	if len(diagnostics) == 0 {
		t.Fatal("expected the unknown keyword to be reported")
	}
	if diagnostics[0].Line != 5 || !strings.Contains(diagnostics[0].Message, "wobble") {
		t.Fatalf("expected the line and the word, got %+v", diagnostics[0])
	}
}

/* -------------------------------------------------------------------------- */
/*  Merging                                                                   */
/* -------------------------------------------------------------------------- */

func TestALaterLayerWins(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	configuration, diagnostics := Resolve(read, []Source{
		{Name: "defconfig", Values: map[string]string{"CONFIG_UART_BAUD": "9600"}},
		{Name: "module.yml", Values: map[string]string{"CONFIG_UART_BAUD": "57600"}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean merge, got %+v", diagnostics)
	}
	value, found := configuration.Get("CONFIG_UART_BAUD")
	if !found {
		t.Fatal("expected the symbol")
	}
	if value.Value != "57600" {
		t.Fatalf("expected the later layer to win, got %q from %s", value.Value, value.From)
	}
	if value.From != "module.yml" {
		t.Fatalf("expected the layer to be named, got %q", value.From)
	}
}

func TestASymbolNothingSetsKeepsItsDefault(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	configuration, _ := Resolve(read, nil)

	if value, _ := configuration.Get("CONFIG_SPI_MASTER"); !value.Bool() {
		t.Fatalf("expected the default to stand, got %q", value.Value)
	}
	// A flag with no default is off, which is what a header a driver includes
	// has to find.
	if value, _ := configuration.Get("CONFIG_SPI_SLAVE"); value.Bool() {
		t.Fatalf("expected a flag with no default to be off, got %q", value.Value)
	}
}

func TestAValueForASymbolNothingDeclaresIsRefused(t *testing.T) {
	// A value nothing reads is a mistake worth reporting rather than a line in a
	// header that is never consulted.
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	_, diagnostics := Resolve(read, []Source{
		{Name: "module.yml", Values: map[string]string{"CONFIG_NOT_A_SYMBOL": "1"}},
	})
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "no Kconfig fragment declares") {
		t.Fatalf("expected the unknown symbol to be reported, got %+v", diagnostics)
	}
}

func TestAValueOfTheWrongTypeIsRefused(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	configuration, diagnostics := Resolve(read, []Source{
		{Name: "module.yml", Values: map[string]string{
			"CONFIG_UART_BAUD":  "fast",
			"CONFIG_SPI_MASTER": "maybe",
		}},
	})
	if len(diagnostics) != 2 {
		t.Fatalf("expected both values to be refused, got %+v", diagnostics)
	}
	// The symbol keeps the value it had, which is the default.
	if value, _ := configuration.Get("CONFIG_UART_BAUD"); value.Value != "115200" {
		t.Fatalf("expected the default to stand, got %q", value.Value)
	}
}

func TestASelectedSymbolIsOnEvenWhenNothingSetsIt(t *testing.T) {
	// `select` is how one symbol turns another on, and a header that ignored it
	// would say the opposite of what the fragment says.
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", "config CONFIG_SPI_MASTER\n\tbool\n\tdefault n\n\nconfig CONFIG_SPI\n\tbool\n\tdefault n\n\tselect CONFIG_SPI_MASTER\n")

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})
	configuration, _ := Resolve(read, nil)
	if value, _ := configuration.Get("CONFIG_SPI_MASTER"); !value.Bool() {
		t.Fatalf("expected the selected symbol to be on, got %q", value.Value)
	}
}

func TestADefconfigIsRead(t *testing.T) {
	// A defconfig is `key=value` lines, and a bare symbol is one that is on,
	// which is how a defconfig is written by hand.
	root := t.TempDir()
	write(t, root, "kconfig/rho-defconfig", "# the baseline\nCONFIG_SPI_MASTER=y\nCONFIG_LOG_LEVEL\nCONFIG_UART_BAUD=9600\n")

	data, err := os.ReadFile(filepath.Join(root, "kconfig", "rho-defconfig"))
	if err != nil {
		t.Fatal(err)
	}
	source, diagnostics := ParseDefconfig(string(data), "defconfig")
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean read, got %+v", diagnostics)
	}
	if source.Values["CONFIG_SPI_MASTER"] != "y" {
		t.Errorf("expected a value with an explicit one, got %q", source.Values["CONFIG_SPI_MASTER"])
	}
	if source.Values["CONFIG_LOG_LEVEL"] != "y" {
		t.Errorf("expected a bare symbol to read as on, got %q", source.Values["CONFIG_LOG_LEVEL"])
	}
	if source.Values["CONFIG_UART_BAUD"] != "9600" {
		t.Errorf("expected a value, got %q", source.Values["CONFIG_UART_BAUD"])
	}
}

/* -------------------------------------------------------------------------- */
/*  A Rho module of constants                                                 */
/* -------------------------------------------------------------------------- */

func TestAModuleOfConstantsIsALayer(t *testing.T) {
	// A project's own opinion is written in Rho, and read with the compiler's own
	// parser so the file is held to the same syntax the compiler holds it to.
	root := t.TempDir()
	write(t, root, "kconfig/board.Kconfig", fragment)
	write(t, root, "config.rho", `module config

extern fn board() u32

fn init() {
    let CONFIG_UART_BAUD: u32 = 57600
    let CONFIG_SPI_MASTER: bool = true
    let CONFIG_LOG_LEVEL: string = "debug"
    let not_a_setting: u32 = 3
}
`)

	read, _ := ReadFragments(root, []string{"kconfig/board.Kconfig"})

	// The module's bindings are a layer, read through the same path the merge
	// uses for a manifest, so a value written in Rho is weighed the same way.
	source, sourceDiagnostics := readRhoModule(root, "config.rho")
	if len(sourceDiagnostics) != 0 {
		t.Fatalf("expected the module to be read, got %+v", sourceDiagnostics)
	}
	if source.Values["CONFIG_UART_BAUD"] != "57600" {
		t.Errorf("expected the module's value, got %q", source.Values["CONFIG_UART_BAUD"])
	}
	if source.Values["CONFIG_SPI_MASTER"] != "y" {
		t.Errorf("expected a boolean to read as yes, got %q", source.Values["CONFIG_SPI_MASTER"])
	}
	if _, present := source.Values["not_a_setting"]; present {
		t.Error("expected a binding that names no symbol to be left alone")
	}

	merged, _ := Resolve(read, []Source{source})
	if value, _ := merged.Get("CONFIG_UART_BAUD"); value.Value != "57600" {
		t.Fatalf("expected the module to win over the default, got %q", value.Value)
	}
}

func TestAModuleWithAValueThatIsNotALiteralIsReported(t *testing.T) {
	root := t.TempDir()
	write(t, root, "config.rho", "module config\n\nfn init() {\n    let CONFIG_UART_BAUD: u32 = board()\n}\n")

	_, diagnostics := readRhoModule(root, "config.rho")
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "not a literal") {
		t.Fatalf("expected the value to be reported, got %+v", diagnostics)
	}
}

func TestAModuleThatDoesNotParseIsReported(t *testing.T) {
	// The module is held to the same syntax as any other: a config file that
	// does not parse says so rather than configuring nothing.
	root := t.TempDir()
	write(t, root, "config.rho", "module config\n\nfn init( {\n")

	_, diagnostics := readRhoModule(root, "config.rho")
	if len(diagnostics) == 0 {
		t.Fatal("expected the module to be reported")
	}
}
