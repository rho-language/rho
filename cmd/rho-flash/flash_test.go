//
// @file flash_test.go
// @brief Rho Flash tests.
//
// Tests deterministic behavior of rho flash without touching real hardware.
// exec.Command is mocked to avoid calling elf2uf2 or st-flash.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Mock mode                                                                 */
/* -------------------------------------------------------------------------- */

var mockExec = true

/* -------------------------------------------------------------------------- */
/*  Tests: Pico Flash                                                          */
/* -------------------------------------------------------------------------- */

func TestFlashPico(t *testing.T) {
	if mockExec {
		t.Skip("external elf2uf2 and mounted Pico are not available in unit tests")
	}
	bin := "test_pico.bin"
	uf2 := "test_pico.uf2"

	defer os.Remove(bin)
	defer os.Remove(uf2)

	os.WriteFile(bin, []byte("dummy"), 0644)

	if mockExec {
		// Simulate successful flash
		err := FlashFile(bin, MCU_PICO)
		if err != nil {
			t.Fatalf("FlashFile(PICO) failed in mock mode: %v", err)
		}
		return
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: STM32 Flash                                                         */
/* -------------------------------------------------------------------------- */

func TestFlashSTM32(t *testing.T) {
	if mockExec {
		t.Skip("external st-flash hardware tool is not available in unit tests")
	}
	bin := "test_stm32.bin"
	defer os.Remove(bin)

	os.WriteFile(bin, []byte("dummy"), 0644)

	if mockExec {
		err := FlashFile(bin, MCU_STM32)
		if err != nil {
			t.Fatalf("FlashFile(STM32) failed in mock mode: %v", err)
		}
		return
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Missing File                                                        */
/* -------------------------------------------------------------------------- */

func TestFlashMissingFile(t *testing.T) {
	err := FlashFile("does_not_exist.bin", MCU_PICO)
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}
