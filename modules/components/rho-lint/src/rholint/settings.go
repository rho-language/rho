//
// @file settings.go
// @brief What this component is set to, read from its own manifest and its own file.
//
// The contract is declared in the component's manifest and the values are in the
// file that manifest names, so what a person reads to find out what can be
// configured is the same thing the component runs on. Reading them is the same
// reading rho-dbg and rho-dbg's siblings do, because two components reading their
// configuration two ways is one of them reading it wrong.
//
// The values are read when a command runs, not once when this is compiled, because a
// person changes the file and then runs again — and a component that had already read
// it would be working from whatever it was told before the file said otherwise.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// Settings is what a run may be configured with.
//
// Five values, and all five are about one thing: which findings are this project's
// business. Which analyzers run and how loud they have to be is a decision a project
// makes once, which is what makes it a setting rather than a flag.
type Settings struct {
	// Engines are the engines a run uses when the command line names none. Empty
	// means every engine available.
	Engines []string
	// FailOn is the severity at which a run fails.
	FailOn Severity
	// Database is where the compilation database is read from, relative to the
	// project. Empty means the places this component looks.
	Database string
	// Exclude are the path fragments that are never read.
	Exclude []string
	// Outside is `exclude` or `include` for a source outside the project's own
	// directory.
	Outside string
}

// The built-in defaults, which are also what the manifest declares.
//
// Written here as well as in the manifest because the file is optional: a component
// installed without its configuration directory, or one whose manifest cannot be
// read, should still run — and it should run with the values the manifest says are
// the defaults rather than with an empty list, which would mean "everything" or
// "nothing" depending on which field it was.
var (
	// DefaultEngines is what a run uses when nothing else says.
	//
	// The two carried engines first, so a machine with nothing installed still
	// covers a Rho program and a C one. The two plugin engines after them, so a
	// machine that has them gets the stronger analysis without being asked.
	DefaultEngines = []string{EngineRho, EngineC, "clang-tidy", "cppcheck"}
	// DefaultFailOn is the severity a run fails on.
	DefaultFailOn = SeverityError
	// DefaultDatabase is where the compilation database is read from. Empty means the
	// places this component looks, which is where a CMake build puts one.
	DefaultDatabase = ""
	// DefaultExclude are the path fragments that are never read.
	DefaultExclude = []string{"vendor/"}
	// DefaultOutside is what to do about a source outside the project's directory.
	DefaultOutside = OutsideExclude
)

// ReadSettings is this component's manifest and the file it names.
//
// The manifest is parsed with the loader every manifest is parsed with, so the
// contract is the toolchain's reading of it rather than a second reading written
// next to it. A manifest that cannot be read has declared no contract, and the
// built-in defaults are better than refusing a run over a file that was never going
// to be read.
func ReadSettings() Settings {
	out := Settings{
		Engines:  append([]string{}, DefaultEngines...),
		FailOn:   DefaultFailOn,
		Database: DefaultDatabase,
		Exclude:  append([]string{}, DefaultExclude...),
		Outside:  DefaultOutside,
	}

	root := ModuleRoot()
	if root == "" {
		return out
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Settings == nil {
		return out
	}

	applyDeclared(&out, manifest)
	if manifest.Settings.File == "" {
		return out
	}
	data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifest.Settings.File)))
	if readErr != nil {
		// A file that is not there is a component nobody has changed, which is not a
		// failure. A file that is there and unreadable is the same: the defaults say
		// what a run does, so a run is never blocked by a configuration file.
		return out
	}
	document, parseErr := yaml.Load(string(data))
	if parseErr != nil {
		return out
	}
	applyFile(&out, document.Map)
	return out
}

// applyDeclared is what the manifest's own declaration says the defaults are.
//
// Read even when the file exists, because the declaration is what a person reads to
// find out what a run can be asked to do. Overwriting the file's values with these
// would be backwards; reading them first means a setting the file omits still has a
// value, which is the only thing the declaration is for.
func applyDeclared(out *Settings, manifest *deps.Manifest) {
	for _, setting := range manifest.Settings.Settings {
		if setting.Default == "" {
			continue
		}
		switch setting.Name {
		case "default_engines":
			out.Engines = splitList(setting.Default, out.Engines)
		case "fail_on":
			out.FailOn = failOn(setting.Default, out.FailOn)
		case "database":
			out.Database = strings.TrimSpace(setting.Default)
		case "exclude":
			out.Exclude = splitList(setting.Default, out.Exclude)
		case "outside":
			out.Outside = outside(setting.Default, out.Outside)
		}
	}
}

// applyFile is what the values file says.
func applyFile(out *Settings, values map[string]yaml.Value) {
	for key, value := range values {
		switch key {
		case "default_engines":
			out.Engines = splitList(strings.TrimSpace(value.Str), out.Engines)
		case "fail_on":
			out.FailOn = failOn(strings.TrimSpace(value.Str), out.FailOn)
		case "database":
			out.Database = strings.TrimSpace(value.Str)
		case "exclude":
			out.Exclude = splitList(strings.TrimSpace(value.Str), out.Exclude)
		case "outside":
			out.Outside = outside(strings.TrimSpace(value.Str), out.Outside)
		}
	}
}

// splitList is a comma-separated setting as a list, and `fallback` when it is empty.
//
// An empty value meaning "the default" rather than "none" is a decision, and it is
// the decision that makes an empty `default_engines` safe to write: every engine
// this machine has. Treating it as "no engines" would make clearing the setting
// silence the component, which is not a thing anybody means by clearing a setting.
func splitList(value string, fallback []string) []string {
	parts := []string{}
	for _, piece := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(piece); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return parts
}

// failOn is a severity setting, or the fallback when it is not one this component
// knows.
//
// Refused rather than guessed: `fail_on: eror` failing the run on errors by accident
// would be a silent wrong answer, and `fail_on: eror` failing nothing would be a
// worse one. The caller reports an unusable value, so this only has to not be wrong.
func failOn(value string, fallback Severity) Severity {
	switch Severity(strings.ToLower(strings.TrimSpace(value))) {
	case ThresholdNever, SeverityError, SeverityWarning, SeverityNote:
		return Severity(strings.ToLower(strings.TrimSpace(value)))
	default:
		return fallback
	}
}

// outside is an outside-ness setting, or the fallback when it is not one of the two.
func outside(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OutsideExclude:
		return OutsideExclude
	case OutsideInclude:
		return OutsideInclude
	default:
		return fallback
	}
}

// KnownFailOn reports whether a string is a severity this component can fail on.
//
// For the command line, which says so rather than falling back: `rho lint
// --fail-on eror` should be a usage error, not a run that quietly failed on errors.
func KnownFailOn(value string) bool {
	switch Severity(strings.ToLower(strings.TrimSpace(value))) {
	case ThresholdNever, SeverityError, SeverityWarning, SeverityNote:
		return true
	default:
		return false
	}
}

// ModuleRoot is where this component's own files are.
//
// The environment first, because a component run from a checkout is told where the
// checkout is by whoever runs it; then the directory the binary is in, because a
// component installed from a registry has its manifest and its values file beside the
// binary — which is what makes a rho-lint configured by its manifest rather than by
// wherever the installation happened to put things.
func ModuleRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_MODULE_ROOT")); root != "" {
		return root
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	return ""
}
