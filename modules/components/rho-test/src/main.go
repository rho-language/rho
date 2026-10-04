//
// @file main.go
// @brief The `rho test` command.
//
// A run is four questions — what does this project have, which of it is wanted,
// who can run it, what came of it — and each is asked from here by handing
// `rhotest` a set of options. Nothing about a run is decided in this file, which
// is what keeps the same run reachable from a library, from a test of the runner
// itself, and from the standalone binary.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
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

// log is the runner's own name in a shared log, so a test run is identifiable
// without every line having to say so. Nothing here logs through it: the runner
// reports on stdout where a person can read it, and this is here so that a log a
// project collects already carries the name of whatever wrote to it.
var log = rholog.Default("rho-test")

func main() {
	os.Exit(run(os.Args[1:]))
}

// usage is what a person gets when they get it wrong. It leads with the
// selection flags, because those are what a person reaches for: a run over a
// project with thirty suites wants a way to say which five.
const usage = `Usage: rho test [path] [options]

  What to run
    --suite NAME       run this suite; repeatable, and a matrix's base name
                       runs every cell of it
    --kind KIND        run suites of this kind (unit, smoke, ...)
    --tag TAG          run suites carrying this tag; repeatable
    --without TAG      skip suites carrying this tag; repeatable

  What to report
    --list             say what the project declares and stop
    --dry-run          say what this run would do, without doing it
    --verbose          print each step as it is taken
    --echo             print a suite's output whether it passed or failed
    --report-dir DIR   where a plugin's report is written

  How to run
    --target TARGET    the backend Rho tests run on (default c)
    --rho PATH         the rho to build tests with
    --list-plugins     list the plugins that extend this runner
    --no-plugins       run without them

  A path names a project or a test file. Without one, the working directory is
  the project, and its tests/tests.yml says what it has.`

func run(args []string) int {
	options := rhotest.Options{Target: rhotest.DefaultTarget}
	rest := []string{}
	noPlugins := false
	listPlugins := false
	selection := rhotest.Selection{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func(flag, example string) (string, bool) {
			if i+1 >= len(args) {
				fmt.Printf("rho test: %s needs a value\n  %s\n", flag, example)
				return "", false
			}
			i++
			return args[i], true
		}
		switch arg {
		case "-h", "--help", "help":
			fmt.Println(usage)
			return 0
		case "-no-plugins", "--no-plugins":
			noPlugins = true
		case "-list-plugins", "--list-plugins":
			listPlugins = true
		case "-list", "--list":
			options.List = true
		case "-dry-run", "--dry-run":
			options.DryRun = true
		case "-verbose", "--verbose", "-v":
			options.Verbose = true
		case "-echo", "--echo":
			options.Echo = true
		case "-report-dir", "--report-dir":
			value, ok := next(arg, "rho test --report-dir build/test-reports")
			if !ok {
				return 2
			}
			options.ReportDir = value
		case "-target", "--target":
			value, ok := next(arg, "rho test --target go")
			if !ok {
				return 2
			}
			options.Target = value
		case "-rho", "--rho":
			value, ok := next(arg, "rho test --rho ./bin/rho")
			if !ok {
				return 2
			}
			options.RhoBinary = value
		case "-suite", "--suite":
			value, ok := next(arg, "rho test --suite error")
			if !ok {
				return 2
			}
			selection.Names = append(selection.Names, value)
		case "-kind", "--kind":
			value, ok := next(arg, "rho test --kind unit")
			if !ok {
				return 2
			}
			selection.Kinds = append(selection.Kinds, value)
		case "-tag", "--tag":
			value, ok := next(arg, "rho test --tag platform")
			if !ok {
				return 2
			}
			selection.Tags = append(selection.Tags, value)
		case "-without", "--without":
			value, ok := next(arg, "rho test --without slow")
			if !ok {
				return 2
			}
			selection.Excluded = append(selection.Excluded, value)
		default:
			rest = append(rest, arg)
		}
	}
	options.Selection = selection
	if len(rest) > 1 {
		fmt.Printf("rho test: expected one path, got %d\n  %s\n", len(rest), usage)
		return 2
	}
	if len(rest) == 1 {
		options.Path = rest[0]
		options.Root = rhotest.DefaultRoot(rest[0])
	} else {
		options.Root = rhotest.DefaultRoot("")
	}

	// The plugins are found before the run rather than during it, so a run is
	// either answered by them or not at all, and never halfway.
	if !noPlugins {
		source := rhotest.Source{Tree: options.Root, RhoHome: deps.RhoHome()}
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
