//
// @file flash.go
// @brief Rho Flash - Enterprise.
//
// Implements `rho flash`, a deterministic tool for flashing Rho-generated
// firmware to microcontrollers (Pico UF2, STM32 via st-flash).
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
    "flag"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

type MCU string

const (
    MCU_PICO  MCU = "pico"
    MCU_STM32 MCU = "stm32"
)

// FlashFile flashes a binary to the selected MCU.
func FlashFile(bin string, mcu MCU) error {
    switch mcu {

    case MCU_PICO:
        return flashPico(bin)

    case MCU_STM32:
        return flashSTM32(bin)

    default:
        return fmt.Errorf("unknown MCU: %s", mcu)
    }
}

/* -------------------------------------------------------------------------- */
/*  Pico Flash (UF2)                                                          */
/* -------------------------------------------------------------------------- */

func flashPico(bin string) error {
    uf2 := replaceExt(bin, ".uf2")

    // Convert ELF → UF2 using elf2uf2
    cmd := exec.Command("elf2uf2", bin, uf2)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("UF2 conversion failed: %w", err)
    }

    // Copy UF2 to mounted Pico drive
    picoPath := "/media/pico/RPI-RP2"
    dst := filepath.Join(picoPath, filepath.Base(uf2))

    if err := copyFile(uf2, dst); err != nil {
        return fmt.Errorf("copy to Pico failed: %w", err)
    }

    return nil
}

/* -------------------------------------------------------------------------- */
/*  STM32 Flash (st-flash)                                                    */
/* -------------------------------------------------------------------------- */

func flashSTM32(bin string) error {
    cmd := exec.Command("st-flash", "write", bin, "0x8000000")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("st-flash failed: %w", err)
    }

    return nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func main() {
    fs := flag.NewFlagSet("rho flash", flag.ExitOnError)
    mcu := fs.String("mcu", "pico", "target MCU: pico | stm32")

    fs.Parse(os.Args[1:])
    if fs.NArg() != 1 {
        fmt.Fprintln(os.Stderr, "usage: rho flash -mcu <pico|stm32> <file.bin>")
        os.Exit(2)
    }

    bin := fs.Arg(0)

    if err := FlashFile(bin, MCU(*mcu)); err != nil {
        fmt.Fprintln(os.Stderr, "rho flash:", err)
        os.Exit(1)
    }
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func replaceExt(path, newExt string) string {
    for i := len(path) - 1; i >= 0; i-- {
        if path[i] == '.' {
            return path[:i] + newExt
        }
    }
    return path + newExt
}

func copyFile(src, dst string) error {
    in, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(dst, in, 0644)
}
