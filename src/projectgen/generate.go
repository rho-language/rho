//
// @file generate.go
// @brief Descriptor-driven generation of Rho target projects.
//
// The generator walks a descriptor target, asks the matching backend for the
// artifact of each role, and writes the files the descriptor lists. Adding a
// backend therefore means registering an emitter and describing its files,
// with no changes here.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/ir"
)

// errGoUnsupportedMain reports a Rho entry point that has no Go equivalent.
var errGoUnsupportedMain = errors.New("go target: entry point main has a return type and cannot be generated")

// Artifacts holds generated file contents keyed by role.
type Artifacts struct {
	Implementation string
	Interface      string
	Runtime        string
	Entrypoint     string
	Server         string
	Build          string
}

// RoleContent returns the artifact registered for a role.
func (a Artifacts) RoleContent(role string) (string, bool) {
	switch role {
	case "implementation":
		return a.Implementation, true
	case "interface":
		return a.Interface, true
	case "runtime":
		return a.Runtime, true
	case "entrypoint":
		return a.Entrypoint, true
	case "server":
		return a.Server, true
	case "build":
		return a.Build, true
	default:
		return "", false
	}
}

// Context carries everything a backend needs to render a project.
type Context struct {
	// ModuleName is the Rho module name, used for banners and build files.
	ModuleName string
	// ProjectRoot is the Rho project directory.
	ProjectRoot string
	// OutputDir is the directory the generated project is written to.
	OutputDir string
	// IR is the compiled Rho module.
	IR ir.ModuleIR
	// Target is the descriptor entry being generated, so a backend can read
	// kind, module_path and requires.
	Target Target
	// IncludePaths and SourceFiles carry C dependency inputs.
	IncludePaths []string
	SourceFiles  []string
}

// Backend renders the artifacts for one emitter id.
type Backend func(Context) (Artifacts, error)

var backends = map[string]Backend{
	"c":  EmitCProject,
	"go": EmitGoProject,
}

// RegisterBackend makes a backend available to descriptors.
func RegisterBackend(emitter string, backend Backend) {
	backends[emitter] = backend
}

// RegisteredEmitters lists the available emitter ids.
func RegisteredEmitters() []string {
	out := make([]string, 0, len(backends))
	for name := range backends {
		out = append(out, name)
	}
	return out
}

// Generate writes one generated project for a descriptor target.
// It returns the directory the project was written to.
func Generate(ctx Context, target Target) (string, error) {
	backend, ok := backends[target.Emitter]
	if !ok {
		return "", fmt.Errorf("target %q uses unknown emitter %q (registered: %s)",
			target.ID, target.Emitter, strings.Join(sortedEmitters(), ", "))
	}

	// Let the backend read the descriptor entry it is rendering.
	ctx.Target = target

	// The directory must exist before the backend runs, because a backend may
	// write auxiliary files of its own.
	if err := os.MkdirAll(ctx.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("create generated project: %w", err)
	}

	artifacts, err := backend(ctx)
	if err != nil {
		return "", fmt.Errorf("target %q: %w", target.ID, err)
	}

	for _, file := range target.Files {
		dest := filepath.Join(ctx.OutputDir, filepath.FromSlash(file.Path))

		// A dir entry creates a folder and carries no contents.
		if file.IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return "", fmt.Errorf("create directory %s: %w", file.Path, err)
			}
			continue
		}

		// File paths may name folders; create them before writing.
		if dir := filepath.Dir(dest); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("create %s: %w", dir, err)
			}
		}

		content, known := artifacts.RoleContent(file.Role)
		if !known {
			return "", fmt.Errorf("target %q file %q uses unknown role %q", target.ID, file.Path, file.Role)
		}
		if content == "" {
			if file.Optional {
				continue
			}
			return "", fmt.Errorf("target %q file %q has no %s content", target.ID, file.Path, file.Role)
		}

		if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", file.Path, err)
		}
	}

	return ctx.OutputDir, nil
}

func sortedEmitters() []string {
	out := RegisteredEmitters()
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
