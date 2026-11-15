package main

import (
	"fmt"
	"os"

	"rho/internal/paths"
	clicore "rho/src/cli"
)

// envCLI is `rho env`: what the toolchain resolved about this machine and this
// directory, and where each answer came from. Paths in a manifest are written
// against these, so a build that cannot find a sibling is answered here first.
func envCLI(args []string) int {
	base := "."
	if clicore.WantsHelp(args) {
		fmt.Println("usage: rho env [<project-dir>]")
		fmt.Println("Prints RHO_HOME and RHO_WORKSPACE, with where each value came from.")
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: rho env [<project-dir>]")
		return 2
	}
	if len(args) > 0 {
		base = args[0]
	}
	home, err := paths.GetRhoHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho env:", err)
		return 1
	}
	homeSource := "default"
	if os.Getenv(paths.RhoHomeEnvVar) != "" {
		homeSource = "$" + paths.RhoHomeEnvVar
	}
	workspace, source := paths.Workspace(base)
	fmt.Printf("%-18s %s  (%s)\n", "RHO_HOME", home, homeSource)
	fmt.Printf("%-18s %s  (%s)\n", paths.WorkspaceEnvVar, workspace, source)
	found := paths.WorkspaceVariables()
	for _, name := range paths.SortedKeys(found) {
		if name == paths.WorkspaceEnvVar {
			continue
		}
		fmt.Printf("%-18s %s  ($%s)\n", name, found[name], name)
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "rho env: the workspace %s is not a directory\n", workspace)
		return 1
	}
	return 0
}
