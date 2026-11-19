//
// @file tsv_test.go
// @brief `--tsv`: one escaped record per line, the same answers as --json.
//
// @project Rho
// @version 0.3
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

// records is the output as fields per line, unescaped, by kind.
func records(t *testing.T, out string) map[string][][]string {
	t.Helper()
	byKind := map[string][][]string{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		for index := range fields {
			fields[index] = TSVUnescape(fields[index])
		}
		byKind[fields[0]] = append(byKind[fields[0]], fields[1:])
	}
	return byKind
}

func TestTSVEscapeRoundTrips(t *testing.T) {
	for _, value := range []string{"", "plain", "a\tb", "line\nnext", `back\slash`, "cr\r\n", `\n literal`} {
		escaped := TSVEscape(value)
		if strings.ContainsAny(escaped, "\t\n\r") {
			t.Fatalf("%q escapes to %q", value, escaped)
		}
		if back := TSVUnescape(escaped); back != value {
			t.Fatalf("%q -> %q -> %q", value, escaped, back)
		}
	}
}

func TestTSVList(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	status, out, _ := run("list", gateway, "--workspace", "--tsv")
	if status != 0 {
		t.Fatalf("list --tsv: %d", status)
	}
	got := records(t, out)
	if len(got["repo"]) != 2 {
		t.Fatalf("the family's repositories: %v", got["repo"])
	}
	var gw []string
	for _, repo := range got["repo"] {
		if repo[0] == "gateway" {
			gw = repo
		}
	}
	if gw == nil || gw[1] != gateway || gw[2] != "GW" || gw[3] != "robohub" || !strings.HasSuffix(gw[4], "MAINTAINERS.yml") || gw[5] != "SYS,NODE,SUB,UNIT" {
		t.Fatalf("repo record: %v", gw)
	}
	var first []string
	for _, req := range got["req"] {
		if req[0] == "GW-0001" {
			first = req
		}
	}
	// id repo level status verification title satisfies tags file line owner changed
	if first == nil || first[1] != "gateway" || first[2] != "NODE" || first[3] != "approved" || first[4] != "test" ||
		first[5] != "Sessions survive a gateway restart" || first[6] != "SYS-0001" || first[7] != "security" ||
		first[8] != "docs/requirements/node.yml" || first[9] == "" || first[10] != "maintainer" || first[11] != "2026-11-19" {
		t.Fatalf("req record: %v", first)
	}
	_, out, _ = run("list", gateway, "--tsv", "--status", "draft", "--workspace")
	if reqs := records(t, out)["req"]; len(reqs) != 1 || reqs[0][0] != "SYS-0003" {
		t.Fatalf("filtered: %v", reqs)
	}
}

func TestTSVShowTraceCheck(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")

	status, out, _ := run("show", "GW-0001", "--dir", gateway, "--tsv")
	got := records(t, out)
	if status != 0 || got["req"][0][0] != "GW-0001" || !strings.HasPrefix(got["statement"][0][0], "After a restart") ||
		got["rationale"][0][0] != "A power cut must not need a person." || got["evidence"][0][0] != EvidencePassing {
		t.Fatalf("show --tsv: %s", out)
	}
	if got["up"][0][1] != "SYS-0001" || got["up"][0][0] != "0" || len(got["down"]) == 0 || len(got["adr"]) != 2 || len(got["test"]) == 0 {
		t.Fatalf("show --tsv trace: %s", out)
	}
	_, out, _ = run("show", "SYS-0001", "--dir", gateway, "--tsv")
	if statement := records(t, out)["statement"][0][0]; strings.Contains(statement, "\t") || !strings.Contains(statement, "every node") {
		t.Fatalf("a folded statement: %q", statement)
	}

	status, out, _ = run("show", "GW-9999", "--dir", gateway, "--tsv")
	if errs := records(t, out)["error"]; status != 1 || len(errs) != 1 || !strings.Contains(errs[0][0], "does not exist") {
		t.Fatalf("an error is a record: %d %s", status, out)
	}

	status, out, _ = run("trace", gateway, "--workspace", "--tsv", "--release")
	got = records(t, out)
	if status != 1 || len(got["row"]) == 0 || len(got["gap"]) == 0 || got["summary"][0][1] != "no" {
		t.Fatalf("trace --tsv: %d %s", status, out)
	}
	for _, row := range got["row"] {
		if len(row) != 12 {
			t.Fatalf("a row has 12 fields: %v", row)
		}
	}

	status, out, _ = run("check", gateway, "--tsv")
	got = records(t, out)
	summary := got["summary"][0]
	if status != 1 || summary[0] != "no" || summary[1] != "1" || summary[2] != "2" || len(got["issue"]) != 3 {
		t.Fatalf("check --tsv: %d %s", status, out)
	}
	for _, issue := range got["issue"] {
		if issue[0] == "error" && (issue[3] == "" || issue[4] == "0") {
			t.Fatalf("an error says where: %v", issue)
		}
	}
}

func TestTSVChangesAndADRStatus(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	status, out, _ := run("new", "--dir", gateway, "--tsv", "--level=NODE", "--title=T", "--statement=S")
	if created := records(t, out)["created"]; status != 0 || len(created) != 1 || created[0][0] != "GW-0004" || created[0][1] != "gateway" || created[0][2] != "docs/requirements/node.yml" {
		t.Fatalf("new --tsv: %s", out)
	}
	status, out, _ = run("set-status", "GW-0004", "proposed", "--dir", gateway, "--tsv")
	if updated := records(t, out)["updated"]; status != 0 || updated[0][0] != "GW-0004" || updated[0][2] != "proposed" || updated[0][3] != "no" {
		t.Fatalf("set-status --tsv: %s", out)
	}
	status, out, _ = run("adr", "new", "--dir", gateway, "--tsv", "--title", "Use Y")
	if created := records(t, out)["created"]; status != 0 || created[0][0] != "ADR-0003" || !strings.HasPrefix(created[0][2], "docs/architecture/adr/0003-") {
		t.Fatalf("adr new --tsv: %s", out)
	}
	adrFile := filepath.Join(gateway, "docs/architecture/adr/0003-use-y.md")
	before := read(t, adrFile)
	status, out, _ = run("adr", "set-status", "ADR-0003", "accepted", "--dir", gateway, "--tsv")
	if updated := records(t, out)["updated"]; status != 0 || updated[0][0] != "gateway:ADR-0003" || updated[0][2] != "accepted" {
		t.Fatalf("adr set-status --tsv: %d %s", status, out)
	}
	after := read(t, adrFile)
	if after != strings.Replace(before, "status: proposed", "status: accepted", 1) {
		t.Fatalf("only the status line changes:\n%s", after)
	}
	if status, _, _ := run("adr", "set-status", "ADR-0003", "superseded", "--dir", gateway); status != 1 {
		t.Fatal("superseded is supersede's")
	}
	if status, _, _ := run("adr", "set-status", "ADR-0003", "accepted", "--dir", gateway); status != 1 {
		t.Fatal("the same status again is refused")
	}
	if status, _, _ := run("adr", "set-status", "ADR-0003", "maybe", "--dir", gateway); status != 1 {
		t.Fatal("an unknown status is refused")
	}
	status, out, _ = run("adr", "supersede", "ADR-0001", "ADR-0003", "--dir", gateway, "--tsv")
	if status != 0 || records(t, out)["superseded"][0][1] != "ADR-0003" {
		t.Fatalf("supersede --tsv: %s", out)
	}
	if status, _, _ := run("adr", "set-status", "ADR-0001", "accepted", "--dir", gateway); status != 1 {
		t.Fatal("a superseded decision stays superseded")
	}
	_, out, _ = run("adr", "list", gateway, "--tsv")
	got := records(t, out)
	if len(got["adr"]) != 3 || got["adr"][0][2] != "superseded" || got["adr"][0][7] != "ADR-0003" || got["adr"][0][8] != "docs/architecture/adr/0001-reset-sessions.md" {
		t.Fatalf("adr list --tsv: %s", out)
	}
}

func TestARelativeDirIsOneRepository(t *testing.T) {
	dir := fixture(t)
	t.Setenv("ROBOHUB_WORKSPACE", dir)
	t.Chdir(dir)
	_, out, _ := run("list", "--workspace", "--tsv", "--dir", "robohub")
	if repos := records(t, out)["repo"]; len(repos) != 2 || repos[0][1] != filepath.Join(dir, "robohub") {
		t.Fatalf("a repository named by a relative path is counted once: %v", repos)
	}
}
