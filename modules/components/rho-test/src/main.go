//
// @file main.go
// @brief Standalone Rho test runner.
//
// The implementation lives in src/rhotest, which `rho test` calls directly.
// This binary is the same runner for use on its own, or in a shell that
// should not go through the rho CLI.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
	"strings"

	"rho/src/deps"

	rholog "github.com/rho/rho-log/src/logger"
	"github.com/rho/rho-test/src/rhotest"
)

// log is the runner's own logger, so a test run is identifiable in a shared log
// without every line having to say so.
var log = rholog.Default("rho-test")

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// A test is an ordinary program, so it runs on one backend. The default is
	// the native one; a Go project is tested with --target go.
	options := rhotest.Options{Target: rhotest.DefaultTarget}
	rest := []string{}
	noPlugins := false
	listPlugins := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-no-plugins", "--no-plugins":
			noPlugins = true
		case "-list-plugins", "--list-plugins":
			listPlugins = true
		case "-report-dir", "--report-dir":
			if i+1 >= len(args) {
				fmt.Println("Usage: rho-test <path> [--report-dir DIR]")
				return 2
			}
			options.ReportDir = args[i+1]
			i++
		case "-target", "--target":
			if i+1 >= len(args) {
				fmt.Println("Usage: rho-test <path> [--target c|go]")
				return 2
			}
			options.Target = args[i+1]
			i++
		case "-rho", "--rho":
			if i+1 >= len(args) {
				fmt.Println("Usage: rho-test <path> [--rho <path-to-rho>]")
				return 2
			}
			options.RhoBinary = args[i+1]
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) < 1 {
		fmt.Println("Usage: rho-test <path> [--target c|go] [--rho <path-to-rho>]")
		return 2
	}
	options.Path = rest[0]

	// The plugins are found before the run rather than during it, so a run is
	// either answered by them or not at all, and never halfway.
	if !noPlugins {
		source := rhotest.Source{Tree: options.Path, RhoHome: deps.RhoHome()}
		if listPlugins {
			list(rhotest.DiscoverPlugins(source))
			return 0
		}
		options.Plugins = rhotest.WithPlugins(rhotest.DiscoverPlugins(source))
	}
	return rhotest.Run(options)
}

// list names the plugins that extend this runner, so an author can see whether
// the one they wrote is the one that would run.
func list(found []rhotest.Plugin) {
	if len(found) == 0 {
		fmt.Println("No plugin extends the test runner.")
		return
	}
	fmt.Printf("%d plugin(s) extend the test runner:\n", len(found))
	for _, plugin := range found {
		hooks := []string{}
		for _, command := range plugin.Commands {
			hooks = append(hooks, command.Stage())
		}
		fmt.Printf("  %s %s hooks: %s\n", plugin.Name, plugin.Version, strings.Join(hooks, ", "))
	}
}
