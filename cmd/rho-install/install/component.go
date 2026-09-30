//
// @file component.go
// @brief Rho Install - Component registration subsystem.
//
// Registers a pre-built component binary so `rho <command>` dispatches to it
// without requiring `rho module publish`. This is how the LSP and other
// bundled components are made available during a source-free deployment.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"rho/src/deps"
)

// RegisterComponent installs a pre-built component binary and its manifest
// into ~/.rho/modules/installed/component/<name>@<version>/ so the dynamic
// module runner in the rho CLI can dispatch to it via `rho <command>`.
//
// manifestDir is the directory containing module.yml for the component, and
// binaryPath is the compiled binary. The command a component answers to is
// read from the manifest rather than passed in, so the two cannot disagree:
// a component whose manifest says `command: fmt` is reached as `rho fmt`
// because the manifest says so.
func RegisterComponent(name, version, manifestDir, binaryPath string) error {
	rhoHome, err := GetRhoHome()
	if err != nil {
		return err
	}

	componentDir := filepath.Join(rhoHome, "modules", "installed", "component", name+"@"+version)
	if err := os.MkdirAll(componentDir, 0o755); err != nil {
		return fmt.Errorf("cannot create component directory: %w", err)
	}

	manifestSrc := filepath.Join(manifestDir, "module.yml")
	if _, err := os.Stat(manifestSrc); err != nil {
		return fmt.Errorf("component manifest not found: %s", manifestSrc)
	}

	manifestData, err := os.ReadFile(manifestSrc)
	if err != nil {
		return fmt.Errorf("cannot read manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(componentDir, "module.yml"), manifestData, 0o644); err != nil {
		return fmt.Errorf("cannot write manifest: %w", err)
	}

	binaryDest := filepath.Join(componentDir, filepath.Base(binaryPath))
	in, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("cannot open binary: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(binaryDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("cannot write binary: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("cannot copy binary: %w", err)
	}

	return nil
}

// BuiltinComponent is a component that ships with the toolchain rather than
// being fetched from a registry.
type BuiltinComponent struct {
	Name    string
	Version string
	// ManifestDir is the directory holding module.yml, relative to the root the
	// install was run from.
	ManifestDir string
	// BinaryPaths are where the built binary may be, in the order they are tried.
	// A checkout builds it into ./bin, because that is where the toolchain's
	// other executables go; a package carries it beside its manifest, because
	// that is where `main:` in the manifest says it is. Both are the same file
	// and an install that looked for only one of them would ship a toolchain
	// whose language server is missing from whichever layout it did not expect.
	BinaryPaths []string
}

// BuiltinComponents are the components an install puts in place itself.
//
// The language server is here because an editor without one is an editor that
// cannot tell whether the code in front of it compiles. Every other component
// is published and installed deliberately, and an install that quietly added
// them would take that decision away.
func BuiltinComponents() []BuiltinComponent {
	return []BuiltinComponent{
		{
			Name:        "rho-lsp",
			Version:     "0.1.0",
			ManifestDir: filepath.Join("modules", "components", "rho-lsp"),
			BinaryPaths: []string{
				filepath.Join("bin", "rho-lsp"),
				filepath.Join("modules", "components", "rho-lsp", "rho-lsp"),
			},
		},
	}
}

// RegisterBuiltinComponents puts the components that ship with the toolchain in
// place, so `rho <command>` reaches them on a machine that has the toolchain
// and not a registry to fetch them from.
//
// It also turns them on, which is the one thing it does that installing a
// component from a registry deliberately does not do. A component fetched from a
// registry is installed and not enabled, because a program that runs code its
// author did not ask for is a program running code nobody chose — that decision
// belongs to the person who installed it. A component that is part of the
// toolchain is different: the person asked for Rho, and a Rho without its
// language server is an editor that cannot tell them whether their code
// compiles. Enabling goes through the same record `rho module enable` writes,
// so there is one way to be enabled rather than two that can disagree.
//
// Enabling happens only where the component was just put in place. An install
// that found it already there leaves it exactly as it is, so a component
// somebody deliberately turned off stays off.
//
// sourceRoot is the directory the install was run from, and is empty-safe: a
// component whose manifest or binary is not there is left alone rather than
// failing the install, because an install that refuses to finish over a
// component that is not part of it leaves a machine with no toolchain at all.
func RegisterBuiltinComponents(sourceRoot string) error {
	for _, component := range BuiltinComponents() {
		manifestDir := filepath.Join(sourceRoot, component.ManifestDir)
		if _, err := os.Stat(filepath.Join(manifestDir, "module.yml")); err != nil {
			continue
		}

		binary := ""
		for _, candidate := range component.BinaryPaths {
			path := filepath.Join(sourceRoot, candidate)
			if _, err := os.Stat(path); err == nil {
				binary = path
				break
			}
		}
		if binary == "" {
			continue
		}

		// An already-registered component is left as it is. It may be a newer
		// version than this toolchain carries, published deliberately by
		// whoever put it there, and an install that overwrote it would replace
		// a choice with a default.
		home, err := GetRhoHome()
		if err != nil {
			return err
		}
		installed := filepath.Join(home, "modules", "installed", "component",
			component.Name+"@"+component.Version)
		if _, err := os.Stat(installed); err == nil {
			continue
		}

		if err := RegisterComponent(component.Name, component.Version, manifestDir, binary); err != nil {
			return fmt.Errorf("cannot register component %s: %w", component.Name, err)
		}
		if err := deps.SetVersionEnabled(deps.KindComponent, component.Name, component.Version); err != nil {
			return fmt.Errorf("cannot enable component %s: %w", component.Name, err)
		}
	}
	return nil
}
