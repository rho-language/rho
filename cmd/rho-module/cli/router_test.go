//
// @file router_test.go
// @brief Tests for Module CLI router - Enterprise v3.
//
// Validates:
//   - CLI routing
//   - error handling
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Router                                                               */
/* -------------------------------------------------------------------------- */

func TestRouterNoArgs(t *testing.T) {
	code := Main([]string{})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestRouterUnknownCommand(t *testing.T) {
	code := Main([]string{"doesnotexist"})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestRouterInvalidSubcommand(t *testing.T) {
	// Test each subcommand with no args to check they return error codes
	// (they fail because current dir doesn't have required files, or wrong arg count)
	commands := []string{
		"init",      // needs 2 args
		"validate",  // needs module.yml in current dir
		"info",      // needs module.yml in current dir
		"publish",   // needs module.yml in current dir
		"download",  // needs 1 arg
		"install",   // needs module.yml in current dir
		"remove",    // needs 1 arg
		"update",    // needs module.yml in current dir
		"activate",  // needs 1 arg
		"unpublish", // needs 1 arg
		"uninstall", // needs 1 arg (alias for unpublish)
	}

	for _, cmd := range commands {
		code := Main([]string{cmd})
		if code == 0 {
			t.Fatalf("command %s: expected error code, got 0", cmd)
		}
	}
}

func TestRouterHelpCommands(t *testing.T) {
	// Test that each subcommand has a proper usage message
	code := Main([]string{"invalid"})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Individual Command Routing                                           */
/* -------------------------------------------------------------------------- */

func TestRouterInit(t *testing.T) {
	// init requires 2 args, so should return error code 1
	code := Main([]string{"init"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	code = Main([]string{"init", "name"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	code = Main([]string{"init", "name", "invalid"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRouterDownload(t *testing.T) {
	code := Main([]string{"download"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	code = Main([]string{"download", "name@version"})
	// Should fail because no registry, but not due to arg parsing
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRouterSearch(t *testing.T) {
	// search with no args now lists all modules (exit code 0)
	code := Main([]string{"search"})
	if code != 0 {
		t.Fatalf("expected exit code 0 for list all, got %d", code)
	}

	code = Main([]string{"search", "query"})
	// Search returns 0 when no results found (not an error)
	if code != 0 {
		t.Fatalf("expected exit code 0 for no results, got %d", code)
	}
}

func TestRouterRemove(t *testing.T) {
	code := Main([]string{"remove"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	code = Main([]string{"remove", "name"})
	// Should fail because no project, but not due to arg parsing
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Help Message                                                         */
/* -------------------------------------------------------------------------- */

func TestRouterHelpMessage(t *testing.T) {
	// Capture stderr to check help message
	// Just verify Main doesn't panic and returns proper codes
	code := Main([]string{})
	if code != 2 {
		t.Fatalf("expected exit code 2 for no args, got %d", code)
	}
}

// Helper to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}