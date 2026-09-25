//
// @file module_manifest_test.go
// @brief Tests for Rho module.yml schema validation.
//
// Validates:
//   - valid module.yml passes validation
//   - missing required fields produce errors
//   - invalid module type produces error
//   - type-specific rules (e.g., component command warning)
//   - depends entry validation (git/path requirement)
//   - build section validation
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Valid Rho App                                                        */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestValidRho(t *testing.T) {
	src := `module:
  name: hello
  type: rho
  version: 0.1.0
main: src/main.rho
features:
  - math
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if result.HasErrors() {
		t.Fatalf("expected no errors for valid rho app, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Valid Component                                                      */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestValidComponent(t *testing.T) {
	src := `module:
  name: rho-lsp
  type: component
  version: 0.1.0
  command: lsp
main: rho-lsp
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "component")
	if result.HasErrors() {
		t.Fatalf("expected no errors for valid component, got: %s", result.String())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Missing Required Fields                                              */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestMissingName(t *testing.T) {
	src := `module:
  type: rho
  version: 0.1.0
main: src/main.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for missing module.name")
	}
}

func TestValidateModuleManifestMissingType(t *testing.T) {
	src := `module:
  name: hello
  version: 0.1.0
main: src/main.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "")
	if !result.HasErrors() {
		t.Fatal("expected error for missing module.type")
	}
}

func TestValidateModuleManifestMissingVersion(t *testing.T) {
	src := `module:
  name: hello
  type: rho
main: src/main.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for missing module.version")
	}
}

func TestValidateModuleManifestMissingMain(t *testing.T) {
	src := `module:
  name: hello
  type: rho
  version: 0.1.0
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for missing main")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Invalid Module Type                                                */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestInvalidType(t *testing.T) {
	src := `module:
  name: hello
  type: invalid
  version: 0.1.0
main: src/main.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "invalid")
	if !result.HasErrors() {
		t.Fatal("expected error for invalid module type")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Component Command Warning                                          */
/* -------------------------------------------------------------------------- */

func TestValidateComponentMissingCommandWarning(t *testing.T) {
	src := `module:
  name: mycomp
  type: component
  version: 0.1.0
main: mycomp
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "component")
	// Component without command should have a warning
	hasWarning := false
	for _, e := range result {
		if e.Severity == SeverityWarning && e.Path == "module.command" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Fatal("expected warning for component without command")
	}
	// But no errors
	if result.HasErrors() {
		t.Fatal("expected no errors for component without command")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Plugin Should Not Have Command                                     */
/* -------------------------------------------------------------------------- */

func TestValidatePluginWithCommandError(t *testing.T) {
	src := `module:
  name: myplug
  type: plugin
  version: 0.1.0
  command: bad
main: src/plugin.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "plugin")
	if !result.HasErrors() {
		t.Fatal("expected error for plugin with command")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Rho/Module Main Must Be .rho                                       */
/* -------------------------------------------------------------------------- */

func TestValidateRhoMainNotRhoFile(t *testing.T) {
	src := `module:
  name: hello
  type: rho
  version: 0.1.0
main: main.bin
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for non-.rho main file in rho module")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Depends Validation                                                 */
/* -------------------------------------------------------------------------- */

func TestValidateDependsMissingGitAndPath(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
depends:
  - name: dep
    type: c
    version: 1.0.0
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for depends entry without git or path")
	}
}

func TestValidateDependsWithTagAndBranch(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
depends:
  - name: dep
    type: c
    git: https://example.com/dep.git
    tag: v1.0.0
    branch: main
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for depends entry with both tag and branch")
	}
}

func TestValidateDependsValid(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
depends:
  - name: dep
    type: c
    git: https://example.com/dep.git
    tag: v1.0.0
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if result.HasErrors() {
		t.Fatalf("expected no errors for valid depends, got: %s", result.String())
	}
}

// type is optional on a depends entry: the resolver infers it from the
// dependency's own manifest when it is absent.
func TestValidateDependsTypeOptional(t *testing.T) {
	src := `module:
  name: robohub-common-app
  type: rho
  version: 0.1.0
main: src/main.rho
depends:
  - name: robohub-common
    version: 1.0.0
    path: /opt/robohub-common
    git: https://github.com/RoboHubPlatform/robohub-common
    branch: main
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if result.HasErrors() {
		t.Fatalf("expected no errors when depends.type is omitted, got: %s", result.String())
	}
}

func TestValidateDependsInvalidType(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
depends:
  - name: dep
    type: rust
    git: https://example.com/dep.git
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for invalid depends type")
	}
}

func TestValidateDependsMissingName(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
depends:
  - type: c
    git: https://example.com/dep.git
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for depends entry without name")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Build Section Validation                                           */
/* -------------------------------------------------------------------------- */

func TestValidateBuildSectionValid(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
build:
  language: c
  include_dirs:
    - /opt/include
  sources:
    - src/foo.c
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if result.HasErrors() {
		t.Fatalf("expected no errors for valid build section, got: %s", result.String())
	}
}

func TestValidateBuildSectionInvalidLanguage(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
build:
  language: rust
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for invalid build language")
	}
}

func TestValidateBuildSectionMissingLanguage(t *testing.T) {
	src := `module:
  name: app
  type: rho
  version: 1.0.0
main: src/main.rho
build:
  include_dirs:
    - /opt/include
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	result := ValidateModuleManifest(data, "rho")
	if !result.HasErrors() {
		t.Fatal("expected error for build without language")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: ValidateModuleYAMLFile                                            */
/* -------------------------------------------------------------------------- */

func TestValidateModuleYAMLFileValid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "module.yml")
	content := `module:
  name: hello
  type: rho
  version: 0.1.0
main: src/main.rho
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateModuleYAMLFile(path)
	if err != nil {
		t.Fatalf("ValidateModuleYAMLFile error: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("expected no errors, got: %s", result.String())
	}
}

func TestValidateModuleYAMLFileMissing(t *testing.T) {
	_, err := ValidateModuleYAMLFile("/nonexistent/path/module.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateModuleYAMLFileInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "module.yml")
	// Duplicate keys are rejected by the loader.
	if err := os.WriteFile(path, []byte("module:\n  name: test\n  name: other\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateModuleYAMLFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
