//
// @file check_test.go
// @brief The workspace, how it resolves, and every problem check reports.
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

func TestTheFamilyResolves(t *testing.T) {
	dir := fixture(t)
	w := load(t, filepath.Join(dir, "gateway", "src"), false)
	if w.Focus.Name != "gateway" || w.Root.Name != "robohub" {
		t.Fatalf("focus %s, root %s", w.Focus.Name, w.Root.Name)
	}
	names := []string{}
	for _, repo := range w.Repos {
		names = append(names, repo.Name)
	}
	if got := strings.Join(names, ","); got != "robohub,gateway" {
		t.Fatalf("family = %s (the standalone repository is not in it)", got)
	}
	if !strings.HasSuffix(w.Focus.LevelsFrom, filepath.Join("robohub", LevelsFile)) {
		t.Errorf("the gateway's levels come from %s", w.Focus.LevelsFrom)
	}
	if !strings.HasSuffix(w.Focus.Maintainers, filepath.Join("robohub", MaintainersFile)) {
		t.Errorf("the gateway is governed by %q", w.Focus.Maintainers)
	}
	if w.Requirement("GW-AUTH-0001") == nil || w.Requirement("SYS-0002") == nil {
		t.Fatal("requirements in sub-folders and in the parent are read")
	}
	if got := w.Requirement("SYS-0001").Title; got != "Survive a power cut" {
		t.Fatalf("docs/doc was read as requirements: SYS-0001 is %q", got)
	}
	if adr := w.ADR("gateway", "robohub:ADR-0001"); adr == nil || adr.Repo != "robohub" {
		t.Fatal("a qualified ADR reference resolves to the other repository")
	}
	if adr := w.ADR("gateway", "ADR-0001"); adr == nil || adr.Repo != "gateway" {
		t.Fatal("a bare ADR reference is the repository's own")
	}

	standalone := load(t, filepath.Join(dir, "standalone"), false)
	if len(standalone.Repos) != 1 || standalone.Focus.LevelsFrom != "default" || standalone.Focus.Maintainers != "" {
		t.Fatalf("a standalone repository is a family of one with the default levels: %+v", standalone.Focus)
	}
	if issues := Check(standalone, CheckOptions{}); len(issues) != 0 {
		t.Fatalf("the standalone fixture is clean, got %v", issues)
	}
}

func TestWorkspaceEnvironmentFindsTheParent(t *testing.T) {
	dir := fixture(t)
	// A node checked out somewhere else finds its parent through the variable.
	elsewhere := t.TempDir()
	copyTree(t, filepath.Join(dir, "gateway"), filepath.Join(elsewhere, "gateway"))
	w := load(t, filepath.Join(elsewhere, "gateway"), false)
	if w.Root.Name == "robohub" {
		t.Fatal("found a parent with no workspace set")
	}
	if !hasCode(Check(w, CheckOptions{}), SeverityError, "parent.unresolved") {
		t.Fatal("an unresolved parent is an error")
	}
	t.Setenv("ROBOHUB_WORKSPACE", dir)
	w = load(t, filepath.Join(elsewhere, "gateway"), false)
	if w.Root.Name != "robohub" || w.Root.Dir != filepath.Join(dir, "robohub") {
		t.Fatalf("root %s at %s", w.Root.Name, w.Root.Dir)
	}
}

func TestTheFixtureChecks(t *testing.T) {
	dir := fixture(t)
	w := load(t, filepath.Join(dir, "gateway"), false)
	issues := Check(w, CheckOptions{})
	want := "warning:adr.orphan warning:evidence.missing error:tag.unresolved"
	if got := codes(issues); got != want {
		t.Fatalf("got %s\nwant %s\n%v", got, want, issues)
	}
	// The root's own requirements are clean.
	root := load(t, filepath.Join(dir, "robohub"), false)
	if issues := Check(root, CheckOptions{}); issues.Errors() != 0 {
		t.Fatalf("robohub: %v", issues)
	}
}

// checkCase writes a two-repository workspace — a root with levels and one SYS
// requirement, and a node — with the case's files on top, and checks the node.
func checkCase(t *testing.T, files map[string]string) Issues {
	t.Helper()
	isolate(t)
	dir := t.TempDir()
	base := map[string]string{
		"root/module.yml":                   "module:\n  name: root\n  type: rho\n  version: 0.1.0\n\nrequirements:\n  prefix: RT\n",
		"root/docs/requirements/levels.yml": "levels:\n  - { name: SYS, title: System }\n  - { name: NODE, title: Node }\n  - { name: SUB, title: Subsystem }\n",
		"root/docs/requirements/sys.yml": "level: SYS\nprefix: SYS\nrequirements:\n  - id: SYS-0001\n    title: T\n    statement: S\n    status: approved\n" +
			"    verification: inspection\n    changed: 2026-11-19\n",
		"node/module.yml": "module:\n  name: node\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: ND, parent: root }\n",
	}
	for k, v := range files {
		base[k] = v
	}
	writeFiles(t, dir, base)
	return Check(load(t, filepath.Join(dir, "node"), false), CheckOptions{})
}

func nodeFile(items string) map[string]string {
	return map[string]string{"node/docs/requirements/node.yml": "level: NODE\nprefix: ND\nrequirements:\n" + items}
}

const okItem = "  - id: ND-0001\n    title: T\n    statement: S\n    satisfies: [SYS-0001]\n    status: draft\n    verification: test\n    changed: 2026-11-19\n"

func adrFile(name, header string) map[string]string {
	files := nodeFile(okItem)
	files["node/docs/architecture/adr/"+name] = "---\n" + header + "---\n\nbody\n"
	return files
}

func TestEveryCheckError(t *testing.T) {
	item := func(replace, with string) map[string]string {
		return nodeFile(strings.Replace(okItem, replace, with, 1))
	}
	cases := []struct {
		name  string
		files map[string]string
		code  string
		line  int
	}{
		{"unknown key", item("    status:", "    colour: red\n    status:"), "schema.unknown-key", 8},
		{"missing key", item("    statement: S\n", ""), "schema.missing-key", 4},
		{"status not allowed", item("status: draft", "status: done"), "schema.enum", 8},
		{"verification not allowed", item("verification: test", "verification: hope"), "schema.enum", 9},
		{"bad date", item("2026-11-19", "2026-13-01"), "schema.date", 10},
		{"date format", item("2026-11-19", "19.11.2026"), "schema.date", 10},
		{"list where a list belongs", item("[SYS-0001]", "SYS-0001"), "schema.type", 7},
		{"id format", item("ND-0001", "ND-12"), "id.format", 4},
		{"id prefix", item("ND-0001", "XX-0001"), "id.prefix", 4},
		{"file prefix", map[string]string{"node/docs/requirements/node.yml": "level: NODE\nprefix: ZZ\nrequirements: []\n"}, "file.prefix", 2},
		{"sys prefix", map[string]string{"node/docs/requirements/node.yml": "level: SYS\nprefix: ND\nrequirements: []\n"}, "file.prefix", 2},
		{"unknown level", map[string]string{"node/docs/requirements/node.yml": "level: UNIT\nprefix: ND\nrequirements: []\n"}, "file.level", 1},
		{"file key missing", map[string]string{"node/docs/requirements/node.yml": "level: NODE\nrequirements: []\n"}, "schema.missing-key", 1},
		{"duplicate in a file", nodeFile(okItem + okItem), "id.duplicate", 11},
		{"duplicate across repositories", map[string]string{"node/docs/requirements/sys.yml": "level: SYS\nprefix: SYS\nrequirements:\n" +
			strings.Replace(strings.Replace(okItem, "ND-0001", "SYS-0001", 1), "    satisfies: [SYS-0001]\n", "", 1)}, "id.duplicate", 4},
		{"satisfies nothing that exists", item("[SYS-0001]", "[SYS-0042]"), "satisfies.unresolved", 4},
		{"satisfies the same level", nodeFile(okItem + strings.Replace(strings.Replace(okItem, "ND-0001", "ND-0002", 1), "SYS-0001", "ND-0001", 1)), "satisfies.level", 11},
		{"satisfies itself", item("[SYS-0001]", "[ND-0001]"), "satisfies.self", 4},
		{"decided by nothing", item("    status:", "    decided_by: [ADR-0007]\n    status:"), "decided-by.unresolved", 4},
		{"decided by a malformed ref", item("    status:", "    decided_by: [adr7]\n    status:"), "decided-by.format", 4},
		{"yaml", map[string]string{"node/docs/requirements/node.yml": "level: NODE\nprefix: ND\nrequirements:\n  - id: ND-0001\n   title: x\n"}, "yaml", 5},
		{"tab", map[string]string{"node/docs/requirements/node.yml": "level: NODE\nprefix: ND\nrequirements:\n\t- id: ND-0001\n"}, "yaml", 4},
		{"adr unknown key", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0001]\nauthor: me\n"), "schema.unknown-key", 7},
		{"adr missing date", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\nrequirements: [ND-0001]\n"), "schema.missing-key", 1},
		{"adr status", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: maybe\ndate: 2026-11-19\nrequirements: [ND-0001]\n"), "schema.enum", 4},
		{"adr numbered differently", adrFile("0002-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0001]\n"), "adr.id", 2},
		{"adr requirement missing", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0404]\n"), "adr.requirements-unresolved", 0},
		{"adr supersedes nothing", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0001]\nsupersedes: [ADR-0009]\n"), "adr.supersedes-unresolved", 0},
		{"adr superseded by nobody", adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: superseded\ndate: 2026-11-19\nrequirements: [ND-0001]\n"), "adr.superseded", 0},
		{"no front matter", func() map[string]string {
			files := nodeFile(okItem)
			files["node/docs/architecture/adr/0001-a.md"] = "# no header\n"
			return files
		}(), "adr.front-matter", 1},
		{"adr cycle", func() map[string]string {
			files := adrFile("0001-a.md", "id: ADR-0001\ntitle: A\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0001]\nsupersedes: [ADR-0002]\n")
			files["node/docs/architecture/adr/0002-b.md"] = "---\nid: ADR-0002\ntitle: B\nstatus: accepted\ndate: 2026-11-19\nrequirements: [ND-0001]\nsupersedes: [ADR-0001]\n---\n"
			return files
		}(), "cycle", 0},
		{"tag pointing at nothing", func() map[string]string {
			files := nodeFile(okItem)
			files["node/src/a.c"] = "/* @req ND-0404 */\nint a(void) { return 0; }\n"
			return files
		}(), "tag.unresolved", 1},
		{"unknown manifest key", func() map[string]string {
			files := nodeFile(okItem)
			files["node/module.yml"] = "module:\n  name: node\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: ND, parent: root, level: NODE }\n"
			return files
		}(), "schema.unknown-key", 6},
		{"parent missing", func() map[string]string {
			files := nodeFile(okItem)
			files["node/module.yml"] = "module:\n  name: node\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: ND, parent: nowhere }\n"
			return files
		}(), "parent.unresolved", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issues := checkCase(t, c.files)
			for _, issue := range issues {
				if issue.Code == c.code && issue.Severity == SeverityError {
					if c.line != 0 && issue.Line != c.line {
						t.Fatalf("%s reported on line %d, want %d: %s", c.code, issue.Line, c.line, issue)
					}
					return
				}
			}
			t.Fatalf("no %s error; got %s\n%v", c.code, codes(issues), issues)
		})
	}
}

func TestTheBaseCaseIsClean(t *testing.T) {
	if issues := checkCase(t, nodeFile(okItem)); len(issues) != 0 {
		t.Fatalf("got %v", issues)
	}
}

func TestLevelsAreChecked(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"root/module.yml":                   "module:\n  name: root\n  type: rho\n  version: 0.1.0\n",
		"root/docs/requirements/levels.yml": "levels:\n  - { name: SYS, title: System }\n  - { name: SYS, title: Again }\n  - { name: Box, title: Lower }\n  - { name: NODE }\n",
	})
	issues := Check(load(t, filepath.Join(dir, "root"), false), CheckOptions{})
	for _, code := range []string{"levels.duplicate", "levels.name", "schema.missing-key"} {
		if !hasCode(issues, SeverityError, code) {
			t.Errorf("no %s in %v", code, issues)
		}
	}
}

func TestWarningsAndTransitions(t *testing.T) {
	approved := strings.Replace(strings.Replace(okItem, "status: draft", "status: approved", 1), "    changed: 2026-11-19\n", "", 1)
	issues := checkCase(t, nodeFile(approved))
	if issues.Errors() != 0 || !hasCode(issues, SeverityWarning, "evidence.missing") || !hasCode(issues, SeverityWarning, "changed.missing") {
		t.Fatalf("got %v", issues)
	}
	for _, c := range []struct {
		from, to string
		ok       bool
	}{
		{"draft", "proposed", true}, {"draft", "approved", false}, {"proposed", "approved", true},
		{"approved", "implemented", true}, {"implemented", "verified", true}, {"verified", "proposed", true},
		{"approved", "verified", false}, {"verified", "retired", true}, {"retired", "draft", false}, {"rejected", "draft", true},
	} {
		if CanTransition(c.from, c.to) != c.ok {
			t.Errorf("%s -> %s allowed = %v", c.from, c.to, !c.ok)
		}
	}
}
