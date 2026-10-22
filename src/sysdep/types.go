//
// @file types.go
// @brief What a module needs from the machine, as it declares it.
//
// Two different kinds of dependency have always been written in the same place.
// `deps:` is a requirement on other modules of this toolchain, and it is a graph
// this toolchain resolves: every requirement is another module, every module is
// here, and nothing has to be installed from anywhere. A requirement on the
// *machine* has none of those properties — `arm-none-eabi-gdb` is not a module, it
// is a program, and it is either installed or it is not.
//
// Writing both as `deps:` is what makes a machine's state indistinguishable from a
// toolchain's, and a person whose board is not debuggable because a debugger is
// missing is told to look at `rho deps`, which cannot help them. So they are two
// blocks: `deps:` is what this toolchain resolves, and `system_deps:` is what
// somebody has to install, check, or be reminded of.
//
// This package is the domain both of them are read through: `rho-doctor` inspects
// and reports, `rho-keeper` acts on the report, and neither has to know how the
// other works.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

// Package sysdep is what a module needs from the machine it runs on: the programs
// it runs, the environment it expects, and the permissions the host has to grant.
//
// Read-only in itself. Everything here describes a requirement and, where a check
// is asked for, what was found — and none of it changes anything, because the one
// thing that must never happen silently is a session installing a debugger.
package sysdep

// Status is what a check found.
type Status string

// The four answers a check can give.
//
// Not a pair of booleans, because "installed but too old" and "installed and right"
// want different things done about them, and a report that reduced both to `ok`
// would make the older one indistinguishable from the newer one — which is the
// failure this whole block exists to catch.
const (
	// StatusOK is what a requirement asked for and the machine has.
	StatusOK Status = "OK"
	// StatusMissing is a required program that is not installed.
	StatusMissing Status = "MISSING"
	// StatusOutdated is a program installed at a version the requirement excludes.
	StatusOutdated Status = "OUTDATED"
	// StatusWarning is something worth saying that is not a failure: an optional
	// program that is absent, an environment variable nobody set.
	StatusWarning Status = "WARNING"
	// StatusUnknown is a check that could not be made — a version flag that timed
	// out, a permission this process is not allowed to read. Its own answer,
	// because "I could not tell" and "it is fine" are not the same thing.
	StatusUnknown Status = "UNKNOWN"
)

// NeedsWork reports whether a status is something somebody has to do something about.
func (s Status) NeedsWork() bool {
	return s == StatusMissing || s == StatusOutdated
}

// SystemDeps is what a module needs from the machine.
type SystemDeps struct {
	// Executables are the programs it runs: a compiler, a debugger, a probe server,
	// any tool it shells out to.
	Executables []ExecutableDep `yaml:"executables" json:"executables"`
	// EnvVars are the environment variables it reads.
	EnvVars []EnvVarDep `yaml:"env_vars" json:"env_vars"`
	// UdevRules are the host permissions it needs, which is the one requirement on a
	// machine that no package manager will install for you: a rule that is missing
	// is a device that is present and not permitted.
	UdevRules []UdevRuleDep `yaml:"udev_rules" json:"udev_rules"`
}

// Declared reports whether a module has said anything about the machine at all.
func (s *SystemDeps) Declared() bool {
	if s == nil {
		return false
	}
	return len(s.Executables) > 0 || len(s.EnvVars) > 0 || len(s.UdevRules) > 0
}

// ExecutableDep is one program, and what is wanted of it.
type ExecutableDep struct {
	// Name is the program, as it is run: `arm-none-eabi-gdb`.
	Name string `yaml:"name" json:"name"`
	// Version is the constraint the version found must satisfy, in the same grammar
	// `deps` uses: `>=10.0`, `=0.12.0`, `>=0.11 <0.13`, `~1.2`, `^2`, and
	// alternatives with `||`. Empty means any version will do, which is what a program
	// whose output format has been stable for a decade should say.
	//
	// One field, because it is one system: a minimum, a maximum and a range are the
	// same thing written three ways, and this toolchain already has a grammar for
	// them. A separate `min_version` would be a second way to say the same thing, and
	// a second one is a second thing to document, parse and get wrong.
	Version string `yaml:"version" json:"version"`
	// VersionFlag is the argument that makes the program print its version.
	// Empty means the common `--version`, tried first and then `-version`.
	VersionFlag string `yaml:"version_flag" json:"version_flag"`
	// VersionPattern extracts the version from what the flag printed: a regular
	// expression whose first group is the version. Empty means the first thing in
	// the output that looks like a version.
	VersionPattern string `yaml:"version_pattern" json:"version_pattern"`
	// Required says whether the module works without it.
	Required bool `yaml:"required" json:"required"`
	// Install is how to get it, by package manager: `apt`, `brew`, `pacman`, `dnf`,
	// `choco`, and `manual` for the ones nothing installs.
	//
	// Declared rather than executed. A manifest is a statement about a requirement
	// and the machine's package manager is a thing that changes; what is written here
	// is the recipe, and somebody runs it.
	Install map[string]string `yaml:"install" json:"install,omitempty"`
	// Fetch is how to get it without a package manager at all: a prebuilt archive,
	// put under the Rho home and run from there.
	//
	// The preferred recipe wherever one exists, and not only because it avoids a
	// password. A toolchain fetched into the user's own directory belongs to that
	// user, does not change with the machine's package manager, and is removed with
	// the directory it was put in — where a system package is shared, upgrades out
	// from under a build, and can only be removed by the machine's owner.
	Fetch *FetchRecipe `yaml:"fetch" json:"fetch,omitempty"`
	// Description says what the program is for, so a report that names a missing
	// program also says what loses its purpose.
	Description string `yaml:"description" json:"description,omitempty"`
}

// FetchRecipe is a prebuilt archive, put somewhere in the user's own directory.
type FetchRecipe struct {
	// URL is where the archive is. The place a vendor publishes it, so a person can
	// go and look at what is going to be downloaded before it is.
	URL string `yaml:"url" json:"url"`
	// Archive is what it is: `tar.gz`, `tar.xz`, `zip`, or a bare binary.
	Archive string `yaml:"archive" json:"archive,omitempty"`
	// Dir is the directory inside the archive to keep — the one with `bin` in it.
	// Which is most archives' root, and not all of them.
	Dir string `yaml:"dir" json:"dir,omitempty"`
	// Binary is the program's path inside the kept directory, when it is not simply
	// `bin/<name>`: some archives put it one level deeper.
	Binary string `yaml:"binary" json:"binary,omitempty"`
	// Version is what the module wants of it, repeated here because a fetched
	// toolchain is not the distribution's build of it and the distribution's number
	// says nothing about what was downloaded.
	Version string `yaml:"version" json:"version,omitempty"`
	// SHA256 is the digest of the archive, as the vendor publishes it: sixty-four
	// hex digits. It is what makes the download the vendor's file rather than
	// whatever answered at the URL — a mirror that was compromised, a redirect, a
	// proxy that rewrote the body. A recipe without one is refused by the program
	// that fetches, because a program fetched from the internet and run is the
	// worst thing here to take on trust.
	SHA256 string `yaml:"sha256" json:"sha256,omitempty"`
}

// EnvVarDep is one environment variable.
type EnvVarDep struct {
	Name        string `yaml:"name" json:"name"`
	Required    bool   `yaml:"required" json:"required"`
	Description string `yaml:"description" json:"description,omitempty"`
}

// UdevRuleDep is one host permission: a rule file, and where it has to be.
type UdevRuleDep struct {
	Name        string `yaml:"name" json:"name"`
	TargetPath  string `yaml:"target_path" json:"target_path"`
	Description string `yaml:"description" json:"description,omitempty"`
	// Source is where the rule file is, in the repository that carries it — which is
	// what makes installing it a copy rather than a written-out guess at the
	// vendor's rules.
	Source string `yaml:"source" json:"source,omitempty"`
	// Optional says a board that works without it. Most do not, and the difference is
	// between a warning and the reason nothing can be probed.
	Optional bool `yaml:"optional" json:"optional,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Checks                                                                     */
/* -------------------------------------------------------------------------- */

// CheckResult is what was found for one requirement.
type CheckResult struct {
	// Module is who declared it, so a report from the whole installation says whose
	// requirement failed.
	Module string `json:"module"`
	// Dep is the program this was, or the variable or the rule.
	Dep string `json:"dep"`
	// Kind is which kind of requirement it is: `executable`, `env`, `udev`.
	Kind string `json:"kind"`
	// Required is whether the module works without it.
	Required bool `json:"required"`
	// Status is what the check found.
	Status Status `json:"status"`
	// FoundPath is where the program is, when it was found. Reported because "it is
	// installed" and "this is the one that will be run" are different facts, and on a
	// machine with two toolchains only one of them is the answer to the question.
	FoundPath string `json:"found_path,omitempty"`
	// FoundVersion is what it printed.
	FoundVersion string `json:"found_version,omitempty"`
	// WantVersion is the constraint, for a report to repeat next to what it found.
	WantVersion string `json:"want_version,omitempty"`
	// Message is a sentence for a person. Never empty: a report whose entries have no
	// message is a list a person has to interpret themselves, which is the work this
	// is meant to have done.
	Message string `json:"message"`
	// Install is how to get it, when there is a way.
	Install map[string]string `json:"install,omitempty"`
	// Fetch is the prebuilt archive it can be got from without a package manager, and
	// the preferred way to get a toolchain where the module has declared one.
	Fetch *FetchRecipe `json:"fetch,omitempty"`
	// RuleSource is where the rule file is on this machine, resolved. Present because
	// installing a rule is a copy of a file a person can read first, and the report
	// has to be able to say which file.
	RuleSource string `json:"ruleSource,omitempty"`
	// Description is what the requirement is for, when the module said.
	Description string `json:"description,omitempty"`
}

// The kinds of requirement, as a report names them.
const (
	KindExecutable = "executable"
	KindEnv        = "env"
	KindUdev       = "udev"
)

// Report is what was found across everything a module set asked for.
type Report struct {
	// Generated is when the report was made, so a report pasted into a question about
	// a machine says which state of it the answer was about.
	Generated string `json:"generated"`
	// Results are the checks, by module then requirement.
	Results []CheckResult `json:"results"`
	// Scanned are the modules that were read.
	Scanned []string `json:"scanned"`
}

// Counts are how many checks fell into each answer.
type Counts struct {
	OK       int `json:"ok"`
	Warning  int `json:"warning"`
	Missing  int `json:"missing"`
	Outdated int `json:"outdated"`
	Unknown  int `json:"unknown"`
}

// Tally is the report counted.
func (r Report) Tally() Counts {
	var counts Counts
	for _, result := range r.Results {
		switch result.Status {
		case StatusOK:
			counts.OK++
		case StatusWarning:
			counts.Warning++
		case StatusMissing:
			counts.Missing++
		case StatusOutdated:
			counts.Outdated++
		case StatusUnknown:
			counts.Unknown++
		}
	}
	return counts
}

// Healthy is whether anything in the report needs a person.
func (r Report) Healthy() bool {
	counts := r.Tally()
	return counts.Missing == 0 && counts.Outdated == 0
}

// NeedsWork is the results somebody has to act on, in the order they were found.
//
// Ordered by how much they stop: a missing required program first, because nothing
// that uses it will run at all; then an old one, because it will run and be wrong.
func (r Report) NeedsWork() []CheckResult {
	out := []CheckResult{}
	for _, result := range r.Results {
		if result.Status.NeedsWork() {
			out = append(out, result)
		}
	}
	return out
}

// Optional says whether a result is something the module can do without.
func (r CheckResult) Optional() bool { return !r.Required }
