//
// @file main.go
// @brief `rho doctor` — what this machine has, against what it needs.
//
// One command, and it changes nothing. Every module that says what it needs from
// the machine is checked against the machine, and the answer is printed or given as
// JSON for something else to read. Installing what is missing is `rho keeper`'s
// half, deliberately: this reports, that acts, and a report that had installed
// things would be a report you could not tell from the machine it had changed.
//
// @project Rho
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

	rholog "github.com/rho/rho-log/src/logger"

	"github.com/rho/rho-doctor/src/rhodoctor"
)

// log is this component's own logger, so a health check is identifiable in a log
// several components write to.
var log = rholog.Default("rho-doctor")

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	options := rhodoctor.Options{}
	asJSON := false
	quiet := false

	for index := 0; index < len(args); index++ {
		argument := strings.TrimLeft(args[index], "-")
		name, value, hasValue := strings.Cut(argument, "=")
		take := func() (string, bool) {
			if hasValue {
				return value, true
			}
			if index+1 < len(args) {
				index++
				return args[index], true
			}
			return "", false
		}
		var err error
		switch name {
		case "json":
			asJSON = true
		case "quiet", "q":
			// The one line, for a status bar or a prompt.
			quiet = true
		case "component", "module":
			if given, ok := take(); ok {
				options.Module = given
			} else {
				fmt.Fprintln(os.Stderr, "rho doctor: --component needs a module name")
				return 2
			}
		case "enabled", "active":
			options.EnabledOnly = true
		case "help", "h":
			usage(os.Stdout)
			return 0
		default:
			err = fmt.Errorf("unknown flag %q", args[index])
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho doctor:", err)
			usage(os.Stderr)
			return 2
		}
	}

	report := rhodoctor.Check(options)
	log.Info("system checked", "modules", len(report.Scanned), "checks", len(report.Results))

	switch {
	case quiet:
		fmt.Println(rhodoctor.Summary(report))
	case asJSON:
		encoded, err := rhodoctor.JSON(report)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho doctor:", err)
			return 1
		}
		os.Stdout.Write(encoded)
	default:
		fmt.Print(rhodoctor.Text(report))
	}

	// The exit status is a question a script asks: a report that found work to do
	// is not a report that failed, and conflating the two makes `rho doctor` unusable
	// in the one place it is most useful — deciding whether to go and fix something.
	if !report.Healthy() {
		return 1
	}
	return 0
}

func usage(out *os.File) {
	fmt.Fprint(out, `usage: rho doctor [flags]

  What every installed module needs from this machine, and what this machine has.
  Changes nothing: installing what is missing is `+"`rho keeper sync`"+`.

flags:
  --json              the report as JSON, for rho keeper and for an editor
  --component NAME    check one module rather than all of them
  --enabled           check only the modules that are enabled
  --quiet, -q         one line, for a status bar

exit status:
  0  nothing is missing and nothing is too old
  1  something is, or something could not be read
`)
}
