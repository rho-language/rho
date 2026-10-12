//
// @file suites.go
// @brief Turning a declared gtest suite into steps that run it.
//
// A project states how each of its suites is built and where its binary is; this
// turns that into the steps a run takes, and knows nothing else about how tests
// are built. That is the whole design: a plugin that assumed CMake would be a
// plugin that could only run the projects it happened to suit, and a project whose
// tests build with Bazel would be a project with no way to say so.
//
// Three steps, in the order a suite is actually run:
//
//	configure, build  the steps the manifest declared, if it declared any
//	run               the binary, told to write a report
//	report            this plugin, reading that report and saying what it says
//
// The third step is what makes the second one useful. GoogleTest writes XML
// because it can, and that XML is the only place it states its results
// unambiguously; a run that parsed the console instead would be reading a format
// meant for people. So the run step produces a file and this plugin reads it,
// which means the runner sees one line per test in the form it already
// understands rather than a format it would have to know about.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package gtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// settings is a suite's `gtest:` mapping as this plugin reads it.
type settings struct {
	// Binary is the test executable, relative to the project root. Required: a
	// plugin that guessed where a project's test binary was would be a plugin that
	// ran somebody else's tests.
	Binary string `json:"binary"`
	// Configure and Build are how to produce the binary, if the project wants this
	// plugin to do it. Left out, the binary is taken to be there already — which is
	// what a person who ran `cmake --build` themselves wants.
	Configure command `json:"configure"`
	Build     command `json:"build"`
	// Filter narrows the run to tests whose name contains it, passed to the binary
	// as `--gtest_filter`. There for a developer running one case, not for a suite
	// that is meant to be a whole one.
	Filter string `json:"filter"`
	// XML is where the report is written, relative to the project root. It goes
	// under the project's build directory when left out, because it is an output.
	XML string `json:"xml"`
	// Args are extra arguments for the binary.
	Args command `json:"args"`
}

// command is a command line, written either as a list of words or as one string.
//
// Both, because a manifest is read by people and both of these are what a person
// writes. Deciding it here rather than at each call site is what keeps a setting
// from quietly failing to read: a command written as a string is not a list, and
// decoding it as one is an error whose message says nothing about what was meant.
type command []string

// UnmarshalJSON accepts either spelling.
func (c *command) UnmarshalJSON(data []byte) error {
	var words []string
	if err := json.Unmarshal(data, &words); err == nil {
		*c = command(words)
		return nil
	}
	var one string
	if err := json.Unmarshal(data, &one); err != nil {
		return fmt.Errorf("a command is a list of words or a single line, not %s", string(data))
	}
	*c = strings.Fields(one)
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                 */
/* -------------------------------------------------------------------------- */

// Resolve turns a declared gtest suite into the steps that run it.
//
// A suite that says nothing about its binary is skipped rather than guessed at,
// and says so. A smoke test that quietly tested nothing is the outcome this
// exists to prevent, and so is a test suite that reports itself as present while
// running no tests at all.
func Resolve(declared DeclaredSuite, root string) ResolvedSuite {
	resolved := ResolvedSuite{Declared: declared.Name, Name: declared.Name}

	var given settings
	if raw, ok := declared.Options["gtest"]; ok {
		if err := decodeSettings(raw, &given); err != nil {
			resolved.Skip = fmt.Sprintf("%s: %v", declared.Name, err)
			return resolved
		}
	}
	if given.Binary == "" {
		resolved.Skip = fmt.Sprintf("%s declares no `gtest.binary`: a plugin that guessed where a "+
			"project's test binary was would be a plugin that ran somebody else's tests", declared.Name)
		return resolved
	}

	// Where GoogleTest lives, and what the build is told about it.
	framework := googletestPath(root)
	frameworkEnv := []string{"GTEST_ROOT=" + framework}

	binary := filepath.Join(root, filepath.FromSlash(given.Binary))
	reportPath := reportPathFor(root, declared.Name, given.XML)

	// The steps the project declared to produce the binary, in the order a build
	// needs them, and each named for what it is rather than for what it was called.
	// The fetch, first, and only when the project says how to build the suite: a
	// suite whose binary is already built does not need a framework fetched to run
	// it.
	if len(given.Configure) > 0 || len(given.Build) > 0 {
		resolved.Steps = append(resolved.Steps, fetchStep(root, googletestPath(root)))
	}

	if len(given.Configure) > 0 {
		resolved.Steps = append(resolved.Steps, Step{
			Name:    "configure " + filepath.Base(filepath.Dir(binary)),
			Dir:     root,
			Command: given.Configure,
			Env:     frameworkEnv,
		})
	}
	if len(given.Build) > 0 {
		resolved.Steps = append(resolved.Steps, Step{
			Name:    "build " + filepath.Base(binary),
			Dir:     root,
			Command: given.Build,
			Env:     frameworkEnv,
		})
	}

	// GoogleTest, fetched if it is not there.
	//
	// Part of this plugin's contract rather than a thing a project fetches itself:
	// a project that declares a GoogleTest suite has said it wants GoogleTest, and
	// a test runner that needs a manual `git clone` before it will run a test is a
	// step between the declaration and the result. It goes where the project's own
	// vendor tree is, and the configure step is told where that is, so the build
	// finds it without being told twice.

	run := append([]string{binary}, given.Args...)
	run = append(run, "--gtest_output=xml:"+reportPath)
	if given.Filter != "" {
		run = append(run, "--gtest_filter="+given.Filter)
	}
	resolved.Steps = append(resolved.Steps, Step{
		Name:    "run " + filepath.Base(binary),
		Dir:     root,
		Command: run,
	})

	// The report step is this plugin again, asked to read what the run wrote. It is
	// a separate step rather than part of the run command so that a run that crashed
	// — which writes no report — is distinguishable from one that ran and failed,
	// and so that the report a reader reads was produced by a program whose only job
	// was to produce it.
	resolved.Steps = append(resolved.Steps, Step{
		Name:     "read the google test report",
		Dir:      root,
		Command:  []string{selfPath(), "--report", reportPath},
		Produces: reportPath,
	})
	return resolved
}

// reportPathFor is where a suite's report goes.
//
// Under the project's build directory when the manifest did not say, because a
// report is an output and an output belongs with the other ones — and because a
// report written beside the sources is a file a later build does not know to clean.
func reportPathFor(root, suite, given string) string {
	if given != "" {
		return filepath.Join(root, filepath.FromSlash(given))
	}
	return filepath.Join(root, "build", "test-reports", suite+".gtest.xml")
}

// selfPath is this plugin's own executable, so the report step can call back into
// it.
//
// Resolved from the process rather than written down, because the path a plugin is
// installed at is not something the manifest that describes the tests should have
// to know.
func selfPath() string {
	if self, err := executablePath(); err == nil {
		return self
	}
	return "rho-test-gtest"
}

// ResolveAll turns every declared suite of a kind this plugin knows into steps,
// and leaves every other kind alone.
//
// A run may have several plugins asked about the same declarations at once, so
// this one answers only for its own kinds rather than for whatever it can make
// steps for.
func ResolveAll(root string, suites []DeclaredSuite) []ResolvedSuite {
	out := []ResolvedSuite{}
	for _, declared := range suites {
		if !claims(declared.Kind) {
			continue
		}
		out = append(out, Resolve(declared, root))
	}
	return out
}

func claims(kind string) bool {
	return strings.EqualFold(kind, KindGTest)
}

// decodeSettings reads a suite's `gtest:` mapping.
//
// The mapping arrives from the runner as whatever the manifest held, decoded from
// YAML and marshalled again to reach here, so a command written as a list is a
// list of strings and one written as a string is a single string. Both are read,
// because both are what a person writes.
func decodeSettings(raw any, into *settings) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("the `gtest:` settings are not readable: %w", err)
	}
	if err := json.Unmarshal(encoded, into); err != nil {
		return fmt.Errorf("the `gtest:` settings do not fit this plugin: %w", err)
	}
	return nil
}

// googletestRepository is where GoogleTest is fetched from. Named here because a
// plugin that fetched a framework by a URL a project had to keep in step with it
// would be a plugin whose fetch and whose declaration disagree the first time the
// framework moved.
const googletestRepository = "https://github.com/google/googletest"

// fetchStep retrieves GoogleTest if it is not already there.
//
// Not a command anyone has to run before a test: a project that declares a
// GoogleTest suite has said it wants GoogleTest, and a runner that needs a manual
// clone first is a step between the declaration and the result. It is a step
// rather than something done during resolution because resolution must not have
// side effects — a `--list` should not fetch anything — and because a fetch is
// exactly the kind of thing that belongs in the run, where its output is reported.
func fetchStep(root, framework string) Step {
	// Already there: a step that runs nothing, so the framework is fetched once
	// and a second run is not a second clone.
	if info, err := os.Stat(framework); err == nil && info.IsDir() {
		return Step{Name: "GoogleTest is present"}
	}
	// `git` is run directly, not through a shell. A shell here was a shell
	// around a path the project chose, and a path with a quote in it was a path
	// that ran something else.
	return Step{
		Name: "fetch GoogleTest",
		Dir:  root,
		Command: []string{
			"git", "clone", "--depth", "1", "--quiet",
			"--", googletestRepository, framework,
		},
	}
}

// googletestPath is where GoogleTest is fetched to: the project's own vendor tree,
// which is where everything else a project builds against is put and where the
// build already looks for it.
func googletestPath(root string) string {
	return filepath.Join(root, "vendor", "googletest")
}
