//
// @file plugin_test.go
// @brief Tests for extending the route metadata with plugins.
//
// What matters about a plugin here is what it can and cannot do to a reading.
// A plugin extends the metadata rather than taking it over, a failure is a
// warning rather than an error, and a run with no plugins changes nothing. Those
// three properties are what keep one broken extension from costing a reader the
// API of everything else.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/yaml"
)

// manifestHooks reads the hooks the component's own manifest offers, so the
// check against this package's list is a check against the file a plugin is
// validated by rather than against a copy of it.
func manifestHooks(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "module.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the component manifest is not present: %v", err)
	}
	document, err := yaml.Load(string(content))
	if err != nil {
		t.Fatalf("the component manifest does not parse: %v", err)
	}
	out := []string{}
	for _, entry := range document.Map["provides"].Map["hooks"].List {
		out = append(out, entry.Str)
	}
	return out
}

// The hooks a run goes through are declared in the manifest, and a plugin is
// validated against that manifest rather than against this package. The two
// have to agree or a plugin can claim a stage that is never run.
func TestOfferedHooksAreDeclaredInTheManifest(t *testing.T) {
	declared := manifestHooks(t)
	if len(declared) != len(OfferedHooks) {
		t.Fatalf("the manifest offers %v, this component runs %v", declared, OfferedHooks)
	}
	for _, hook := range OfferedHooks {
		if !containsString(declared, hook) {
			t.Errorf("the hook %q is run here but not declared in the manifest", hook)
		}
	}
}

func TestTheComponentNameIsThisComponentsOwn(t *testing.T) {
	// A component knows its own name; that is the one thing it does not have
	// to be told, and a plugin has to say it in order to be loaded.
	if ComponentName != "rho-router" {
		t.Errorf("ComponentName = %q", ComponentName)
	}
	for _, hook := range OfferedHooks {
		if !offers(hook) {
			t.Errorf("the hook %q is offered but not run", hook)
		}
	}
	if offers("a-hook-nobody-offered") {
		t.Error("a hook nobody offered is run")
	}
}

// A run with no plugins is a run that does nothing at all.
func TestARunWithNoPluginsIsANoOp(t *testing.T) {
	spec := serviceSpec(t)
	answered, diagnostics := RunSpec(nil, spec)
	if len(diagnostics) != 0 {
		t.Errorf("a run with no plugins reported %v", diagnostics)
	}
	if len(answered.Operations) != len(spec.Operations) {
		t.Errorf("a run with no plugins changed the spec: %d operations", len(answered.Operations))
	}
	entry, diagnostics := RunOperation(nil, spec, spec.Operations[0])
	if len(diagnostics) != 0 {
		t.Errorf("a run with no plugins reported %v", diagnostics)
	}
	if entry.ID != spec.Operations[0].ID {
		t.Errorf("a run with no plugins changed the operation: %q", entry.ID)
	}
	document, diagnostics := RunOpenAPI(nil, spec, "text")
	if len(diagnostics) != 0 {
		t.Errorf("a run with no plugins reported %v", diagnostics)
	}
	if document != "text" {
		t.Errorf("a run with no plugins replaced the document with %q", document)
	}
}

// A replacement fills a gap without restating what was read: an answer that
// says nothing about a field leaves the field as it was.
func TestAPluginAnswerFillsRatherThanReplaces(t *testing.T) {
	spec := serviceSpec(t)
	before := spec.Operations[0]
	answer := PluginResponse{Replacements: map[string]Operation{
		before.ID: {ID: before.ID, Description: "A replacement description."},
	}}
	folded, _ := fold(spec, answer, nil)

	found := false
	for _, entry := range folded.Operations {
		if entry.ID != before.ID {
			continue
		}
		found = true
		if entry.Description != "A replacement description." {
			t.Errorf("the description is %q", entry.Description)
		}
		// Everything the answer said nothing about is still what was read.
		if entry.Method != before.Method || entry.Path != before.Path {
			t.Errorf("a replacement changed the method and path: %+v", entry)
		}
		if len(entry.Parameters) != len(before.Parameters) {
			t.Errorf("a replacement dropped the parameters: %+v", entry.Parameters)
		}
	}
	if !found {
		t.Fatal("the operation the answer named is not in the spec")
	}
}

// An operation a plugin adds must not collide with one the sources declared.
func TestAPluginCannotAddAnOperationTwice(t *testing.T) {
	spec := serviceSpec(t)
	before := len(spec.Operations)
	answer := PluginResponse{Operations: []Operation{
		{ID: spec.Operations[0].ID, Method: "GET", Path: "/elsewhere"},
		{ID: "healthz", Method: "GET", Path: "/healthz"},
	}}
	folded, _ := fold(spec, answer, nil)
	if len(folded.Operations) != before+1 {
		t.Errorf("a plugin added %d operations, want 1", len(folded.Operations)-before)
	}
	for _, entry := range folded.Operations {
		if entry.ID == spec.Operations[0].ID && entry.Path == "/elsewhere" {
			t.Error("a plugin replaced an operation the sources declared")
		}
	}
}

// A plugin that reports a problem is warning about the metadata it was given,
// not a failure of the run.
func TestAPluginDiagnosticIsCarriedThrough(t *testing.T) {
	spec := serviceSpec(t)
	answer := PluginResponse{Diagnostics: []PluginDiagnostic{
		{Level: "warning", Message: "operation 3 has no summary"},
	}}
	_, diagnostics := fold(spec, answer, nil)
	if len(diagnostics) != 1 || diagnostics[0].Message != "operation 3 has no summary" {
		t.Errorf("the diagnostic was not carried through: %v", diagnostics)
	}
}

// A finished document a plugin says nothing about is left as it was, rather
// than emptied.
func TestAPluginThatSaysNothingLeavesTheDocumentAlone(t *testing.T) {
	spec := serviceSpec(t)
	document, _ := RunOpenAPI(nil, spec, "the document")
	if document != "the document" {
		t.Errorf("the document is %q", document)
	}
}

// The shared envelope is the toolchain's, so a plugin written against it is a
// plugin every component that takes one understands.
func TestTheProtocolIsTheToolchains(t *testing.T) {
	if ProtocolVersion == 0 {
		t.Error("the protocol version is unset")
	}
}

// serviceSpec reads the shared fixture, so the plugin tests and the reading
// tests are about the same service.
func serviceSpec(t *testing.T) Spec {
	t.Helper()
	root := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("module.yml", manifestText)
	write("src/main.rho", serviceSource)
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	return result.Spec
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
