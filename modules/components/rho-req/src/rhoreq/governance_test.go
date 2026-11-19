//
// @file governance_test.go
// @brief --base: approvals in a range of commits, by a maintainer and by someone else.
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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAs runs git in a repository as an author.
func gitAs(t *testing.T, dir, name, email string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email, "GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email,
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

const govFile = `level: SYS
prefix: SYS
requirements:
  - id: SYS-0001
    title: One
    statement: One.
    status: proposed
    verification: inspection
    changed: 2026-11-19
  - id: SYS-0002
    title: Two
    statement: Two.
    status: proposed
    verification: inspection
    changed: 2026-11-19
`

func TestOnlyAMaintainerApproves(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	isolate(t)
	dir := filepath.Join(t.TempDir(), "root")
	writeFiles(t, dir, map[string]string{
		"module.yml":                      "module:\n  name: root\n  type: rho\n  version: 0.1.0\n",
		"MAINTAINERS.yml":                 "maintainers:\n  - name: Ada Maintainer\n    github: ada\n    email: ada@example.org\n",
		"docs/requirements/sys.yml":       govFile,
		"docs/architecture/adr/0001-a.md": "---\nid: ADR-0001\ntitle: A\nstatus: proposed\ndate: 2026-11-19\nrequirements: [SYS-0001]\n---\n",
	})
	gitAs(t, dir, "Ada Maintainer", "ada@example.org", "init", "-q", "-b", "main")
	gitAs(t, dir, "Ada Maintainer", "ada@example.org", "add", "-A")
	gitAs(t, dir, "Ada Maintainer", "ada@example.org", "commit", "-q", "-m", "start")
	base := gitAs(t, dir, "Ada Maintainer", "ada@example.org", "rev-parse", "HEAD")

	// The maintainer approves SYS-0001, through a GitHub noreply address.
	path := filepath.Join(dir, "docs/requirements/sys.yml")
	doc := NewDoc(read(t, path), false)
	doc.Set(RequirementItem("SYS-0001"), "status", "approved", RequirementKeys)
	os.WriteFile(path, []byte(doc.String()), 0644)
	gitAs(t, dir, "A. M.", "12345+ada@users.noreply.github.com", "commit", "-q", "-am", "approve SYS-0001")

	if issues := Check(load(t, dir, false), CheckOptions{Base: base}); issues.Errors() != 0 {
		t.Fatalf("a maintainer's approval was refused: %v", issues)
	}

	// Someone else approves SYS-0002, adds themselves to MAINTAINERS.yml in the
	// same commit, and accepts the ADR.
	doc = NewDoc(read(t, path), false)
	doc.Set(RequirementItem("SYS-0002"), "status", "approved", RequirementKeys)
	os.WriteFile(path, []byte(doc.String()), 0644)
	adr := filepath.Join(dir, "docs/architecture/adr/0001-a.md")
	os.WriteFile(adr, []byte(strings.Replace(read(t, adr), "status: proposed", "status: accepted", 1)), 0644)
	os.WriteFile(filepath.Join(dir, "MAINTAINERS.yml"), []byte(read(t, filepath.Join(dir, "MAINTAINERS.yml"))+"  - name: Eve Outsider\n    email: eve@example.org\n"), 0644)
	gitAs(t, dir, "Eve Outsider", "eve@example.org", "commit", "-q", "-am", "approve SYS-0002")

	issues := Check(load(t, dir, false), CheckOptions{Base: base})
	refused := []string{}
	for _, issue := range issues {
		if issue.Code == "governance.not-maintainer" {
			refused = append(refused, issue.ID)
			if !strings.Contains(issue.Message, "only a maintainer approves") || !strings.Contains(issue.Message, "eve@example.org") {
				t.Errorf("message: %s", issue.Message)
			}
		}
	}
	if got := strings.Join(refused, ","); got != "ADR-0001,SYS-0002" {
		t.Fatalf("refused %q: %v", got, issues)
	}

	// No maintainers file at all.
	os.Remove(filepath.Join(dir, "MAINTAINERS.yml"))
	gitAs(t, dir, "Ada Maintainer", "ada@example.org", "commit", "-q", "-am", "drop maintainers")
	if !hasCode(Check(load(t, dir, false), CheckOptions{Base: base}), SeverityError, "governance.no-maintainers") {
		t.Fatal("approvals with no MAINTAINERS.yml are an error")
	}
	if !hasCode(Check(load(t, dir, false), CheckOptions{Base: "no-such-ref"}), SeverityError, "governance.git") {
		t.Fatal("a base that is not a commit is an error")
	}
}

func TestIsMaintainer(t *testing.T) {
	list := []Maintainer{{Name: "Ada Maintainer", GitHub: "ada", Email: "Ada@Example.org"}}
	for _, c := range []struct {
		name, email string
		ok          bool
	}{
		{"Someone", "ada@example.org", true},
		{"ada maintainer", "x@y", true},
		{"ada", "x@y", true},
		{"X", "99+ada@users.noreply.github.com", true},
		{"X", "ada@users.noreply.github.com", true},
		{"Eve", "eve@example.org", false},
		{"X", "eve@users.noreply.github.com", false},
	} {
		if IsMaintainer(list, c.name, c.email) != c.ok {
			t.Errorf("%s <%s>: %v", c.name, c.email, !c.ok)
		}
	}
}
