//
// @file robohub_test.go
// @brief What the RoboHub platform contributes to the toolchain's contracts.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorobohub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An STM32-family target is given the macro that selects its family in the
// vendor's own driver sources and the vendor header its build has to produce a
// configuration for. That mapping is the reason this plugin exists: it used to
// be written into rho-target, which is what made the toolchain a language for
// one ecosystem.
func TestAnStm32TargetGetsItsFamilyMacroAndVendorHeader(t *testing.T) {
	augment := specAugment([]TargetDetail{
		{Name: "nucleo-h743zi", Family: "STM32H7", Defines: []string{"USE_HAL_DRIVER"}},
	})

	if len(augment) != 1 {
		t.Fatalf("expected the STM32 target to be augmented, got %+v", augment)
	}
	entry := augment[0]
	if entry.Target != "nucleo-h743zi" {
		t.Errorf("expected the target named, got %q", entry.Target)
	}
	if len(entry.Defines) != 1 || entry.Defines[0] != "ROBOHUB_STM32_FAMILY_H7=1" {
		t.Errorf("expected the family macro, got %+v", entry.Defines)
	}
	if entry.HALHeader != "stm32h7xx_hal.h" {
		t.Errorf("expected the vendor umbrella header, got %q", entry.HALHeader)
	}
}

// A target that is not an STM32 part is left entirely alone. This is what lets
// one component build for every platform: the host, a Pico and an ESP32 are
// built by the same rho-target with this plugin installed, because it touches
// nothing they do not name a family for.
func TestANonStm32TargetIsLeftAlone(t *testing.T) {
	augment := specAugment([]TargetDetail{
		{Name: "pico", MCU: "rp2040"},
		{Name: "esp32c3"},
		{Name: "native"},
	})
	if len(augment) != 0 {
		t.Fatalf("expected a non-STM32 target to be untouched, got %+v", augment)
	}
}

// A fact the platform module already declares is not declared again here. The
// platform's own spec and this plugin are two descriptions of one platform, and
// a fact stated in both is a fact that will eventually disagree.
func TestAFactTheSpecAlreadyDeclaresIsNotRepeated(t *testing.T) {
	// The spec already selects the family and names the vendor header.
	augment := specAugment([]TargetDetail{{
		Name: "nucleo-h743zi", Family: "STM32H7",
		HALHeader: "stm32h7xx_hal.h",
		Defines:   []string{"ROBOHUB_STM32_FAMILY_H7=1"},
	}})
	if len(augment) != 0 {
		t.Fatalf("expected nothing added when the spec already says it, got %+v", augment)
	}
}

// The family suffix is the family with the vendor's `STM32` stem dropped, and
// the vendor header follows it. A family this naming does not cover is reported
// as such rather than guessed at.
func TestFamilySuffixAndHeaderFollowTheVendorNaming(t *testing.T) {
	cases := []struct {
		family  string
		suffix  string
		header  string
		covered bool
	}{
		{"STM32H7", "H7", "stm32h7xx_hal.h", true},
		{"stm32f2", "F2", "stm32f2xx_hal.h", true},
		{"STM32WB", "WB", "stm32wbxx_hal.h", true},
		{"STM32", "", "", false},
		{"rp2040", "", "", false},
		{"", "", "", false},
	}
	for _, one := range cases {
		suffix, ok := familySuffix(one.family)
		if ok != one.covered || suffix != one.suffix {
			t.Errorf("family %q: expected (%q,%v), got (%q,%v)", one.family, one.suffix, one.covered, suffix, ok)
			continue
		}
		if ok && "stm32"+strings.ToLower(suffix)+"xx_hal.h" != one.header {
			t.Errorf("family %q: expected header %q", one.family, one.header)
		}
	}
}

// One program answers all four contracts; the envelope says which. A request for
// a component this plugin does not fill is reported, not answered.
func TestEachContractIsAnsweredByTheOneProgram(t *testing.T) {
	cases := []struct {
		component string
		hook      string
		payload   string
		reads     string
	}{
		{ComponentTarget, "spec", `{"details":[{"name":"b","family":"STM32F2"}]}`, "augment"},
		{ComponentKconfig, "render", `{"dialect":"linux"}`, "files"},
		{ComponentCimport, "map", `{"headers":["platform.h"]}`, "types"},
		// The flash contract answers with no override: a RoboHub board names
		// its own flash command in its spec. An empty object is the answer.
		{ComponentFlash, "flash", `{"target":"b"}`, ""},
	}
	for _, one := range cases {
		request := Request{
			Version: ProtocolVersion, Hook: one.hook, Command: "x",
			Component: one.component, Payload: json.RawMessage(one.payload),
		}
		response := answer(request)
		if len(response.Diagnostics) != 0 {
			t.Errorf("%s: unexpected diagnostics %+v", one.component, response.Diagnostics)
		}
		if len(response.Payload) == 0 {
			t.Errorf("%s: expected a payload", one.component)
			continue
		}
		if one.reads != "" && !strings.Contains(string(response.Payload), one.reads) {
			t.Errorf("%s: expected the payload to carry %q, got %s", one.component, one.reads, response.Payload)
		}
	}
}

// A tool the platform ships is resolved to its own entry point, derived from
// the platform's directory rather than found on PATH. The answer is the same on
// every machine, which is what keeps a board programmed with the platform's own
// converter and not another build's.
func TestAPlatformToolResolvesToThePlatformsOwnEntryPoint(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "tools", "elf2uf2", "flash")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	request := FlashRequest{PlatformRoot: root}
	request.Flash.Tool = "elf2uf2"
	request.Flash.Command = "elf2uf2 {binary} --family-id 0xe48bff59"
	answer := flashAnswer(request)

	command, _ := answer.Flash["command"].(string)
	if command != entry+" {binary} --family-id 0xe48bff59" {
		t.Fatalf("expected only the program replaced, got %q", command)
	}
}

// What the board declared after the tool is the board's and is carried through:
// the family ID is what the bootrom checks the image against, and dropping it
// would produce a UF2 the part refuses.
func TestTheBoardsOwnArgumentsAreKept(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "tools", "elf2uf2", "flash")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	request := FlashRequest{PlatformRoot: root}
	request.Flash.Tool = "elf2uf2"
	request.Flash.Command = "elf2uf2 {binary} --family-id 0xdeadbeef --mount /media/board"
	command, _ := flashAnswer(request).Flash["command"].(string)
	if !strings.HasSuffix(command, "{binary} --family-id 0xdeadbeef --mount /media/board") {
		t.Fatalf("expected the board's arguments kept, got %q", command)
	}
}

// A tool the platform does not ship is left to the spec: an STM32 board names
// openocd, which is the machine's program and not the platform's.
func TestAToolThePlatformDoesNotShipIsLeftToTheSpec(t *testing.T) {
	root := t.TempDir()
	request := FlashRequest{PlatformRoot: root}
	request.Flash.Tool = "openocd"
	if answer := flashAnswer(request); len(answer.Flash) != 0 {
		t.Fatalf("expected no override for a system tool, got %+v", answer.Flash)
	}
}

// A tool name that is a path or a space is not one of the platform's, so it
// cannot be turned into one.
func TestAToolNameThatIsNotABareNameIsNotResolved(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../evil", "sh -c"} {
		request := FlashRequest{PlatformRoot: root}
		request.Flash.Tool = name
		if answer := flashAnswer(request); len(answer.Flash) != 0 {
			t.Fatalf("expected %q to be refused, got %+v", name, answer.Flash)
		}
	}
}

// A component this plugin does not fill is reported rather than answered: a
// wrong answer to a hook nobody asked this plugin for would be worse than a
// diagnostic.
func TestAComponentThisPluginDoesNotFillIsReported(t *testing.T) {
	response := answer(Request{Version: ProtocolVersion, Component: "rho-doc", Hook: "index"})
	if len(response.Diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %+v", response.Diagnostics)
	}
	if !strings.Contains(response.Diagnostics[0].Message, "does not answer") {
		t.Errorf("unexpected diagnostic %+v", response.Diagnostics[0])
	}
}
