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

	"rho/src/codegen"
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
	// Extra are files the backend decided a project needs, named by their path
	// relative to the generated project: the configuration glue is one. They are
	// written beside the role artifacts, and a project that already has a file
	// of that name keeps it, because a generated file the project wrote is not
	// one to overwrite.
	Extra map[string]string
}

// ExtraFile is a generated file the backend added, named by its path.
type ExtraFile struct {
	Name    string
	Content string
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
	// Config is the Kconfig bridge a project declared, or nil when it declared
	// none. A backend that generates a build file runs the merge and includes
	// what it wrote, so the configuration a project declares is resolved by the
	// build rather than assumed by the generator.
	Config *Config
}

// Config is the Kconfig bridge a project declared: the fragments that define
// its symbols, the values it sets, and the dialect the headers are written in.
//
// The generator carries it and does not interpret it. Merging fragments and
// writing a dialect is the rho-kconfig component's work, and a build that wants
// another dialect asks that component for it rather than growing a case in the
// backend.
type Config struct {
	// Dialect is which headers the merge writes: linux, espidf or freertos.
	Dialect string
	// Fragments are the Kconfig files, already copied into the generated
	// project's kconfig directory, as paths relative to that directory.
	Fragments []string
	// Values is a defconfig the merge starts from: the symbols a project commits
	// as its baseline.
	Defconfig string
	// ValuesFile is where the resolved configuration is written.
	ValuesFile string
	// OutputDir is the generated directory the configuration is written into.
	OutputDir string
	// ExtraArgs are anything else a project asked the merge to be given.
	ExtraArgs []string
}

// Configured reports whether a project declared a Kconfig bridge.
func (c *Config) Configured() bool { return c != nil }

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

// availablePorts lists the ports a backend can generate.
func availablePorts(emitter string) []string {
	switch emitter {
	case "go":
		return codegen.RegisteredPorts()
	default:
		return nil
	}
}

// generateAuth renders the authentication support a server target needs.
func generateAuth(ctx Context, target Target) error {
	if target.TargetKind() != "server" {
		return nil
	}
	rendered := codegen.EmitGoAuth()
	if rendered == "" {
		return nil
	}
	if err := os.WriteFile(filepath.Join(ctx.OutputDir, codegen.AuthFileName), []byte(rendered), 0644); err != nil {
		return fmt.Errorf("write %s: %w", codegen.AuthFileName, err)
	}
	return nil
}

// generateTemplates embeds the page templates a target carries, so a handler
// renders markup that lives in template files rather than in Rho string
// literals.
func generateTemplates(ctx Context, target Target) error {
	if len(target.Templates) == 0 {
		return nil
	}
	contents := map[string]string{}
	for _, template := range target.Templates {
		data, err := os.ReadFile(filepath.Join(ctx.ProjectRoot, filepath.FromSlash(template.File)))
		if err != nil {
			return fmt.Errorf("read template %s: %w", template.File, err)
		}
		contents[template.Name] = string(data)
	}
	rendered := codegen.EmitGoTemplates(contents)
	if rendered == "" {
		return nil
	}
	if err := os.WriteFile(filepath.Join(ctx.OutputDir, codegen.TemplateFileName), []byte(rendered), 0644); err != nil {
		return fmt.Errorf("write %s: %w", codegen.TemplateFileName, err)
	}
	return nil
}

// generatePorts writes the port implementations a target asked for. A port is
// supplied by the backend, so a Rho project needs no hand-written host code.
func generatePorts(ctx Context, target Target) error {
	if len(target.Ports) == 0 {
		return nil
	}
	available := availablePorts(target.Emitter)
	for _, name := range target.Ports {
		if !containsString(available, name) {
			return fmt.Errorf("target %q: unknown port %q (available: %s)",
				target.ID, name, strings.Join(available, ", "))
		}
	}

	content := codegen.EmitGoPorts(ctx.IR, target.Ports)
	if content == "" {
		return nil
	}
	if err := os.WriteFile(filepath.Join(ctx.OutputDir, codegen.PortFileName), []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", codegen.PortFileName, err)
	}
	return nil
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
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

	// Authentication, then page templates, then the built-in ports that
	// satisfy the module's extern declarations without shipping host code.
	if err := generateAuth(ctx, target); err != nil {
		return "", err
	}
	if err := generateTemplates(ctx, target); err != nil {
		return "", err
	}
	if err := generatePorts(ctx, target); err != nil {
		return "", err
	}

	// The files a backend added that no role names, such as the glue that
	// resolves a project's Kconfig bridge. A project that already has one keeps
	// it: a generated file the project wrote is not one to overwrite.
	for name, content := range artifacts.Extra {
		dest := filepath.Join(ctx.OutputDir, filepath.FromSlash(name))
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return "", fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Hand-written files the project optionally supplies.
	for _, entry := range target.Copy {
		if err := copyInto(ctx.ProjectRoot, entry.From, ctx.OutputDir, entry.To); err != nil {
			return "", fmt.Errorf("target %q: %w", target.ID, err)
		}
	}

	return ctx.OutputDir, nil
}

// copyInto copies one project file into the generated project.
func copyInto(projectRoot, from, outputDir, to string) error {
	source := filepath.Join(projectRoot, filepath.FromSlash(from))
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	dest := filepath.Join(outputDir, filepath.FromSlash(to))
	if dir := filepath.Dir(dest); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", to, err)
	}
	return nil
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
