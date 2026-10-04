//
// @file cmake_test.go
// @brief Tests for the files a build needs from a target.
//
// A build reads these files and does what they say, so a mistake in one is a
// build that cross-compiles with the host's compiler or links a program into the
// wrong memory. These tests are about the facts reaching the right file in the
// right shape.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhtarget

import (
	"strings"
	"testing"
)

// testSpec is a chip with everything a build reads.
func testSpec() Spec {
	link := false
	return Spec{
		Kind: KindMCU, Name: "chip", Core: "cortex-m4",
		Toolchain: Toolchain{
			Compiler: "arm-none-eabi-gcc",
			Objcopy:  "arm-none-eabi-objcopy",
			Size:     "arm-none-eabi-size",
			Flags:    []string{"-mcpu=cortex-m4", "-mthumb"},
		},
		Flags:   []string{"-Wl,--gc-sections"},
		Defines: []string{"CHIP=1"},
		Memory: Memory{
			Flash: Region{Origin: "0x08000000", Length: "0x100000"},
			RAM:   Region{Origin: "0x20000000", Length: "0x20000"},
			Extra: map[string]Region{
				"ccm":    {Origin: "0x10000000", Length: "0x10000"},
				"window": {Origin: "0x14000000", Length: "0x1000", Link: &link},
			},
		},
		Flash: Flash{Tool: "openocd", Command: "openocd -c \"program {binary} exit\"", Offset: "0x08000000"},
	}
}

func TestTheToolchainFileNamesTheCompilerAndDeniesTheHost(t *testing.T) {
	// CMake compiles for the host unless it is told otherwise, and flags for a
	// Cortex-M reach a compiler that cannot use them.
	content := ToolchainCMake(testSpec())
	for _, want := range []string{
		"set(CMAKE_SYSTEM_NAME Generic)",
		"set(CMAKE_SYSTEM_PROCESSOR \"cortex-m4\")",
		"set(CMAKE_C_COMPILER \"arm-none-eabi-gcc\")",
		"CMAKE_TRY_COMPILE_TARGET_TYPE STATIC_LIBRARY",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q, got:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "DO NOT EDIT") {
		t.Error("expected the file to say it is generated")
	}
}

func TestTheTargetFileCarriesFlagsDefinesAndMemory(t *testing.T) {
	content := TargetCMake(testSpec(), "cmake/linker.ld")
	for _, want := range []string{
		"add_compile_definitions(CHIP=1)",
		"-mcpu=cortex-m4",
		"-mthumb",
		"-Wl,--gc-sections",
		"set(RHO_FLASH_ORIGIN 0x08000000)",
		"set(RHO_FLASH_LENGTH 0x100000)",
		"set(RHO_RAM_LENGTH 0x20000)",
		"set(RHO_CCM_ORIGIN 0x10000000)",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q, got:\n%s", want, content)
		}
	}
	// The script the target generates is the one it links.
	if !strings.Contains(content, "add_link_options(-T${RHO_LINKER_SCRIPT})") {
		t.Errorf("expected the linker script to be linked, got:\n%s", content)
	}
}

func TestTheTargetFileNamesTheLibraryAndTheFlashing(t *testing.T) {
	spec := testSpec()
	spec.VendorLibrary = "device_board"
	content := TargetCMake(spec, "cmake/linker.ld")
	if !strings.Contains(content, "set(RHO_VENDOR_LIBRARY device_board)") {
		t.Errorf("expected the library, got:\n%s", content)
	}
	if !strings.Contains(content, "add_custom_target(flash") {
		t.Errorf("expected a flash target, got:\n%s", content)
	}
	if !strings.Contains(content, "set(RHO_FLASH_TOOL openocd)") {
		t.Errorf("expected the tool, got:\n%s", content)
	}
}

func TestALinkerScriptIsGeneratedFromTheMemoryMap(t *testing.T) {
	content := LinkerScript(testSpec())
	for _, want := range []string{
		"MEMORY",
		"FLASH (rx): ORIGIN = 0x08000000, LENGTH = 0x100000",
		"RAM (rw): ORIGIN = 0x20000000, LENGTH = 0x20000",
		"CCM",
		"_estack = ORIGIN(RAM) + LENGTH(RAM);",
		"ENTRY(Reset_Handler)",
		".isr_vector",
		"> FLASH",
		"_ebss",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q, got:\n%s", want, content)
		}
	}

	// Every CMSIS startup assembly copies the initialised data from flash to RAM
	// before `main`, and it finds the flash address through `_sidata`. A script
	// that omits it produces a link that fails on one undefined reference, so the
	// symbol is asserted here rather than discovered on a board.
	for _, want := range []string{
		"AT> FLASH",
		"_sidata = LOADADDR(.data);",
		"__bss_start__",
		"__bss_end__",
		"_Min_Stack_Size",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("a CMSIS startup needs %q, and the script does not define it:\n%s", want, content)
		}
	}
	// A region that is a window onto another region is not somewhere a section
	// goes, and the end of the flash is not a symbol: moving the location counter
	// there would put a hole the size of the flash in the image.
	if strings.Contains(content, "WINDOW") {
		t.Errorf("expected a non-linkable region to be left out, got:\n%s", content)
	}
	if strings.Contains(content, "_eflash") {
		t.Errorf("expected no flash-end symbol, got:\n%s", content)
	}
	// The stack is at the top of RAM, which only works if the region the ELF
	// calls RAM is the one the stack is in.
	if !strings.Contains(content, "/* The stack grows down from the end of RAM. */") {
		t.Errorf("expected the stack to be explained, got:\n%s", content)
	}
}

func TestNoScriptIsGeneratedWhenTheTargetLinksItsOwn(t *testing.T) {
	// An ESP-IDF board is linked by the framework's own script, and a generated
	// one would lay the program out somewhere it cannot run from.
	spec := testSpec()
	spec.LinkerScript = "idf/esp32c3.ld"
	spec.Memory = Memory{Flash: Region{Origin: "0x10000", Length: "0x100000"}}

	files := Files(spec, "cmake")
	for _, file := range files {
		if strings.HasSuffix(file.Path, "linker.ld") {
			t.Fatalf("expected no generated script, got %+v", file.Path)
		}
	}
	content := TargetCMake(spec, "cmake/linker.ld")
	if !strings.Contains(content, "idf/esp32c3.ld") {
		t.Errorf("expected the target's own script, got:\n%s", content)
	}
}

func TestFilesAreWrittenWhereTheProjectExpectsThem(t *testing.T) {
	files := Files(testSpec(), "cmake")
	if len(files) != 3 {
		t.Fatalf("expected three files, got %+v", files)
	}
	want := []string{"cmake/toolchain.cmake", "cmake/target.cmake", "cmake/linker.ld"}
	for index := range want {
		if files[index].Path != want[index] {
			t.Fatalf("expected %v, got %+v", want, files)
		}
	}
	// A generated file says how to generate it again.
	for _, file := range files {
		if !strings.Contains(file.Content, "rho target generate") {
			t.Errorf("expected %s to say how to regenerate it", file.Path)
		}
	}
}
