//
// @file deps.go
// @brief `rho cimport deps` and `rho cimport preset` — the C dependency manager.
//
// The two commands a build uses and a person asks questions with. `preset` says
// what a project builds against; `deps` fetches it and says what that turned
// into. Both are read-only with respect to the project: neither writes to it,
// and a build arranges itself from what they report.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"rho/src/deps"

	"github.com/rho/rho-cimport/src/cimport"
)

// presetCmd prints the preset a project builds against.
func presetCmd(args []string) int {
	fs := flag.NewFlagSet("rho cimport preset", flag.ExitOnError)
	project := fs.String("project", "", "the project to read (default: the working directory)")
	fs.Parse(args)

	root, err := projectRoot(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport preset:", err)
		return 1
	}

	preset, err := cimport.ReadPreset(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport preset:", err)
		return 1
	}
	if preset.Name == "" {
		fmt.Fprintf(os.Stderr, "rho cimport preset: %s builds for nothing in particular; it names no preset\n", root)
		return 1
	}

	if preset.Description != "" {
		fmt.Println(preset.Description)
		fmt.Println()
	}
	fmt.Println("preset:", preset.Name)
	reportHardware(preset)
	if len(preset.Dependencies) == 0 {
		return 1
	}
	for _, one := range preset.Dependencies {
		source := one.Git
		if one.Path != "" {
			source = one.Path
		}
		if source != "" && one.Revision() != "" && one.Branch != "" {
			source += " @ " + one.Revision()
		} else if one.Git != "" {
			source += " @ " + one.Revision()
		}
		fmt.Printf("  %-24s %s\n", one.Name, source)
		for _, include := range one.Include {
			fmt.Printf("      include %s\n", include)
		}
		for _, src := range one.Src {
			fmt.Printf("      src     %s\n", src)
		}
	}
	return 0
}

// reportHardware prints the descriptive half of a preset.
func reportHardware(preset cimport.Preset) {
	rows := [][2]string{
		{"family", preset.Family},
		{"mcu", preset.MCU},
		{"core", preset.Core},
		{"device define", preset.DeviceDefine},
		{"board", preset.Board},
		{"external clock", preset.ExternalClock},
		{"flash offset", preset.FlashOffset},
	}
	for _, row := range rows {
		if row[1] != "" {
			label := row[0]
			fmt.Println("  "+label, row[1])
		}
	}
}

// depsCmd fetches a project's C dependencies and reports what they contribute.
func depsCmd(args []string) int {
	fs := flag.NewFlagSet("rho cimport deps", flag.ExitOnError)
	project := fs.String("project", "", "the project whose dependencies these are")
	fetch := fs.Bool("fetch", true, "fetch what is not already fetched")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	fs.Parse(args)

	root, err := projectRoot(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport deps:", err)
		return 1
	}

	preset, err := cimport.ReadPreset(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport deps:", err)
		return 1
	}
	if preset.Name == "" {
		fmt.Fprintf(os.Stderr, "rho cimport deps: %s names no preset, so it has no C dependencies to fetch\n", root)
		return 1
	}
	if err := preset.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport deps:", err)
		return 1
	}

	cache, err := cimport.CacheDir(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport deps:", err)
		return 1
	}

	resolver := &cimport.Resolver{ProjectRoot: root, Cache: cache}
	if !*fetch || *quiet {
		// A check must not reach the network, and a check that fetched would
		// make a build depend on whether it had run before.
		resolver.Out = nil
	} else {
		resolver.Out = os.Stderr
	}
	if !*fetch {
		// Reading only: report what is already there without fetching anything.
		resolver.Out = nil
	}

	resolution, err := resolver.Resolve(preset)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport deps:", err)
		return 1
	}

	if *quiet {
		if len(resolution.SourceFiles) == 0 && len(resolution.IncludeDirs) == 0 {
			return 1
		}
		return 0
	}

	fmt.Println("preset:", preset.Describe())
	if len(resolution.Fetched) > 0 {
		fmt.Println("fetched:", joinNames(resolution.Fetched))
	}
	if len(resolution.Missing) > 0 {
		fmt.Fprintln(os.Stderr, "not found:", joinNames(resolution.Missing))
	}
	fmt.Printf("include dirs: %d\n", len(resolution.IncludeDirs))
	for _, dir := range resolution.IncludeDirs {
		fmt.Println("  ", dir)
	}
	fmt.Printf("sources: %d\n", len(resolution.SourceFiles))
	return 0
}

// projectRoot is the directory a command works on, defaulting to here.
func projectRoot(given string) (string, error) {
	root := given
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

// joinNames is a list of names, separated the way a sentence would.
func joinNames(names []string) string {
	out := ""
	for index, name := range names {
		switch {
		case index == 0:
			out = name
		case index == len(names)-1:
			out += " and " + name
		default:
			out += ", " + name
		}
	}
	return out
}

// ensure the shared manifest reader stays linked in: the project root and the
// Rho home a build uses both come from it, and a project that declares its
// build section is read through it.
var _ = deps.RhoHome
