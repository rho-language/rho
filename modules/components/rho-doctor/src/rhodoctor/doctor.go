//
// @file doctor.go
// @brief What a module needs from the machine, found out.
//
// A session fails for two reasons that look the same from outside: the code is
// wrong, or the program it needs is not installed. Only the second is worth a
// command, and this is it — every module that says what it needs from the machine,
// checked against the machine, and reported in one list.
//
// Read-only, deliberately. A tool that inspects and also installs is a tool whose
// report you cannot trust, because you cannot tell what it found from what it
// changed; and the thing it would be changing is the machine, with root, on the
// strength of a file in a module. So this answers and stops. `rho keeper` is the
// half that acts, and it acts on this report.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

// Package rhodoctor is the diagnostic half of the system's health: it reads what
// modules require from the machine and reports what the machine has.
//
// Nothing here changes anything. Every check is a question asked and an answer
// recorded, and the report is the whole output — which is what makes it safe for
// `rho keeper` to act on, and safe for a person to read before deciding to.
package rhodoctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"rho/src/deps"
	"rho/src/sysdep"
)

// Options are the questions to ask.
type Options struct {
	// Module scopes the report to one module, by name. Empty means every installed
	// module that has declared anything.
	Module string
	// Enabled scopes the report to the modules that run. A machine usually has a few
	// modules installed and not enabled — installed is not a decision to run — and
	// their requirements are not the machine's problem today.
	EnabledOnly bool
	// LookPath is how a program is found. A function rather than a setting, so that
	// the whole checker is testable without a machine that has these programs on it.
	LookPath func(string) (string, error)
	// FetchedBinary is where a program fetched into the Rho home is, when the path
	// does not have it. A program put there is installed whether or not anybody put
	// it on PATH, and a check that could not see it would offer to fetch a second
	// copy of one that is already here.
	FetchedBinary func(sysdep.ExecutableDep) (string, bool)
	// CacheRoot is where fetched programs are kept, for the same reason.
	CacheRoot string
	// Environment is the environment to read variables from.
	Environment map[string]string
	// ReadFile is how a host file is read, for a rule this process may not be
	// permitted to see.
	ReadFile func(string) ([]byte, error)
	// Stat is how a path is asked about.
	Stat func(string) (os.FileInfo, error)
	// RunVersion asks a program what version it is. Separated from the command
	// runner so a test can answer without a process, and so the timeout lives in one
	// place.
	RunVersion func(path string, flag string) (string, error)
	// Now is when the report was made.
	Now func() time.Time
}

// Kinds of module the report reads. A system dependency can be declared by
// anything — a component that runs a program, a plugin that reads a rule — so
// every kind is scanned rather than only the components.
var scannedKinds = []string{"component", "plugin", "module", "library"}

// Check is everything a report is made of, answered.
func Check(options Options) sysdep.Report {
	options = withDefaults(options)

	report := sysdep.Report{
		Generated: options.Now().UTC().Format(time.RFC3339),
		Results:   []sysdep.CheckResult{},
		Scanned:   []string{},
	}

	for _, module := range installedModules(options) {
		manifest, err := deps.LoadManifest(filepath.Join(module.Root, "module.yml"))
		if err != nil || manifest == nil || !manifest.SystemDeps.Declared() {
			continue
		}
		if options.Module != "" && manifest.ModuleName != options.Module {
			continue
		}
		report.Scanned = append(report.Scanned, manifest.ModuleName)
		for _, dep := range manifest.SystemDeps.Executables {
			report.Results = append(report.Results, checkExecutable(manifest.ModuleName, dep, options))
		}
		for _, dep := range manifest.SystemDeps.EnvVars {
			report.Results = append(report.Results, checkEnv(manifest.ModuleName, dep, options))
		}
		for _, dep := range manifest.SystemDeps.UdevRules {
			report.Results = append(report.Results,
				checkUdev(manifest.ModuleName, dep, module.Root, options))
		}
	}
	sort.SliceStable(report.Results, func(i, j int) bool {
		if report.Results[i].Module != report.Results[j].Module {
			return report.Results[i].Module < report.Results[j].Module
		}
		return byUrgency(report.Results[i].Status) > byUrgency(report.Results[j].Status)
	})
	sort.Strings(report.Scanned)
	return report
}

// byUrgency orders statuses by how much they stop. A missing required program
// comes before an old one, and both before a warning: what a person should read
// first is what stops them, not what is alphabetically before it.
func byUrgency(status sysdep.Status) int {
	switch status {
	case sysdep.StatusMissing:
		return 5
	case sysdep.StatusOutdated:
		return 4
	case sysdep.StatusUnknown:
		return 3
	case sysdep.StatusWarning:
		return 2
	case sysdep.StatusOK:
		return 1
	default:
		return 0
	}
}

// installedModules is what is on this machine to ask about.
func installedModules(options Options) []deps.InstalledModule {
	out := []deps.InstalledModule{}
	for _, kind := range scannedKinds {
		for _, module := range deps.ListInstalled(kind) {
			if options.EnabledOnly && !module.Enabled {
				continue
			}
			out = append(out, module)
		}
	}
	return out
}

func withDefaults(options Options) Options {
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.CacheRoot == "" {
		options.CacheRoot = sysdep.ToolchainRoot(deps.RhoHome())
	}
	if options.FetchedBinary == nil {
		cache := options.CacheRoot
		options.FetchedBinary = func(dep sysdep.ExecutableDep) (string, bool) {
			return sysdep.FetchedBinary(cache, dep, sysdep.FetchedVersion(dep))
		}
	}
	if options.Environment == nil {
		options.Environment = environ()
	}
	if options.ReadFile == nil {
		options.ReadFile = os.ReadFile
	}
	if options.Stat == nil {
		options.Stat = os.Stat
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RunVersion == nil {
		options.RunVersion = runVersionFlag
	}
	return options
}

func environ() map[string]string {
	out := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, found := strings.Cut(entry, "="); found {
			out[name] = value
		}
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Programs                                                                   */
/* -------------------------------------------------------------------------- */

// checkExecutable is one program: is it here, and is it the version wanted.
func checkExecutable(module string, dep sysdep.ExecutableDep, options Options) sysdep.CheckResult {
	result := sysdep.CheckResult{
		Module:      module,
		Dep:         dep.Name,
		Kind:        sysdep.KindExecutable,
		Required:    dep.Required,
		WantVersion: dep.Version,
		Install:     dep.Install,
		Fetch:       dep.Fetch,
		Description: dep.Description,
	}

	path, err := options.LookPath(dep.Name)
	if err != nil {
		// A program fetched into the Rho home is installed whether or not anybody put
		// it on PATH — and a check that could not see it would report a program the
		// machine has as missing, and offer to fetch a second copy of it.
		if fetched, found := options.FetchedBinary(dep); found {
			path, err = fetched, nil
		}
	}
	if err != nil {
		result.Status = sysdep.StatusMissing
		result.Message = missingProgramMessage(dep)
		if !dep.Required {
			result.Status = sysdep.StatusWarning
		}
		return result
	}
	result.FoundPath = path

	// A requirement with no version is satisfied by the program being there, and
	// there is nothing to gain by running it: a version flag that hangs or waits for
	// input would then be a report that hangs.
	if strings.TrimSpace(dep.Version) == "" {
		result.Status = sysdep.StatusOK
		result.Message = "installed"
		return result
	}

	// A pattern that does not compile is a module's mistake and is said as one, once
	// the version has been read the way it would have been without it — because the
	// requirement is still checkable and a report that skipped it would be a report
	// with a hole in it.
	patternNote := ""
	if patternErr := sysdep.PatternCompiles(dep.VersionPattern); patternErr != nil {
		patternNote = " (the version pattern the module wrote does not compile, so the version was read without it: " +
			patternErr.Error() + ")"
	}

	output, runErr := options.RunVersion(path, dep.VersionFlag)
	if runErr != nil {
		result.Status = sysdep.StatusUnknown
		result.Message = "installed at " + path + ", but it would not say what version it is: " + runErr.Error() + patternNote
		return result
	}

	version, found := sysdep.ExtractVersion(output, dep.VersionPattern)
	if !found {
		result.Status = sysdep.StatusUnknown
		result.Message = fmt.Sprintf("installed at %s, and no version could be read from what it printed for the version flag%s", path, patternNote)
		return result
	}
	result.FoundVersion = version.String()

	satisfied, why := sysdep.Satisfies(dep.Version, version)
	if !satisfied {
		result.Status = sysdep.StatusOutdated
		result.Message = fmt.Sprintf("installed at %s: %s, and the module wants %s", path, why, dep.Version)
		if !dep.Required {
			result.Status = sysdep.StatusWarning
			result.Message += " — and the module does not require it"
		}
		return result
	}

	result.Status = sysdep.StatusOK
	result.Message = fmt.Sprintf("%s at %s, which satisfies %s%s", version, path, dep.Version, patternNote)
	return result
}

// missingProgramMessage says what is missing and what it costs.
//
// "not installed" on its own is a fact with no consequence attached, and the
// consequence is the whole reason a person is reading this: which module stops,
// and what loses its purpose. The install recipe goes with it, because a report
// that says what is missing and then leaves the search for how to get it to the
// reader has only done half the work.
func missingProgramMessage(dep sysdep.ExecutableDep) string {
	out := dep.Name + " is not installed"
	if dep.Description != "" {
		out += "; " + dep.Description
	}
	if !dep.Required {
		out += ", and nothing requires it"
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  The environment                                                            */
/* -------------------------------------------------------------------------- */

// checkEnv is one variable.
//
// Required or not, a variable nobody set is only worth saying something about when
// something looks for it — and the thing that looks for it is the module that
// declared it, which is why the report says so.
func checkEnv(module string, dep sysdep.EnvVarDep, options Options) sysdep.CheckResult {
	result := sysdep.CheckResult{
		Module:      module,
		Dep:         dep.Name,
		Kind:        sysdep.KindEnv,
		Required:    dep.Required,
		Description: dep.Description,
	}
	if value := options.Environment[dep.Name]; value != "" {
		result.Status = sysdep.StatusOK
		result.FoundPath = value
		result.Message = "set"
		return result
	}
	result.Status = sysdep.StatusMissing
	result.Message = dep.Name + " is not set"
	if dep.Description != "" {
		result.Message += "; " + dep.Description
	}
	if !dep.Required {
		result.Status = sysdep.StatusWarning
		result.Message += "; the module does not require it"
	}
	return result
}

/* -------------------------------------------------------------------------- */
/*  Host permissions                                                           */
/* -------------------------------------------------------------------------- */

// checkUdev is one rule file.
//
// A rule is a file in a system directory, and the question is whether it is there
// — not whether this process can read it, because a rule installed and not
// readable by the person running it is still a rule the host will honour.
func checkUdev(module string, dep sysdep.UdevRuleDep, moduleRoot string, options Options) sysdep.CheckResult {
	result := sysdep.CheckResult{
		Module:      module,
		Dep:         dep.Name,
		Kind:        sysdep.KindUdev,
		Required:    !dep.Optional,
		Description: dep.Description,
	}
	// Where the rule file is, resolved against the module that declares it. Resolved
	// here rather than by whoever acts on it, because the report is the one thing
	// both a terminal and a page read, and a path that means different things in the
	// two is a path one of them gets wrong.
	if source := strings.TrimSpace(dep.Source); source != "" && moduleRoot != "" {
		candidate := source
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(moduleRoot, filepath.FromSlash(source))
		}
		if _, err := options.Stat(candidate); err == nil {
			result.RuleSource = candidate
		}
	}
	path := strings.TrimSpace(dep.TargetPath)
	if path == "" {
		result.Status = sysdep.StatusUnknown
		result.Message = "the rule is declared with no target path, so there is nothing to look for"
		return result
	}
	result.FoundPath = path

	if _, err := options.Stat(path); err != nil {
		result.Status = sysdep.StatusMissing
		result.Message = "no rule at " + path
		if dep.Description != "" {
			result.Message += "; " + dep.Description
		}
		if dep.Optional {
			result.Status = sysdep.StatusWarning
			result.Message += "; the module does not require it"
		}
		return result
	}
	result.Status = sysdep.StatusOK
	result.Message = "present at " + path
	return result
}

/* -------------------------------------------------------------------------- */
/*  Running a program for its version                                          */
/* -------------------------------------------------------------------------- */

// How long a program gets to say what version it is.
//
// Short, because a version flag is a program that is expected to print one line
// and exit. One that has not by now is a program waiting for something — a licence
// prompt, a pager, a debugger — and a report that hangs on it is a report nobody
// reads.
const versionTimeout = 5 * time.Second

// runVersionFlag asks a program what version it is, and gives up rather than wait.
//
// The flag tried is the module's when it named one, then `--version`, then
// `-version`. All three because every distribution has settled on a different one,
// and a check that only tried the first would report a perfectly good tool as one
// that cannot answer.
func runVersionFlag(path string, flag string) (string, error) {
	flags := []string{}
	if trimmed := strings.TrimSpace(flag); trimmed != "" {
		flags = append(flags, trimmed)
	}
	flags = append(flags, "--version", "-version", "-v")

	var last error
	for _, one := range flags {
		output, err := runOne(path, one)
		if err == nil && strings.TrimSpace(output) != "" {
			return output, nil
		}
		if err != nil {
			last = err
		}
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return "", last
}

func runOne(path string, flag string) (string, error) {
	if runtime.GOOS == "windows" {
		// There is no pipe to kill a program that will not exit on, so this is left
		// to the program's own timeout rather than pretending to bound it here.
		command := exec.Command(path, flag)
		output, err := command.CombinedOutput()
		return string(output), err
	}
	command := exec.Command(path, flag)
	done := make(chan struct{})
	var (
		output []byte
		err    error
	)
	go func() {
		output, err = command.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(output), err
	case <-time.After(versionTimeout):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-done
		return "", fmt.Errorf("it did not say its version within %s", versionTimeout)
	}
}
