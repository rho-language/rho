//
// @file ops_test.go
// @brief new, edit, set-status, the ADR commands and init, on a copied workspace.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAllocatesAndAppends(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	nodeFile := filepath.Join(gateway, "docs/requirements/node.yml")
	before := read(t, nodeFile)

	id, file, err := New(load(t, gateway, false), NewRequest{Level: "NODE", Title: "Report the version", Statement: "The gateway shall report: its version.", Satisfies: []string{"SYS-0001"}})
	if err != nil {
		t.Fatal(err)
	}
	if id != "GW-0004" || file != "docs/requirements/node.yml" {
		t.Fatalf("got %s in %s", id, file)
	}
	want := before + "\n  - id: GW-0004\n    title: Report the version\n    statement: \"The gateway shall report: its version.\"\n" +
		"    satisfies: [SYS-0001]\n    status: draft\n    verification: test\n    changed: 2026-11-19\n"
	if got := read(t, nodeFile); got != want {
		t.Fatalf("got:\n%s", got)
	}

	// A sub-prefix file is found by level and prefix; a new one is created.
	if id, file, err = New(load(t, gateway, false), NewRequest{Level: "SUB", Prefix: "GW-AUTH", Title: "Keys rotate", Statement: "Keys shall rotate."}); err != nil || id != "GW-AUTH-0002" || file != "docs/requirements/auth/sub.yml" {
		t.Fatalf("got %s in %s: %v", id, file, err)
	}
	if id, file, err = New(load(t, gateway, false), NewRequest{Level: "UNIT", Prefix: "GW-NET", Title: "Retry", Statement: "It shall retry."}); err != nil || id != "GW-NET-0001" || file != "docs/requirements/unit-gw-net.yml" {
		t.Fatalf("got %s in %s: %v", id, file, err)
	}
	if issues := Check(load(t, gateway, false), CheckOptions{}); issues.Errors() != 1 { // the fixture's dangling tag
		t.Fatalf("the new requirements check: %v", issues)
	}

	if _, _, err = New(load(t, gateway, false), NewRequest{Level: "NODE", Title: "x", Statement: "y", Satisfies: []string{"GW-0001"}}); err == nil {
		t.Fatal("a NODE requirement satisfied a NODE requirement")
	}
	if NextID(load(t, gateway, false), "SYS") != "SYS-0004" {
		t.Fatal("the next SYS id is allocated across the family")
	}
}

func TestEditKeepsTheRestOfTheFile(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	nodeFile := filepath.Join(gateway, "docs/requirements/node.yml")
	before := read(t, nodeFile)

	result, err := Edit(load(t, gateway, false), "GW-0002", []EditOp{
		{Op: "add", Key: "satisfies", Value: "SYS-0001"},
		{Op: "set", Key: "owner", Value: "bob"},
	}, EditOptions{})
	if err != nil || result.Reopened {
		t.Fatalf("%+v %v", result, err)
	}
	want := strings.Replace(before, "      - SYS-0002\n    status: verified\n    verification: test\n    changed: 2026-11-10\n",
		"      - SYS-0002\n      - SYS-0001\n    status: verified\n    verification: test\n    owner: bob\n    changed: 2026-11-19\n", 1)
	if got := read(t, nodeFile); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// A statement change on an approved requirement sends it back to proposed.
	result, err = Edit(load(t, gateway, false), "GW-0001", []EditOp{{Op: "set", Key: "statement", Value: "Sessions come back."}, {Op: "remove", Key: "owner"}}, EditOptions{})
	if err != nil || !result.Reopened || result.Status != StatusProposed {
		t.Fatalf("%+v %v", result, err)
	}
	got := read(t, nodeFile)
	if !strings.Contains(got, "    statement: Sessions come back.\n    rationale:") || !strings.Contains(got, "    status: proposed\n") || strings.Contains(got, "owner: maintainer") {
		t.Fatalf("got:\n%s", got)
	}
	if !strings.HasPrefix(got, "# Gateway requirements.\nlevel: NODE\n") {
		t.Fatal("the header comment went")
	}
	// --keep-status keeps it.
	if result, err = Edit(load(t, gateway, false), "GW-0003", []EditOp{{Op: "set", Key: "statement", Value: "Uptime, in seconds."}}, EditOptions{KeepStatus: true}); err != nil || result.Status != StatusApproved {
		t.Fatalf("%+v %v", result, err)
	}

	// A failed edit writes nothing.
	snapshot := read(t, nodeFile)
	for _, ops := range [][]EditOp{
		{{Op: "set", Key: "owner", Value: "x"}, {Op: "remove", Key: "tags", Value: "nope"}},
		{{Op: "set", Key: "id", Value: "GW-0100"}},
		{{Op: "set", Key: "colour", Value: "red"}},
		{{Op: "add", Key: "satisfies", Value: "GW-0001"}},
		{{Op: "set", Key: "verification", Value: "hope"}},
	} {
		if _, err := Edit(load(t, gateway, false), "GW-0002", ops, EditOptions{}); err == nil {
			t.Errorf("%+v was accepted", ops)
		}
	}
	if read(t, nodeFile) != snapshot {
		t.Fatal("a refused edit changed the file")
	}
}

func TestSetStatusFollowsTheLifecycle(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	if _, err := SetStatus(load(t, gateway, false), "GW-0003", StatusVerified, EditOptions{}); err == nil {
		t.Fatal("approved went straight to verified")
	}
	for _, status := range []string{StatusImplemented, StatusVerified, StatusRetired} {
		if _, err := SetStatus(load(t, gateway, false), "GW-0003", status, EditOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SetStatus(load(t, gateway, false), "GW-0003", StatusDraft, EditOptions{}); err == nil {
		t.Fatal("a retired requirement came back")
	}
	if _, err := SetStatus(load(t, gateway, false), "GW-0003", StatusDraft, EditOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	// The parent's file is edited where it is.
	if _, err := SetStatus(load(t, gateway, false), "SYS-0003", StatusProposed, EditOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "robohub/docs/requirements/system.yml")), "    status: proposed\n    verification: inspection\n    changed: 2026-11-19\n") {
		t.Fatal("SYS-0003 was not moved")
	}
}

func TestADRNewAndSupersede(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	id, file, err := NewADR(load(t, gateway, false), "Session table in SQLite", []string{"GW-0001"}, "")
	if err != nil || id != "ADR-0003" || file != "docs/architecture/adr/0003-session-table-in-sqlite.md" {
		t.Fatalf("%s %s %v", id, file, err)
	}
	text := read(t, filepath.Join(gateway, file))
	if !strings.HasPrefix(text, "---\nid: ADR-0003\ntitle: Session table in SQLite\nstatus: proposed\ndate: 2026-11-19\nrequirements: [GW-0001]\n") || !strings.Contains(text, "## Decision") {
		t.Fatalf("got:\n%s", text)
	}
	if err := Supersede(load(t, gateway, false), "ADR-0002", "ADR-0003"); err != nil {
		t.Fatal(err)
	}
	if err := Supersede(load(t, gateway, false), "robohub:ADR-0001", "ADR-0003"); err != nil {
		t.Fatal(err)
	}
	old := read(t, filepath.Join(gateway, "docs/architecture/adr/0002-keep-a-session-table.md"))
	if want := "---\nid: ADR-0002\ntitle: Keep a session table on disk\nstatus: superseded\ndate: 2026-11-19\nrequirements: []\nsupersedes: []\nsuperseded_by: [ADR-0003]\n---\n\nAn ADR about nothing yet.\n"; old != want {
		t.Fatalf("got:\n%s", old)
	}
	if got := read(t, filepath.Join(dir, "robohub/docs/architecture/adr/0001-mesh-bus.md")); !strings.Contains(got, "status: superseded\n") || !strings.Contains(got, "superseded_by: [gateway:ADR-0003]\n") {
		t.Fatalf("got:\n%s", got)
	}
	if got := read(t, filepath.Join(gateway, file)); !strings.Contains(got, "supersedes: [ADR-0002, robohub:ADR-0001]\n") {
		t.Fatalf("got:\n%s", got)
	}
	issues := Check(load(t, gateway, true), CheckOptions{})
	for _, issue := range issues {
		if strings.HasPrefix(issue.Code, "adr.") && issue.Severity == SeverityError {
			t.Fatalf("the superseded ADRs check: %v", issues)
		}
	}
}

func TestInitScaffolds(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	manifest := "module:\n  name: x\n  type: rho\n  version: 0.1.0\n"
	writeFiles(t, dir, map[string]string{"node/module.yml": manifest, "node/docs/doc/index.md": "# pages\n"})
	created, kept, err := Init(InitRequest{Dir: filepath.Join(dir, "node"), Level: "NODE", Prefix: "NEW", Parent: "root"})
	if err != nil || len(kept) != 0 {
		t.Fatal(created, kept, err)
	}
	if got := strings.Join(created, ","); got != "docs/requirements/node-new.yml,docs/architecture/adr/0000-template.md,module.yml (requirements:)" {
		t.Fatalf("created %s", got)
	}
	if got := read(t, filepath.Join(dir, "node/module.yml")); !strings.HasPrefix(got, manifest) || !strings.HasSuffix(got, "requirements:\n  prefix: NEW\n  parent: root\n") {
		t.Fatalf("module.yml:\n%s", got)
	}
	if read(t, filepath.Join(dir, "node/docs/doc/index.md")) != "# pages\n" {
		t.Fatal("docs/doc was touched")
	}
	created, kept, _ = Init(InitRequest{Dir: filepath.Join(dir, "node"), Level: "NODE", Prefix: "NEW", Parent: "root"})
	if len(created) != 0 || len(kept) != 3 {
		t.Fatalf("a second init created %v", created)
	}

	if _, _, err := Init(InitRequest{Dir: filepath.Join(dir, "root"), Root: true, Level: "SYS"}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{LevelsFile, MaintainersFile, "docs/requirements/sys-sys.yml", ADRDir + "/0000-template.md"} {
		if _, err := os.Stat(filepath.Join(dir, "root", rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	// What init wrote checks clean, and the template is not read as an ADR.
	writeFiles(t, dir, map[string]string{"root/module.yml": manifest})
	w := load(t, filepath.Join(dir, "node"), true)
	if issues := Check(w, CheckOptions{}); len(issues) != 0 {
		t.Fatalf("a fresh scaffold: %v", issues)
	}
	if len(w.ADRs()) != 0 {
		t.Fatal("the template was read as an ADR")
	}
	if _, err := ParseMaintainers(read(t, filepath.Join(dir, "root", MaintainersFile))); err != nil {
		t.Fatal(err)
	}
}
