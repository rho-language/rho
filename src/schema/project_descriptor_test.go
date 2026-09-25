//
// @file project_descriptor_test.go
// @brief Tests for the project descriptor schema.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"testing"

	"rho/src/yaml"
)

const validDescriptor = `version: 1
output_dir: build/generated
build_dir: build
targets:
  - id: c
    emitter: c
    kind: cli
    description: Native C project
    files:
      - path: app.c
        role: implementation
      - path: CMakeLists.txt
        role: build
  - id: go-server
    emitter: go
    kind: server
    module_path: example.com/api
    requires:
      - github.com/go-chi/chi/v5
    files:
      - path: main.go
        role: entrypoint
      - path: server.go
        role: server
      - path: static
        kind: dir
`

/* -------------------------------------------------------------------------- */
/*  Test: Valid Descriptor                                                    */
/* -------------------------------------------------------------------------- */

func TestProjectDescriptorValid(t *testing.T) {
	data, err := yaml.Load(validDescriptor)
	if err != nil {
		t.Fatal(err)
	}

	result := Validate(data, ProjectDescriptorSchema())
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

func TestValidateProjectDescriptorValid(t *testing.T) {
	data, err := yaml.Load(validDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateProjectDescriptor(data); errs.HasErrors() {
		t.Fatalf("expected no errors, got: %s", errs.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Required Fields                                                     */
/* -------------------------------------------------------------------------- */

func TestProjectDescriptorRequiresVersion(t *testing.T) {
	data, err := yaml.Load("targets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for a missing version")
	}
}

func TestProjectDescriptorRequiresTargets(t *testing.T) {
	data, err := yaml.Load("version: 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for missing targets")
	}
}

func TestProjectDescriptorRequiresFiles(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for a target without files")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Enumerations                                                        */
/* -------------------------------------------------------------------------- */

func TestProjectDescriptorRejectsUnknownKind(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n    kind: weird\n    files:\n      - path: app.c\n        role: implementation\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for an unknown target kind")
	}
}

func TestProjectDescriptorRejectsUnknownRole(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: mystery\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for an unknown role")
	}
}

func TestProjectDescriptorRejectsUnknownEntryKind(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        kind: symlink\n        role: implementation\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for an unknown entry kind")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Cross-Field Rules                                                  */
/* -------------------------------------------------------------------------- */

// A file entry with no role leaves the generator with nothing to emit.
func TestValidateProjectDescriptorFileNeedsRole(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n")
	if err != nil {
		t.Fatal(err)
	}
	errs := ValidateProjectDescriptor(data)
	if !errs.HasErrors() {
		t.Fatal("expected an error for a file entry without a role")
	}
	if !hasPath(errs, "targets[0].files[0].role") {
		t.Fatalf("expected the role path reported, got: %s", errs.String())
	}
}

// A directory has no contents, so naming a role is contradictory.
func TestValidateProjectDescriptorDirRejectsRole(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n      - path: static\n        kind: dir\n        role: runtime\n")
	if err != nil {
		t.Fatal(err)
	}
	errs := ValidateProjectDescriptor(data)
	if !errs.HasErrors() {
		t.Fatal("expected an error for a dir entry carrying a role")
	}
	if !hasPath(errs, "targets[0].files[1].role") {
		t.Fatalf("expected the role path reported, got: %s", errs.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Type Checks                                                         */
/* -------------------------------------------------------------------------- */

func TestProjectDescriptorRejectsNonStringVersion(t *testing.T) {
	data, err := yaml.Load("version:\n  nested: 1\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for a non-scalar version")
	}
}

func TestProjectDescriptorRejectsNonListRequires(t *testing.T) {
	data, err := yaml.Load("version: 1\ntargets:\n  - id: go\n    emitter: go\n    requires: not-a-list\n    files:\n      - path: main.go\n        role: entrypoint\n")
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(data, ProjectDescriptorSchema()).HasErrors() {
		t.Fatal("expected an error for a non-list requires")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func hasPath(errs ValidationErrors, path string) bool {
	for _, e := range errs {
		if e.Path == path {
			return true
		}
	}
	return false
}
