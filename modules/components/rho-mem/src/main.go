//
// @file main.go
// @brief `rho mem` — where a firmware's memory went, and whether that is allowed.
//
// Two commands, and they are the same analysis with a different question at the
// end. `report` prints and exits; `check` prints and exits non-zero when a budget
// is exceeded or the image grew past its limit. A person reads the text, a
// pipeline reads the exit code, and both are the same numbers.
//
// The arguments are read by hand rather than with a flag package, for the reason
// rho-lint and rho-dbg read their own: both `-flag value` and `--flag=value`
// arrive, from an editor's launch configuration as much as from a person, and a
// flag package that refused the second would fail on configurations that are
// perfectly ordinary.
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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rho/rho-mem/src/rhomem"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// The exit codes, which a pipeline reads.
//
// Three, because "the image does not fit" and "this run could not happen" are
// different failures: the first is the project's to fix and the second is nobody's,
// and a pipeline told the second as the first gets its gate switched off.
const (
	// exitClean means the image is inside every budget.
	exitClean = 0
	// exitOverBudget means a region is over its capacity, or grew past its limit.
	exitOverBudget = 1
	// exitUsage means the command was asked for something that cannot be done.
	exitUsage = 2
	// exitCannotMeasure means nothing could be read to check.
	exitCannotMeasure = 3
)

func run(args []string) int {
	if len(args) == 0 {
		return report(args)
	}
	switch args[0] {
	case "report":
		return report(args[1:])
	case "check":
		return check(args[1:])
	case "-h", "--help", "help":
		usage()
		return exitClean
	}
	// A first argument that is not a command belongs to the report: a project
	// directory is what a person types.
	return report(args)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rho mem [project] [options]        where the program's memory went
       rho mem check [project] [options]   check it against the budgets and fail

options:
  --elf PATH            the program to read
  --map PATH            the linker map to read
  --top N               how many symbols the attribution lists (0 for none)
  --diff REF            compare against the baseline recorded at REF
  --write-baseline      record this run's figures as the baseline
  --width N             how wide the usage bars are drawn
  --bytes               print exact byte counts as well
  --json                write the analysis as a document, for another program
  --no-warn             do not apply the warn threshold
  -o, --out PATH        write the report to a file rather than to standard output`)
}

// options is what a run was asked for, from the command line and the project's
// own declaration, with the command line winning.
type options struct {
	projectRoot string
	elfPath     string
	mapPath     string
	top         int
	topSet      bool
	diffRef     string
	writeBase   bool
	width       int
	bytes       bool
	asJSON      bool
	noWarn      bool
	enforcing   bool
	outPath     string
}

func report(args []string) int { return analyse(args, false) }
func check(args []string) int  { return analyse(args, true) }

// analyse is the run, with or without enforcement.
func analyse(args []string, enforcing bool) int {
	settings := rhomem.ReadSettings()
	options := options{top: settings.Top, width: settings.Width, enforcing: enforcing}

	// The project: the first argument that is not a flag, or the working
	// directory. Which is what a person types and what a pipeline does.
	rest := parseArgs(args, &options)
	if len(rest) > 0 {
		options.projectRoot = rest[0]
	}
	if options.projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho mem:", err)
			return exitUsage
		}
		options.projectRoot = cwd
	}
	if absolute, err := filepath.Abs(options.projectRoot); err == nil {
		options.projectRoot = absolute
	}

	config := rhomem.ReadConfig(options.projectRoot)
	if !config.Declared && options.elfPath == "" && options.mapPath == "" {
		// Not an error: a project with no budgets is a project that has not
		// decided yet, and a report of where its memory went is the thing to look
		// at before deciding. Said, because a check that silently enforced nothing
		// would look exactly like one that passed.
		fmt.Fprintf(os.Stderr,
			"rho mem: %s declares no `mem:` block, so there is nothing to check against; reporting only\n",
			filepath.Base(options.projectRoot))
	}

	// The artifacts: what the project named, or what the build produced.
	artifacts, notes := config.Resolve(options.projectRoot)
	if options.elfPath != "" {
		artifacts.ELFFile = absPath(options.projectRoot, options.elfPath)
	}
	if options.mapPath != "" {
		artifacts.MapFile = absPath(options.projectRoot, options.mapPath)
	}
	for _, note := range notes {
		fmt.Fprintln(os.Stderr, "rho mem:", note)
	}
	if artifacts.ELFFile == "" && artifacts.MapFile == "" {
		fmt.Fprintln(os.Stderr, "rho mem: nothing to analyse; build the project, or name the artifacts with --elf and --map")
		return exitCannotMeasure
	}

	target := rhomem.TargetOf(artifacts, config, options.projectRoot)
	analysis, err := rhomem.Analyze(artifacts.MapFile, artifacts.ELFFile, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho mem:", err)
		return exitCannotMeasure
	}
	// The diagnostics are printed by the report rather than here: a run that says
	// the same thing on stderr and in its output is a run nobody reads twice.
	if len(analysis.Usage) == 0 {
		fmt.Fprintln(os.Stderr, "rho mem: no memory region could be established; declare them under `mem.regions` to check against budgets")
	}

	// The baseline, for a comparison. Named either as a ref to read it from or as
	// the file a previous run recorded.
	var baseline *rhomem.Baseline
	baselinePath := config.BaselinePath(options.projectRoot)
	switch {
	case options.diffRef != "":
		from, err := rhomem.BaselineFromGit(options.projectRoot, options.diffRef, baselinePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho mem:", err)
			return exitCannotMeasure
		}
		baseline = from
	default:
		if isFile(baselinePath) {
			read, err := rhomem.ReadBaseline(baselinePath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "rho mem:", err)
			} else {
				baseline = read
			}
		}
	}
	if baseline != nil && baseline.Target != "" && analysis.Target != "" && baseline.Target != analysis.Target {
		// A comparison across boards is a number without a meaning: the regions
		// and their sizes differ, so "grew by 4 kB" would be comparing two
		// different machines.
		fmt.Fprintf(os.Stderr, "rho mem: the baseline is for %s and this build is for %s; not comparing\n",
			baseline.Target, analysis.Target)
		baseline = nil
	}

	enforcement := config.Enforce
	if options.enforcing {
		// `check` enforces what the project declared, and where it declared
		// nothing it enforces the capacity anyway: a command whose whole purpose
		// is to fail a build that does not fit should not exit zero because the
		// project has not yet written down how much room it has.
		enforcement.FailOnOverflow = true
	}
	budgets := config.Budgets
	if options.noWarn {
		budgets = rhomem.WithoutWarnings(budgets)
	}
	verdict := rhomem.Evaluate(analysis, budgets, enforcement, baseline)

	reportOptions := rhomem.Options{
		Top:   options.top,
		Width: options.width,
		Title: analysis.Target,
		Bytes: options.bytes,
	}
	if options.topSet && options.top == 0 {
		reportOptions.Top = 0
	}

	out := os.Stdout
	if options.outPath != "" {
		file, err := os.Create(absPath(options.projectRoot, options.outPath))
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho mem:", err)
			return exitUsage
		}
		defer file.Close()
		out = file
	}
	if options.asJSON {
		// A document rather than a picture of a terminal, for the runs where the
		// reader is a program. The report is still the default, because a person
		// reading it is the point of most runs.
		if err := rhomem.WriteJSON(out, rhomem.AsResult(analysis, &verdict)); err != nil {
			fmt.Fprintln(os.Stderr, "rho mem:", err)
			return exitUsage
		}
		if baseline != nil && len(verdict.Growth) > 0 {
			rhomem.WriteGrowth(out, &verdict)
		}
		return exitClean
	}

	rhomem.WriteReport(out, analysis, &verdict, reportOptions)
	if baseline != nil && len(verdict.Growth) > 0 {
		rhomem.WriteGrowth(out, &verdict)
	}

	if options.writeBase {
		recorded := rhomem.Record(analysis, reportOptions.Top)
		if err := rhomem.WriteBaseline(baselinePath, recorded); err != nil {
			fmt.Fprintln(os.Stderr, "rho mem:", err)
			return exitUsage
		}
		fmt.Fprintf(out, "\nBaseline recorded: %s\n", baselinePath)
	}

	if !options.enforcing {
		// A report does not fail a build, whatever it found. A report that exited
		// non-zero would be a check, and having two of them spelled differently
		// would be one too many.
		return exitClean
	}
	if verdict.OverBudget || verdict.OverGrowth {
		return exitOverBudget
	}
	return exitClean
}

// parseArgs reads a run's arguments, in both the spellings an editor's launch
// configuration produces and the ones a person types.
func parseArgs(args []string, into *options) []string {
	rest := []string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		// Both `-flag value` and `--flag=value`, and both with one or two
		// dashes, because all four arrive.
		name, value, hasValue := splitFlag(arg)
		// A flag that takes a value and was given as two arguments.
		takeValue := func() (string, bool) {
			if hasValue {
				return value, true
			}
			if index+1 < len(args) {
				index++
				return args[index], true
			}
			return "", false
		}
		switch name {
		case "elf":
			if v, ok := takeValue(); ok {
				into.elfPath = v
			}
		case "map":
			if v, ok := takeValue(); ok {
				into.mapPath = v
			}
		case "top":
			if v, ok := takeValue(); ok {
				if parsed, err := strconv.Atoi(v); err == nil {
					into.top, into.topSet = parsed, true
				}
			}
		case "width":
			if v, ok := takeValue(); ok {
				if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
					into.width = parsed
				}
			}
		case "diff":
			if v, ok := takeValue(); ok {
				into.diffRef = v
			}
		case "out", "o":
			if v, ok := takeValue(); ok {
				into.outPath = v
			}
		case "write-baseline":
			into.writeBase = !isFalse(value, hasValue)
		case "bytes":
			into.bytes = !isFalse(value, hasValue)
		case "json":
			into.asJSON = !isFalse(value, hasValue)
		case "no-warn":
			into.noWarn = !isFalse(value, hasValue)
		case "":
			rest = append(rest, arg)
		default:
			// An unrecognised flag is somebody else's mistake to report, and
			// guessing at it would be worse than saying so.
			fmt.Fprintf(os.Stderr, "rho mem: unknown option %q\n", arg)
			fmt.Fprintln(os.Stderr, "rho mem --help lists what this takes")
		}
	}
	return rest
}

// splitFlag is a flag's name and, if it was written `=value`, its value.
func splitFlag(arg string) (string, string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", "", false
	}
	trimmed := strings.TrimLeft(arg, "-")
	if index := strings.Index(trimmed, "="); index >= 0 {
		return trimmed[:index], trimmed[index+1:], true
	}
	return trimmed, "", false
}

// isFile is whether a path is there and is a file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// absPath is a path a command line gave, resolved against the project.
func absPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

// isFalse is whether a flag written `=false` or `=0` means off.
func isFalse(value string, hasValue bool) bool {
	if !hasValue {
		return false
	}
	switch strings.TrimSpace(value) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}
