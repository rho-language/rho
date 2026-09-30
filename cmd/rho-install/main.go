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
	"path/filepath"

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
		{"Installing standard library", func() error {
			return install.InstallStd(".")
		}},
		{"Installing documentation", func() error {
			return install.InstallDocs(".")
		}},
		{"Registering built-in components", func() error { return install.RegisterBuiltinComponents(".") }},
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
	keepData := false
	for _, arg := range os.Args[1:] {
		if arg == "--keep-data" {
			keepData = true
		}
	}

	home, err := install.GetRhoHome()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// Say what is about to go, because the tool home holds more than the
	// toolchain: the registry, the installed modules, the module cache and the
	// credentials all live here.
	if keepData {
		fmt.Println("Removing the Rho toolchain, keeping the registry, modules, cache and credentials...")
	} else {
		fmt.Println("Uninstalling the Rho ecosystem, including the registry, installed modules,")
		fmt.Println("module cache and stored credentials. Keep them with: rho-install --uninstall --keep-data")
	}

	for _, name := range install.HomeContents() {
		path := filepath.Join(home, name)
		if keepData && install.IsUserData(name) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			fmt.Printf("ERROR: removing %s: %v\n", name, err)
			os.Exit(1)
		}
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
