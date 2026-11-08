//
// @file descriptor.go
// @brief Generated-project descriptor for Rho build targets.
//
// A descriptor declares, per target language, the files that make up a
// generated project and the role each file plays. The generator is driven
// entirely by this data, so a new backend only needs a descriptor entry and
// an emitter registered under the same id.
//
// Descriptors are validated with rho/src/schema before use, so a malformed
// descriptor stops the build instead of producing a half-written project.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/codegen"
	"rho/src/schema"
	"rho/src/yaml"
)

// DescriptorFileName is the per-project descriptor a build looks for.
const DescriptorFileName = "project.yml"

// FileRole identifies the part an emitted file plays in a generated project.
type FileRole = string

// Template is a page template placed in the generated project.
type Template struct {
	// Name is how the template is requested, for example "index".
	Name string
	// File is the path relative to the Rho project root.
	File string
}

// Locale is one flat JSON catalog for a language. Several files may name the
// same locale; later files override earlier ones, so an application can layer
// its own strings over a module's.
type Locale struct {
	// Code is the language tag, lower-case, for example "en" or "ro".
	Code string
	// File is the path relative to the Rho project root.
	File string
}

// CopyEntry is a hand-written file copied into a generated project, given as a
// path relative to the Rho project root.
type CopyEntry struct {
	// From is the source path, relative to the Rho project root.
	From string
	// To is the destination path, relative to the generated project directory.
	To string
}

// DescriptorFile is one output file of a generated project.
type DescriptorFile struct {
	// Path is the file path relative to the generated project directory.
	// It may contain folders, which are created as needed.
	Path string
	// Kind is "file" (default) or "dir". A dir entry creates a folder and
	// carries no Role.
	Kind string
	// Role selects which emitter supplies the file contents.
	Role FileRole
	// Optional marks a file the generator may skip when it has no content.
	Optional bool
}

// IsDir reports whether the entry creates a folder rather than a file.
func (f DescriptorFile) IsDir() bool { return f.Kind == "dir" }

// Target describes one generated-project backend.
type Target struct {
	// ID is the target name used in module.yml build.targets.
	ID string
	// Emitter names the backend that produces file contents for this target.
	Emitter string
	// Description is human-readable documentation.
	Description string
	// Kind selects the entry point shape: "cli" or "server". Empty means "cli".
	Kind string
	// ModulePath overrides the derived Go module path.
	ModulePath string
	// OutputDir overrides the descriptor-wide output root for this target.
	OutputDir string
	// Requires lists extra Go module requirements for go.mod.
	Requires []string
	// BuildFlags lists extra flags to record for the build role.
	BuildFlags []string
	// Ports names built-in port implementations the backend generates, so a
	// module's extern declarations are supplied without shipping host code.
	Ports []string
	// Router names the HTTP router a server target dispatches through. Empty
	// means the backend's own router, which needs no module download.
	Router string
	// Middleware lists the wrappers composed around the router, outermost
	// first, in the order the target declared them.
	Middleware []Middleware
	// Templates are page templates carried into the generated project, which
	// keeps markup out of the Rho sources.
	Templates []Template
	// Locales are flat JSON catalogs carried into the generated project.
	Locales []Locale
	// DefaultLocale is the language used when a request names none. Empty
	// means "en".
	DefaultLocale string
	// Copy lists hand-written files to place in the generated project.
	Copy []CopyEntry
	// Files lists the output files for this target.
	Files []DescriptorFile
}

// Middleware is one entry of a target's middleware pipeline: the wrapper to
// compose, and the settings it is composed with.
type Middleware struct {
	// Name is the wrapper: request_id, cors, rate_limit or jwt.
	Name string
	// Options configures the wrapper. Every option a middleware understands
	// has a default, so a target names one and overrides what it cares about.
	Options map[string]string
}

// TargetKind returns the effective kind, defaulting to cli.
func (t Target) TargetKind() string {
	if t.Kind == "" {
		return "cli"
	}
	return t.Kind
}

// RoutingPortName is the port that supplies a router, the typed parameter
// binding and the middleware pipeline.
const RoutingPortName = schema.PortHTTPRoute

// SQLPortName is the port that supplies a database to a project whose
// statements were generated from SQL.
//
// It is supplied by declaring `sql:` rather than by naming the port, so a
// project that wrote SQL gets the code to run it without also having to
// remember that the two are the same decision.
const SQLPortName = "sql"

// EffectivePorts are the ports the generator renders for this target: the ones
// it declared, plus the routing port when it named a router or composed a
// middleware. Asking for a router is how a target asks for the routing port, so
// the two cannot drift apart.
func (t Target) EffectivePorts() []string {
	out := make([]string, 0, len(t.Ports)+1)
	for _, name := range t.Ports {
		if name == RoutingPortName {
			continue
		}
		out = append(out, name)
	}
	if t.Router != "" || len(t.Middleware) > 0 {
		out = append(out, RoutingPortName)
	}
	return out
}

// RouterRequirement returns the go.mod line the target's router adapter needs,
// or "" when it routes through the standard library. A project that chooses an
// adapter gets its dependency written for it.
func (t Target) RouterRequirement() string {
	if t.Router == "" {
		return ""
	}
	return codegen.RouterRequirement(t.Router)
}

// Descriptor is a parsed and validated project descriptor.
type Descriptor struct {
	Version string
	// OutputDir is the project-relative root generated projects are written
	// to. A target may override it.
	OutputDir string
	// BuildDir is the project-relative root compiled artefacts go to.
	BuildDir string
	Targets  []Target
}

// DefaultOutputDir is where generated projects go when the descriptor does
// not say otherwise.
const DefaultOutputDir = "build/generated"

// DefaultBuildDir is where compiled artefacts go when the descriptor does not
// say otherwise.
const DefaultBuildDir = "build"

// ParseDescriptor parses and validates descriptor YAML content.
func ParseDescriptor(content []byte) (*Descriptor, error) {
	root, err := yaml.Load(string(content))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in project descriptor: %w", err)
	}

	result := schema.ValidateProjectDescriptor(root)
	if result.HasErrors() {
		return nil, fmt.Errorf("project descriptor schema validation failed:\n%s", result.ErrorString())
	}

	descriptor := &Descriptor{
		Version:   root.Map["version"].Str,
		OutputDir: root.Map["output_dir"].Str,
		BuildDir:  root.Map["build_dir"].Str,
		Targets:   []Target{},
	}

	// A generated project must stay inside the project tree, so the output
	// root is project-relative and cannot climb out.
	if descriptor.OutputDir != "" && !isSafeRelativeDir(descriptor.OutputDir) {
		return nil, fmt.Errorf("project descriptor output_dir must stay inside the project: %q", descriptor.OutputDir)
	}
	if descriptor.BuildDir != "" && !isSafeRelativeDir(descriptor.BuildDir) {
		return nil, fmt.Errorf("project descriptor build_dir must stay inside the project: %q", descriptor.BuildDir)
	}

	targetsNode, ok := root.Map["targets"]
	if !ok || targetsNode.Kind != yaml.KindList {
		return nil, fmt.Errorf("project descriptor has no targets")
	}
	if len(targetsNode.List) == 0 {
		return nil, fmt.Errorf("project descriptor declares no targets")
	}

	seenIDs := map[string]bool{}
	for _, targetNode := range targetsNode.List {
		target, err := ParseTarget(targetNode)
		if err != nil {
			return nil, err
		}
		if seenIDs[target.ID] {
			return nil, fmt.Errorf("project descriptor declares target %q more than once", target.ID)
		}
		seenIDs[target.ID] = true
		descriptor.Targets = append(descriptor.Targets, target)
	}
	return descriptor, nil
}

// ParseTarget reads one target. A project descriptor's own entry and a
// `build.targets` definition in a module manifest are the same shape, so both
// are read here and a project can carry its build layout in one file.
func ParseTarget(targetNode yaml.Value) (Target, error) {
	if targetNode.Kind != yaml.KindMap {
		return Target{}, fmt.Errorf("target entries must be maps")
	}

	target := Target{
		ID:            targetNode.Map["id"].Str,
		Emitter:       targetNode.Map["emitter"].Str,
		Description:   targetNode.Map["description"].Str,
		Kind:          targetNode.Map["kind"].Str,
		ModulePath:    targetNode.Map["module_path"].Str,
		OutputDir:     targetNode.Map["output_dir"].Str,
		Requires:      []string{},
		BuildFlags:    []string{},
		Ports:         []string{},
		Router:        targetNode.Map["router"].Str,
		Middleware:    []Middleware{},
		Templates:     []Template{},
		Locales:       []Locale{},
		DefaultLocale: "en",
		Copy:          []CopyEntry{},
		Files:         []DescriptorFile{},
	}

	if target.OutputDir != "" && !isSafeRelativeDir(target.OutputDir) {
		return Target{}, fmt.Errorf("target %q output_dir must stay inside the project: %q", target.ID, target.OutputDir)
	}

	if target.ID == "" {
		return Target{}, fmt.Errorf("a target is missing an id")
	}

	if target.Emitter == "" {
		return Target{}, fmt.Errorf("target %q is missing an emitter", target.ID)
	}

	filesNode, ok := targetNode.Map["files"]
	if !ok || filesNode.Kind != yaml.KindList || len(filesNode.List) == 0 {
		return Target{}, fmt.Errorf("target %q declares no files", target.ID)
	}

	for _, item := range targetNode.Map["requires"].List {
		if item.Str != "" {
			target.Requires = append(target.Requires, item.Str)
		}
	}
	for _, item := range targetNode.Map["build_flags"].List {
		if item.Str != "" {
			target.BuildFlags = append(target.BuildFlags, item.Str)
		}
	}

	// Page templates placed in the generated project.
	for _, entry := range targetNode.Map["templates"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		item := Template{
			Name: entry.Map["name"].Str,
			File: entry.Map["file"].Str,
		}
		if item.Name == "" || item.File == "" {
			return Target{}, fmt.Errorf("target %q has a template entry without a name and file", target.ID)
		}
		if filepath.IsAbs(item.File) || strings.Contains(item.File, "..") {
			return Target{}, fmt.Errorf("target %q template file must stay inside the project: %q",
				target.ID, item.File)
		}
		target.Templates = append(target.Templates, item)
	}

	// Locale catalogs and the language a request gets when it names none.
	if value := targetNode.Map["default_locale"].Str; value != "" {
		target.DefaultLocale = strings.ToLower(value)
	}
	for _, entry := range targetNode.Map["locales"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		item := Locale{
			Code: strings.ToLower(entry.Map["locale"].Str),
			File: entry.Map["file"].Str,
		}
		if item.Code == "" || item.File == "" {
			return Target{}, fmt.Errorf("target %q has a locale entry without a locale and file", target.ID)
		}
		if !isLocaleCode(item.Code) {
			return Target{}, fmt.Errorf("target %q names an invalid locale %q", target.ID, item.Code)
		}
		// Locale files may live beside a module the project includes, so a
		// relative path may step out of the project root. The path is read at
		// build time from the trusted project descriptor.
		if filepath.IsAbs(item.File) || strings.HasPrefix(item.File, "~") {
			return Target{}, fmt.Errorf("target %q locale file must be project-relative: %q",
				target.ID, item.File)
		}
		target.Locales = append(target.Locales, item)
	}
	if len(target.Locales) > 0 {
		found := false
		for _, item := range target.Locales {
			if item.Code == target.DefaultLocale {
				found = true
				break
			}
		}
		if !found {
			return Target{}, fmt.Errorf("target %q default_locale %q has no locale file",
				target.ID, target.DefaultLocale)
		}
	}

	// Built-in port implementations the backend generates.
	for _, item := range targetNode.Map["ports"].List {
		if item.Str != "" {
			target.Ports = append(target.Ports, item.Str)
		}
	}

	// The middleware pipeline, in the order it was declared. A wrapper with
	// no options is composed with that wrapper's defaults, so the common
	// case stays a single name.
	seenMiddleware := map[string]bool{}
	for _, entry := range targetNode.Map["middleware"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		item := Middleware{Name: entry.Map["name"].Str, Options: map[string]string{}}
		if item.Name == "" {
			return Target{}, fmt.Errorf("target %q has a middleware entry without a name", target.ID)
		}
		if seenMiddleware[item.Name] {
			return Target{}, fmt.Errorf("target %q declares middleware %q more than once", target.ID, item.Name)
		}
		seenMiddleware[item.Name] = true
		for key, value := range entry.Map["options"].Map {
			if value.Kind != yaml.KindString {
				return Target{}, fmt.Errorf("target %q middleware %q option %q must be a string",
					target.ID, item.Name, key)
			}
			item.Options[key] = value.Str
		}
		target.Middleware = append(target.Middleware, item)
	}

	// A pipeline is a property of the router it wraps, so it only means
	// something on a target that routes at all.
	if len(target.Middleware) > 0 && target.Router == "" {
		return Target{}, fmt.Errorf("target %q composes middleware but names no router", target.ID)
	}
	if target.Router != "" && target.TargetKind() != "server" {
		return Target{}, fmt.Errorf("target %q names a router but its kind is %q", target.ID, target.TargetKind())
	}

	// Hand-written files placed in the generated project.
	for _, entry := range targetNode.Map["copy"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		item := CopyEntry{
			From: entry.Map["from"].Str,
			To:   entry.Map["to"].Str,
		}
		if item.To == "" {
			item.To = item.From
		}
		if item.From == "" {
			return Target{}, fmt.Errorf("target %q has a copy entry without a from path", target.ID)
		}
		if filepath.IsAbs(item.From) || strings.Contains(item.From, "..") ||
			filepath.IsAbs(item.To) || strings.Contains(item.To, "..") {
			return Target{}, fmt.Errorf("target %q copy paths must stay inside the project: %q -> %q",
				target.ID, item.From, item.To)
		}
		target.Copy = append(target.Copy, item)
	}

	seenPaths := map[string]bool{}
	for _, fileNode := range filesNode.List {
		if fileNode.Kind != yaml.KindMap {
			return Target{}, fmt.Errorf("target %q file entries must be maps", target.ID)
		}
		file := DescriptorFile{
			Path:     fileNode.Map["path"].Str,
			Kind:     fileNode.Map["kind"].Str,
			Role:     fileNode.Map["role"].Str,
			Optional: isTrueWord(fileNode.Map["optional"].Str),
		}
		if file.Kind == "" {
			file.Kind = "file"
		}
		if file.Path == "" {
			return Target{}, fmt.Errorf("target %q has a file without a path", target.ID)
		}
		if filepath.IsAbs(file.Path) || strings.Contains(file.Path, "..") {
			return Target{}, fmt.Errorf("target %q file path must stay inside the generated project: %q", target.ID, file.Path)
		}
		if seenPaths[file.Path] {
			return Target{}, fmt.Errorf("target %q declares file %q more than once", target.ID, file.Path)
		}
		seenPaths[file.Path] = true
		target.Files = append(target.Files, file)
	}

	return target, nil
}

// ParseDescriptorFile reads and validates a descriptor from disk.
func ParseDescriptorFile(path string) (*Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	descriptor, err := ParseDescriptor(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return descriptor, nil
}

// WithTargets returns a descriptor with the given targets merged in by id: a
// target that names one the descriptor already declares replaces it, and one it
// does not is appended. A module manifest defines its targets this way, so the
// descriptor's own and the manifest's are one list.
func (d *Descriptor) WithTargets(overrides []Target) *Descriptor {
	if len(overrides) == 0 {
		return d
	}
	merged := *d
	merged.Targets = append([]Target(nil), d.Targets...)
	for _, override := range overrides {
		replaced := false
		for i, target := range merged.Targets {
			if target.ID == override.ID {
				merged.Targets[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			merged.Targets = append(merged.Targets, override)
		}
	}
	return &merged
}

// Target returns the named target, or false when the descriptor lacks it.
func (d *Descriptor) Target(id string) (Target, bool) {
	for _, target := range d.Targets {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

// TargetIDs returns every declared target id in order.
func (d *Descriptor) TargetIDs() []string {
	out := make([]string, 0, len(d.Targets))
	for _, target := range d.Targets {
		out = append(out, target.ID)
	}
	return out
}

// OutputRoot returns the project-relative directory that generated projects
// are written to, preferring a target-specific override.
func (d *Descriptor) OutputRoot(target Target) string {
	if target.OutputDir != "" {
		return target.OutputDir
	}
	if d.OutputDir != "" {
		return d.OutputDir
	}
	return DefaultOutputDir
}

// BuildRoot returns the project-relative directory for compiled artefacts.
func (d *Descriptor) BuildRoot() string {
	if d.BuildDir != "" {
		return d.BuildDir
	}
	return DefaultBuildDir
}

// isSafeRelativeDir reports whether a project-relative directory stays inside
// the project tree.
func isSafeRelativeDir(dir string) bool {
	if dir == "" {
		return false
	}
	if filepath.IsAbs(dir) || strings.HasPrefix(dir, "~") {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(dir)))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned == "." {
		return false
	}
	return !strings.Contains(cleaned, ":")
}

// ResolveTargets picks the targets to generate for a request, validating that
// each requested id exists in the descriptor. An empty request keeps the
// descriptor's own order, which preserves the historical C-only default.
func (d *Descriptor) ResolveTargets(requested []string) ([]Target, error) {
	if len(requested) == 0 {
		return append([]Target{}, d.Targets...), nil
	}

	out := make([]Target, 0, len(requested))
	seen := map[string]bool{}
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		target, ok := d.Target(id)
		if !ok {
			return nil, fmt.Errorf("unknown build target %q (descriptor defines: %s)", id, strings.Join(d.TargetIDs(), ", "))
		}
		out = append(out, target)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no build targets selected")
	}
	return out, nil
}

func isTrueWord(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

// isLocaleCode accepts the tags a flat catalog may name: a lower-case primary
// language of two or three letters, optionally followed by subtags.
func isLocaleCode(code string) bool {
	parts := strings.Split(code, "-")
	if len(parts) == 0 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, r := range parts[0] {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	for _, part := range parts[1:] {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
				return false
			}
		}
	}
	return true
}
