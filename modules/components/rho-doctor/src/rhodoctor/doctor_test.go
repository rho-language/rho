//
// @file doctor_test.go
// @brief Tests for the checks, against a machine that is made up.
//
// Every dependency this has is injected — the path, the environment, the filesystem,
// what a program says about itself — because the alternative is a test that passes
// on the machine that wrote it and fails on every other one. A check that finds a
// program must be a check that can be shown *not* to find one.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoctor

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"rho/src/sysdep"
)

// machine is a fabricated machine, and the way every check is pointed at it.
type machine struct {
	// programs are the ones on the path.
	programs map[string]string
	// fetched are the ones fetched into the Rho home instead, which is how a program
	// is installed without being on the path at all.
	fetched map[string]string
	// environment is what the variables are set to.
	environment map[string]string
	// files are the host's files that exist.
	files map[string]bool
	// versions is what a program's version flag answered, for the programs whose
	// output is not simply their path.
	versions map[string]string
	// failing are the programs whose version flag will not answer.
	failing map[string]bool
}

func newMachine() *machine {
	return &machine{
		programs:    map[string]string{},
		fetched:     map[string]string{},
		environment: map[string]string{},
		files:       map[string]bool{},
		versions:    map[string]string{},
		failing:     map[string]bool{},
	}
}

func (m machine) options() Options {
	return Options{
		LookPath: func(name string) (string, error) {
			if path, found := m.programs[name]; found {
				return path, nil
			}
			return "", os.ErrNotExist
		},
		FetchedBinary: func(dep sysdep.ExecutableDep) (string, bool) {
			if path, found := m.fetched[dep.Name]; found {
				return path, true
			}
			return "", false
		},
		Environment: m.environment,
		ReadFile:    func(string) ([]byte, error) { return nil, os.ErrNotExist },
		Stat: func(path string) (os.FileInfo, error) {
			if m.files[path] {
				return fileInfo{path: path}, nil
			}
			return nil, os.ErrNotExist
		},
		RunVersion: func(path string, flag string) (string, error) {
			name := path[strings.LastIndex(path, "/")+1:]
			if m.failing[name] {
				return "", &runError{"it would not answer"}
			}
			if output, found := m.versions[name]; found {
				return output, nil
			}
			return m.programs[name], nil
		},
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
	}
}

// fileInfo is the little of os.FileInfo a check asks for, so a test need not build
// one.
type fileInfo struct {
	fs.FileInfo
	path string
}

func (f fileInfo) Name() string      { return f.path }
func (f fileInfo) IsDir() bool       { return false }
func (f fileInfo) Size() int64       { return 0 }
func (f fileInfo) Mode() os.FileMode { return 0o644 }

type runError struct{ why string }

func (e *runError) Error() string { return e.why }

// check runs one requirement through a machine and answers the one result.
func (m machine) check(t *testing.T, dep sysdep.ExecutableDep) sysdep.CheckResult {
	t.Helper()
	return checkExecutable("module", dep, m.options())
}

/* -------------------------------------------------------------------------- */
/*  Programs                                                                   */
/* -------------------------------------------------------------------------- */

// The whole point of the block: a program that is needed and absent says how to get
// it, because "not installed" on its own is a fact with no consequence attached.
func TestARequiredProgramThatIsNotThereSaysHowToGetIt(t *testing.T) {
	machine := newMachine()
	result := machine.check(t, sysdep.ExecutableDep{
		Name:     "openocd",
		Version:  ">=0.11.0",
		Required: true,
		Install:  map[string]string{"apt": "openocd", "pacman": "openocd"},
	})
	if result.Status != sysdep.StatusMissing {
		t.Fatalf("expected MISSING, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "openocd") {
		t.Errorf("expected the program to be named, got %q", result.Message)
	}
	if result.Install["apt"] != "openocd" {
		t.Errorf("expected the recipe to travel with the result, got %v", result.Install)
	}
}

// A program with no version wanted is satisfied by being there, and nothing is run
// to find that out: a requirement with no constraint must not depend on a version
// flag that hangs.
func TestAProgramWithNoVersionWantedIsNotAskedForOne(t *testing.T) {
	machine := newMachine()
	machine.programs["openocd"] = "/usr/bin/openocd"
	machine.failing["openocd"] = true
	result := machine.check(t, sysdep.ExecutableDep{Name: "openocd", Required: true})
	if result.Status != sysdep.StatusOK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
}

func TestAVersionThatDoesNotSatisfyTheRequirementIsOutdated(t *testing.T) {
	machine := newMachine()
	machine.programs["openocd"] = "/usr/bin/openocd"
	machine.versions["openocd"] = "Open On-Chip Debugger 0.10.0"
	result := machine.check(t, sysdep.ExecutableDep{
		Name: "openocd", Version: ">=0.11.0", Required: true,
	})
	if result.Status != sysdep.StatusOutdated {
		t.Fatalf("expected OUTDATED, got %s: %s", result.Status, result.Message)
	}
	if result.FoundVersion != "0.10.0" {
		t.Errorf("expected the version found, got %q", result.FoundVersion)
	}
	if !strings.Contains(result.Message, ">=0.11.0") {
		t.Errorf("expected the requirement in the message, got %q", result.Message)
	}

	machine.versions["openocd"] = "Open On-Chip Debugger 0.11.0"
	result = machine.check(t, sysdep.ExecutableDep{Name: "openocd", Version: ">=0.11.0", Required: true})
	if result.Status != sysdep.StatusOK || result.FoundVersion != "0.11.0" {
		t.Errorf("expected OK at 0.11.0, got %s: %s", result.Status, result.Message)
	}
}

// A program that will not say what version it is is UNKNOWN, never MISSING: it is
// installed, and reporting it as absent sends somebody to install a second copy of
// something they already have.
func TestAProgramThatWillNotSayItsVersionIsUnknownNotMissing(t *testing.T) {
	machine := newMachine()
	machine.programs["gdb"] = "/usr/bin/gdb"
	machine.failing["gdb"] = true
	result := machine.check(t, sysdep.ExecutableDep{Name: "gdb", Version: ">=10.0", Required: true})
	if result.Status != sysdep.StatusUnknown {
		t.Errorf("expected UNKNOWN, got %s: %s", result.Status, result.Message)
	}
	if result.FoundPath == "" {
		t.Error("expected the path found to be reported: it is installed")
	}
}

func TestOutputWithNoVersionIsUnknown(t *testing.T) {
	machine := newMachine()
	machine.programs["tool"] = "/usr/bin/tool"
	machine.versions["tool"] = "a tool that does not say"
	result := machine.check(t, sysdep.ExecutableDep{Name: "tool", Version: ">=1.0", Required: true})
	if result.Status != sysdep.StatusUnknown {
		t.Errorf("expected UNKNOWN, got %s: %s", result.Status, result.Message)
	}
}

// A program fetched into the Rho home is installed whether or not anybody put it on
// PATH — and a check that could not see it would offer to fetch a second copy of one
// that is already there.
func TestAFetchedProgramIsInstalled(t *testing.T) {
	machine := newMachine()
	machine.fetched["arm-none-eabi-gdb"] = "/home/a/.rho/toolchains/arm-none-eabi-gdb-14.2/bin/arm-none-eabi-gdb"
	machine.versions["arm-none-eabi-gdb"] = "GNU gdb (Ubuntu 14.2-0ubuntu1~22.04) 14.2"
	result := machine.check(t, sysdep.ExecutableDep{
		Name: "arm-none-eabi-gdb", Version: ">=10.0", Required: true,
		Fetch: &sysdep.FetchRecipe{URL: "https://example.com/gdb.tar.gz", Version: "14.2"},
	})
	if result.Status != sysdep.StatusOK {
		t.Fatalf("expected a fetched program to be found, got %s: %s", result.Status, result.Message)
	}
	if result.FoundPath != machine.fetched["arm-none-eabi-gdb"] {
		t.Errorf("expected the fetched path, got %q", result.FoundPath)
	}
}

func TestAnOptionalRequirementThatIsAbsentIsAWarning(t *testing.T) {
	machine := newMachine()
	result := machine.check(t, sysdep.ExecutableDep{Name: "optional", Required: false})
	if result.Status != sysdep.StatusWarning {
		t.Errorf("expected a warning, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "nothing requires it") {
		t.Errorf("expected it to say the module does not need it, got %q", result.Message)
	}
}

// A module's own pattern that does not compile is its mistake, and the best thing to
// do about it is read the version the way it would have been without one — so the
// requirement is still checked rather than skipped.
func TestAPatternThatDoesNotCompileIsSaidAndStillChecked(t *testing.T) {
	machine := newMachine()
	machine.programs["gdb"] = "/usr/bin/gdb"
	machine.versions["gdb"] = "GNU gdb (Ubuntu 12.1-3ubuntu1~22.04) 12.1"
	result := machine.check(t, sysdep.ExecutableDep{
		Name: "gdb", Version: ">=10.0", Required: true, VersionPattern: "[unclosed",
	})
	if !strings.Contains(result.Message, "does not compile") {
		t.Errorf("expected the module's mistake to be said, got %q", result.Message)
	}
	if result.FoundVersion == "" {
		t.Error("expected the version read anyway, so the requirement is still checked")
	}
}

/* -------------------------------------------------------------------------- */
/*  The environment and the rules                                              */
/* -------------------------------------------------------------------------- */

func TestAnEnvironmentVariableIsRead(t *testing.T) {
	machine := newMachine()
	machine.environment["ARM_TOOLCHAIN_PATH"] = "/opt/arm"

	set := checkEnv("module", sysdep.EnvVarDep{Name: "ARM_TOOLCHAIN_PATH", Required: true}, machine.options())
	if set.Status != sysdep.StatusOK || set.FoundPath != "/opt/arm" {
		t.Errorf("expected it set, got %s: %s", set.Status, set.Message)
	}

	// A different name: the one above is set on this machine, and asking about the
	// same variable twice would be testing the map as much as the check.
	unset := checkEnv("module", sysdep.EnvVarDep{Name: "NOT_SET_ANYWHERE", Required: true}, machine.options())
	if unset.Status != sysdep.StatusMissing {
		t.Errorf("expected it missing, got %s: %s", unset.Status, unset.Message)
	}

	optional := checkEnv("module", sysdep.EnvVarDep{Name: "OTHER", Required: false}, machine.options())
	if optional.Status != sysdep.StatusWarning {
		t.Errorf("expected an optional variable to be a warning, got %s", optional.Status)
	}
}

// A rule is a file in a system directory, and the question is whether the host has
// it — not whether this process can read it, because a rule installed and not
// readable is still a rule the host will honour.
func TestAHostRuleIsLookedForByItsPath(t *testing.T) {
	machine := newMachine()
	dep := sysdep.UdevRuleDep{
		Name:       "60-rho-probes.rules",
		TargetPath: "/etc/udev/rules.d/60-rho-probes.rules",
		Source:     "probe/60-rho-probes.rules",
	}

	missing := checkUdev("module", dep, "/opt/module", machine.options())
	if missing.Status != sysdep.StatusMissing {
		t.Errorf("expected it missing, got %s: %s", missing.Status, missing.Message)
	}
	if !strings.Contains(missing.Message, "/etc/udev/rules.d/60-rho-probes.rules") {
		t.Errorf("expected the path named, got %q", missing.Message)
	}

	machine.files["/etc/udev/rules.d/60-rho-probes.rules"] = true
	machine.files["/opt/module/probe/60-rho-probes.rules"] = true
	present := checkUdev("module", dep, "/opt/module", machine.options())
	if present.Status != sysdep.StatusOK {
		t.Errorf("expected it present, got %s: %s", present.Status, present.Message)
	}
	// The rule's own file is resolved, because installing it is a copy of something a
	// person should be able to read before it goes into a system directory.
	if present.RuleSource != "/opt/module/probe/60-rho-probes.rules" {
		t.Errorf("expected the file resolved against the module, got %q", present.RuleSource)
	}

	blank := checkUdev("module", sysdep.UdevRuleDep{Name: "x"}, "/opt/module", machine.options())
	if blank.Status != sysdep.StatusUnknown {
		t.Errorf("expected a rule with no path to be unknown, got %s", blank.Status)
	}
}

/* -------------------------------------------------------------------------- */
/*  The report                                                                 */
// -------------------------------------------------------------------------- */

// What a person reads first is what stops them: a missing required program, then an
// old one, then a warning — whatever the names are.
func TestTheReportIsOrderedByHowMuchEachItemStops(t *testing.T) {
	results := []sysdep.CheckResult{
		{Dep: "z-optional", Status: sysdep.StatusWarning},
		{Dep: "a-missing", Status: sysdep.StatusMissing},
		{Dep: "m-outdated", Status: sysdep.StatusOutdated},
		{Dep: "b-ok", Status: sysdep.StatusOK},
	}
	for outer := 1; outer < len(results); outer++ {
		for inner := outer; inner > 0 &&
			byUrgency(results[inner].Status) > byUrgency(results[inner-1].Status); inner-- {
			results[inner], results[inner-1] = results[inner-1], results[inner]
		}
	}
	if results[0].Dep != "a-missing" || results[1].Dep != "m-outdated" {
		t.Errorf("expected the worst first, got %s then %s", results[0].Dep, results[1].Dep)
	}
}

func TestTheReportIsCountedAndJudged(t *testing.T) {
	healthy := sysdep.Report{Results: []sysdep.CheckResult{
		{Status: sysdep.StatusOK}, {Status: sysdep.StatusWarning},
	}}
	if !healthy.Healthy() {
		t.Error("expected a warning not to make a machine unhealthy")
	}
	if counts := healthy.Tally(); counts.OK != 1 || counts.Warning != 1 {
		t.Errorf("expected one of each, got %+v", counts)
	}

	unhealthy := sysdep.Report{Results: []sysdep.CheckResult{
		{Status: sysdep.StatusOK}, {Status: sysdep.StatusMissing}, {Status: sysdep.StatusOutdated},
	}}
	if unhealthy.Healthy() {
		t.Error("expected a missing program to make a machine unhealthy")
	}
	if got := unhealthy.NeedsWork(); len(got) != 2 {
		t.Errorf("expected two things to do, got %d", len(got))
	}
	if !(sysdep.Report{}).Healthy() {
		t.Error("expected an empty report not to be a problem: nothing was declared")
	}
}

// Both renderings are the same report: what a person reads and what a program reads
// cannot be told apart, because a tool that shows two different things about one
// machine is worse than a tool that shows one thing badly.
func TestTheTextSaysWhatTheJSONSays(t *testing.T) {
	report := sysdep.Report{
		Generated: "2026-01-01T00:00:00Z",
		Scanned:   []string{"rho-dbg"},
		Results: []sysdep.CheckResult{
			{
				Module: "rho-dbg", Dep: "openocd", Kind: sysdep.KindExecutable, Required: true,
				Status: sysdep.StatusMissing, Message: "openocd is not installed",
				Install: map[string]string{"apt": "openocd", "pacman": "openocd"},
			},
			{
				Module: "rho-dbg", Dep: "cmake", Kind: sysdep.KindExecutable, Required: true,
				Status: sysdep.StatusOK, FoundPath: "/usr/bin/cmake", Message: "4.4.3 at /usr/bin/cmake",
			},
		},
	}
	text := Text(report)
	for _, wanted := range []string{"openocd", "cmake", "MISSING", "OK", "rho-dbg", "apt", "rho keeper sync"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("expected %q in the report:\n%s", wanted, text)
		}
	}
	// A recipe beside an entry that is already right is noise, and noise is why
	// reports go unread. One broken thing with two managers is two lines; a satisfied
	// thing adds none.
	if got := strings.Count(text, "install with"); got != 2 {
		t.Errorf("expected only the missing entry's recipes, got %d:\n%s", got, text)
	}
	if strings.Contains(text, "install with pacman: cmake") {
		t.Errorf("expected no recipe for an entry that is already satisfied:\n%s", text)
	}

	encoded, err := JSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"openocd"`) || !strings.Contains(string(encoded), `"status": "MISSING"`) {
		t.Errorf("expected the JSON to carry the same facts:\n%s", encoded)
	}
}

// A report with nothing in it says so, rather than looking like a machine with no
// problems — which is the difference between "nothing is declared" and "everything is
// satisfied", and only one of them is a machine you can debug on.
func TestAnEmptyReportSaysNothingIsDeclared(t *testing.T) {
	text := Text(sysdep.Report{})
	if !strings.Contains(strings.ToLower(text), "no installed module has declared") {
		t.Errorf("expected the empty case to say nothing is declared:\n%s", text)
	}
	if Summary(sysdep.Report{}) == "" {
		t.Error("expected a one-line summary even with nothing to say")
	}
}
