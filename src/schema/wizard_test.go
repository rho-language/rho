//
// @file wizard_test.go
// @brief Tests for the deployment wizard schema.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package schema

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/yaml"
)

func TestValidateWizardDocument(t *testing.T) {
	src := `title: "Deployment"
description: "A gateway and a node."
steps:
  plan: "steps/plan.yml"
  verify: "steps/verify.yml"
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if result := ValidateWizardDocument(data); result.HasErrors() {
		t.Fatalf("expected a valid document, got: %s", result.String())
	}
}

func TestValidateWizardStep(t *testing.T) {
	src := `title: "Plan"
description: "Choose the boards."
fields:
  gateway:
    label: "Gateway"
    type: "board"
    role: "gateway"
    default: "native"
    required: "true"
  link:
    label: "Link"
    type: "select"
    options: "usb wifi"
actions:
  build_node:
    label: "Build node"
    handler: "build_node"
    report: "build-node"
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if result := ValidateWizardStep(data); result.HasErrors() {
		t.Fatalf("expected a valid step, got: %s", result.String())
	}
}

func TestValidateWizardStepRejectsBadField(t *testing.T) {
	src := `title: "Plan"
description: "Choose the boards."
fields:
  gateway:
    label: "Gateway"
    type: "board"
  mode:
    label: "Mode"
    type: "colour"
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	result := ValidateWizardStep(data)
	if !result.HasErrors() {
		t.Fatal("expected an unknown field type and a board without a role to be refused")
	}
}

func TestValidateWizardStepRejectsActionWithoutWork(t *testing.T) {
	src := `title: "Build"
description: "Run something."
actions:
  nothing:
    label: "Nothing"
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	result := ValidateWizardStep(data)
	if !result.HasErrors() {
		t.Fatal("expected an action with neither a handler nor a command to be refused")
	}
}

func TestValidateWizardFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("wizard.yml", `title: "Deployment"
description: "A gateway and a node."
steps:
  plan: "steps/plan.yml"
`)
	write("steps/plan.yml", `title: "Plan"
description: "Choose."
fields:
  gateway:
    label: "Gateway"
    type: "board"
    role: "gateway"
`)
	if result := ValidateWizardFiles(root); result.HasErrors() {
		t.Fatalf("expected the files to validate, got: %s", result.String())
	}

	write("steps/plan.yml", `title: "Plan"
description: "Choose."
fields:
  gateway:
    label: "Gateway"
    type: "board"
`)
	if result := ValidateWizardFiles(root); !result.HasErrors() {
		t.Fatal("expected a board field without a role to be refused")
	}
}

// The keeper's own documents are validated when the test is pointed at them.
// A repository that does not carry the keeper skips this; a change to a step
// that would make the engine read a missing key fails here instead.
func TestKeeperWizardDocuments(t *testing.T) {
	root := os.Getenv("ACME_MESH_WIZARD_DIR")
	if root == "" {
		root = os.Getenv("ACME_KEEPER_WIZARD_DIR")
	}
	if root == "" {
		t.Skip("ACME_MESH_WIZARD_DIR is not set")
	}
	result := ValidateWizardFiles(root)
	if result.HasErrors() {
		t.Fatalf("keeper wizard documents are invalid:\n%s", result.ErrorString())
	}
}
