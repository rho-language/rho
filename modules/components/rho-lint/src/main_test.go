//
// @file main_test.go
// @brief Tests for the command line: the flags, and the manifest it is given.
//
// The flags are read by hand because both spellings arrive — from an editor's launch
// configuration as much as from a person — and the arguments that are tested here are
// the ones where getting it wrong is expensive rather than annoying: a flag that
// consumed the next one, a repeated flag that kept only the last, and a manifest that
// does not parse being reported as a project that declares nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rho/rho-lint/src/rholint"
)

// writeFile is a file in a temporary tree, for a test that needs a project on disk.
func writeFile(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestParseAcceptsEitherSpelling(t *testing.T) {
	// Both spellings arrive, and one of them from a person's editor rather than from
	// a person. A parser that refused the second would fail on configurations that are
	// perfectly ordinary.
	spelled := [][]string{
		{"--engine", "cppcheck", "--format=sarif", "--fail-on", "warning"},
		{"-engine", "cppcheck", "-format", "sarif", "-fail-on", "warning"},
		{"--engine=cppcheck", "--format", "sarif", "--fail-on=warning"},
	}
	for _, args := range spelled {
		chosen, err := parse(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if len(chosen.engines) != 1 || chosen.engines[0] != "cppcheck" {
			t.Errorf("%v: engines %v", args, chosen.engines)
		}
		if chosen.format != "sarif" {
			t.Errorf("%v: format %q", args, chosen.format)
		}
		if chosen.failOn != "warning" {
			t.Errorf("%v: threshold %q", args, chosen.failOn)
		}
	}
}

func TestParseAccumulatesARepeatedFlag(t *testing.T) {
	// `--engine a --engine b` is two engines, and a parser that kept only the last
	// would silently run one. The same is true of `--rule` and `--exclude`, which is
	// why the shared reader appends rather than assigns.
	chosen, err := parse([]string{
		"--engine", "rho", "--engine", "c", "--engine", "clang-tidy",
		"--rule", "rho-parse", "--rule", "c-todo-comment",
		"--exclude", "vendor/", "--exclude", "build/generated/",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(chosen.engines) != 3 {
		t.Errorf("engines %v, want three", chosen.engines)
	}
	if len(chosen.rules) != 2 {
		t.Errorf("rules %v, want two", chosen.rules)
	}
	if len(chosen.exclude) != 2 {
		t.Errorf("exclusions %v, want two", chosen.exclude)
	}
}

func TestParseDoesNotSwallowTheNextFlag(t *testing.T) {
	// A flag with no value of its own must not read the next one as its value:
	// `--quiet --engine rho` would otherwise run `--engine` and nothing else.
	chosen, err := parse([]string{"--quiet", "--include-outside", "--engine", "rho"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !chosen.quiet || !chosen.includeOutside {
		t.Errorf("quiet=%v includeOutside=%v", chosen.quiet, chosen.includeOutside)
	}
	if len(chosen.engines) != 1 || chosen.engines[0] != "rho" {
		t.Errorf("engines %v, want rho", chosen.engines)
	}
}

func TestParseSaysWhichFlagHasNoValue(t *testing.T) {
	if _, err := parse([]string{"--engine"}); err == nil {
		t.Error("a flag with nothing after it was accepted")
	}
}

func TestParseRefusesWhatItDoesNotHave(t *testing.T) {
	// A misspelled flag that were accepted and ignored would be a run somebody
	// believes was narrowed when it was not.
	if _, err := parse([]string{"--engin", "cppcheck"}); err == nil {
		t.Error("an unknown flag was accepted")
	}
}

func TestParseTakesTheFirstBarePathAsTheProject(t *testing.T) {
	// `rho lint .` is what a person types and `rho lint` from inside a directory is
	// the other half of it, so one path is the project and a second is a mistake.
	chosen, err := parse([]string{".", "--engine", "rho"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if chosen.root != "." {
		t.Errorf("project %q", chosen.root)
	}
	if _, err := parse([]string{"a", "b"}); err == nil {
		t.Error("two bare paths were accepted")
	}
}

func TestHasFlagIgnoresFlagsThatCarryValues(t *testing.T) {
	// Scanned rather than read with the value reader, because a boolean that looks
	// for the argument after it finds the *next* flag and trusts the wrong thing.
	args := []string{"--tree-plugins", "--target", "pico"}
	if !hasFlag(args, "tree-plugins") {
		t.Error("the flag was not found")
	}
	if hasFlag([]string{"--target", "pico"}, "tree-plugins") {
		t.Error("a flag was reported that was not given")
	}
}

func TestNamedArgReadsEitherSpelling(t *testing.T) {
	for _, args := range [][]string{
		{"--target", "nucleo-h753zi"},
		{"-target", "nucleo-h753zi"},
		{"--target=nucleo-h753zi"},
	} {
		if got := namedArg(args, "-target"); got != "nucleo-h753zi" {
			t.Errorf("%v gave %q", args, got)
		}
	}
	if got := namedArg([]string{"--target", "pico", "--db", "x.json"}, "-db"); got != "x.json" {
		t.Errorf("the second flag gave %q", got)
	}
	if got := namedArg([]string{"--quiet"}, "-target"); got != "" {
		t.Errorf("a flag that was not given came back as %q", got)
	}
}

/* -------------------------------------------------------------------------- */

func TestLoadManifestTellsNoManifestFromABrokenOne(t *testing.T) {
	// The two failures kept apart, because one of them is not a failure. A directory
	// with no manifest is ordinary — a set of C sources and a directory — and a run
	// that refused it could only ever lint Rho projects.
	//
	// A manifest that is *there* and does not parse is different, and swallowing it
	// would make the run say "the project declares no sources", which looks exactly
	// like a project that has none and sends somebody to the wrong place.
	root := t.TempDir()
	manifest, err := loadManifest(root)
	if err != nil || manifest != nil {
		t.Errorf("a directory with no manifest gave %v / %v", manifest, err)
	}

	writeFile(t, filepath.Join(root, "module.yml"), "module:\n  name: broken\n  type: nonsense\n")
	if _, err := loadManifest(root); err == nil {
		t.Error("a manifest that does not parse was accepted as a project with none")
	}

	writeFile(t, filepath.Join(root, "module.yml"),
		"module:\n  name: fine\n  type: module\n  version: 0.1.0\nmain: src/main.c\n")
	manifest, err = loadManifest(root)
	if err != nil {
		t.Errorf("a manifest that parses was refused: %v", err)
	}
	if manifest == nil || manifest.ModuleName != "fine" {
		t.Errorf("read %v", manifest)
	}
}

/* -------------------------------------------------------------------------- */

func TestRunRefusesAnUnknownCommandRatherThanGuessing(t *testing.T) {
	// A command that is not one of the four belongs to the run — it is a project
	// directory, or a flag — because `rho lint .` is what a person types. What it may
	// not be is silently treated as a subcommand nobody implements.
	chosen, err := parse([]string{"/tmp/somewhere"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if chosen.root != "/tmp/somewhere" {
		t.Errorf("project %q, want the path", chosen.root)
	}
}

func TestKnownFailOnIsCheckedBeforeARunStarts(t *testing.T) {
	// `--fail-on eror` should be a usage error, not a run that quietly failed on
	// errors — the caller asked for something and got the opposite of it.
	if rholint.KnownFailOn("eror") {
		t.Error("a misspelling was accepted as a severity")
	}
	for _, good := range []string{"error", "note", "never"} {
		if !rholint.KnownFailOn(good) {
			t.Errorf("%q was refused", good)
		}
	}
}

func TestSentenceCapitalisesOneSummary(t *testing.T) {
	// A report that reads "No findings. the compilation database was derived" is
	// correct and reads as two halves of one line somebody wrote badly, which is a
	// poor first impression of the one line that says whether to trust the rest.
	if got := rholint.Sentence("the compilation database was derived"); got != "The compilation database was derived" {
		t.Errorf("sentence %q", got)
	}
	if got := rholint.Sentence("2 warnings"); got != "2 warnings" {
		t.Errorf("sentence %q, want a sentence with nothing to capitalise left alone", got)
	}
}
