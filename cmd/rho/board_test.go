//
// @file board_test.go
// @brief Whether a build is for a board is read from the target's spec.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import "testing"

// The pico as `rho target show -json pico` prints it, trimmed to what is read.
const picoSpecJSON = `{"name": "pico", "kind": "board",
 "memory": {"flash": {"origin": "0x10000000", "length": "0x200000"}, "ram": {"origin": "0x20000000", "length": "0x42000"}},
 "deploy": {}, "linker": {}, "flash": {"tool": "picotool", "command": "picotool load -f -x {binary}"}}`

// A Linux board deployed to over SSH, which also declares RAM.
const linuxSpecJSON = `{"name": "uno-q-linux", "kind": "board", "family": "linux",
 "memory": {"ram": {"origin": "0x0", "length": "0x80000000"}},
 "deploy": {"host": "uno-q.local", "path": "/home/arduino/app"}, "linker": {}}`

func TestABoardIsDecidedByItsSpecAndNotItsName(t *testing.T) {
	if got := boardFromSpec("pico", []byte(picoSpecJSON)); got != "pico" {
		t.Errorf("pico is bare metal and a board, got %q", got)
	}
	if got := boardFromSpec("uno-q-linux", []byte(linuxSpecJSON)); got != "" {
		t.Errorf("a deployed Linux target is a host, got %q", got)
	}
	if got := boardFromSpec("native", []byte(`{"name": "native", "kind": "arch", "deploy": {}, "linker": {}}`)); got != "" {
		t.Errorf("the native host is a host, got %q", got)
	}
	sized := `{"name": "custom", "kind": "board", "linker": {"string_arena_bytes": 2048}}`
	if got := boardFromSpec("custom", []byte(sized)); got != "custom" {
		t.Errorf("a spec that sizes the arena has a fixed one, got %q", got)
	}
	if got := boardFromSpec("broken", []byte("not json")); got != "" {
		t.Errorf("a spec that cannot be read is a host, got %q", got)
	}
}

// A target nothing can resolve is built as a host and never refused.
func TestAnUnresolvableTargetIsAHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := boardFor(t.TempDir(), "no-such-board"); got != "" {
		t.Errorf("expected a host, got %q", got)
	}
}
