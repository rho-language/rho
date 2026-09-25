//
// @file descriptor_test.go
// @brief Tests for the generated-project descriptor.
//
// Validates:
//   - the built-in descriptor parses and covers the built-in targets
//   - targets, roles, kinds, requires and output roots are parsed
//   - malformed descriptors are rejected with actionable errors
//   - path traversal and duplicate entries are refused
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Built-in Descriptor                                                 */
/* -------------------------------------------------------------------------- */

// The default descriptor must stay valid, otherwise every build fails.
func TestDefaultDescriptorParses(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	if descriptor.Version == "" {
		t.Fatal("expected a descriptor version")
	}
	if len(descriptor.Targets) == 0 {
		t.Fatal("expected the default descriptor to declare targets")
	}
}

// Every target the manifest may request must exist in the descriptor.
func TestDefaultDescriptorCoversBuildTargets(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	for _, id := range []string{"c", "go"} {
		if _, ok := descriptor.Target(id); !ok {
			t.Fatalf("default descriptor is missing the %q target", id)
		}
	}
}

// The C target must keep producing the layout C builds have always used.
func TestDefaultDescriptorCTargetLayout(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	target, ok := descriptor.Target("c")
	if !ok {
		t.Fatal("missing c target")
	}
	byRole := map[string]string{}
	for _, file := range target.Files {
		byRole[file.Role] = file.Path
	}
	for role, want := range map[string]string{
		"implementation": "app.c",
		"interface":      "app.h",
		"runtime":        "runtime.c",
		"entrypoint":     "main.c",
		"build":          "CMakeLists.txt",
	} {
		if byRole[role] != want {
			t.Fatalf("c target role %q: got %q, want %q", role, byRole[role], want)
		}
	}
}

// A Go server target must exist for web applications.
func TestDefaultDescriptorGoServerTarget(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	target, ok := descriptor.Target("go-server")
	if !ok {
		t.Fatal("missing go-server target")
	}
	if target.TargetKind() != "server" {
		t.Fatalf("expected server kind, got %q", target.TargetKind())
	}
	if target.Emitter != "go" {
		t.Fatalf("expected the go emitter, got %q", target.Emitter)
	}
	hasServer := false
	for _, file := range target.Files {
		if file.Role == "server" {
			hasServer = true
		}
	}
	if !hasServer {
		t.Fatal("go-server target should declare a server role file")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Parsing Target Settings                                             */
/* -------------------------------------------------------------------------- */

const descriptorWithSettings = `version: 1
output_dir: dist/gen
build_dir: dist/build
targets:
  - id: api
    emitter: go
    kind: server
    module_path: example.com/org/api
    output_dir: dist/api-gen
    requires:
      - github.com/go-chi/chi/v5
      - github.com/google/uuid
    files:
      - path: main.go
        role: entrypoint
      - path: app.go
        role: implementation
`

func TestParseDescriptorTargetSettings(t *testing.T) {
	descriptor, err := ParseDescriptor([]byte(descriptorWithSettings))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	if descriptor.Version != "1" {
		t.Fatalf("unexpected version %q", descriptor.Version)
	}
	if descriptor.OutputDir != "dist/gen" {
		t.Fatalf("unexpected output_dir %q", descriptor.OutputDir)
	}
	if descriptor.BuildDir != "dist/build" {
		t.Fatalf("unexpected build_dir %q", descriptor.BuildDir)
	}

	target, ok := descriptor.Target("api")
	if !ok {
		t.Fatal("missing api target")
	}
	if target.Kind != "server" {
		t.Fatalf("unexpected kind %q", target.Kind)
	}
	if target.ModulePath != "example.com/org/api" {
		t.Fatalf("unexpected module_path %q", target.ModulePath)
	}
	if len(target.Requires) != 2 {
		t.Fatalf("expected 2 requires, got %d", len(target.Requires))
	}
	if target.Requires[0] != "github.com/go-chi/chi/v5" {
		t.Fatalf("unexpected first require %q", target.Requires[0])
	}
}

func TestDescriptorOutputRootPrefersTargetOverride(t *testing.T) {
	descriptor, err := ParseDescriptor([]byte(descriptorWithSettings))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	target, _ := descriptor.Target("api")
	if got := descriptor.OutputRoot(target); got != "dist/api-gen" {
		t.Fatalf("expected the target override, got %q", got)
	}
}

func TestDescriptorOutputRootFallsBackToDescriptor(t *testing.T) {
	descriptor, err := ParseDescriptor([]byte(descriptorWithSettings))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	// A target without its own output_dir uses the descriptor-wide value.
	descriptor.Targets[0].OutputDir = ""
	if got := descriptor.OutputRoot(descriptor.Targets[0]); got != "dist/gen" {
		t.Fatalf("expected the descriptor value, got %q", got)
	}
}

func TestDescriptorDefaultsWhenUnset(t *testing.T) {
	descriptor, err := ParseDescriptor([]byte("version: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n"))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	target, _ := descriptor.Target("c")
	if got := descriptor.OutputRoot(target); got != DefaultOutputDir {
		t.Fatalf("expected the default output root, got %q", got)
	}
	if got := descriptor.BuildRoot(); got != DefaultBuildDir {
		t.Fatalf("expected the default build root, got %q", got)
	}
	if target.TargetKind() != "cli" {
		t.Fatalf("expected the default kind cli, got %q", target.TargetKind())
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Folder Entries                                                      */
/* -------------------------------------------------------------------------- */

func TestParseDescriptorDirEntry(t *testing.T) {
	src := `version: 1
targets:
  - id: web
    emitter: go
    kind: server
    files:
      - path: main.go
        role: entrypoint
      - path: static
        kind: dir
`
	descriptor, err := ParseDescriptor([]byte(src))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	target, _ := descriptor.Target("web")
	found := false
	for _, file := range target.Files {
		if file.Path == "static" {
			found = true
			if !file.IsDir() {
				t.Fatal("expected the static entry to be a directory")
			}
			if file.Role != "" {
				t.Fatal("a directory entry must not carry a role")
			}
		}
	}
	if !found {
		t.Fatal("expected a static directory entry")
	}
}

// A file entry must name a role; without one the generator cannot fill it.
func TestParseDescriptorFileWithoutRole(t *testing.T) {
	src := `version: 1
targets:
  - id: broken
    emitter: c
    files:
      - path: app.c
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for a file entry without a role")
	}
}

// A directory entry has no contents, so naming a role is contradictory.
func TestParseDescriptorDirWithRole(t *testing.T) {
	src := `version: 1
targets:
  - id: broken
    emitter: c
    files:
      - path: app.c
        role: implementation
      - path: static
        kind: dir
        role: runtime
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for a dir entry carrying a role")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Malformed Descriptors                                               */
/* -------------------------------------------------------------------------- */

func TestParseDescriptorRejectsMissingVersion(t *testing.T) {
	if _, err := ParseDescriptor([]byte("targets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n")); err == nil {
		t.Fatal("expected an error for a missing version")
	}
}

func TestParseDescriptorRejectsUnknownRole(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
      - path: app.c
        role: nonsense
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for an unknown role")
	}
}

func TestParseDescriptorRejectsUnknownKind(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    kind: microkernel
    files:
      - path: app.c
        role: implementation
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for an unknown target kind")
	}
}

func TestParseDescriptorRejectsDuplicateTarget(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
      - path: app.c
        role: implementation
  - id: c
    emitter: c
    files:
      - path: other.c
        role: implementation
`
	err := parseDescriptorErr(t, src)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected a duplicate target error, got %v", err)
	}
}

func TestParseDescriptorRejectsDuplicateFile(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
      - path: app.c
        role: implementation
      - path: app.c
        role: implementation
`
	err := parseDescriptorErr(t, src)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected a duplicate file error, got %v", err)
	}
}

func TestParseDescriptorRejectsNoFiles(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for a target with no files")
	}
}

func TestParseDescriptorRejectsNoTargets(t *testing.T) {
	if _, err := ParseDescriptor([]byte("version: 1\ntargets:\n")); err == nil {
		t.Fatal("expected an error for a descriptor with no targets")
	}
}

func TestParseDescriptorRejectsEscapingPath(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
      - path: ../escape.c
        role: implementation
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for a file path outside the project")
	}
}

func TestParseDescriptorRejectsAbsolutePath(t *testing.T) {
	src := `version: 1
targets:
  - id: c
    emitter: c
    files:
      - path: /etc/passwd
        role: implementation
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for an absolute file path")
	}
}

func TestParseDescriptorRejectsEscapingOutputDir(t *testing.T) {
	src := `version: 1
output_dir: ../outside
targets:
  - id: c
    emitter: c
    files:
      - path: app.c
        role: implementation
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for an output_dir outside the project")
	}
}

func TestParseDescriptorRejectsEscapingBuildDir(t *testing.T) {
	src := `version: 1
build_dir: ../outside
targets:
  - id: c
    emitter: c
    files:
      - path: app.c
        role: implementation
`
	if _, err := ParseDescriptor([]byte(src)); err == nil {
		t.Fatal("expected an error for a build_dir outside the project")
	}
}

func TestParseDescriptorRejectsInvalidYAML(t *testing.T) {
	if _, err := ParseDescriptor([]byte("version: 1\nversion: 2\n")); err == nil {
		t.Fatal("expected an error for duplicate keys")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Target Resolution                                                   */
/* -------------------------------------------------------------------------- */

func TestResolveTargetsDefaultsToAll(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	targets, err := descriptor.ResolveTargets(nil)
	if err != nil {
		t.Fatalf("ResolveTargets failed: %v", err)
	}
	if len(targets) != len(descriptor.Targets) {
		t.Fatalf("expected all targets, got %d", len(targets))
	}
}

func TestResolveTargetsSelectsRequested(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	targets, err := descriptor.ResolveTargets([]string{"go"})
	if err != nil {
		t.Fatalf("ResolveTargets failed: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "go" {
		t.Fatalf("expected only the go target, got %+v", targets)
	}
}

func TestResolveTargetsDeduplicates(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	targets, err := descriptor.ResolveTargets([]string{"go", "go", " c "})
	if err != nil {
		t.Fatalf("ResolveTargets failed: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 unique targets, got %d", len(targets))
	}
}

func TestResolveTargetsRejectsUnknown(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	_, err = descriptor.ResolveTargets([]string{"rust"})
	if err == nil {
		t.Fatal("expected an error for an unknown target")
	}
	if !strings.Contains(err.Error(), "rust") {
		t.Fatalf("expected the unknown target named in the error, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Descriptor File I/O                                                 */
/* -------------------------------------------------------------------------- */

func TestParseDescriptorFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), DescriptorFileName)
	if err := os.WriteFile(path, []byte(descriptorWithSettings), 0644); err != nil {
		t.Fatal(err)
	}
	descriptor, err := ParseDescriptorFile(path)
	if err != nil {
		t.Fatalf("ParseDescriptorFile failed: %v", err)
	}
	if _, ok := descriptor.Target("api"); !ok {
		t.Fatal("expected the api target")
	}
}

func TestParseDescriptorFileMissing(t *testing.T) {
	if _, err := ParseDescriptorFile(filepath.Join(t.TempDir(), "absent.yml")); err == nil {
		t.Fatal("expected an error for a missing descriptor")
	}
}

func parseDescriptorErr(t *testing.T, src string) error {
	t.Helper()
	_, err := ParseDescriptor([]byte(src))
	return err
}
