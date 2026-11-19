//
// @file tags.go
// @brief The `@req` and `@verifies` tags in code and tests.
//
// A tag is a comment line that begins with the tag:
//
//	// @req GW-0012
//	// @verifies GW-0012, GW-0013
//	# @verifies SYS-0007        (Python, shell, a YAML test manifest)
//
// The tag is read by line rather than through each language's parser, so one
// scanner covers Rho, C, C++, Go, Python, shell and the YAML test manifests
// alike. The Rho parser's doc-comment reader (`doctags.go`) is attached to
// declarations of a parsed module, which a C file or a smoke script is not, and
// a tag above a `test fn` reads the same either way.
//
// What the tag sits above is the next line that is neither blank nor a comment;
// the symbol is read from that line when it is a declaration this scanner knows.
// A `@verifies` in a Python or shell script verifies through the suite that runs
// the script, found by its path in the repository's tests.yml.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tag kinds.
const (
	TagReq      = "req"
	TagVerifies = "verifies"
)

// Tag is one `@req` or `@verifies` line.
type Tag struct {
	Kind   string   `json:"kind"`
	IDs    []string `json:"ids"`
	Repo   string   `json:"repo"`
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Symbol string   `json:"symbol,omitempty"`
	// Test is the test a `@verifies` stands for: a case name, or for a script
	// the name of the suite that runs it.
	Test *TestRef `json:"test,omitempty"`
}

// TestRef names a test as a report names it.
type TestRef struct {
	// Kind is "case" (a test function), "suite" (a suite in tests.yml), or
	// "script" (a script no suite runs).
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// skippedDirs are never scanned.
var skippedDirs = map[string]bool{"build": true, "vendor": true, ".git": true, "node_modules": true, "testdata": true}

var scannedExt = map[string]string{
	".rho": "rho", ".c": "c", ".h": "c", ".cpp": "c", ".cc": "c", ".hpp": "c", ".cxx": "c",
	".go": "go", ".py": "py", ".sh": "sh", ".yml": "yml", ".yaml": "yml",
}

// tagLine is a comment line that begins with a tag.
var tagLine = regexp.MustCompile(`^\s*(?://+|#+|/\*+|\*+|--)\s*@(req|verifies)\b:?(.*)$`)

var tokenID = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-[0-9]+$`)

// Tags is every tag in a repository, read once.
func (r *Repo) Tags() []*Tag {
	if r.tagsDone {
		return r.tags
	}
	r.tagsDone = true
	suites := readSuites(r.Dir)
	filepath.WalkDir(r.Dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != r.Dir && (skippedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			// A nested module is a repository of its own, and its tags are its.
			if path != r.Dir && declaresRequirements(path) {
				return filepath.SkipDir
			}
			return nil
		}
		lang, ok := scannedExt[filepath.Ext(path)]
		if !ok {
			return nil
		}
		rel, _ := filepath.Rel(r.Dir, path)
		rel = filepath.ToSlash(rel)
		if lang == "yml" && !isTestManifest(rel) {
			return nil
		}
		r.tags = append(r.tags, scanFile(r.Name, path, rel, lang, suites)...)
		return nil
	})
	sort.SliceStable(r.tags, func(a, b int) bool {
		if r.tags[a].File != r.tags[b].File {
			return r.tags[a].File < r.tags[b].File
		}
		return r.tags[a].Line < r.tags[b].Line
	})
	return r.tags
}

// isTestManifest reports whether a YAML file is one whose tags count: a suite
// manifest, or a YAML file under tests/.
func isTestManifest(rel string) bool {
	if strings.HasPrefix(rel, RequirementsDir+"/") {
		return false
	}
	base := filepath.Base(rel)
	return base == "tests.yml" || base == "tests.yaml" || strings.HasPrefix(rel, "tests/") || strings.Contains(rel, "/tests/")
}

func scanFile(repo, path, rel, lang string, suites []suite) []*Tag {
	handle, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer handle.Close()
	lines := []string{}
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	out := []*Tag{}
	for index, line := range lines {
		match := tagLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		tag := &Tag{Kind: match[1], Repo: repo, File: rel, Line: index + 1, IDs: tagIDs(match[2])}
		declaration := declarationAfter(lines, index+1)
		tag.Symbol = symbolOf(declaration, lang)
		if tag.Kind == TagVerifies {
			tag.Test = testRef(lang, rel, tag.Symbol, suites)
		}
		out = append(out, tag)
	}
	return out
}

// tagIDs are the ids at the start of a tag's text, up to the first word that is
// not one: `@req GW-0012, GW-0013 because ...` names two.
func tagIDs(text string) []string {
	text = strings.TrimSuffix(strings.TrimSpace(text), "*/")
	ids := []string{}
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if !tokenID.MatchString(word) {
			break
		}
		ids = append(ids, word)
	}
	return ids
}

func declarationAfter(lines []string, from int) string {
	for index := from; index < len(lines) && index < from+20; index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "@") {
			continue
		}
		return trimmed
	}
	return ""
}

var (
	rhoDecl   = regexp.MustCompile(`^(?:pub\s+)?(?:test\s+)?(?:fn|struct|enum|type|const|let)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	goDecl    = regexp.MustCompile(`^(?:func\s+(?:\([^)]*\)\s*)?|type\s+|var\s+|const\s+)([A-Za-z_][A-Za-z0-9_]*)`)
	pyDecl    = regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	shDecl    = regexp.MustCompile(`^(?:function\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)`)
	ymlDecl   = regexp.MustCompile(`^-?\s*name:\s*(?:"([^"]*)"|'((?:[^']|'')*)'|([^'"#\s][^#]*?))\s*(?:#.*)?$`)
	cTestDecl = regexp.MustCompile(`^(?:TEST(?:_F|_P)?)\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*,\s*([A-Za-z_][A-Za-z0-9_]*)`)
	cDecl     = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*$`)
)

// symbolOf is the name a declaration line declares, when it is one.
func symbolOf(line, lang string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	switch lang {
	case "rho":
		if m := rhoDecl.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	case "go":
		if m := goDecl.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	case "py":
		if m := pyDecl.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	case "sh":
		if m := shDecl.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	case "yml":
		// Quoted or not; an unquoted name may hold an apostrophe
		// ("the toolchain's schema").
		if m := ymlDecl.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1] + strings.ReplaceAll(m[2], "''", "'") + m[3])
		}
	case "c":
		if m := cTestDecl.FindStringSubmatch(line); m != nil {
			return m[1] + "." + m[2]
		}
		if strings.HasPrefix(line, "#") {
			return ""
		}
		if m := cDecl.FindStringSubmatch(line); m != nil && !contains([]string{"if", "while", "for", "switch", "return", "sizeof"}, m[1]) {
			return m[1]
		}
	}
	return ""
}

// testRef is the test a `@verifies` tag stands for.
func testRef(lang, rel, symbol string, suites []suite) *TestRef {
	switch lang {
	case "py", "sh":
		for _, s := range suites {
			if s.runs(rel) {
				return &TestRef{Kind: "suite", Name: s.name}
			}
		}
		return &TestRef{Kind: "script", Name: rel}
	case "yml":
		if symbol != "" {
			return &TestRef{Kind: "suite", Name: symbol}
		}
		return nil
	}
	if symbol == "" {
		return nil
	}
	return &TestRef{Kind: "case", Name: symbol}
}

// suite is a suite a tests.yml declares, as far as a tag needs it.
type suite struct {
	name, command string
}

func (s suite) runs(rel string) bool {
	if s.command == "" {
		return false
	}
	for _, word := range strings.Fields(s.command) {
		word = strings.Trim(word, `"'`)
		if word == rel || word == "./"+rel || strings.HasSuffix(word, "/"+rel) {
			return true
		}
	}
	return false
}

// readSuites reads tests/tests.yml or tests.yml.
func readSuites(dir string) []suite {
	for _, candidate := range []string{"tests/tests.yml", "tests.yml"} {
		data, err := os.ReadFile(filepath.Join(dir, candidate))
		if err != nil {
			continue
		}
		root, err := ParseYAML(string(data))
		if err != nil {
			return nil
		}
		out := []suite{}
		if entry := root.Get("suites"); entry != nil && entry.Value.Kind == List {
			for _, item := range entry.Value.Items {
				if item.Kind == Map {
					// A command is written either as one string or as a list of
					// arguments (`[python3, tools/smoke.py]`); both name the script.
					command := item.Str("command")
					if command == "" {
						command = strings.Join(item.Strings("command"), " ")
					}
					out = append(out, suite{name: item.Str("name"), command: command})
				}
			}
		}
		return out
	}
	return nil
}
