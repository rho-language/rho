//
// @file nested_test.go
// @brief Modules nested inside a repository are members of the family of their own.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"path/filepath"
	"strings"
	"testing"
)

// nestedWorkspace is a root with a module nested in it (providers/mesh, named
// robohub-provider-mesh), a nested directory that is not a member
// (modules/plain, no requirements block), a test project under the nested
// module (tests/module.yml, no block), and a sibling node that refers to the
// nested module's decision.
func nestedWorkspace(t *testing.T, extra map[string]string) string {
	t.Helper()
	isolate(t)
	dir := t.TempDir()
	files := map[string]string{
		"robohub/module.yml":                   "module:\n  name: robohub\n  type: rho\n  version: 0.1.0\n\nrequirements:\n  prefix: HUB\n",
		"robohub/docs/requirements/levels.yml": "levels:\n  - { name: SYS, title: System }\n  - { name: NODE, title: Node }\n  - { name: SUB, title: Subsystem }\n",
		"robohub/docs/requirements/sys.yml": "level: SYS\nprefix: SYS\nrequirements:\n  - id: SYS-0001\n    title: T\n    statement: S\n    status: proposed\n" +
			"    verification: inspection\n    changed: 2026-11-19\n",
		"robohub/src/main.rho":             "// @req SYS-0001\nfn main() void {}\n",
		"robohub/modules/plain/module.yml": "module:\n  name: plain\n  type: rho\n  version: 0.1.0\n",
		"robohub/modules/plain/src/a.rho":  "// @req SYS-0001\nfn a() void {}\n",
		"robohub/providers/mesh/module.yml": "module:\n  name: robohub-provider-mesh\n  type: rho\n  version: 0.1.0\n\n" +
			"requirements: { prefix: PMESH, parent: robohub }\n",
		"robohub/providers/mesh/docs/requirements/node.yml": "level: NODE\nprefix: PMESH\nrequirements:\n  - id: PMESH-0001\n    title: T\n    statement: S\n" +
			"    satisfies: [SYS-0001]\n    status: proposed\n    verification: test\n    decided_by: [ADR-0001]\n    changed: 2026-11-19\n",
		"robohub/providers/mesh/docs/architecture/adr/0001-one-socket.md": "---\nid: ADR-0001\ntitle: One socket\nstatus: proposed\ndate: 2026-11-19\n" +
			"requirements: [PMESH-0001]\n---\n\nbody\n",
		"robohub/providers/mesh/src/mesh.rho":              "// @req PMESH-0001\nfn send() void {}\n",
		"robohub/providers/mesh/tests/module.yml":          "module:\n  name: mesh-tests\n  type: rho\n  version: 0.1.0\n",
		"robohub/providers/mesh/tests/mesh_test.rho":       "// @verifies PMESH-0001\ntest fn sends() void {}\n",
		"robohub/providers/mesh/build/x/module.yml":        "module:\n  name: generated\n\nrequirements: { prefix: GEN, parent: robohub }\n",
		"robohub/providers/mesh/node_modules/y/module.yml": "module:\n  name: vendored\n\nrequirements: { prefix: VEN, parent: robohub }\n",
		"gateway/module.yml":                               "module:\n  name: robohub-gateway\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: GW, parent: robohub }\n",
		"gateway/docs/requirements/node.yml": "level: NODE\nprefix: GW\nrequirements:\n  - id: GW-0001\n    title: T\n    statement: S\n" +
			"    satisfies: [SYS-0001]\n    status: proposed\n    verification: test\n    decided_by: [robohub-provider-mesh:ADR-0001]\n    changed: 2026-11-19\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	writeFiles(t, dir, files)
	return dir
}

func repoNames(w *Workspace) string {
	names := []string{}
	for _, repo := range w.Repos {
		names = append(names, repo.Name)
	}
	return strings.Join(names, ",")
}

func tagFiles(repo *Repo) string {
	files := []string{}
	for _, tag := range repo.Tags() {
		files = append(files, tag.File)
	}
	return strings.Join(files, ",")
}

func TestANestedModuleIsAMemberOfTheFamily(t *testing.T) {
	dir := nestedWorkspace(t, nil)
	for _, from := range []string{"gateway", "robohub", filepath.Join("robohub", "providers", "mesh", "src")} {
		w := load(t, filepath.Join(dir, from), true)
		if got := repoNames(w); got != "robohub,gateway,robohub-provider-mesh" {
			t.Fatalf("from %s: family = %s (build/ and node_modules/ are not looked in; a module.yml with no block is no member)", from, got)
		}
		if issues := Check(w, CheckOptions{}); len(issues) != 0 {
			t.Fatalf("from %s: %v", from, issues)
		}
	}

	w := load(t, filepath.Join(dir, "robohub", "providers", "mesh", "src"), false)
	if w.Focus.Name != "robohub-provider-mesh" || !w.Focus.Nested || w.Root.Name != "robohub" {
		t.Fatalf("focus %s (nested %v), root %s", w.Focus.Name, w.Focus.Nested, w.Root.Name)
	}
	if !strings.HasSuffix(w.Focus.LevelsFrom, filepath.Join("robohub", LevelsFile)) {
		t.Errorf("the nested module's levels come from %s", w.Focus.LevelsFrom)
	}
	if req := w.Requirement("PMESH-0001"); req == nil || req.Repo != "robohub-provider-mesh" {
		t.Fatal("the nested module's requirements are its own")
	}
	if adr := w.ADR("robohub-gateway", "robohub-provider-mesh:ADR-0001"); adr == nil || adr.Repo != "robohub-provider-mesh" {
		t.Fatal("a nested module's decision is named by its module name")
	}
	if w.Repo("gateway").Nested {
		t.Fatal("a repository of the top level is not nested")
	}

	// Each repository's tags are its own: the root's scan stops at the nested
	// module, and takes in a nested directory that is no member.
	if got := tagFiles(w.Repo("robohub")); got != "modules/plain/src/a.rho,src/main.rho" {
		t.Fatalf("the root's tags: %s", got)
	}
	if got := tagFiles(w.Repo("robohub-provider-mesh")); got != "src/mesh.rho,tests/mesh_test.rho" {
		t.Fatalf("the nested module's tags: %s", got)
	}
	row := Trace(load(t, filepath.Join(dir, "robohub"), true)).Row("PMESH-0001")
	if row == nil || len(row.Code) != 1 || len(row.Tests) != 1 {
		t.Fatalf("the trace counts the nested module's tags once: %+v", row)
	}
}

func TestANestedModuleFindsItsParentWithoutAWorkspace(t *testing.T) {
	// The parent encloses the module, so no variable and no sibling is needed;
	// the family is still found beside the outermost repository.
	dir := nestedWorkspace(t, nil)
	w := load(t, filepath.Join(dir, "robohub", "providers", "mesh"), true)
	if w.Root.Dir != filepath.Join(dir, "robohub") || repoNames(w) != "robohub,gateway,robohub-provider-mesh" {
		t.Fatalf("root %s, family %s", w.Root.Dir, repoNames(w))
	}
}

func TestNamesStayUnique(t *testing.T) {
	clash := map[string]string{
		"robohub/contracts/gateway/module.yml": "module:\n  name: gateway\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: GWC, parent: robohub }\n",
	}
	dir := nestedWorkspace(t, clash)
	w := load(t, filepath.Join(dir, "robohub"), true)
	if !hasCode(Check(w, CheckOptions{}), SeverityError, "repo.duplicate") {
		t.Fatalf("a nested module named like a repository is an error: %v", Check(w, CheckOptions{}))
	}

	clash["robohub/contracts/gateway/module.yml"] = "module:\n  name: gateway\n  type: rho\n  version: 0.1.0\n\n" +
		"requirements: { name: gateway-contract, prefix: GWC, parent: robohub }\n"
	dir = nestedWorkspace(t, clash)
	w = load(t, filepath.Join(dir, "robohub"), true)
	if issues := Check(w, CheckOptions{}); len(issues) != 0 || w.Repo("gateway-contract") == nil {
		t.Fatalf("the block's name names it apart: %s; %v", repoNames(w), issues)
	}

	dir = nestedWorkspace(t, map[string]string{
		"gateway/module.yml": "module:\n  name: robohub-gateway\n\nrequirements: { name: gw, prefix: GW, parent: robohub }\n",
	})
	w = load(t, filepath.Join(dir, "gateway"), false)
	if w.Focus.Name != "gateway" || !hasCode(Check(w, CheckOptions{}), SeverityWarning, "manifest.name") {
		t.Fatalf("a repository of the top level is named by its directory, not by name:, and is warned: %s", w.Focus.Name)
	}
	// A nested module copied to the top level under its name is no warning.
	dir = nestedWorkspace(t, map[string]string{
		"gw-contract/module.yml": "module:\n  name: robohub-gateway\n\nrequirements: { name: gw-contract, prefix: GWC, parent: robohub }\n",
	})
	if issues := Check(load(t, filepath.Join(dir, "gw-contract"), false), CheckOptions{}); len(issues) != 0 {
		t.Fatalf("got %v", issues)
	}
}

func TestATreeInBuildIsAWorkspaceOfItsOwn(t *testing.T) {
	// A documentation tree assembled in the root's build/ copies the root and
	// the nested module side by side: there they are repositories of the top
	// level, named by their directories, and their parent is the copy beside
	// them, not the repository the tree is built in.
	tree := "robohub/build/doctree/"
	dir := nestedWorkspace(t, map[string]string{
		tree + "robohub/module.yml":                   "module:\n  name: robohub\n\nrequirements:\n  prefix: HUB\n",
		tree + "robohub/docs/requirements/levels.yml": "levels:\n  - { name: SYS, title: System }\n  - { name: NODE, title: Node }\n",
		tree + "robohub/docs/requirements/sys.yml": "level: SYS\nprefix: SYS\nrequirements:\n  - id: SYS-0001\n    title: T\n    statement: S\n    status: proposed\n" +
			"    verification: inspection\n    changed: 2026-11-19\n",
		tree + "mesh/module.yml": "module:\n  name: robohub-provider-mesh\n\nrequirements: { prefix: PMESH, parent: robohub }\n",
		tree + "mesh/docs/requirements/node.yml": "level: NODE\nprefix: PMESH\nrequirements:\n  - id: PMESH-0001\n    title: T\n    statement: S\n" +
			"    satisfies: [SYS-0001]\n    status: proposed\n    verification: inspection\n    changed: 2026-11-19\n",
	})
	w := load(t, filepath.Join(dir, "robohub", "build", "doctree", "mesh"), true)
	if w.Focus.Name != "mesh" || w.Focus.Nested || w.Root.Dir != filepath.Join(dir, "robohub", "build", "doctree", "robohub") {
		t.Fatalf("focus %s (nested %v), root %s", w.Focus.Name, w.Focus.Nested, w.Root.Dir)
	}
	if got := repoNames(w); got != "robohub,mesh" {
		t.Fatalf("the tree's family = %s", got)
	}
	if issues := Check(w, CheckOptions{}); issues.Errors() != 0 {
		t.Fatalf("%v", issues)
	}
}
