//
// @file governance.go
// @brief Only a maintainer approves: the approvals in a range of commits, checked.
//
// `rho req check --base REF` reads every commit in REF..HEAD (first parents
// only) that touched a requirements file or an ADR, and finds each requirement
// whose status became approved, implemented or verified and each ADR that became
// accepted. The author of that commit must be in the MAINTAINERS.yml that
// governs the repository — the repository's own, else its parent's, up the
// chain — matched by email, by name, or by the GitHub login in a GitHub noreply
// address. When the maintainers file is in the same git repository it is read as
// it was at REF, so a change cannot approve itself by adding its author.
//
// This is a guard, not an authority. A commit's author is whatever the committer
// typed; anybody can write a maintainer's name into a commit. What actually stops
// an unapproved change is the forge: branch protection that requires a review,
// and a CODEOWNERS file that makes the maintainers the reviewers of
// docs/requirements/ and docs/architecture/. This check catches the honest
// mistake and makes the rule visible in CI; it does not replace those.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Maintainer is one entry of MAINTAINERS.yml.
type Maintainer struct {
	Name   string `json:"name"`
	GitHub string `json:"github,omitempty"`
	Email  string `json:"email,omitempty"`
}

// ParseMaintainers reads a MAINTAINERS.yml.
func ParseMaintainers(text string) ([]Maintainer, error) {
	root, err := ParseYAML(text)
	if err != nil {
		return nil, err
	}
	w := &Workspace{}
	ctx := schemaContext{w: w, repo: &Repo{}, file: MaintainersFile}
	ctx.check(root, 1, "", maintainersFileSchema)
	out := []Maintainer{}
	if entry := root.Get("maintainers"); entry != nil && entry.Value.Kind == List {
		for _, item := range entry.Value.Items {
			if item.Kind != Map {
				return nil, fmt.Errorf("line %d: a maintainer is a mapping with name, github and email", item.Line)
			}
			ctx.check(item, item.Line, "", maintainerSchema)
			out = append(out, Maintainer{Name: item.Str("name"), GitHub: item.Str("github"), Email: item.Str("email")})
		}
	}
	if len(w.Issues) > 0 {
		return out, fmt.Errorf("%s", w.Issues[0].Message)
	}
	return out, nil
}

var noreply = regexp.MustCompile(`^(?:[0-9]+\+)?([A-Za-z0-9-]+)@users\.noreply\.github\.com$`)

// IsMaintainer reports whether a commit author is one of the maintainers.
func IsMaintainer(list []Maintainer, name, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	login := ""
	if m := noreply.FindStringSubmatch(email); m != nil {
		login = strings.ToLower(m[1])
	}
	for _, m := range list {
		switch {
		case m.Email != "" && strings.ToLower(m.Email) == email:
			return true
		case m.Name != "" && strings.EqualFold(m.Name, name):
			return true
		case m.GitHub != "" && (strings.EqualFold(m.GitHub, name) || strings.ToLower(m.GitHub) == login):
			return true
		}
	}
	return false
}

// Approval is one status change a maintainer had to make.
type Approval struct {
	Commit, Author, Email string
	ID, Status, File      string
}

func git(dir string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C")
	out, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// Approvals are the approvals made in base..HEAD in a repository.
func Approvals(repoDir, base string) ([]Approval, error) {
	top, err := git(repoDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	top = strings.TrimSpace(top)
	realRepo, _ := filepath.EvalSymlinks(repoDir)
	realTop, _ := filepath.EvalSymlinks(top)
	prefix, _ := filepath.Rel(realTop, realRepo)
	prefix = filepath.ToSlash(prefix)
	if prefix == "." {
		prefix = ""
	} else {
		prefix += "/"
	}
	if _, err := git(repoDir, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("the base %q is not a commit in %s", base, top)
	}
	list, err := git(repoDir, "rev-list", "--reverse", "--first-parent", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	paths := []string{prefix + RequirementsDir, prefix + ADRDir}
	out := []Approval{}
	for _, commit := range strings.Fields(list) {
		who, err := git(repoDir, "show", "-s", "--format=%an%x00%ae", commit)
		if err != nil {
			return nil, err
		}
		author, email, _ := strings.Cut(strings.TrimSpace(who), "\x00")
		changed, err := git(repoDir, append([]string{"diff", "--name-only", commit + "^1", commit, "--"}, paths...)...)
		if err != nil {
			// The first commit of a history has no parent.
			changed, err = git(repoDir, append([]string{"ls-tree", "-r", "--name-only", commit, "--"}, paths...)...)
			if err != nil {
				return nil, err
			}
		}
		for _, path := range strings.Fields(changed) {
			before, _ := git(top, "show", commit+"^1:"+path)
			after, err := git(top, "show", commit+":"+path)
			if err != nil {
				continue // deleted
			}
			rel := strings.TrimPrefix(path, prefix)
			switch {
			case strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml"):
				old, now := statusesIn(before), statusesIn(after)
				for _, id := range now.order {
					status := now.status[id]
					if Committed(status) && old.status[id] != status {
						out = append(out, Approval{Commit: commit, Author: author, Email: email, ID: id, Status: status, File: rel})
					}
				}
			case strings.HasSuffix(path, ".md"):
				if adrStatus(after) == "accepted" && adrStatus(before) != "accepted" {
					out = append(out, Approval{Commit: commit, Author: author, Email: email, ID: adrID(after, rel), Status: "accepted", File: rel})
				}
			}
		}
	}
	return out, nil
}

type statusMap struct {
	order  []string
	status map[string]string
}

func statusesIn(text string) statusMap {
	out := statusMap{status: map[string]string{}}
	root, err := ParseYAML(text)
	if err != nil || root == nil {
		return out
	}
	if entry := root.Get("requirements"); entry != nil && entry.Value.Kind == List {
		for _, item := range entry.Value.Items {
			if id := item.Str("id"); id != "" {
				out.order = append(out.order, id)
				out.status[id] = item.Str("status")
			}
		}
	}
	return out
}

func adrHeader(text string) *Node {
	lines := splitLines(text)
	start, end, ok := frontMatter(lines)
	if !ok {
		return nil
	}
	root, err := parseLines(lines[start:end], start)
	if err != nil {
		return nil
	}
	return root
}

func adrStatus(text string) string {
	if root := adrHeader(text); root != nil {
		return root.Str("status")
	}
	return ""
}

func adrID(text, file string) string {
	if root := adrHeader(text); root != nil && root.Str("id") != "" {
		return root.Str("id")
	}
	return file
}

// CheckApprovals is the maintainer check for one repository.
func CheckApprovals(w *Workspace, repo *Repo, base string) Issues {
	out := Issues{}
	approvals, err := Approvals(repo.Dir, base)
	if err != nil {
		return append(out, Issue{Severity: SeverityError, Code: "governance.git", Repo: repo.Name, Message: err.Error()})
	}
	if len(approvals) == 0 {
		return out
	}
	if repo.Maintainers == "" {
		return append(out, Issue{Severity: SeverityError, Code: "governance.no-maintainers", Repo: repo.Name,
			Message: fmt.Sprintf("%d approvals since %s, and no MAINTAINERS.yml governs this repository (its own or a parent's)", len(approvals), base)})
	}
	text, source := maintainersAt(repo, base)
	list, err := ParseMaintainers(text)
	if err != nil {
		return append(out, Issue{Severity: SeverityError, Code: "governance.maintainers", Repo: repo.Name, Message: fmt.Sprintf("%s: %v", source, err)})
	}
	for _, approval := range approvals {
		if IsMaintainer(list, approval.Author, approval.Email) {
			continue
		}
		short := approval.Commit
		if len(short) > 10 {
			short = short[:10]
		}
		out = append(out, Issue{Severity: SeverityError, Code: "governance.not-maintainer", Repo: repo.Name, File: approval.File, ID: approval.ID,
			Message: fmt.Sprintf("only a maintainer approves: %s became %s in %s by %s <%s>, who is not in %s", approval.ID, approval.Status, short, approval.Author, approval.Email, source)})
	}
	return out
}

// maintainersAt is the governing maintainers file as it was at base when it is in
// the repository's own git history, else as it is now.
func maintainersAt(repo *Repo, base string) (string, string) {
	path := repo.Maintainers
	if top, err := git(repo.Dir, "rev-parse", "--show-toplevel"); err == nil {
		realTop, _ := filepath.EvalSymlinks(strings.TrimSpace(top))
		realPath, _ := filepath.EvalSymlinks(path)
		if rel, err := filepath.Rel(realTop, realPath); err == nil && !strings.HasPrefix(rel, "..") {
			if text, err := git(repo.Dir, "show", base+":"+filepath.ToSlash(rel)); err == nil {
				return text, filepath.ToSlash(rel) + " at " + base
			}
		}
	}
	data, _ := os.ReadFile(path)
	return string(data), path
}
