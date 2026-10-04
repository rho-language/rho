//
// @file sysdeps.go
// @brief Acting on what the doctor found.
//
// `rho doctor` says what this machine is missing. This is the half that does
// something about it, and the split is the point: a report that also installed
// would be a report you could not tell from the changes it had made, and what it
// would change is the machine — with root, on the strength of a line in a module
// manifest.
//
// So every action here is one a person asked for by name, is shown in full before
// it runs, and is reported afterwards with the command that ran it. The same rule
// this dashboard has followed for every change it has ever made: a change made here
// is a change that can be repeated in a terminal and read in a log.
//
// The finding is read from `rho doctor --json` rather than worked out here, so that
// the terminal and the page cannot disagree about what the machine has — which they
// would, the moment one of the two were changed alone.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/sysdep"
)

// The report is read from the doctor rather than computed here, for one reason:
// if this worked out for itself what the machine had, the two would answer
// differently the moment one of them changed, and the fix a person applied from
// this page would be for a problem this page had diagnosed differently from the
// terminal.
const doctorCommand = "doctor"

// ReadReport is what `rho doctor` found, or why it could not be read.
//
// A doctor that is not installed is not a failure here: everything else this
// dashboard says is still true, and a report that says "install rho-doctor to see
// this" is more useful than one that says nothing.
func ReadReport() (sysdep.Report, string) {
	command, found := ToolchainCommand()
	if !found {
		return sysdep.Report{}, "the Rho toolchain was not found, so nothing could be checked"
	}
	collected, err := exec.Command(command, doctorCommand, "--json").Output()
	if err != nil && len(collected) == 0 {
		// A non-zero exit with output on standard output is a report that found work
		// to do, which is what this is for. Only a command that produced nothing at
		// all is a failure here.
		return sysdep.Report{}, fmt.Sprintf("rho doctor could not be read: %v", err)
	}
	report := sysdep.Report{}
	if err := json.Unmarshal(collected, &report); err != nil {
		return sysdep.Report{}, fmt.Sprintf("what rho doctor printed is not a report: %v", err)
	}
	return report, ""
}

// SystemRequirements is what the installed modules need from this machine, as rows a
// page shows and a wizard acts on.
//
// Empty with a reason when the doctor could not be read, because a page that
// showed an empty list would be a page saying the machine is ready.
func SystemRequirements() ([]SystemRow, string) {
	report, problem := ReadReport()
	if problem != "" {
		return nil, problem
	}
	rows := make([]SystemRow, 0, len(report.Results))
	for _, result := range report.Results {
		rows = append(rows, SystemRow{
			Module:     result.Module,
			Dep:        result.Dep,
			Kind:       result.Kind,
			Required:   result.Required,
			Status:     result.Status,
			Version:    result.FoundVersion,
			Wanted:     result.WantVersion,
			Message:    result.Message,
			Recipes:    Recipes(result),
			Installs:   result.Install,
			Fixable:    Fixable(result),
			FixCommand: FixCommand(result),
		})
	}
	return rows, ""
}

// readReportOrEmpty is the report, or an empty one when it could not be read — for
// a caller that is answering "what would you do" rather than "what is wrong", and
// must not answer a question it does not have.
func readReportOrEmpty() sysdep.Report {
	report, _ := ReadReport()
	return report
}

// SystemRow is one requirement, as a page shows it.
type SystemRow struct {
	Module   string        `json:"module"`
	Dep      string        `json:"dep"`
	Kind     string        `json:"kind"`
	Required bool          `json:"required"`
	Status   sysdep.Status `json:"status"`
	Version  string        `json:"version,omitempty"`
	Wanted   string        `json:"wanted,omitempty"`
	Message  string        `json:"message"`
	// Recipes are the ways to get it that work on *this* machine, worded as commands
	// and shown before they run. What is not here is in `Installs`, which is what a
	// report pasted into a question on another machine needs.
	Recipes []Recipe `json:"recipes,omitempty"`
	// Installs is every manager's answer, whether or not this host has that manager.
	Installs map[string]string `json:"installs,omitempty"`
	// Fixable is whether anything can be done about it here and now, which is what
	// decides whether a page offers a button at all.
	Fixable bool `json:"fixable"`
	// FixCommand is what would fix it, shown so the change is repeatable in a
	// terminal — which is the rule every change this page makes has followed.
	FixCommand string `json:"fixCommand,omitempty"`
}

// Recipe is one way to install something on this machine.
type Recipe struct {
	Manager string `json:"manager"`
	Package string `json:"package"`
	Command string `json:"command"`
}

// Recipes is what a requirement could be installed with here, as commands.
func Recipes(result sysdep.CheckResult) []Recipe {
	current := detectHost()
	managers := make([]string, 0, len(result.Install))
	for manager := range result.Install {
		managers = append(managers, manager)
	}
	sort.Strings(managers)

	out := []Recipe{}
	for _, manager := range managers {
		// `pkg`, not `package`: one of them is a keyword.
		pkg := strings.TrimSpace(result.Install[manager])
		// `manual` is a URL rather than a package, and no package manager installs one.
		if manager == "manual" || pkg == "" {
			continue
		}
		command, args, ok := current.installCommand(manager, pkg)
		if !ok {
			continue
		}
		out = append(out, Recipe{
			Manager: manager, Package: pkg,
			Command: strings.Join(append([]string{command}, args...), " "),
		})
	}
	return out
}

// Fixable is whether a requirement can be dealt with here and now.
//
// A fetch counts: it needs no privileges and is the preferred way to get a
// toolchain. A rule counts when the module carries the file, because installing it
// is a copy of something a person can read — which is what makes asking for root
// for it reasonable rather than merely convenient.
func Fixable(result sysdep.CheckResult) bool {
	// A warning counts: it says the module does not need this, not that there is
	// nothing to do. A person clicking Install on one is asking for it.
	if result.Status == sysdep.StatusOK || result.Status == sysdep.StatusUnknown {
		return false
	}
	switch {
	case result.Fetch != nil:
		return true
	case len(Recipes(result)) > 0:
		return true
	case result.Kind == sysdep.KindUdev:
		return result.FoundPath != "" && strings.TrimSpace(result.RuleSource) != ""
	}
	return false
}

// FixCommand is what would fix one requirement, as a person would run it.
//
// Written the way a person would type it — including `sudo` where it is genuinely
// needed, because a command that hides its own privileges is a command nobody can
// read before running.
func FixCommand(result sysdep.CheckResult) string {
	if !Fixable(result) {
		return ""
	}
	if result.Kind == sysdep.KindUdev {
		return fmt.Sprintf("rho keeper install %s %s", result.Module, result.Dep)
	}
	return fmt.Sprintf("rho keeper install %s %s", result.Module, result.Dep)
}

// SystemSummary is the machine's system health in one line, for a header.
func SystemSummary() string {
	rows, problem := SystemRequirements()
	if problem != "" {
		return "system status unknown: " + problem
	}
	if len(rows) == 0 {
		return "nothing declared: no installed module has said what it needs from this machine"
	}
	missing, outdated := 0, 0
	for _, row := range rows {
		switch row.Status {
		case sysdep.StatusMissing:
			missing++
		case sysdep.StatusOutdated:
			outdated++
		}
	}
	if missing == 0 && outdated == 0 {
		return fmt.Sprintf("system ok: %d checks, all satisfied", len(rows))
	}
	return fmt.Sprintf("system needs attention: %d missing, %d too old (%d checked)",
		missing, outdated, len(rows))
}

/* -------------------------------------------------------------------------- */
/*  Installing                                                                 */
/* -------------------------------------------------------------------------- */

// Plan is what fixing the outstanding requirements would take, before any of it
// happens.
//
// Built first and shown whole, because this runs a package manager with root and a
// person who was not shown the list will not be able to say so afterwards.
type Plan struct {
	// Steps are the commands, in order.
	Steps []PlannedStep `json:"steps,omitempty"`
	// Skipped are the requirements that need work and have no recipe here, with the
	// reason. Reported rather than passed over: a plan that silently omits what it
	// cannot do reads as a plan with nothing left.
	Skipped []SkippedStep `json:"skipped,omitempty"`
	// NeedsRoot says whether any step will ask for it.
	NeedsRoot bool `json:"needsRoot"`
	// Host is what this machine is, so a plan says which machine it was made for.
	Host string `json:"host"`
}

// PlannedStep is one command a plan would run.
type PlannedStep struct {
	Module  string   `json:"module"`
	Dep     string   `json:"dep"`
	Manager string   `json:"manager"`
	Package string   `json:"package"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Line is the step as a person reads it.
func (s PlannedStep) Line() string {
	return strings.Join(append([]string{s.Command}, s.Args...), " ")
}

// SkippedStep is a requirement with nothing that can be done about it here.
type SkippedStep struct {
	Module string `json:"module"`
	Dep    string `json:"dep"`
	Reason string `json:"reason"`
}

// PlanFor is what fixing the outstanding requirements would take on this machine.
//
// Only this host's own package manager is used, because a recipe for another host's
// manager is a command that cannot work here — and running it would fail after the
// password prompt rather than before it.
func PlanFor(report sysdep.Report) Plan {
	current := detectHost()
	plan := Plan{Host: current.Description()}
	for _, result := range report.Results {
		if !result.Status.NeedsWork() {
			continue
		}
		// A fetch comes before a package manager, always. It needs no privileges, it
		// belongs to the person rather than to the machine, and it does not change
		// when the machine's package manager does — so a build cannot change out from
		// under a project between two runs.
		if result.Fetch != nil {
			plan.Steps = append(plan.Steps, PlannedStep{
				Module: result.Module, Dep: result.Dep,
				Manager: "fetch", Package: result.Fetch.URL,
				Command: "rho keeper fetch " + result.Module + " " + result.Dep,
			})
			continue
		}
		recipe, ok := recipeFor(current, result)
		if !ok {
			plan.Skipped = append(plan.Skipped, SkippedStep{
				Module: result.Module, Dep: result.Dep,
				Reason: whyNothingCan(result),
			})
			continue
		}
		plan.Steps = append(plan.Steps, PlannedStep{
			Module: result.Module, Dep: result.Dep,
			Manager: recipe.Manager, Package: recipe.Package,
			Command: recipe.Command, Args: strings.Fields(recipe.Command)[1:],
		})
		if current.elevation.command != "" {
			plan.NeedsRoot = true
		}
	}
	return plan
}

// whyNothingCan is why a requirement cannot be dealt with here, which is a different
// sentence for each kind: an environment variable is set by a person, a host
// permission is a file that needs root, and a program with only a URL is not in any
// manager.
func whyNothingCan(result sysdep.CheckResult) string {
	switch {
	case strings.TrimSpace(result.Install["manual"]) != "" && len(result.Install) == 1:
		return "not in a package manager: " + result.Install["manual"]
	case result.Kind == sysdep.KindEnv:
		return "an environment variable is set by a person, not by a package manager"
	case result.Kind == sysdep.KindUdev:
		return "a host permission, so it needs root: a file to put at " + result.FoundPath
	case result.Install == nil && result.Fetch == nil:
		return "the module said nothing about how to install it"
	default:
		return "nothing on this machine installs it"
	}
}

// Text is a plan as it is shown before anything runs.
func (p Plan) Text() string {
	out := &strings.Builder{}
	fmt.Fprintf(out, "%s\n", p.Host)
	if len(p.Steps) == 0 && len(p.Skipped) == 0 {
		fmt.Fprintln(out, "\nNothing to install: every requirement is satisfied.")
		return out.String()
	}
	for _, step := range p.Steps {
		fmt.Fprintf(out, "  %-24s %s\n", step.Dep, step.Line())
	}
	for _, skipped := range p.Skipped {
		fmt.Fprintf(out, "  %-24s skipped: %s\n", skipped.Dep, skipped.Reason)
	}
	fmt.Fprintf(out, "\n%d to install, %d that nothing on this machine can install.\n",
		len(p.Steps), len(p.Skipped))
	return out.String()
}

// Run carries out a plan, one step at a time, reporting each.
//
// One at a time so that a failure does not hide the steps after it, and so that
// what is reported afterwards is per step rather than one verdict for the lot.
func (p Plan) Run(onStep func(PlannedStep, error)) {
	for _, step := range p.Steps {
		command := exec.Command(step.Command, step.Args...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if onStep != nil {
			onStep(step, command.Run())
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  What this machine is                                                       */
/* -------------------------------------------------------------------------- */

// host is how this machine installs things.
type host struct {
	// OS is the kernel and Distro the distribution: `linux` alone does not say which
	// package manager, and that is the question being asked.
	OS     string
	Distro string
	// Manager is the package manager found, or "" when there is none.
	Manager string
	// elevation is how a step gets the privileges an install needs, or empty on a
	// host that needs none.
	elevation elevation
}

// Description is this machine in one line, so a plan says which machine it was made
// for — a plan pasted into a question is otherwise unreadable.
func (h host) Description() string {
	if h.Manager == "" {
		return fmt.Sprintf("%s %s, with no package manager found", h.OS, h.Distro)
	}
	return fmt.Sprintf("%s %s, installing with %s", h.OS, h.Distro, h.Manager)
}

// elevation is how a command gets privileges. Empty on a host that needs none: on
// Windows the shell is already as privileged as the program was started as, and
// asking for `sudo` there produces a command that cannot run.
type elevation struct {
	command string
	args    []string
}

// useAs is the arguments of a command run with privileges.
func (e elevation) useAs(command string, args []string) []string {
	if e.command == "" {
		return args
	}
	return append(append([]string{e.command}, e.args...), command)
}

// detectHost is what this machine is and how it installs things.
//
// Every answer is a program that is either there or not. A host whose package
// manager is a shell function, or a container whose `apt` is a wrapper, is a host
// where guessing wrong produces a plan that cannot be run at all — so the plan is
// built from what is actually installed.
func detectHost() host {
	out := host{OS: runtime.GOOS}
	switch out.OS {
	case "darwin":
		out.Distro = "macos"
		out.Manager = firstFound("brew", "port")
	case "windows":
		out.Distro = "windows"
		out.Manager = firstFound("choco", "scoop", "winget")
	default:
		out.Distro = distroName()
		// Not alphabetical. dnf before yum because a machine with both is a Fedora
		// that kept yum installed, and dnf is the one that will do what the recipe
		// says. apt before pacman and zypper for the same reason.
		out.Manager = firstFound("apt-get", "dnf", "yum", "pacman", "zypper", "apk", "emerge")
		out.elevation = elevation{command: firstFound("sudo", "doas")}
	}
	return out
}

// distroName is the distribution, read from the file that says so.
func distroName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if name, value, found := strings.Cut(strings.TrimSpace(line), "="); found && name == "ID" {
			return strings.Trim(value, `"`)
		}
	}
	return "linux"
}

func firstFound(names ...string) string {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

// recipeFor is the one way a requirement can be installed on this host, and whether
// there is one.
func recipeFor(current host, result sysdep.CheckResult) (Recipe, bool) {
	managers := make([]string, 0, len(result.Install))
	for manager := range result.Install {
		if manager != "manual" && strings.TrimSpace(result.Install[manager]) != "" {
			managers = append(managers, manager)
		}
	}
	sort.Strings(managers)
	for _, manager := range managers {
		pkg := strings.TrimSpace(result.Install[manager])
		command, args, ok := current.installCommand(manager, pkg)
		if !ok {
			continue
		}
		return Recipe{
			Manager: manager, Package: pkg,
			Command: strings.Join(append([]string{command}, args...), " "),
		}, true
	}
	return Recipe{}, false
}

// installCommand is how this host installs a package with a manager, and whether it
// has one.
//
// Nothing is run and nothing is checked for the package: this is how a step would
// be *worded*, and a host without the manager has no wording for it.
func (h host) installCommand(manager string, pkg string) (string, []string, bool) {
	if h.Manager == "" || manager != h.Manager {
		return "", nil, false
	}
	switch h.Manager {
	case "apt-get":
		return h.elevation.command, h.elevation.useAs("apt-get", []string{"install", "-y", pkg}),
			h.elevation.command != ""
	case "dnf":
		return "dnf", []string{"install", "-y", pkg}, true
	case "yum":
		return "yum", []string{"install", "-y", pkg}, true
	case "pacman":
		return h.elevation.command, h.elevation.useAs("pacman", []string{"-S", "--needed", "--noconfirm", pkg}),
			h.elevation.command != ""
	case "zypper":
		return h.elevation.command, h.elevation.useAs("zypper", []string{"--non-interactive", "install", pkg}),
			h.elevation.command != ""
	case "apk":
		return "apk", []string{"add", pkg}, true
	case "emerge":
		return "emerge", []string{pkg}, true
	case "brew":
		return "brew", []string{"install", pkg}, true
	case "port":
		return "sudo", []string{"port", "install", pkg}, true
	case "choco":
		return "choco", []string{"install", "-y", pkg}, true
	case "scoop":
		return "scoop", []string{"install", pkg}, true
	case "winget":
		return "winget", []string{"install", "--id", pkg, "--accept-source-agreements"}, true
	}
	return "", nil, false
}

// HostDescription is what this machine is, in one line.
func HostDescription() string { return detectHost().Description() }

// ToolchainCommand is the Rho toolchain on this machine, and whether it was found.
func ToolchainCommand() (string, bool) {
	if command := strings.TrimSpace(os.Getenv("RHO_BIN")); command != "" {
		if _, err := os.Stat(command); err == nil {
			return command, true
		}
	}
	if home := deps.RhoHome(); home != "" {
		candidate := filepath.Join(home, "bin", "rho")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	if found, err := exec.LookPath("rho"); err == nil {
		return found, true
	}
	return "", false
}
