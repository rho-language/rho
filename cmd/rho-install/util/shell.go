//
// @file shell.go
// @brief Rho Install - Shell utilities.
//
// Responsibilities:
//   - Detect user shell
//   - Determine correct RC file
//   - Ensure RC file exists
//   - Provide helpers for PATH injection
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package util

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

/* -------------------------------------------------------------------------- */
/*  Shell Detection                                                            */
/* -------------------------------------------------------------------------- */

// DetectShell returns the shell name: bash, zsh, fish, sh.
func DetectShell() string {
    shell := os.Getenv("SHELL")

    if strings.Contains(shell, "zsh") {
        return "zsh"
    }
    if strings.Contains(shell, "bash") {
        return "bash"
    }
    if strings.Contains(shell, "fish") {
        return "fish"
    }
    return "sh"
}

/* -------------------------------------------------------------------------- */
/*  RC File Resolution                                                         */
/* -------------------------------------------------------------------------- */

// GetShellRC returns the path to the shell RC file.
func GetShellRC() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("cannot determine user home directory: %w", err)
    }

    switch DetectShell() {
    case "zsh":
        return filepath.Join(home, ".zshrc"), nil
    case "bash":
        return filepath.Join(home, ".bashrc"), nil
    case "fish":
        return filepath.Join(home, ".config", "fish", "config.fish"), nil
    default:
        return filepath.Join(home, ".profile"), nil
    }
}

/* -------------------------------------------------------------------------- */
/*  RC File Creation                                                           */
/* -------------------------------------------------------------------------- */

// EnsureRC ensures the RC file exists.
func EnsureRC() (string, error) {
    rc, err := GetShellRC()
    if err != nil {
        return "", err
    }

    if _, err := os.Stat(rc); os.IsNotExist(err) {
        if err := os.WriteFile(rc, []byte{}, 0o644); err != nil {
            return "", fmt.Errorf("cannot create RC file %s: %w", rc, err)
        }
    }

    return rc, nil
}

/* -------------------------------------------------------------------------- */
/*  PATH Injection Helpers                                                     */
/* -------------------------------------------------------------------------- */

// RCContains checks if RC file contains a specific line.
func RCContains(rc string, line string) (bool, error) {
    content, err := os.ReadFile(rc)
    if err != nil {
        return false, fmt.Errorf("cannot read RC file: %w", err)
    }

    return strings.Contains(string(content), line), nil
}

// AppendToRC appends a line to the RC file.
func AppendToRC(rc string, line string) error {
    f, err := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return fmt.Errorf("cannot open RC file for writing: %w", err)
    }
    defer f.Close()

    if _, err := f.WriteString("\n" + line + "\n"); err != nil {
        return fmt.Errorf("failed to append to RC file: %w", err)
    }

    return nil
}
