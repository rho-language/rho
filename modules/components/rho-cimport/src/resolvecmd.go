//
// @file resolvecmd.go
// @brief `rho cimport resolve` — find the preset a build is for.
//
// The build's question, asked of the component that knows about presets. It
// prints what building the named preset needs as JSON: the C sources to fetch,
// the hardware they are for, and the target spec that builds them.
//
// JSON because the reader is not this component: the toolchain reads it and
// hands the target to rho-target and the repositories to this component's own
// dependency manager. A format one side invented is a format the other has to be
// taught, and the teaching is the coupling this avoids.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rho/rho-cimport/src/cimport"
)

// resolveCmd finds a preset and reports what building it needs.
func resolveCmd(args []string) int {
	fs := flag.NewFlagSet("rho cimport resolve", flag.ExitOnError)
	project := fs.String("project", "", "the application the build is for")
	preset := fs.String("preset", "", "the preset to resolve")
	fs.Parse(args)

	if *preset == "" {
		fmt.Fprintln(os.Stderr, "rho cimport resolve: --preset names the preset to resolve")
		return 2
	}
	root, err := projectRoot(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport resolve:", err)
		return 1
	}

	resolver := &cimport.PresetResolver{ProjectRoot: root}
	resolution, err := resolver.Resolve(*preset)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport resolve:", err)
		return 1
	}
	if err := cimport.WriteResolution(os.Stdout, resolution); err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport resolve:", err)
		return 1
	}
	return 0
}
