//
// @file report.go
// @brief What a report says, to a person and to a program.
//
// Two renderings of one report, and the second is the first's data rather than a
// second reading of the machine. `rho keeper` acts on the JSON and the dashboard
// shows it, and both act on what `rho doctor` prints — so a person who reads the
// text and a program that reads the JSON cannot be told different things about the
// same machine.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodoctor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"rho/src/sysdep"
)

// JSON is the report as another program reads it.
func JSON(report sysdep.Report) ([]byte, error) {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render the report: %w", err)
	}
	return append(encoded, '\n'), nil
}

// Text is the report as a person reads it.
//
// Grouped by module, because a missing program is one module's problem and a person
// wants to know *whose* board is not going to work. Within a module, ordered by how
// much each item stops: what is missing first, then what is too old, then what is
// worth knowing.
func Text(report sysdep.Report) string {
	out := &strings.Builder{}
	counts := report.Tally()

	if len(report.Results) == 0 {
		fmt.Fprintln(out, "Nothing on this machine needs checking: no installed module has declared")
		fmt.Fprintln(out, "what it needs from the system.")
		return out.String()
	}

	fmt.Fprintf(out, "%s\n\n", headline(counts, len(report.Results)))
	for _, module := range modulesIn(report) {
		fmt.Fprintf(out, "%s\n", module)
		for _, result := range resultsOf(report, module) {
			fmt.Fprintf(out, "  %-9s %-14s %s\n", result.Status, result.Dep, result.Message)
			for _, line := range detailLines(result) {
				fmt.Fprintf(out, "  %-9s %-14s %s\n", "", "", line)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "checked %s\n", strings.Join(report.Scanned, ", "))
	return out.String()
}

// headline is the one line a person reads before anything else: whether they have
// a problem, and of what kind.
func headline(counts sysdep.Counts, total int) string {
	switch {
	case counts.Missing > 0 && counts.Outdated > 0:
		return fmt.Sprintf("%d checks: %d missing, %d too old. Nothing that needs them will run.",
			total, counts.Missing, counts.Outdated)
	case counts.Missing > 0:
		return fmt.Sprintf("%d checks: %d missing. Nothing that needs them will run.", total, counts.Missing)
	case counts.Outdated > 0:
		return fmt.Sprintf("%d checks: %d too old. They may run and be wrong.", total, counts.Outdated)
	case counts.Unknown > 0:
		return fmt.Sprintf("%d checks: all satisfied, and %d could not be read.", total, counts.Unknown)
	default:
		return fmt.Sprintf("%d checks, all satisfied.", total)
	}
}

// detailLines are the extra lines under a result: where a program is, what was
// wanted, and how to get it. Only the ones that say something.
func detailLines(result sysdep.CheckResult) []string {
	out := []string{}
	// Only what a person who is about to do something needs. A recipe beside an
	// entry that is already right is noise, and a report full of noise is a report
	// whose one real line goes unread.
	if !result.Status.NeedsWork() {
		return out
	}
	for _, recipe := range recipes(result) {
		out = append(out, recipe)
	}
	out = append(out, "fix it with: rho keeper sync")
	return out
}

// recipes is the install recipe for one requirement, one line per package manager,
// and what to do when there is no manager that has it.
func recipes(result sysdep.CheckResult) []string {
	managers := make([]string, 0, len(result.Install))
	for manager := range result.Install {
		if manager != "manual" {
			managers = append(managers, manager)
		}
	}
	sort.Strings(managers)
	out := []string{}
	for _, manager := range managers {
		out = append(out, "install with "+manager+": "+result.Install[manager])
	}
	// `manual` last, and only when it is the only way — which is what "not in a
	// package manager here" means, and a claim not to make when a manager has it.
	if manual := strings.TrimSpace(result.Install["manual"]); manual != "" && len(managers) == 0 {
		out = append(out, "not in any package manager: "+manual)
	}
	return out
}

// modulesIn is the modules the report has results for, in name order.
func modulesIn(report sysdep.Report) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, result := range report.Results {
		if result.Module == "" || seen[result.Module] {
			continue
		}
		seen[result.Module] = true
		out = append(out, result.Module)
	}
	sort.Strings(out)
	return out
}

func resultsOf(report sysdep.Report, module string) []sysdep.CheckResult {
	out := []sysdep.CheckResult{}
	for _, result := range report.Results {
		if result.Module == module {
			out = append(out, result)
		}
	}
	return out
}

// Summary is the report in one line, for a status bar and for the dashboard's
// header.
func Summary(report sysdep.Report) string {
	counts := report.Tally()
	if counts.Missing == 0 && counts.Outdated == 0 {
		return fmt.Sprintf("system ok (%d checked)", len(report.Results))
	}
	return fmt.Sprintf("system needs attention (%d missing, %d too old)", counts.Missing, counts.Outdated)
}
