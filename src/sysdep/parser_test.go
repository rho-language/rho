//
// @file parser_test.go
// @brief Tests for reading what a module declared.
//
// A manifest is read by this toolchain's own YAML reader and nothing else, so this
// is that reader's path into the block. The cases are the ones that are easy to
// write wrongly: a version constraint that YAML would read as something else, an
// install recipe keyed by manager name, and a required flag that means false.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package sysdep

import (
	"testing"

	"rho/src/yaml"
)

const manifest = `
module:
  name: rho-dbg
system_deps:
  executables:
    - name: arm-none-eabi-gdb
      version: ">=10.0"
      version_flag: "--version"
      version_pattern: '[^\\n]*?\)\s*([0-9][^\s(]*)'
      description: sessions drive a board through the Machine Interface with it
      fetch:
        url: https://example.com/toolchain.tar.gz
        archive: tar.gz
        dir: toolchain-1.0
        binary: bin/arm-none-eabi-gdb
        version: "14.2"
      install:
        apt: gdb-multiarch
        brew: arm-none-eabi-gcc
        manual: https://example.com/downloads
    - name: openocd
      version: ">=0.11.0"
      required: false
      install:
        apt: openocd
  env_vars:
    - name: ARM_TOOLCHAIN_PATH
      required: false
      description: where the cross toolchain is
  udev_rules:
    - name: 60-rho-probes.rules
      target_path: /etc/udev/rules.d/60-rho-probes.rules
      source: probe/60-rho-probes.rules
      description: a probe that is plugged in and not permitted is not there
      optional: true
`

func parsed(t *testing.T) *SystemDeps {
	t.Helper()
	document, err := yaml.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	deps := Parse(yaml.Value{Kind: yaml.KindMap, Map: document.Map})
	if deps == nil {
		t.Fatal("expected the block to be read")
	}
	return deps
}

// The digest a recipe declares is read, and lower-cased: a vendor's page may print
// it either way, and the comparison with a computed one is not a question of case.
func TestARecipesDigestIsReadAndLowerCased(t *testing.T) {
	document, err := yaml.Load(`
system_deps:
  executables:
    - name: thing
      fetch:
        url: https://example.com/thing.tar.gz
        sha256: "ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef0123456789"
`)
	if err != nil {
		t.Fatal(err)
	}
	deps := Parse(yaml.Value{Kind: yaml.KindMap, Map: document.Map})
	if deps == nil || len(deps.Executables) != 1 || deps.Executables[0].Fetch == nil {
		t.Fatal("the recipe was not read")
	}
	want := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if got := deps.Executables[0].Fetch.SHA256; got != want {
		t.Errorf("the digest was read as %q, want %q", got, want)
	}
}

func TestAModulesRequirementsAreRead(t *testing.T) {
	deps := parsed(t)
	if len(deps.Executables) != 2 {
		t.Fatalf("expected two programs, got %d", len(deps.Executables))
	}

	gdb := deps.Executables[0]
	if gdb.Name != "arm-none-eabi-gdb" || gdb.Version != ">=10.0" {
		t.Errorf("expected the program and its constraint, got %+v", gdb)
	}
	// The constraint is quoted in the manifest because `>=10.0` would otherwise be
	// read as something else entirely — which is why it comes back without the quotes
	// rather than with them, and why a requirement read with its quotes left on
	// would satisfy nothing.
	if gdb.VersionFlag != "--version" {
		t.Errorf("expected the flag, got %q", gdb.VersionFlag)
	}
	if gdb.VersionPattern == "" {
		t.Error("expected the version pattern to be read")
	}
	if !gdb.Required {
		t.Error("expected a program with no `required` to be required")
	}
	if gdb.Fetch == nil || gdb.Fetch.Archive != "tar.gz" || gdb.Fetch.Binary != "bin/arm-none-eabi-gdb" {
		t.Errorf("expected the prebuilt archive to be read, got %+v", gdb.Fetch)
	}
	if gdb.Install["apt"] != "gdb-multiarch" || gdb.Install["brew"] != "arm-none-eabi-gcc" {
		t.Errorf("expected the install recipes, got %v", gdb.Install)
	}
	if gdb.Install["manual"] != "https://example.com/downloads" {
		t.Errorf("expected the manual recipe to be kept, got %q", gdb.Install["manual"])
	}

	// `required: false` is honoured: a system dependency a module does not need is not
	// one, and a block of optional programs nobody checks for reports nothing.
	openocd := deps.Executables[1]
	if openocd.Required {
		t.Error("expected `required: false` to be believed")
	}
}

func TestTheEnvironmentAndTheRulesAreRead(t *testing.T) {
	deps := parsed(t)
	if len(deps.EnvVars) != 1 || deps.EnvVars[0].Name != "ARM_TOOLCHAIN_PATH" {
		t.Fatalf("expected the variable, got %+v", deps.EnvVars)
	}
	if deps.EnvVars[0].Required {
		t.Error("expected an optional variable not to be required")
	}
	if deps.EnvVars[0].Description == "" {
		t.Error("expected the description to be read, so a report can say what loses its purpose")
	}

	if len(deps.UdevRules) != 1 {
		t.Fatalf("expected the rule, got %+v", deps.UdevRules)
	}
	rule := deps.UdevRules[0]
	if rule.TargetPath != "/etc/udev/rules.d/60-rho-probes.rules" {
		t.Errorf("expected the target path, got %q", rule.TargetPath)
	}
	// The source is what makes installing the rule a copy of something a person can
	// read, rather than a written-out guess at the vendor's rules.
	if rule.Source != "probe/60-rho-probes.rules" {
		t.Errorf("expected the rule's own file to be named, got %q", rule.Source)
	}
	if !rule.Optional {
		t.Error("expected an optional rule not to be required")
	}
}

// Most manifests say nothing at all about the machine, and that is the normal case
// rather than an empty declaration: a block that is not there must read as nothing,
// not as an empty requirement somebody will try to satisfy.
func TestAModuleThatSaysNothingNeedsNothing(t *testing.T) {
	for name, document := range map[string]string{
		"no block at all":    "module:\n  name: x\n",
		"an empty block":     "module:\n  name: x\nsystem_deps:\n",
		"empty lists":        "module:\n  name: x\nsystem_deps:\n  executables: []\n  env_vars: []\n  udev_rules: []\n",
		"only a description": "module:\n  name: x\nsystem_deps:\n  description: nothing\n",
	} {
		parsed, err := yaml.Load(document)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		deps := Parse(yaml.Value{Kind: yaml.KindMap, Map: parsed.Map})
		if deps.Declared() {
			t.Errorf("%s: expected nothing declared, got %+v", name, deps)
		}
	}
}

// An entry with no name is not a requirement: it is a line somebody left behind,
// and a report that tried to look for a program named nothing would either crash or
// invent one.
func TestAnEntryWithNoNameIsSkipped(t *testing.T) {
	document, err := yaml.Load(`
system_deps:
  executables:
    - version: ">=1.0"
    - name: real
  env_vars:
    - description: no name here
  udev_rules:
    - target_path: /etc/udev/rules.d/x.rules
`)
	if err != nil {
		t.Fatal(err)
	}
	deps := Parse(yaml.Value{Kind: yaml.KindMap, Map: document.Map})
	if deps == nil {
		t.Fatal("expected the block with one usable entry to be read")
	}
	if len(deps.Executables) != 1 || deps.Executables[0].Name != "real" {
		t.Errorf("expected only the named program, got %+v", deps.Executables)
	}
	if len(deps.EnvVars) != 0 || len(deps.UdevRules) != 0 {
		t.Errorf("expected the unnamed entries to be skipped, got %+v %+v", deps.EnvVars, deps.UdevRules)
	}
}
