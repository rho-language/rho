//
// @file env.go
// @brief Rho Install - Environment setup subsystem.
//
// Responsibilities:
//   - Detect user shell
//   - Add ~/.rho/bin to PATH
//   - Modify shell configuration files safely
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

/* -------------------------------------------------------------------------- */
/*  Shell Detection                                                            */
/* -------------------------------------------------------------------------- */

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
/*  PATH Injection                                                             */
/* -------------------------------------------------------------------------- */

func AddRhoToPath() error {
	binPath, err := GetBinPath()
	if err != nil {
		return err
	}

	shell := DetectShell()
	rhoHome, err := GetRhoHome()
	if err != nil {
		return err
	}
	home := filepath.Dir(rhoHome)

	var rcFile string

	switch shell {
	case "zsh":
		rcFile = filepath.Join(home, ".zshrc")
	case "bash":
		rcFile = filepath.Join(home, ".bashrc")
	case "fish":
		rcFile = filepath.Join(home, ".config", "fish", "config.fish")
	default:
		rcFile = filepath.Join(home, ".profile")
	}

	// Ensure file exists
	if _, err := os.Stat(rcFile); os.IsNotExist(err) {
		if err := os.WriteFile(rcFile, []byte{}, 0o644); err != nil {
			return fmt.Errorf("cannot create shell rc file: %w", err)
		}
	}

	content, err := os.ReadFile(rcFile)
	if err != nil {
		return fmt.Errorf("cannot read shell rc file: %w", err)
	}

	exportLine := fmt.Sprintf(`export PATH="%s:$PATH"`, binPath)

	// Avoid duplicate PATH entries
	if strings.Contains(string(content), exportLine) {
		return nil
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open rc file for writing: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + exportLine + "\n"); err != nil {
		return fmt.Errorf("failed to write PATH entry: %w", err)
	}

	return nil
}
