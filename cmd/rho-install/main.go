//
// @file main.go
// @brief Rho Install - CLI orchestrator.
//
// Responsibilities:
//   - Parse CLI arguments
//   - Execute installation pipeline
//   - Provide uninstall and check commands
//   - Orchestrate install/* modules
//
// @component rho-install
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

    "rho/cmd/rho-install/install"
)

/* -------------------------------------------------------------------------- */
/*  CLI Entry                                                                  */
/* -------------------------------------------------------------------------- */

func main() {
    installCmd := flag.Bool("install", false, "Install Rho ecosystem")
    uninstallCmd := flag.Bool("uninstall", false, "Uninstall Rho ecosystem")
    checkCmd := flag.Bool("check", false, "Check installation status")

    flag.Parse()

    switch {
    case *installCmd:
        runInstall()
    case *uninstallCmd:
        runUninstall()
    case *checkCmd:
        runCheck()
    default:
        fmt.Println("Usage:")
        fmt.Println("  rho-install --install")
        fmt.Println("  rho-install --uninstall")
        fmt.Println("  rho-install --check")
    }
}

/* -------------------------------------------------------------------------- */
/*  Install Pipeline                                                           */
/* -------------------------------------------------------------------------- */

func runInstall() {
    fmt.Println("Installing Rho ecosystem...")

    steps := []struct {
        name string
        fn   func() error
    }{
        {"Creating directory structure", install.EnsurePaths},
        {"Installing binaries", func() error {
            // Binaries are expected to be in ./bin relative to rho-install
            return install.InstallAllBinaries("./bin")
        }},
        {"Initializing registry", install.InitLocalRegistry},
        {"Initializing cache", install.InitCache},
        {"Initializing config", install.InitConfig},
        {"Adding Rho to PATH", install.AddRhoToPath},
    }

    for _, step := range steps {
        fmt.Printf(" - %s...\n", step.name)
        if err := step.fn(); err != nil {
            fmt.Printf("   ERROR: %v\n", err)
            os.Exit(1)
        }
    }

    fmt.Println("Rho installation complete.")
}

/* -------------------------------------------------------------------------- */
/*  Uninstall Pipeline                                                         */
/* -------------------------------------------------------------------------- */

func runUninstall() {
    fmt.Println("Uninstalling Rho ecosystem...")

    home, err := install.GetRhoHome()
    if err != nil {
        fmt.Printf("ERROR: %v\n", err)
        os.Exit(1)
    }

    if err := os.RemoveAll(home); err != nil {
        fmt.Printf("ERROR: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("Rho uninstalled successfully.")
}

/* -------------------------------------------------------------------------- */
/*  Check Installation                                                         */
/* -------------------------------------------------------------------------- */

func runCheck() {
    if install.IsInstalled() {
        fmt.Println("Rho is installed correctly.")
    } else {
        fmt.Println("Rho is NOT installed.")
        fmt.Println("Run: rho-install --install")
    }
}
