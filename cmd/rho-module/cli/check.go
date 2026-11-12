//
// @file check.go
// @brief `rho module check` — whether a module keeps the promise of its exports.
//
// A module's exports are what other modules build against. The check compares
// the module's working tree with its previous release — the version recorded
// when it was published, or the version its manifest declared before in git —
// says what was removed, changed and added, says whether the version the
// manifest now declares may carry that, and lists the modules in the workspace
// that use what a breaking change touches.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"rho/internal/paths"
	"rho/src/exportsig"
)

// Check runs `rho module check [dir]`. It fails when the declared version
// cannot carry the changes.
func Check(args []string) error {
	dir := "."
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Println("usage: rho module check [dir]")
			return nil
		}
		dir = arg
	}
	return check(os.Stdout, dir)
}

func check(out io.Writer, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	current, err := exportsig.Read(abs)
	if err != nil {
		return fmt.Errorf("rho module check: %w", err)
	}
	baseline, found, err := exportsig.FindBaseline(abs, current)
	if err != nil {
		return fmt.Errorf("rho module check: %w", err)
	}
	if !found {
		fmt.Fprintf(out, "%s %s has no earlier release to compare with; it exports %d names.\n", current.Module, current.Version, len(current.Entries))
		return nil
	}
	fmt.Fprintf(out, "%s %s, compared with %s\n", current.Module, current.Version, baseline.Source)
	diff := exportsig.Compare(baseline.Set, current)
	for _, entry := range diff.Removed {
		fmt.Fprintf(out, "  removed  %s\n", entry.Display)
	}
	for _, change := range diff.Changed {
		fmt.Fprintf(out, "  changed  %s\n           was %s\n", change.After.Display, change.Before.Display)
	}
	for _, entry := range diff.Added {
		fmt.Fprintf(out, "  added    %s\n", entry.Display)
	}
	if !diff.Breaking() && len(diff.Added) == 0 {
		fmt.Fprintln(out, "  the exports are unchanged")
	}

	verdict := exportsig.CheckVersion(baseline.Set.Version, current.Version, diff)
	if baseline.Set.Version == current.Version && diff.Breaking() {
		verdict = fmt.Errorf("%s was already released with other exports; a change to them needs a new version", current.Version)
	}

	if diff.Breaking() {
		consumers := exportsig.FindConsumers(abs, current, diff.Names())
		if len(consumers) == 0 {
			fmt.Fprintln(out, "No module in the workspace depends on it.")
		}
		for _, consumer := range consumers {
			relation := "depends on it"
			if consumer.Implements {
				relation = "implements it"
			}
			rel := consumer.Root
			if workspace, _ := paths.Workspace(abs); workspace != "" {
				if workspaceRel, err := filepath.Rel(workspace, consumer.Root); err == nil {
					rel = workspaceRel
				}
			}
			if len(consumer.Uses) == 0 {
				fmt.Fprintf(out, "%s (%s) %s and uses none of what changed\n", consumer.Module, rel, relation)
				continue
			}
			fmt.Fprintf(out, "%s (%s) %s and would break:\n", consumer.Module, rel, relation)
			for _, use := range consumer.Uses {
				fmt.Fprintf(out, "  %s:%d  %s\n", use.File, use.Line, use.Text)
			}
		}
	}

	if verdict != nil {
		return fmt.Errorf("rho module check: %w", verdict)
	}
	if diff.Breaking() {
		fmt.Fprintf(out, "%s may break what %s promised, which its version says.\n", current.Version, baseline.Set.Version)
	} else {
		fmt.Fprintf(out, "%s keeps every export %s promised.\n", current.Version, baseline.Set.Version)
	}
	return nil
}
