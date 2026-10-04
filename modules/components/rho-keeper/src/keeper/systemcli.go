//
// @file systemcli.go
// @brief `rho keeper doctor`, `sync`, `fix` and `install` — the system half.
//
// Four commands over one reading. `doctor` shows what this machine has against
// what the installed modules need, in the same words `rho doctor` uses — because
// they are the same report, and two renderings of one report is how a terminal and
// a page come to disagree. `sync` fixes what it can in one pass, `fix` asks about
// each thing first, and `install` does one named thing.
//
// The last one exists because a wizard in a browser cannot be the only way to run
// anything with root: a page is a good place to *read* a command and a terrible
// place to be the only way to *run* one. So every action a page offers is a
// command, and `install` is what that command reaches.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"rho/src/sysdep"
)

// Doctor prints what this machine has, against what the installed modules need.
//
// The same report `rho doctor` prints, through the same reading, so the two cannot
// disagree — which matters because this is the command a person reaches for after
// `rho doctor` told them something is missing.
func Doctor(out io.Writer) error {
	rows, problem := SystemRequirements()
	if problem != "" {
		fmt.Fprintf(out, "Cannot check what this machine has: %s\n", problem)
		fmt.Fprintln(out, "Install it with: rho module publish modules/components/rho-doctor")
		return nil
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "Nothing to check: no installed module has declared what it needs from this machine.")
		fmt.Fprintf(out, "This machine: %s\n", HostDescription())
		return nil
	}

	fmt.Fprintf(out, "%s\n\n", SystemSummary())
	current := ""
	for _, row := range rows {
		if row.Module != current {
			if current != "" {
				fmt.Fprintln(out)
			}
			current = row.Module
			fmt.Fprintln(out, current)
		}
		fmt.Fprintf(out, "  %-9s %-22s %s\n", row.Status, row.Dep, row.Message)
		for _, recipe := range row.Recipes {
			fmt.Fprintf(out, "  %-9s %-22s %s\n", "", "", recipe.Command)
		}
	}
	fmt.Fprintf(out, "\nThis machine: %s\n", HostDescription())
	return nil
}

// SyncOptions are the choices a fix takes.
type SyncOptions struct {
	// DryRun says what would happen and does none of it. The default for anything
	// that needs root: this runs a package manager, and a person who did not ask for
	// that must not find out from the password prompt.
	DryRun bool
	// Yes answers every question in advance, which is what a non-interactive caller
	// means by it.
	Yes bool
	// Module narrows the fix to one module's requirements.
	Module string
	// Reader is where the questions are asked, so that a wizard can answer them
	// without a terminal.
	Reader io.Reader
	// Writer is where the plan and each step are reported.
	Writer io.Writer
}

// Sync fixes what the report says is missing, asking first.
//
// Asked first because it installs packages with root, and a machine that asks for
// a password it did not expect is a machine whose owner finds out from someone
// else's automation. `--dry-run` is the same reading without the doing, and it is
// what the page shows before offering the button.
func Sync(options SyncOptions) error {
	if options.Writer == nil {
		options.Writer = os.Stdout
	}
	if options.Reader == nil {
		options.Reader = os.Stdin
	}

	report, problem := ReadReport()
	if problem != "" {
		return fmt.Errorf("cannot check what this machine has: %s", problem)
	}
	if options.Module != "" {
		report = onlyModule(report, options.Module)
	}

	plan := PlanFor(report)
	if len(plan.Steps) == 0 && len(plan.Skipped) == 0 {
		fmt.Fprintln(options.Writer, "Nothing to install: every requirement is satisfied.")
		return nil
	}

	fmt.Fprint(options.Writer, plan.Text())
	if options.DryRun {
		fmt.Fprintln(options.Writer, "\nThat is what would happen. Run it without --dry-run to do it.")
		return nil
	}
	if !options.Yes {
		question := "Run these?"
		if plan.NeedsRoot {
			question = "Run these? Some need root."
		}
		answer, err := ask(options.Reader, options.Writer, question)
		if err != nil {
			return err
		}
		if !answer {
			fmt.Fprintln(options.Writer, "Nothing was installed.")
			return nil
		}
	}

	fmt.Fprintln(options.Writer)
	failed := 0
	plan.Run(func(step PlannedStep, err error) {
		if err != nil {
			failed++
			fmt.Fprintf(options.Writer, "  %s: not installed — %v\n", step.Dep, err)
			fmt.Fprintf(options.Writer, "    to do it by hand: %s\n", step.Line())
			return
		}
		fmt.Fprintf(options.Writer, "  %s: installed\n", step.Dep)
	})
	if failed > 0 {
		return fmt.Errorf("%d of the steps did not finish", failed)
	}
	fmt.Fprintln(options.Writer, "\nCheck again with: rho doctor")
	return nil
}

// Fix fixes the outstanding requirements one at a time, asking about each.
//
// Step by step rather than in one pass because the questions are different: one
// install is a thing somebody expects and another is a thing they have not thought
// about, and a single "install everything?" cannot say which is which.
func Fix(options SyncOptions) error {
	if options.Writer == nil {
		options.Writer = os.Stdout
	}
	if options.Reader == nil {
		options.Reader = os.Stdin
	}

	report, problem := ReadReport()
	if problem != "" {
		return fmt.Errorf("cannot check what this machine has: %s", problem)
	}
	if options.Module != "" {
		report = onlyModule(report, options.Module)
	}

	current := detectHost()
	outstanding := 0
	failed := 0
	for _, result := range report.Results {
		if !result.Status.NeedsWork() {
			continue
		}
		outstanding++
		fmt.Fprintf(options.Writer, "%s  %s\n  %s\n", result.Status, result.Dep, result.Message)

		if reason := whyNothingCan(result); len(Recipes(result)) == 0 {
			fmt.Fprintf(options.Writer, "  cannot be done from here: %s\n", reason)
			continue
		}
		if options.DryRun {
			for _, recipe := range Recipes(result) {
				fmt.Fprintf(options.Writer, "  would run: %s\n", recipe.Command)
			}
			continue
		}
		recipe, _ := recipeFor(current, result)
		answer, err := ask(options.Reader, options.Writer, "  run: "+recipe.Command+" ?")
		if err != nil {
			return err
		}
		if !answer {
			fmt.Fprintln(options.Writer, "  skipped")
			continue
		}
		if err := runRecipe(recipe); err != nil {
			failed++
			fmt.Fprintf(options.Writer, "  not installed: %v\n", err)
			continue
		}
		fmt.Fprintf(options.Writer, "  installed\n")
	}
	switch {
	case outstanding == 0:
		fmt.Fprintln(options.Writer, "Nothing to fix: every requirement is satisfied.")
	case failed == 0:
		fmt.Fprintf(options.Writer, "\n%d checked.\n", outstanding)
	default:
		return fmt.Errorf("%d of %d could not be installed", failed, outstanding)
	}
	return nil
}

// Install fixes one named requirement of one named module.
//
// The command a wizard runs. It takes the module and the dependency rather than a
// recipe, because a recipe is what the report says and what this looks up again:
// a page that ran the recipe it was sent would be running whatever the request
// said, and this is the thing that decides what runs.
func Install(module string, dependency string, options SyncOptions) error {
	if options.Writer == nil {
		options.Writer = os.Stdout
	}
	report, problem := ReadReport()
	if problem != "" {
		return fmt.Errorf("cannot check what this machine has: %s", problem)
	}

	result, found := findRequirement(report, module, dependency)
	if !found {
		// Said with what was declared, because "not found" on its own sends a person
		// looking through the report for a name that is spelled differently.
		declared := []string{}
		for _, one := range report.Results {
			if one.Module == module {
				declared = append(declared, one.Dep)
			}
		}
		if len(declared) == 0 {
			return fmt.Errorf("%s declares nothing this machine is asked about", module)
		}
		return fmt.Errorf("%s needs none of these: %s", dependency, strings.Join(declared, ", "))
	}
	// Only a satisfied requirement stops here. An optional one that is absent is a
	// warning rather than a failure, and a person who has named it is asking for it —
	// calling that "already satisfied" would refuse a fix to a thing that is missing.
	if result.Status == sysdep.StatusOK {
		fmt.Fprintf(options.Writer, "%s is already satisfied: %s\n", result.Dep, result.Message)
		return nil
	}
	if result.Status == sysdep.StatusWarning {
		fmt.Fprintf(options.Writer, "%s is not needed and not there: %s\n", result.Dep, result.Message)
	}

	// The user-space way first, always. It needs no privileges, and it leaves
	// something in the person's own directory that they can see and remove — which
	// is the whole argument for it over a package manager's copy.
	if result.Fetch != nil {
		return installByFetching(result, options)
	}
	if result.Kind == sysdep.KindUdev {
		return installRule(result, options)
	}

	current := detectHost()
	recipe, ok := recipeFor(current, result)
	if !ok {
		return fmt.Errorf("%s: %s", result.Dep, whyNothingCan(result))
	}
	fmt.Fprintf(options.Writer, "%s\n", recipe.Command)
	if options.DryRun {
		return nil
	}
	if err := runRecipe(recipe); err != nil {
		return fmt.Errorf("%s: %w", result.Dep, err)
	}
	fmt.Fprintf(options.Writer, "%s installed\n", result.Dep)
	return nil
}

// installByFetching puts a program in the user's own directory from the vendor's own
// published archive.
//
// Nothing here needs privileges, and the directory it lands in is the one the
// toolchain's own components live in — so a program fetched here is removed by
// removing that directory, and is visible to the person who fetched it.
func installByFetching(result sysdep.CheckResult, options SyncOptions) error {
	fmt.Fprintf(options.Writer, "fetching %s from %s\n", result.Dep, result.Fetch.URL)
	fmt.Fprintf(options.Writer, "  into %s\n", FetchedDir(result.Dep, sysdep.FetchedVersion(depOf(result))))
	if options.DryRun {
		return nil
	}
	path, err := Fetch(depOf(result), func(line string) {
		fmt.Fprintf(options.Writer, "  %s\n", line)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", result.Dep, err)
	}
	if err := verifyFetched(depOf(result)); err != nil {
		return fmt.Errorf("%s: %w", result.Dep, err)
	}
	fmt.Fprintf(options.Writer, "%s installed at %s\n", result.Dep, path)
	fmt.Fprintln(options.Writer, "  no privileges were needed, and nothing outside your own directory changed.")
	fmt.Fprintln(options.Writer, "  it is on the path for rho, without your shell being changed.")
	return nil
}

// installRule puts a udev rule where the host reads it — the one thing here that
// needs root, asked for once, for one file, with the file named.
func installRule(result sysdep.CheckResult, options SyncOptions) error {
	source := strings.TrimSpace(result.RuleSource)
	if source == "" {
		return fmt.Errorf("%s: the module did not say where the rule file is, so there is nothing to copy: %s",
			result.Dep, whyNothingCan(result))
	}
	target := strings.TrimSpace(result.FoundPath)
	fmt.Fprintf(options.Writer, "%s is a file the host reads, so putting it where it belongs needs root.\n",
		result.Dep)
	if options.DryRun {
		fmt.Fprintf(options.Writer, "  would copy %s to %s\n", source, target)
		return nil
	}
	return InstallUdevRule(source, target, options.Writer, options.Reader)
}

// depOf is the dependency a result describes, as far as a recipe needs it. A fetch
// and an install map are both part of the result, so this is a short struct rather
// than a lookup: a result that carried the module's own dependency would be one
// thing to keep in step.
func depOf(result sysdep.CheckResult) sysdep.ExecutableDep {
	return sysdep.ExecutableDep{
		Name:        result.Dep,
		Version:     result.WantVersion,
		Install:     result.Install,
		Fetch:       result.Fetch,
		Description: result.Description,
	}
}

func runRecipe(recipe Recipe) error {
	parts := strings.Fields(recipe.Command)
	if len(parts) == 0 {
		return fmt.Errorf("there is no command to run")
	}
	command := exec.Command(parts[0], parts[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func onlyModule(report sysdep.Report, module string) sysdep.Report {
	out := report
	out.Results = nil
	for _, result := range report.Results {
		if result.Module == module {
			out.Results = append(out.Results, result)
		}
	}
	return out
}

func findRequirement(report sysdep.Report, module string, dependency string) (sysdep.CheckResult, bool) {
	for _, result := range report.Results {
		if result.Module == module && result.Dep == dependency {
			return result, true
		}
	}
	// The module is sometimes left out of a command by hand, and the dependency is
	// unique enough to find on its own.
	if module == "" {
		for _, result := range report.Results {
			if result.Dep == dependency {
				return result, true
			}
		}
	}
	return sysdep.CheckResult{}, false
}

// ask is a yes or no question, defaulting to no.
//
// Defaulting to no because every question here is followed by something with root:
// a person who pressed enter without reading has answered "yes", and the answer
// should be the one that has not changed the machine.
func ask(reader io.Reader, writer io.Writer, question string) (bool, error) {
	fmt.Fprintf(writer, "%s [y/N] ", question)
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
