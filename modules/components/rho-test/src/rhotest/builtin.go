//
// @file builtin.go
// @brief The kinds of suite this runner knows how to run.
//
// A kind is resolved by whoever claims it, and the runner claims two: a Rho test
// file, which it can translate and run itself, and a plain command, which is the
// escape hatch that lets a project run something of its own without anybody
// writing a plugin for it.
//
// Both are here so that everything else can be somebody else's job. A runner that
// could only run what it compiled had a floor of one language, and the way past
// that floor is a plugin, not another case in a switch.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The kinds this runner resolves itself.
const (
	// KindRho is a Rho test file, which this runner translates, compiles and runs.
	KindRho = "rho"
	// KindCommand is a command the project already knows how to run.
	//
	// The escape hatch, and the reason a plugin is not the only way to run
	// something: a project's own smoke script is a command, and making it a
	// plugin would be making a shell script a Go program.
	KindCommand = "command"
	// KindBuild is a project built for a target: `rho build` over a directory.
	//
	// Not a project-specific kind. That is the point of it — whether a board can
	// be built for is a question for the toolchain, which knows about target
	// specs, vendor trees and toolchains, and a test runner that answered it would
	// be carrying a copy of that knowledge that goes stale the moment the platform
	// changes. So this kind asks, and reports what it is told: built, did not build,
	// or cannot be tried here yet.
	KindBuild = "build"
)

// CannotRunHere is what a build exits with when it cannot be tried on this
// machine, as distinct from when the code does not build.
//
// The automake convention, and the same number cmake and ctest use, because it is
// the one every build system already agrees on. A suite that treats it as a failure
// would report every board whose toolchain is not installed as a broken platform,
// which is both wrong and the reason smoke suites usually get turned off.
const CannotRunHere = 77

// BuiltinResolvers are the kinds this runner knows how to run.
func BuiltinResolvers() []Resolver {
	return []Resolver{
		ResolverFunc{Kind: KindRho, Run: resolveRho},
		ResolverFunc{Kind: KindCommand, Run: resolveCommand},
		ResolverFunc{Kind: KindBuild, Run: resolveBuild},
	}
}

/* -------------------------------------------------------------------------- */
/*  A build                                                                    */
/* -------------------------------------------------------------------------- */

// resolveBuild turns a declared build suite into the step that performs it.
//
// The suite's steps are the build and nothing else: what a build needs is the
// build's own business, and the toolchain already reports two different outcomes
// — it did not compile, or it cannot be tried here — which is the only thing a
// smoke suite needs to tell apart.
func resolveBuild(declared Declared, root string) (Suite, error) {
	base := baseName(declared)
	directory, target := buildTargetFor(declared, root)

	command := []string{rhoBinary(), "build"}
	if target != "" {
		command = append(command, "-target", target)
	}
	command = append(command, directory)

	return Suite{
		Name:    declared.Name,
		Kind:    KindBuild,
		Verdict: VerdictStatus,
		Tags:    declared.Tags,
		Root:    root,
		Steps: []Step{{
			Name:    "build " + base,
			Command: command,
			// The build's own exit code says which of the three outcomes this is, so
			// the step is given the whole of it rather than having the runner decide
			// from a message.
			Verdict: StepVerdictBuild,
		}},
	}, nil
}

// buildTargetFor reads the target and the directory a build suite is for.
//
// Both come from the axes the suite was expanded from, falling back to a `build:`
// mapping on the suite for one that was not. An axis is how a smoke matrix says
// what varies; a key is how a single suite says so. Which one is used is the
// suite's choice and both are read, so a project writes whichever fits.
func buildTargetFor(declared Declared, root string) (directory string, target string) {
	directory = "."
	if declared.Matrix != nil {
		for _, axis := range declared.Matrix.Axes {
			if value := AxisValue(declared.Name, axis); value != "" {
				switch axis {
				case "target", "board", "preset":
					target = value
				default:
					directory = value
				}
			}
		}
	}
	if raw, ok := declared.Options["build"]; ok {
		if given, ok := raw.(map[string]any); ok {
			if value, ok := given["target"].(string); ok && value != "" {
				target = value
			}
			if value, ok := given["board"].(string); ok && value != "" {
				target = value
			}
			if value, ok := given["project"].(string); ok && value != "" {
				directory = value
			}
		}
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(root, filepath.FromSlash(directory))
	}
	return directory, target
}

/* -------------------------------------------------------------------------- */
/*  A Rho test file                                                           */
/* -------------------------------------------------------------------------- */

// resolveRho turns a declared Rho suite into a suite this runner can take.
//
// One step rather than several, because the work is this runner's own rather
// than four programs somebody has to know the order of: strip the marker, build
// what is left, compile it, run it. What a failure means is kept apart inside —
// a file that will not compile is a suite that could not be tested, which is not
// the same as a test that failed, and the two are reported differently.
func resolveRho(declared Declared, root string) (Suite, error) {
	path, err := resolveSourcePath(declared, root, KindRho)
	if err != nil {
		return Suite{}, err
	}
	cases, err := LoadTests(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read %s: %w", path, err)
	}
	if len(cases) == 0 {
		// A file with no `test fn` in it compiles, runs, asserts nothing and passes.
		// A test file that declares no tests is a mistake worth naming, because the
		// alternative is a green run over an empty file.
		return Suite{}, fmt.Errorf("%s declares no tests: no %q in it", path, strings.TrimSpace(TestMarker))
	}
	return Suite{
		Name:  declared.Name,
		Kind:  KindRho,
		Tags:  declared.Tags,
		Root:  root,
		Cases: cases,
		Steps: []Step{
			{
				Name:   "build and run " + filepath.Base(path),
				Action: rhoTestAction(path),
			},
		},
	}, nil
}

// rhoTestAction builds one Rho test file and runs it.
//
// The compiled program is what reports the cases, and its exit status is the
// verdict: a test asserts by failing, so a run that reached the end produced
// output rather than a list, and the cases come from what it said.
func rhoTestAction(path string) func(Suite) (string, bool, error) {
	return func(suite Suite) (string, bool, error) {
		source, err := os.ReadFile(path)
		if err != nil {
			return "", false, fmt.Errorf("read test file: %w", err)
		}
		workdir := workDirFor(suite.Root)
		if err := os.MkdirAll(workdir, 0755); err != nil {
			return "", false, err
		}
		// Written beside the file it came from rather than copied into the build
		// directory, because the compiler resolves a program's imports relative to
		// the file, and a file moved out of its project finds nothing.
		stripped := filepath.Join(filepath.Dir(path), ".rho-test-compiled.rho")
		if err := os.WriteFile(stripped, []byte(StripTestMarker(string(source))), 0644); err != nil {
			return "", false, err
		}
		defer os.Remove(stripped)

		base := strings.TrimSuffix(filepath.Base(stripped), ".rho")
		cFile := filepath.Join(workdir, base+".c")
		binary := filepath.Join(workdir, base)
		defer os.Remove(cFile)
		defer os.Remove(binary)

		build := exec.Command(rhoBinary(), "build", "-target", DefaultTarget, "-o", cFile, stripped)
		if out, err := build.CombinedOutput(); err != nil {
			return string(out), false, fmt.Errorf("rho build: %w: %s", err, strings.TrimSpace(string(out)))
		}
		compile := exec.Command("cc", cFile, "-o", binary)
		if out, err := compile.CombinedOutput(); err != nil {
			return string(out), false, fmt.Errorf("cc: %w: %s", err, strings.TrimSpace(string(out)))
		}
		run := exec.Command(binary)
		out, err := run.CombinedOutput()
		return string(out), err != nil, nil
	}
}

func workDirFor(root string) string {
	return filepath.Join(root, "build", "test")
}

// resolveSourcePath finds the file a declared Rho suite names.
//
// A manifest names a suite rather than a file, and the convention that connects
// the two is the one discovery already uses: a suite named `math` is
// `math_test.rho`. So the bare name and the marker name are both tried, and a
// name that is already a path is taken as one.
//
// A suite that names no file is reported rather than skipped quietly. A suite for
// a test nobody wrote has not passed, and a run that cannot find its own tests
// should say which one it could not find.
func resolveSourcePath(declared Declared, root string, kind string) (string, error) {
	name := baseName(declared)
	if kind != KindRho {
		return name, nil
	}
	if filepath.IsAbs(name) {
		return name, nil
	}
	for _, candidate := range []string{name, name + TestFileSuffix} {
		path := filepath.Join(root, candidate)
		if fileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("no test file for %q: expected %s or %s",
		declared.Name, name+".rho", name+TestFileSuffix)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

/* -------------------------------------------------------------------------- */
/*  A command                                                                  */
/* -------------------------------------------------------------------------- */

// resolveCommand turns a declared command suite into a suite the runner takes.
//
// The project's own smoke script, a Makefile target, a ctest invocation: anything
// a project can run by hand from its root, this runner runs the same way, so a
// suite is a command a person could have typed.
//
// The command comes from the manifest when it says one, and from a script beside
// the manifest when it does not. Both, because the second is what a script-only
// project already has and the first is what a suite that is one line long should
// be able to say without a file.
func resolveCommand(declared Declared, root string) (Suite, error) {
	base := baseName(declared)
	command := declared.Command
	if len(command) == 0 {
		command = commandFor(root, base)
	}
	if len(command) == 0 {
		return Suite{}, fmt.Errorf("suite %q declares no command, and there is no script for it in tests/", declared.Name)
	}
	command = fillAxes(command, declared)
	return Suite{
		Name:    declared.Name,
		Kind:    KindCommand,
		Verdict: VerdictStatus,
		Tags:    declared.Tags,
		Root:    root,
		Steps: []Step{
			{
				Name:    "run " + base,
				Command: command,
			},
		},
	}, nil
}

// fillAxes substitutes a cell's own values into its command.
//
// A suite declared over a matrix is one command run many times, and what differs
// between the runs is exactly what the axes hold. So a command may say
// `{board}` and get the board the cell was expanded for — which is what lets one
// command be the whole answer for a smoke matrix instead of a generated command
// per cell.
//
// A placeholder for an axis the suite was not expanded for is an error rather than
// left in the command: a smoke suite that ran with a literal `{board}` on its
// command line would build for a board that does not exist and say so in a way
// nobody would read.
func fillAxes(command []string, declared Declared) []string {
	axes := declared.MatrixAxes()
	if len(axes) == 0 {
		return command
	}
	filled := make([]string, 0, len(command))
	for _, word := range command {
		word = axisPattern.ReplaceAllStringFunc(word, func(match string) string {
			axis := strings.Trim(match, "{}")
			value := AxisValue(declared.Name, axis)
			if value == "" {
				// Left as written rather than silently emptied: a command with a
				// placeholder that could not be filled is a manifest that is wrong,
				// and it should fail as one rather than as a build for no board.
				return match
			}
			return value
		})
		filled = append(filled, word)
	}
	return filled
}

// axisPattern is a placeholder in a command, as `{axis}`.
var axisPattern = regexp.MustCompile(`\{[a-zA-Z][a-zA-Z0-9_-]*\}`)

// baseName is a suite's own name, without the axes it was expanded from: the
// part that names the thing rather than the cell of the matrix.
func baseName(declared Declared) string {
	if declared.Matrix == nil {
		return declared.Name
	}
	if index := strings.Index(declared.Name, AxisSeparator); index > 0 {
		return declared.Name[:index]
	}
	return declared.Name
}

// commandFor reads the command a declared command suite runs.
//
// Named after the suite, and read from the suite's own directory if the project
// keeps one, so a project can put a command beside the manifest that says what it
// is rather than encoding an argv in YAML where a reader cannot check it.
func commandFor(root, name string) []string {
	script := filepath.Join(root, "tests", name)
	if fileExists(script) {
		return []string{script}
	}
	if fileExists(name) {
		return []string{name}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  The binary this runner builds with                                          */
/* -------------------------------------------------------------------------- */

// DefaultRhoBinary is the rho a test is built with unless one is named.
const DefaultRhoBinary = "rho"

// RhoBinaryEnv names the rho to build with, so a run against a toolchain in
// progress uses that toolchain rather than the one on PATH.
const RhoBinaryEnv = "RHO_BIN"

func rhoBinary() string {
	if fromEnv := os.Getenv(RhoBinaryEnv); fromEnv != "" {
		return fromEnv
	}
	return DefaultRhoBinary
}

// LookPath reports whether a program a suite needs is on PATH, which is how a
// suite is skipped with a reason rather than failing for want of a toolchain.
func LookPath(program string) (string, bool) {
	path, err := exec.LookPath(program)
	if err != nil {
		return "", false
	}
	return path, true
}
