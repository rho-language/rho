//
// @file cli_test.go
// @brief The command line: flags anywhere, JSON, exit status, help.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, err bytes.Buffer
	status := Main(args, &out, &err)
	return status, out.String(), err.String()
}

func TestCheckFlagsBeforeOrAfterThePath(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	for _, args := range [][]string{
		{"check", gateway, "--json"},
		{"check", "--json", gateway},
		{"check", "-json", "--dir", gateway},
	} {
		status, out, _ := run(args...)
		if status != 1 {
			t.Fatalf("%v: exit %d (the fixture has a dangling tag)", args, status)
		}
		var report CheckReport
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if report.OK || report.Errors != 1 || report.Warnings != 2 || report.Repos[0] != "gateway" {
			t.Fatalf("%v: %+v", args, report)
		}
	}
	if status, _, _ := run("check", filepath.Join(dir, "standalone")); status != 0 {
		t.Fatal("a clean repository exits 0")
	}
	unit := filepath.Join(dir, "standalone", "docs/requirements/unit.yml")
	os.WriteFile(unit, []byte(strings.Replace(read(t, unit), "status: draft", "status: approved", 1)), 0644)
	if status, _, _ := run("check", filepath.Join(dir, "standalone")); status != 0 {
		t.Fatal("a warning alone exits 0")
	}
	if status, _, _ := run("check", filepath.Join(dir, "standalone"), "--strict"); status != 1 {
		t.Fatal("--strict fails on warnings")
	}
	if status, _, _ := run("check", "--nope"); status != 2 {
		t.Fatal("an unknown flag is a usage error")
	}
}

func TestListShowTraceRender(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")

	_, out, _ := run("list", gateway, "--workspace", "--level", "SYS")
	if strings.Count(out, "\n") != 3 || !strings.Contains(out, "robohub") {
		t.Fatalf("list: %s", out)
	}
	_, out, _ = run("list", "--json", "--status", "approved", gateway)
	var reqs []Requirement
	if err := json.Unmarshal([]byte(out), &reqs); err != nil || len(reqs) != 2 {
		t.Fatalf("list --json: %v %s", err, out)
	}

	status, out, _ := run("show", "GW-0001", "--dir", gateway)
	if status != 0 || !strings.Contains(out, "SYS-0001  Survive a power cut") || !strings.Contains(out, "GW-AUTH-0001") || !strings.Contains(out, "pass") {
		t.Fatalf("show: %s", out)
	}
	_, out, _ = run("show", "--json", "GW-0001", "--dir", gateway)
	var shown ShowReport
	if err := json.Unmarshal([]byte(out), &shown); err != nil || shown.Evidence != EvidencePassing || len(shown.ADRs) != 2 || shown.Upward[0] != "SYS-0001" {
		t.Fatalf("show --json: %v %s", err, out)
	}
	if status, _, _ := run("show", "GW-9999", "--dir", gateway); status != 1 {
		t.Fatal("an unknown id is an error")
	}

	if status, _, _ := run("trace", gateway); status != 0 {
		t.Fatal("trace without --release reports and exits 0")
	}
	status, out, _ = run("trace", gateway, "--release", "--json")
	var trace TraceReport
	if err := json.Unmarshal([]byte(out), &trace); err != nil || status != 1 || trace.Release || trace.Blocking == 0 {
		t.Fatalf("trace --release: %d %v %s", status, err, out)
	}
	if status, _, _ := run("trace", filepath.Join(dir, "standalone"), "--release"); status != 0 {
		t.Fatal("a repository with nothing committed is releasable")
	}

	status, out, _ = run("render", gateway)
	target := filepath.Join(gateway, "build", "requirements", "requirements.md")
	if status != 0 || strings.TrimSpace(out) != target {
		t.Fatalf("render: %d %s", status, out)
	}
	if !strings.HasPrefix(read(t, target), "# Requirements: gateway\n") {
		t.Fatal("render wrote something else")
	}
	other := filepath.Join(dir, "out")
	os.MkdirAll(other, 0755)
	run("render", gateway, "--out", other)
	if _, err := os.Stat(filepath.Join(other, "requirements.md")); err != nil {
		t.Fatal("--out DIR")
	}
	_, out, _ = run("render", gateway, "--out", "-", "--level", "NODE")
	if !strings.Contains(out, "## Node (NODE)") {
		t.Fatal("--out -")
	}
}

func TestChangesFromTheCommandLine(t *testing.T) {
	dir := fixture(t)
	gateway := filepath.Join(dir, "gateway")
	status, out, errOut := run("new", "--dir", gateway, "--level", "NODE", "--title", "T", "--statement", "S", "--satisfies", "SYS-0001")
	if status != 0 || strings.TrimSpace(out) != "GW-0004" || !strings.Contains(errOut, "node.yml") {
		t.Fatalf("new: %d %q %q", status, out, errOut)
	}
	if status, _, errOut = run("edit", "GW-0004", "--dir", gateway, "--set", "owner=ada", "--add", "tags=a,b"); status != 0 {
		t.Fatalf("edit: %s", errOut)
	}
	if !strings.Contains(read(t, filepath.Join(gateway, "docs/requirements/node.yml")), "    owner: ada\n    changed: 2026-11-19\n    tags: [a, b]\n") {
		t.Fatal("edit did not land")
	}
	if status, _, _ = run("edit", "GW-0004", "--dir", gateway); status != 2 {
		t.Fatal("an edit with nothing to change is a usage error")
	}
	if status, _, _ = run("set-status", "GW-0004", "approved", "--dir", gateway); status != 1 {
		t.Fatal("draft -> approved is refused")
	}
	if status, _, _ = run("set-status", "--dir", gateway, "GW-0004", "proposed"); status != 0 {
		t.Fatal("draft -> proposed")
	}
	if status, out, _ = run("adr", "new", "--dir", gateway, "--title", "Use X", "--requirements", "GW-0004"); status != 0 || strings.TrimSpace(out) != "ADR-0003" {
		t.Fatalf("adr new: %q", out)
	}
	if status, _, _ = run("adr", "supersede", "ADR-0002", "ADR-0003", "--dir", gateway); status != 0 {
		t.Fatal("adr supersede")
	}
	_, out, _ = run("adr", "list", gateway, "--json")
	var adrs []ADR
	if err := json.Unmarshal([]byte(out), &adrs); err != nil || len(adrs) != 3 || adrs[1].Status != "superseded" {
		t.Fatalf("adr list: %v %s", err, out)
	}
	fresh := filepath.Join(dir, "fresh")
	os.MkdirAll(fresh, 0755)
	if status, out, _ = run("init", fresh, "--root"); status != 0 || !strings.Contains(out, "created "+LevelsFile) {
		t.Fatalf("init: %s", out)
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"help", "check"}, {"check", "--help"}, {"adr", "--help"}, {"edit", "-h"}} {
		status, out, _ := run(args...)
		if status != 0 || !strings.Contains(out, "usage: rho req") {
			t.Errorf("%v: %d %q", args, status, out)
		}
	}
	if status, _, _ := run(); status != 2 {
		t.Error("no command is a usage error")
	}
	if status, _, _ := run("frobnicate"); status != 2 {
		t.Error("an unknown command is a usage error")
	}
}
