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
// SPDX-License-Identifier: Apache-2.0
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
  name: sample-app
  type: rho
  version: 0.1.0
main: src/main.rho
depends:
  - name: sensor-lib
    version: 1.0.0
    path: /opt/sensor-lib
    git: https://github.com/DevicePlatform/sensor-lib
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

/* -------------------------------------------------------------------------- */
/*  Tests: Doc-as-code sections                                                 */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestDocsSection(t *testing.T) {
	src := `module:
  name: registry
  type: rho
  version: 0.1.0
main: src/main.rho
docs:
  title: Module registry
  section: applications
  order: "20"
  index: docs/index.md
  sources:
    - docs/index.md
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateModuleManifest(data, "rho"); errs.HasErrors() {
		t.Fatalf("expected the docs section to validate, got %v", errs.ErrorString())
	}
}

func TestValidateModuleManifestRejectsDocsSourcesThatAreNotAList(t *testing.T) {
	src := `module:
  name: registry
  type: rho
  version: 0.1.0
main: src/main.rho
docs:
  sources: docs/index.md
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "rho").HasErrors() {
		t.Fatal("expected doc sources that are not a list to be rejected")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: The plugin relation                                                  */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestPluginRelation(t *testing.T) {
	src := `module:
  name: rho-doc-toc
  type: plugin
  version: 0.1.0
main: rho-doc-toc
extends: rho-doc
hooks:
  - index
  - page
commands:
  - name: table-of-contents
    run: rho-doc-toc
    hook: index
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateModuleManifest(data, "plugin"); errs.HasErrors() {
		t.Fatalf("expected the relation to validate, got %v", errs.ErrorString())
	}
}

func TestValidateModuleManifestWhatAComponentOffers(t *testing.T) {
	src := `module:
  name: rho-doc
  type: component
  version: 0.1.0
  command: doc
main: rho-doc
provides:
  hooks:
    - index
    - page
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateModuleManifest(data, "component"); errs.HasErrors() {
		t.Fatalf("expected the offered hooks to validate, got %v", errs.ErrorString())
	}
}

func TestValidateModuleManifestCommandNeedsARunner(t *testing.T) {
	src := `module:
  name: rho-doc-toc
  type: plugin
  version: 0.1.0
main: rho-doc-toc
extends: rho-doc
hooks:
  - index
commands:
  - name: table-of-contents
    hook: index
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "plugin").HasErrors() {
		t.Fatal("expected a command without a runner to be rejected")
	}
}

func TestValidateModuleManifestRejectsHooksThatAreNotAList(t *testing.T) {
	src := `module:
  name: rho-doc-toc
  type: plugin
  version: 0.1.0
main: rho-doc-toc
extends: rho-doc
hooks: index
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "plugin").HasErrors() {
		t.Fatal("expected hooks that are not a list to be rejected")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: The configuration bridge                                            */
/* -------------------------------------------------------------------------- */

func TestValidateModuleManifestConfigBridge(t *testing.T) {
	src := `module:
  name: sensorhub
  type: rho
  version: 0.1.0
main: src/main.rho
config:
  dialect: espidf
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_UART_BAUD: "57600"
  module: config.rho
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateModuleManifest(data, "rho"); errs.HasErrors() {
		t.Fatalf("expected the bridge to validate, got %v", errs.ErrorString())
	}
}

func TestValidateModuleManifestRejectsADialectItDoesNotWrite(t *testing.T) {
	// A dialect is a header a toolchain reads, so naming one this bridge does not
	// write is refused rather than silently treated as the default.
	src := `module:
  name: sensorhub
  type: rho
  version: 0.1.0
main: src/main.rho
config:
  dialect: zephyr
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "rho").HasErrors() {
		t.Fatal("expected an unknown dialect to be rejected")
	}
}

func TestValidateModuleManifestRejectsValuesThatAreNotAMap(t *testing.T) {
	src := `module:
  name: sensorhub
  type: rho
  version: 0.1.0
main: src/main.rho
config:
  values:
    - CONFIG_UART_BAUD
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "rho").HasErrors() {
		t.Fatal("expected values that are not a map to be rejected")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: A board                                                            */
/* -------------------------------------------------------------------------- */

// A board says what it is and what it brings out: the part, the driver that
// speaks for it, the functions it presents, and the pinout — the connector
// positions and the pins behind them.
func TestValidateModuleManifestValidBoard(t *testing.T) {
	src := `module:
  name: ky-009-rgb-led-module
  type: driver-board
  version: 0.1.0
  description: >
    A KY-009 RGB LED board.
board:
  designation: KY_009_RGB_LED_MODULE
  device: ky-009-rgb-led
  category: output
  driver: robohub-driver-rgb-pwm
  mcu: RGB
  family: RGB
  core: RGB
  functions:
    - color
  connectors:
    - name: VCC
      signal: power
      position: 1
    - name: GND
      signal: ground
      position: 2
  pins:
    red:
      gpio: 21
      signal: pwm
    green:
      gpio: 23
      signal: pwm
    blue:
      gpio: 24
      signal: pwm
  peripherals:
    - name: pwm0
      instance: pwm0
      channel: 0
      gpio: 21
  programmer:
    tool: usb
  flash:
    tool: picotool
    command: "elf2uf2 {binary}"
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	result := ValidateModuleManifest(data, "driver-board")
	if result.HasErrors() {
		t.Fatalf("expected a board to validate, got: %s", result.String())
	}
}

// A connector without a name says nothing about which position it is, and a
// board that cannot say that is not a board anybody can wire.
func TestValidateModuleManifestBoardConnectorNeedsAName(t *testing.T) {
	src := `module:
  name: broken-board
  type: driver-board
  version: 0.1.0
  description: A board with a nameless connector.
board:
  connectors:
    - signal: power
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	result := ValidateModuleManifest(data, "driver-board")
	if !result.HasErrors() {
		t.Fatal("expected a connector without a name to be reported")
	}
}
