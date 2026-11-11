//
// @file contract_test.go
// @brief The toolchain loads an ecosystem's module types through its plugin.
//
// The toolchain validates a manifest without knowing an ecosystem's words. The
// ecosystem's platform plugin answers the `rho-module` contract with the types
// it allows, and these tests are the join: an installed plugin, a project whose
// platform is recognized, and a manifest of a type only the plugin names.
//
// @project Rho
// @license Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/deps"
)

const pluginContract = `{"version":1,"types":{"device":{"native":true,"required":["device","build"]}}}`

// writeInstalledPlugin writes a plugin under a temporary Rho home, with a
// command that answers the contract hook with a fixed document.
func writeInstalledPlugin(t *testing.T, rhoHome string) {
	t.Helper()
	root := filepath.Join(rhoHome, "modules", "installed", "plugin", "rho-robohub@0.1.0")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `module:
  name: rho-robohub
  type: plugin
  version: 0.1.0
main: contract.sh
serves:
  - robohub
extends:
  - component: rho-module
    implements:
      - module-manifest
    hooks:
      - contract
commands:
  - name: contract
    run: contract.sh
    hook: contract
    component: rho-module
`
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '" + `{"version":1,"payload":{"contract":` + pluginContract + `}}` + "'\n"
	if err := os.WriteFile(filepath.Join(root, "contract.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeProject writes a device manifest and the platform module that identifies
// the ecosystem it belongs to.
func writeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	platform := filepath.Join(dir, "platform")
	if err := os.MkdirAll(platform, 0o755); err != nil {
		t.Fatal(err)
	}
	platformManifest := `module:
  name: robohub-test-platform
  type: platform
  version: 0.1.0
platform: robohub
`
	if err := os.WriteFile(filepath.Join(platform, "module.yml"), []byte(platformManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `module:
  name: a-device
  type: device
  version: 0.1.0
build:
  language: c
device:
  kind: sensor
  transports: [gpio]
depends:
  - name: robohub-test-platform
    type: c
    path: ` + platform + `
`
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A type the toolchain does not define is validated against the contract its
// ecosystem's plugin publishes.
func TestAnEcosystemTypeIsValidatedThroughItsPlugin(t *testing.T) {
	rhoHome := t.TempDir()
	t.Setenv("RHO_HOME", rhoHome)
	writeInstalledPlugin(t, rhoHome)
	if err := deps.SetVersionEnabled("plugin", "rho-robohub", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	dir := writeProject(t)
	if _, err := LoadModuleYAMLStrict(filepath.Join(dir, "module.yml")); err != nil {
		t.Fatalf("expected the device to validate through its contract, got: %v", err)
	}
}

// Without the plugin there is no contract, and a type the toolchain does not
// define is refused rather than guessed at.
func TestAnEcosystemTypeIsRefusedWithoutItsPlugin(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	dir := writeProject(t)

	if _, err := LoadModuleYAMLStrict(filepath.Join(dir, "module.yml")); err == nil {
		t.Fatal("expected a device to be refused when no contract is installed")
	}
}
