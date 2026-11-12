package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/deps"
	"rho/src/plugins"
)

// An integration for a made-up ecosystem: what the contract asks of any.
const acmeManifest = `module:
  name: rho-acme
  type: plugin
  version: 0.1.0
  description: bridges acme

main: rho-acme

serves:
  - acme

integration:
  ecosystem: acme
  workspace_env: ACME_WORKSPACE
  provides:
    bindings:
      protocol-bus-c: bindings/bus.yml
    ports: ports/

extends:
  - component: rho-target
    implements: [platform]
    hooks: [spec]

commands:
  - name: spec
    run: rho-acme
    hook: spec
    component: rho-target

package:
  include:
    - module.yml
    - bindings
    - ports
`

func writeAcme(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"module.yml":          manifest,
		"bindings/bus.yml":    "publish: acme_publish\n",
		"ports/go.mod":        "module acme/ports\n",
		"ports/relay.go":      "package main\n\nfunc relayOn() {}\n",
		"ports/relay_test.go": "package main\n",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAConformingIntegrationResolvesByKind(t *testing.T) {
	root := writeAcme(t, acmeManifest)
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(root, manifest); len(problems) != 0 {
		t.Fatalf("a conforming integration was refused: %v", problems)
	}
	plugin := plugins.Plugin{Name: "rho-acme", Root: root, Platforms: []string{"acme"}}
	resolved := resolve(plugin, manifest.Integration)
	if resolved.Bindings[BindingProtocolBusC] != filepath.Join(root, "bindings", "bus.yml") {
		t.Errorf("binding = %q", resolved.Bindings[BindingProtocolBusC])
	}
	if resolved.Ecosystem != "acme" || resolved.WorkspaceEnv != "ACME_WORKSPACE" {
		t.Errorf("resolved = %+v", resolved)
	}
	// With nothing installed, a platform has no integration; no folder is guessed.
	if _, err := Binding(plugins.Source{Platform: "acme"}, BindingProtocolBusC); err == nil {
		t.Error("a binding resolved with no integration installed")
	}
	if got := ForPlatform(plugins.Source{}); got != nil {
		t.Error("a project with no platform has integrations")
	}
}

func TestTheContractRefusesWhatItCannotKeep(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"not serving its ecosystem": {"serves:\n  - acme", "serves:\n  - other", "under `serves:`"},
		"unknown binding kind":      {"protocol-bus-c: bindings/bus.yml", "protocol-bus-c: bindings/bus.yml\n      telepathy: bindings/bus.yml", "not one the toolchain reads"},
		"missing binding file":      {"protocol-bus-c: bindings/bus.yml", "protocol-bus-c: bindings/none.yml", "not a file"},
		"workspace variable":        {"workspace_env: ACME_WORKSPACE", "workspace_env: ACME_HOME", "_WORKSPACE"},
		"no ecosystem":              {"  ecosystem: acme\n", "", "integration.ecosystem"},
		"bindings not packaged":     {"    - bindings\n", "", "not in the package"},
	}
	for name, c := range cases {
		manifest := strings.Replace(acmeManifest, c.from, c.to, 1)
		root := writeAcme(t, manifest)
		loaded, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		problems := strings.Join(Validate(root, loaded), "\n")
		if !strings.Contains(problems, c.want) {
			t.Errorf("%s: problems %q do not mention %q", name, problems, c.want)
		}
	}
}

func TestPortsAreTheNonTestGoFiles(t *testing.T) {
	root := writeAcme(t, acmeManifest)
	manifest, _ := deps.LoadManifest(filepath.Join(root, "module.yml"))
	integration := resolve(plugins.Plugin{Root: root, Platforms: []string{"acme"}}, manifest.Integration)
	entries, _ := os.ReadDir(integration.PortsDir)
	var names []string
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry.Name(), PortSuffix); ok && !strings.HasSuffix(name, "_test") {
			names = append(names, name)
		}
	}
	if strings.Join(names, ",") != "relay" {
		t.Errorf("ports = %v", names)
	}
}
