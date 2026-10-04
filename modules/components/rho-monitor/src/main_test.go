//
// @file main_test.go
// @brief The run, from a command line to a stream of formatted output.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"path/filepath"
	"testing"

	"github.com/rho/rho-monitor/src/rhomonitor"
)

// rhomonitorConfigForTest is a default configuration with a parsed command line
// applied to it, which is the path a run takes before it opens anything.
func rhomonitorConfigForTest(opts options) *rhomonitor.Config {
	config := rhomonitor.DefaultConfig()
	apply(&config, opts)
	return &config
}

// Both spellings of a flag are the flag. An editor's launch configuration writes
// `--flag=value` and a person writes `-flag value`, and a run that refused either
// would fail on configurations that are perfectly ordinary.
func TestBothSpellingsOfAFlagAreTheFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--backend", "rtt"},
		{"-backend", "rtt"},
		{"--backend=rtt"},
		{"-backend=rtt"},
	} {
		_, options := parseArgs(args)
		if options.backend != "rtt" {
			t.Errorf("%v: expected the backend to be set, got %q", args, options.backend)
		}
	}
}

// The project directory is the one argument that is not a flag, so `rho monitor .`
// works.
func TestAProjectDirectoryIsNotAFlag(t *testing.T) {
	root, options := parseArgs([]string{".", "--no-color"})
	// The project directory is resolved against the working directory, so what
	// arrives is absolute rather than the `.` that was typed.
	if !filepath.IsAbs(root) {
		t.Errorf("expected the project directory to be resolved, got %q", root)
	}
	if !options.colorSet || options.color {
		t.Error("expected --no-color to be recorded as a decision")
	}
}

// A flag the command line did not mention must not override what the project
// declared, and a boolean cannot tell "said no" from "said nothing".
func TestAnAbsentFlagIsNotADecision(t *testing.T) {
	_, options := parseArgs(nil)
	if options.timestampsSet {
		t.Error("timestamps was treated as a decision that was never made")
	}
	if options.colorSet {
		t.Error("colour was treated as a decision that was never made")
	}
}

// `--duration` is what a pipeline uses, because a pipeline has no way to press
// Ctrl-C.
func TestADurationIsReadInSeconds(t *testing.T) {
	_, options := parseArgs([]string{"--duration=90"})
	if options.duration.Seconds() != 90 {
		t.Errorf("expected 90s, got %s", options.duration)
	}
}

// The transports this component reads, listed rather than guessed at: a person
// reaching for `--backend` needs to know what the choices are.
func TestTheTransportsAreListed(t *testing.T) {
	if code := listBackends(); code != exitClean {
		t.Fatalf("expected the list to succeed, got %d", code)
	}
}

// A named console is a decision not to guess, so it reaches the config as a port
// with the scan off — which is what keeps a project that named its console from
// being overridden by whichever node appears first.
func TestANamedConsoleTurnsTheScanOff(t *testing.T) {
	_, options := parseArgs([]string{"--console=/dev/ttyUSB3"})
	if options.console != "/dev/ttyUSB3" {
		t.Fatalf("expected the console to be read, got %q", options.console)
	}
	config := rhomonitorConfigForTest(options)
	if config.UART.Port != "/dev/ttyUSB3" || config.UART.AutoDetect {
		t.Errorf("expected the named console with no scan, got %+v", config.UART)
	}
}

// The backend names are the ones a project writes and a person types, so a typo is
// refused rather than resolved to something that works and is not what was meant.
func TestAnUnknownBackendIsNotResolved(t *testing.T) {
	_, options := parseArgs([]string{"--backend=telnet"})
	config := rhomonitorConfigForTest(options)
	if config.Backend == "telnet" {
		t.Error("an unknown backend reached the configuration")
	}
	if config.Backend != "auto" {
		t.Errorf("expected the run to fall back to auto rather than to a guess, got %s", config.Backend)
	}
}

// The transports are the ones a project writes and a person types, so the parser
// and the lister cannot disagree about them.
func TestTheBackendNamesAreTheOnesTheParserAccepts(t *testing.T) {
	for _, name := range rhomonitor.KindNames() {
		if _, known := rhomonitor.ParseBackend(name); !known {
			t.Errorf("%s is listed but not accepted", name)
		}
	}
	if _, known := rhomonitor.ParseBackend("telnet"); known {
		t.Error("a transport this component does not read was accepted")
	}
}
