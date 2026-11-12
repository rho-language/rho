//
// @file source.go
// @brief A new Rho source file, with the header its module already knows.
//
// A source file starts with who it belongs to: the module name it declares, a
// line about what it is for, its author, the date it was started and its
// licence. All of that but the line is already known — the module's import
// name and the file's place under it give the module name, the manifest gives
// the maintainer and the licence, and the clock gives the date — so `rho new`
// writes it rather than leaving a person to copy it from the file beside.
//
// @project Rho Programming Language
// @author Aurel Miron
// @date 2026-11-12
//
// SPDX-License-Identifier: Apache-2.0
//

package scaffold

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rho/src/yaml"
)

// SourceRequest is what a new source file is made from.
type SourceRequest struct {
	// Path is the file to write, ending in .rho.
	Path string
	// Description is the line about what the file is for.
	Description string
	// Author overrides the module's maintainer.
	Author string
	// Date overrides today, as YYYY-MM-DD.
	Date string
	// Force replaces a file that is already there.
	Force bool
}

// SourceResult is what was written.
type SourceResult struct {
	Path   string
	Module string
	// Suite is the test suite a test file was added to, if any.
	Suite string
}

// NewSource writes a Rho source file with its header and a start: the module
// line, and for a file named *_test.rho the shape of a test suite, which is
// also listed in the module's tests/tests.yml when it has one.
func NewSource(req SourceRequest) (SourceResult, error) {
	path, err := filepath.Abs(req.Path)
	if err != nil {
		return SourceResult{}, err
	}
	if filepath.Ext(path) != ".rho" {
		return SourceResult{}, fmt.Errorf("%s: a Rho source file ends in .rho", req.Path)
	}
	if _, err := os.Stat(path); err == nil && !req.Force {
		return SourceResult{}, fmt.Errorf("%s is already there; --force replaces it", req.Path)
	}
	root := moduleRootOf(filepath.Dir(path))
	// A tests/ folder with a manifest of its own is how a module builds its
	// tests; the files in it belong to the module above.
	if root != "" && filepath.Base(root) == "tests" {
		if parent := moduleRootOf(filepath.Dir(root)); parent != "" {
			root = parent
		}
	}
	facts := readSourceFacts(root)
	module := sourceModuleName(root, path, facts.base)
	author := req.Author
	if author == "" {
		author = facts.author
	}
	if author == "" {
		author = gitUser(filepath.Dir(path))
	}
	date := req.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	license := facts.license
	if license == "" {
		license = "NOASSERTION"
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = "What this file is for, in one line."
	}
	test := strings.HasSuffix(filepath.Base(path), "_test.rho")

	var b strings.Builder
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// @file %s\n", filepath.Base(path))
	for i, line := range wrapWords(description, 70) {
		if i == 0 {
			fmt.Fprintf(&b, "// @brief %s\n", line)
		} else {
			fmt.Fprintf(&b, "//        %s\n", line)
		}
	}
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// @module %s\n", module)
	if facts.project != "" {
		fmt.Fprintf(&b, "// @project %s\n", facts.project)
	}
	if author != "" {
		fmt.Fprintf(&b, "// @author %s\n", author)
	}
	fmt.Fprintf(&b, "// @date %s\n", date)
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// SPDX-License-Identifier: %s\n", license)
	b.WriteString("//\n\n")
	fmt.Fprintf(&b, "module %s\n", module)
	if test {
		b.WriteString(testSkeleton)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SourceResult{}, err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return SourceResult{}, err
	}
	result := SourceResult{Path: path, Module: module}
	if test && root != "" {
		result.Suite = listTestSuite(root, path)
	}
	return result, nil
}

const testSkeleton = `
// The functions under test are included rather than imported, so a test sees
// the module's private names too:
//
//   include "../src/main.rho"

extern fn abort()

fn check(condition: bool, what: string) void {
    if !condition {
        print("FAIL: " + what + "\n")
        abort()
    }
}

test fn it_does_what_it_says() void {
    check(true, "say what is being checked")
    print("[PASS] it_does_what_it_says\n")
}

fn main() void {
    it_does_what_it_says()
}
`

// sourceFacts are what a module's manifest says about the files it holds.
type sourceFacts struct {
	base    string
	project string
	author  string
	license string
}

func readSourceFacts(root string) sourceFacts {
	if root == "" {
		return sourceFacts{}
	}
	data, err := os.ReadFile(filepath.Join(root, "module.yml"))
	if err != nil {
		return sourceFacts{}
	}
	doc, err := yaml.Load(string(data))
	if err != nil {
		return sourceFacts{}
	}
	module := doc.Map["module"]
	facts := sourceFacts{
		project: unquoteScalar(module.Map["name"].Str),
		author:  unquoteScalar(module.Map["maintainer"].Str),
		license: unquoteScalar(module.Map["license"].Str),
	}
	if facts.author == "" {
		facts.author = unquoteScalar(module.Map["author"].Str)
	}
	// The name code imports the module by is the root of its files' names.
	for _, candidate := range []string{
		doc.Map["provides"].Map["import"].Str,
		module.Map["import_path"].Str,
		facts.project,
	} {
		if name := unquoteScalar(candidate); name != "" {
			facts.base = name
			break
		}
	}
	return facts
}

func unquoteScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}

// moduleRootOf is the nearest folder at or above dir with a module.yml.
func moduleRootOf(dir string) string {
	for current := dir; ; {
		if _, err := os.Stat(filepath.Join(current, "module.yml")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// sourceModuleName is the dotted name a file declares: the module's import
// name, then the file's folders and name under src/ or tests/. A folder that
// repeats the name's last part is not repeated (src/mesh/fleet.rho in mesh is
// mesh.fleet), and main.rho is the module itself.
func sourceModuleName(root, path, base string) string {
	segments := []string{}
	if base != "" {
		for _, part := range strings.Split(base, ".") {
			segments = append(segments, identifier(part))
		}
	}
	rel := filepath.Base(path)
	if root != "" {
		if r, err := filepath.Rel(root, path); err == nil {
			rel = r
		}
	}
	parts := strings.Split(filepath.ToSlash(strings.TrimSuffix(rel, ".rho")), "/")
	if len(parts) > 1 && (parts[0] == "src" || parts[0] == "tests") {
		parts = parts[1:]
	}
	if len(parts) > 0 && len(segments) > 0 && identifier(parts[0]) == segments[len(segments)-1] {
		parts = parts[1:]
	}
	if len(parts) == 1 && parts[0] == "main" {
		parts = nil
	}
	for _, part := range parts {
		segments = append(segments, identifier(part))
	}
	if len(segments) == 0 {
		return "main"
	}
	return strings.Join(segments, ".")
}

// identifier is one part of a module name: lower case letters, digits and
// underscores.
func identifier(part string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(part) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '.':
			b.WriteRune('_')
		}
	}
	return b.String()
}

func gitUser(dir string) string {
	command := exec.Command("git", "config", "user.name")
	command.Dir = dir
	out, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func wrapWords(text string, width int) []string {
	lines := []string{}
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// listTestSuite adds a test file to its module's tests/tests.yml, when the
// module has one that does not list it yet, and answers the suite's name.
func listTestSuite(root, path string) string {
	manifest := filepath.Join(root, "tests", "tests.yml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	name := filepath.ToSlash(rel)
	text := string(data)
	if strings.Contains(text, "name: "+name+"\n") {
		return ""
	}
	if !strings.Contains(text, "\nsuites:") && !strings.HasPrefix(text, "suites:") {
		return ""
	}
	entry := "  - name: " + name + "\n    kind: rho\n    tags: [unit, rho]\n"
	at := strings.Index(text, "suites:\n")
	if at < 0 {
		return ""
	}
	insert := at + len("suites:\n")
	text = text[:insert] + entry + text[insert:]
	if err := os.WriteFile(manifest, []byte(text), 0o644); err != nil {
		return ""
	}
	return name
}
