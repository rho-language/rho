//
// @file ops.go
// @brief new, edit, set-status, the ADR commands and init: the changes rho req makes.
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
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Today is the date an edit records in `changed:`. SOURCE_DATE_EPOCH, when set,
// fixes it, so a build or a test that edits is reproducible.
func Today() string {
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		if seconds, err := strconv.ParseInt(epoch, 10, 64); err == nil {
			return time.Unix(seconds, 0).UTC().Format("2006-01-02")
		}
	}
	return time.Now().Format("2006-01-02")
}

func writeDoc(path string, doc *Doc) error {
	text := doc.String()
	check := NewDoc(text, doc.FrontMatter)
	if _, err := check.Root(); err != nil {
		return fmt.Errorf("the edit would leave %s unreadable (%v); nothing was written", path, err)
	}
	info, err := os.Stat(path)
	mode := os.FileMode(0644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, []byte(text), mode)
}

func readDoc(path string, frontMatter bool) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return NewDoc(string(data), frontMatter), nil
}

/* -------------------------------------------------------------------------- */
/*  new                                                                       */
/* -------------------------------------------------------------------------- */

// NewRequest is what `rho req new` was given.
type NewRequest struct {
	Level, Prefix, File        string
	Title, Statement           string
	Rationale, Owner           string
	Status, Verification       string
	Satisfies, DecidedBy, Tags []string
}

// NextID is the next free id for a prefix across the family.
func NextID(w *Workspace, prefix string) string {
	highest := 0
	for _, req := range w.AllRequirements() {
		if p, number, ok := SplitID(req.ID); ok && p == prefix && number > highest {
			highest = number
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, highest+1)
}

// New appends a requirement to the focus repository and returns its id and file.
func New(w *Workspace, request NewRequest) (string, string, error) {
	repo := w.Focus
	if request.Title == "" || request.Statement == "" {
		return "", "", fmt.Errorf("a requirement needs --title and --statement")
	}
	var target *ReqFile
	path := ""
	if request.File != "" {
		path = request.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo.Dir, RequirementsDir, path)
		}
		if filepath.Ext(path) == "" {
			path += ".yml"
		}
		for _, file := range repo.Files {
			if file.Path == path {
				target = file
			}
		}
		if target != nil {
			if request.Level == "" {
				request.Level = target.Level
			}
			if request.Prefix == "" {
				request.Prefix = target.Prefix
			}
			if request.Level != target.Level || request.Prefix != target.Prefix {
				return "", "", fmt.Errorf("%s is a %s file with prefix %s, not %s with prefix %s", target.Rel, target.Level, target.Prefix, request.Level, request.Prefix)
			}
		}
	}
	if request.Level == "" {
		return "", "", fmt.Errorf("say which level the requirement is at (--level %s)", strings.ReplaceAll(levelNames(repo.Levels), ", ", "|"))
	}
	if LevelIndex(repo.Levels, request.Level) < 0 {
		return "", "", fmt.Errorf("the level %s is not one of %s", request.Level, levelNames(repo.Levels))
	}
	if request.Prefix == "" {
		if request.Level == repo.Levels[0].Name {
			request.Prefix = repo.Levels[0].Name
		} else if repo.Prefix != "" {
			request.Prefix = repo.Prefix
		} else {
			return "", "", fmt.Errorf("say which prefix the id takes (--prefix), or declare requirements.prefix in module.yml")
		}
	}
	if !prefixPattern.MatchString(request.Prefix) {
		return "", "", fmt.Errorf("the prefix %q is not upper-case words joined by dashes", request.Prefix)
	}
	if target == nil && path == "" {
		for _, file := range repo.Files {
			if file.Level == request.Level && file.Prefix == request.Prefix {
				target = file
				path = file.Path
				break
			}
		}
		if target == nil {
			path = filepath.Join(repo.Dir, RequirementsDir, strings.ToLower(request.Level+"-"+request.Prefix)+".yml")
		}
	}
	if request.Status == "" {
		request.Status = StatusDraft
	}
	if !contains(Statuses, request.Status) {
		return "", "", fmt.Errorf("the status %q is not one of %s", request.Status, strings.Join(Statuses, ", "))
	}
	if request.Verification == "" {
		request.Verification = "test"
	}
	if !contains(Verifications, request.Verification) {
		return "", "", fmt.Errorf("the verification %q is not one of %s", request.Verification, strings.Join(Verifications, ", "))
	}
	for _, id := range request.Satisfies {
		parent := w.Requirement(id)
		if parent == nil {
			return "", "", fmt.Errorf("%s does not exist, so nothing can satisfy it", id)
		}
		if LevelIndex(repo.Levels, parent.Level) >= LevelIndex(repo.Levels, request.Level) {
			return "", "", fmt.Errorf("%s is at %s; a %s requirement satisfies only a higher level", id, parent.Level, request.Level)
		}
	}
	for _, ref := range request.DecidedBy {
		if w.ADR(repo.Name, ref) == nil {
			return "", "", fmt.Errorf("the ADR %s does not exist", ref)
		}
	}

	id := NextID(w, request.Prefix)
	fields := []KV{{"id", id}, {"title", formatScalar(request.Title)}, {"statement", formatScalar(request.Statement)}}
	if request.Rationale != "" {
		fields = append(fields, KV{"rationale", formatScalar(request.Rationale)})
	}
	fields = append(fields, KV{"satisfies", formatFlow(request.Satisfies)}, KV{"status", request.Status}, KV{"verification", request.Verification})
	if len(request.DecidedBy) > 0 {
		fields = append(fields, KV{"decided_by", formatFlow(request.DecidedBy)})
	}
	if request.Owner != "" {
		fields = append(fields, KV{"owner", formatScalar(request.Owner)})
	}
	fields = append(fields, KV{"changed", Today()})
	if len(request.Tags) > 0 {
		fields = append(fields, KV{"tags", formatFlow(request.Tags)})
	}

	var doc *Doc
	if fileExists(path) {
		var err error
		if doc, err = readDoc(path, false); err != nil {
			return "", "", err
		}
		root, err := doc.Root()
		if err != nil {
			return "", "", fmt.Errorf("%s: %v", path, err)
		}
		if level, prefix := root.Str("level"), root.Str("prefix"); level != request.Level || prefix != request.Prefix {
			return "", "", fmt.Errorf("%s is a %s file with prefix %s, not %s with prefix %s", path, level, prefix, request.Level, request.Prefix)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", "", err
		}
		doc = NewDoc(FileHeader(repo.Levels, request.Level, request.Prefix, ""), false)
	}
	if err := doc.AppendItem("requirements", fields); err != nil {
		return "", "", fmt.Errorf("%s: %v", path, err)
	}
	if err := writeDoc(path, doc); err != nil {
		return "", "", err
	}
	rel, _ := filepath.Rel(repo.Dir, path)
	return id, filepath.ToSlash(rel), nil
}

// FileHeader is a new requirements file with no requirements yet.
func FileHeader(levels []Level, level, prefix, title string) string {
	levelTitle := level
	if index := LevelIndex(levels, level); index >= 0 {
		levelTitle = levels[index].Title
	}
	if title == "" {
		title = prefix
	}
	return fmt.Sprintf(`# %s requirements, %s level.
#
# Edited by hand and by `+"`rho req`"+`, which keeps comments, blank lines and key
# order. Check with `+"`rho req check`"+`.
level: %s
prefix: %s
title: %s
requirements: []
`, title, strings.ToLower(levelTitle), level, prefix, formatScalar(title))
}

/* -------------------------------------------------------------------------- */
/*  edit and set-status                                                       */
/* -------------------------------------------------------------------------- */

// EditOp is one change to a requirement.
type EditOp struct {
	// Op is "set", "add" or "remove".
	Op         string
	Key, Value string
}

// EditOptions qualify an edit.
type EditOptions struct {
	// KeepStatus keeps a committed requirement's status when its statement changes.
	KeepStatus bool
	// Force allows a status change the lifecycle does not.
	Force bool
}

// EditResult says what an edit did.
type EditResult struct {
	ID, File string
	// Reopened is set when a statement change sent the requirement back to proposed.
	Reopened bool
	Status   string
}

var listKeys = map[string]bool{"satisfies": true, "decided_by": true, "tags": true}

// Edit applies changes to one requirement of the focus repository.
func Edit(w *Workspace, id string, ops []EditOp, opts EditOptions) (EditResult, error) {
	req := w.Requirement(id)
	if req == nil {
		return EditResult{}, fmt.Errorf("%s does not exist", id)
	}
	repo := w.Repo(req.Repo)
	path := filepath.Join(repo.Dir, filepath.FromSlash(req.File))
	doc, err := readDoc(path, false)
	if err != nil {
		return EditResult{}, err
	}
	loc := RequirementItem(id)
	result := EditResult{ID: id, File: req.File, Status: req.Status}
	statementChanged := false
	statusSet := false
	for _, op := range ops {
		if !contains(RequirementKeys, op.Key) {
			return result, fmt.Errorf("%q is not a requirement key (%s)", op.Key, strings.Join(RequirementKeys, ", "))
		}
		if op.Key == "id" {
			return result, fmt.Errorf("an id does not change; retire the requirement and create a new one")
		}
		if op.Key == "changed" {
			return result, fmt.Errorf("changed is set by every edit")
		}
		switch op.Op {
		case "set":
			if listKeys[op.Key] {
				values := splitList(op.Value)
				if err := validateListValues(w, req, op.Key, values); err != nil {
					return result, err
				}
				err = doc.SetList(loc, op.Key, values, RequirementKeys)
				break
			}
			switch op.Key {
			case "status":
				if !contains(Statuses, op.Value) {
					return result, fmt.Errorf("the status %q is not one of %s", op.Value, strings.Join(Statuses, ", "))
				}
				if op.Value != result.Status && !opts.Force && !CanTransition(result.Status, op.Value) {
					return result, fmt.Errorf("%s cannot go from %s to %s (%s); --force overrides", id, result.Status, op.Value, describeTransitions(result.Status))
				}
				result.Status = op.Value
				statusSet = true
			case "verification":
				if !contains(Verifications, op.Value) {
					return result, fmt.Errorf("the verification %q is not one of %s", op.Value, strings.Join(Verifications, ", "))
				}
			case "statement":
				if strings.TrimSpace(op.Value) != req.Statement {
					statementChanged = true
				}
			case "title":
				if strings.TrimSpace(op.Value) == "" {
					return result, fmt.Errorf("a title cannot be empty")
				}
			}
			err = doc.Set(loc, op.Key, op.Value, RequirementKeys)
		case "add":
			if !listKeys[op.Key] {
				return result, fmt.Errorf("--add works on a list (satisfies, decided_by, tags), not %s", op.Key)
			}
			for _, value := range splitList(op.Value) {
				if err := validateListValues(w, req, op.Key, []string{value}); err != nil {
					return result, err
				}
				if err = doc.Add(loc, op.Key, value, RequirementKeys); err != nil {
					break
				}
			}
		case "remove":
			if op.Value == "" {
				if contains([]string{"title", "statement", "status", "verification"}, op.Key) {
					return result, fmt.Errorf("%s is required and cannot be removed", op.Key)
				}
				err = doc.Delete(loc, op.Key)
				break
			}
			if !listKeys[op.Key] {
				return result, fmt.Errorf("--remove %s=VALUE works on a list; --remove %s removes the key", op.Key, op.Key)
			}
			for _, value := range splitList(op.Value) {
				if err = doc.Remove(loc, op.Key, value); err != nil {
					break
				}
			}
		default:
			return result, fmt.Errorf("unknown edit %q", op.Op)
		}
		if err != nil {
			return result, fmt.Errorf("%s: %v", req.File, err)
		}
	}
	if statementChanged && Committed(result.Status) && !opts.KeepStatus && !statusSet {
		if err := doc.Set(loc, "status", StatusProposed, RequirementKeys); err != nil {
			return result, err
		}
		result.Status, result.Reopened = StatusProposed, true
	}
	if err := doc.Set(loc, "changed", Today(), RequirementKeys); err != nil {
		return result, err
	}
	return result, writeDoc(path, doc)
}

func validateListValues(w *Workspace, req *Requirement, key string, values []string) error {
	levels := w.LevelsOf(req.Repo)
	for _, value := range values {
		switch key {
		case "satisfies":
			parent := w.Requirement(value)
			if parent == nil {
				return fmt.Errorf("%s does not exist", value)
			}
			if value == req.ID || LevelIndex(levels, parent.Level) >= LevelIndex(levels, req.Level) {
				return fmt.Errorf("%s is at %s; %s (%s) satisfies only a higher level", value, parent.Level, req.ID, req.Level)
			}
		case "decided_by":
			if w.ADR(req.Repo, value) == nil {
				return fmt.Errorf("the ADR %s does not exist", value)
			}
		}
	}
	return nil
}

func describeTransitions(from string) string {
	allowed := append(append([]string{}, transitions[from]...), StatusRejected, StatusRetired)
	if from == StatusRejected || from == StatusRetired {
		allowed = append([]string{}, transitions[from]...)
		for _, s := range []string{StatusRejected, StatusRetired} {
			if s != from {
				allowed = append(allowed, s)
			}
		}
	}
	return "from " + from + ": " + strings.Join(allowed, ", ")
}

// SetStatus moves a requirement to a status.
func SetStatus(w *Workspace, id, status string, opts EditOptions) (EditResult, error) {
	return Edit(w, id, []EditOp{{Op: "set", Key: "status", Value: status}}, opts)
}

/* -------------------------------------------------------------------------- */
/*  ADRs                                                                      */
/* -------------------------------------------------------------------------- */

// ADRTemplate is the body of a new ADR when the repository has no 0000-template.md.
const ADRTemplate = `## Context

What is the issue that motivates this decision? Which requirements ask for it?

## Decision

What is being decided, stated so it can be checked.

## Consequences

What becomes easier or harder, and what must now be done.
`

var slugJunk = regexp.MustCompile(`[^a-z0-9]+`)

// Slug is a title as a file name.
func Slug(title string) string {
	slug := strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 60 {
		slug = strings.TrimRight(slug[:60], "-")
	}
	if slug == "" {
		slug = "decision"
	}
	return slug
}

// NewADR writes the next ADR of the focus repository.
func NewADR(w *Workspace, title string, requirements []string, status string) (string, string, error) {
	repo := w.Focus
	if strings.TrimSpace(title) == "" {
		return "", "", fmt.Errorf("an ADR needs --title")
	}
	if status == "" {
		status = "proposed"
	}
	if !contains(ADRStatuses, status) || status == "superseded" {
		return "", "", fmt.Errorf("a new ADR is proposed, accepted or rejected, not %q", status)
	}
	for _, id := range requirements {
		if w.Requirement(id) == nil {
			return "", "", fmt.Errorf("the requirement %s does not exist", id)
		}
	}
	dir := filepath.Join(repo.Dir, ADRDir)
	highest := 0
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if match := adrFileName.FindStringSubmatch(entry.Name()); match != nil {
			if number, _ := strconv.Atoi(match[1]); number > highest {
				highest = number
			}
		}
	}
	number := highest + 1
	id := fmt.Sprintf("ADR-%04d", number)
	body := ADRTemplate
	if template, err := os.ReadFile(filepath.Join(dir, "0000-template.md")); err == nil {
		lines := splitLines(string(template))
		if _, end, ok := frontMatter(lines); ok {
			body = strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n") + "\n"
		}
	}
	text := fmt.Sprintf("---\nid: %s\ntitle: %s\nstatus: %s\ndate: %s\nrequirements: %s\nsupersedes: []\nsuperseded_by: []\n---\n\n# %s\n\n%s",
		id, formatScalar(title), status, Today(), formatFlow(requirements), title, body)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%04d-%s.md", number, Slug(title)))
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", "", err
	}
	rel, _ := filepath.Rel(repo.Dir, path)
	return id, filepath.ToSlash(rel), nil
}

// SetADRStatus moves an ADR to proposed, accepted or rejected. Only its status
// line changes; superseded is what Supersede records, because it needs the
// successor named in both front matters.
func SetADRStatus(w *Workspace, ref, status string) (*ADR, error) {
	adr := w.ADR(w.Focus.Name, ref)
	if adr == nil {
		return nil, fmt.Errorf("the ADR %s does not exist", ref)
	}
	if status == "superseded" {
		return nil, fmt.Errorf("an ADR is superseded with `rho req adr supersede OLD NEW`, which names its successor")
	}
	if !contains(ADRStatuses, status) {
		return nil, fmt.Errorf("an ADR's status is proposed, accepted or rejected, not %q", status)
	}
	if adr.Status == status {
		return nil, fmt.Errorf("%s is already %s", adr.Ref(), status)
	}
	if adr.Status == "superseded" {
		return nil, fmt.Errorf("%s is superseded by %s; a superseded decision stays superseded", adr.Ref(), strings.Join(adr.SupersededBy, ", "))
	}
	path := filepath.Join(w.Repo(adr.Repo).Dir, filepath.FromSlash(adr.File))
	doc, err := readDoc(path, true)
	if err != nil {
		return nil, err
	}
	if err := doc.Set(RootMap, "status", status, ADRKeys); err != nil {
		return nil, fmt.Errorf("%s: %v", adr.File, err)
	}
	if err := writeDoc(path, doc); err != nil {
		return nil, err
	}
	adr.Status = status
	return adr, nil
}

// Supersede records that one ADR replaces another, in both front matters.
func Supersede(w *Workspace, oldRef, newRef string) error {
	from := w.Focus.Name
	old, replacement := w.ADR(from, oldRef), w.ADR(from, newRef)
	if old == nil {
		return fmt.Errorf("the ADR %s does not exist", oldRef)
	}
	if replacement == nil {
		return fmt.Errorf("the ADR %s does not exist", newRef)
	}
	if old == replacement {
		return fmt.Errorf("an ADR does not supersede itself")
	}
	edit := func(adr *ADR, change func(doc *Doc) error) error {
		path := filepath.Join(w.Repo(adr.Repo).Dir, filepath.FromSlash(adr.File))
		doc, err := readDoc(path, true)
		if err != nil {
			return err
		}
		if err := change(doc); err != nil {
			return fmt.Errorf("%s: %v", adr.File, err)
		}
		return writeDoc(path, doc)
	}
	if err := edit(old, func(doc *Doc) error {
		if err := doc.Set(RootMap, "status", "superseded", ADRKeys); err != nil {
			return err
		}
		return doc.Add(RootMap, "superseded_by", RefFrom(old.Repo, replacement), ADRKeys)
	}); err != nil {
		return err
	}
	return edit(replacement, func(doc *Doc) error {
		return doc.Add(RootMap, "supersedes", RefFrom(replacement.Repo, old), ADRKeys)
	})
}

/* -------------------------------------------------------------------------- */
/*  init                                                                      */
/* -------------------------------------------------------------------------- */

// InitRequest is what `rho req init` was given.
type InitRequest struct {
	Dir           string
	Level, Prefix string
	Parent        string
	// Root makes this repository the one that defines the levels and the
	// maintainers.
	Root bool
}

// ADRTemplateFile is the template `init` writes.
const ADRTemplateFile = `---
id: ADR-0000
title: Template
status: proposed          # proposed | accepted | superseded | rejected
date: 2026-01-01
requirements: []          # the requirement ids this decision is about
supersedes: []            # ADR ids, ` + "`<repo>:ADR-NNNN`" + ` for another repository's
superseded_by: []
---

# Template

Copy this file, or run ` + "`rho req adr new --title \"...\"`" + `, which numbers the
next ADR and fills the header. A file numbered 0000 is never read as a decision.

` + ADRTemplate

// LevelsTemplate is the levels.yml `init --root` writes.
const LevelsTemplate = `# The requirement levels of this workspace, highest first. A requirement
# satisfies only requirements of a higher level. Every repository whose
# module.yml names this one as its requirements.parent uses these levels.
levels:
  - { name: SYS,  title: System }
  - { name: NODE, title: Node }
  - { name: SUB,  title: Subsystem }
  - { name: UNIT, title: Unit }
`

// MaintainersTemplate is the MAINTAINERS.yml `init --root` writes.
const MaintainersTemplate = `# Who may approve a requirement or accept an ADR in this workspace.
#
# ` + "`rho req check --base <ref>`" + ` refuses an approval committed by anyone not listed
# here. That is a guard, not an authority: commit authors can be faked. Make it
# real with branch protection and a CODEOWNERS entry for docs/requirements/ and
# docs/architecture/ that names these people.
maintainers:
  - name: Your Name
    github: your-login
    email: you@example.org
`

// Init scaffolds the requirement layout in a repository and returns what it
// created and what it found already there.
func Init(request InitRequest) (created, kept []string, err error) {
	dir, err := filepath.Abs(request.Dir)
	if err != nil {
		return nil, nil, err
	}
	write := func(rel, content string) error {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if fileExists(path) {
			kept = append(kept, rel)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		created = append(created, rel)
		return os.WriteFile(path, []byte(content), 0644)
	}
	if err := os.MkdirAll(filepath.Join(dir, RequirementsDir), 0755); err != nil {
		return nil, nil, err
	}
	levels := DefaultLevels
	if request.Root {
		if err := write(LevelsFile, LevelsTemplate); err != nil {
			return created, kept, err
		}
		if err := write(MaintainersFile, MaintainersTemplate); err != nil {
			return created, kept, err
		}
	}
	if request.Level != "" {
		if LevelIndex(levels, request.Level) < 0 && fileExists(filepath.Join(dir, LevelsFile)) {
			// The repository's own levels decide.
			w := &Workspace{}
			levels = w.loadLevels(&Repo{Dir: dir}, filepath.Join(dir, LevelsFile))
		}
		prefix := request.Prefix
		if request.Level == levels[0].Name {
			prefix = levels[0].Name
		}
		if prefix == "" {
			return created, kept, fmt.Errorf("--level needs --prefix for the ids of that file")
		}
		rel := RequirementsDir + "/" + strings.ToLower(request.Level+"-"+prefix) + ".yml"
		if err := write(rel, FileHeader(levels, request.Level, prefix, "")); err != nil {
			return created, kept, err
		}
	}
	if err := write(ADRDir+"/0000-template.md", ADRTemplateFile); err != nil {
		return created, kept, err
	}
	manifest := filepath.Join(dir, ManifestFile)
	if data, err := os.ReadFile(manifest); err == nil && (request.Prefix != "" || request.Parent != "") {
		if block, _ := readManifestBlock(dir); block.declared {
			kept = append(kept, ManifestFile+" (requirements:)")
		} else {
			text := string(data)
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			text += "\n# Requirements: the prefix of the ids this repository allocates, and the\n# repository that holds levels.yml and the system level (rho req).\nrequirements:\n"
			if request.Prefix != "" {
				text += "  prefix: " + request.Prefix + "\n"
			}
			if request.Parent != "" {
				text += "  parent: " + request.Parent + "\n"
			}
			if err := os.WriteFile(manifest, []byte(text), 0644); err != nil {
				return created, kept, err
			}
			created = append(created, ManifestFile+" (requirements:)")
		}
	}
	sort.Strings(kept)
	return created, kept, nil
}
