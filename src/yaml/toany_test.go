//
// @file toany_test.go
// @brief Tests for converting a parsed YAML document into plain Go values.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package yaml

import "testing"

// A document converted to plain values keeps its shape: the maps are maps, the
// lists are lists, and nothing about the nesting is lost. This is what a caller
// that decodes into a struct depends on.
func TestToAnyKeepsTheDocumentShape(t *testing.T) {
	document, err := Load(`
name: stm32f4
toolchain:
  compiler: arm-none-eabi-gcc
  flags:
    - -O2
    - -Wall
memory:
  flash:
    origin: 0x08000000
`)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	converted, ok := ToAny(document).(map[string]any)
	if !ok {
		t.Fatalf("the document converted to %T, want a map", ToAny(document))
	}
	if converted["name"] != "stm32f4" {
		t.Errorf("name is %v, want stm32f4", converted["name"])
	}

	toolchain, ok := converted["toolchain"].(map[string]any)
	if !ok {
		t.Fatalf("toolchain converted to %T, want a map", converted["toolchain"])
	}
	if toolchain["compiler"] != "arm-none-eabi-gcc" {
		t.Errorf("compiler is %v, want arm-none-eabi-gcc", toolchain["compiler"])
	}

	flags, ok := toolchain["flags"].([]any)
	if !ok {
		t.Fatalf("flags converted to %T, want a list", toolchain["flags"])
	}
	if len(flags) != 2 || flags[0] != "-O2" || flags[1] != "-Wall" {
		t.Errorf("flags are %v, want [-O2 -Wall]", flags)
	}

	memory, ok := converted["memory"].(map[string]any)
	if !ok {
		t.Fatalf("memory converted to %T, want a map", converted["memory"])
	}
	flash, ok := memory["flash"].(map[string]any)
	if !ok {
		t.Fatalf("memory.flash converted to %T, want a map", memory["flash"])
	}
	if flash["origin"] != "0x08000000" {
		t.Errorf("origin is %v, want 0x08000000", flash["origin"])
	}
}

// A key declared with nothing after it is the difference between a key that was
// declared and one that was left out, so it becomes nil rather than an empty
// string. A reader that cannot tell those apart cannot tell an unset field from
// one that was set to nothing.
func TestToAnyDistinguishesAnEmptyKeyFromAnAbsentOne(t *testing.T) {
	document, err := Load("declared:\nother: value\n")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	converted := ToAny(document).(map[string]any)

	empty, present := converted["declared"]
	if !present {
		t.Fatalf("a key declared with no value is missing from the converted document")
	}
	if empty != nil {
		t.Errorf("a key declared with no value is %v, want nil", empty)
	}
	if converted["other"] != "value" {
		t.Errorf("a key with a value is %v, want value", converted["other"])
	}
}

// A list that is empty is still a list. Collapsing it to nil would make an
// empty `flags:` indistinguishable from a missing one, and those differ: one
// says "no flags", the other says "this spec does not say".
func TestToAnyKeepsAnEmptyListAList(t *testing.T) {
	document, err := Load("flags: []\n")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	converted := ToAny(document).(map[string]any)
	flags, ok := converted["flags"].([]any)
	if !ok {
		t.Fatalf("an empty list converted to %T, want a list", converted["flags"])
	}
	if len(flags) != 0 {
		t.Errorf("an empty list has %d items, want 0", len(flags))
	}
}
