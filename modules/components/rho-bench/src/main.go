//
// @file main.go
// @brief `rho bench` — what a firmware costs to run, and what it grew by.
//
//   rho bench                       measure what the project declares
//   rho bench --filter crypto       measure one group of them
//   rho bench --diff main           against the baseline on another branch
//   rho bench check                 fail if a budget or a threshold broke
//
// This component measures; it does not compute anything about the board itself.
// The static footprint is `rho mem`'s and a capture is `rho-monitor`'s, both reached
// as components rather than linked, so that the number in a benchmark report and the
// number in a memory report come from the same code that produces them.
//
// The arguments are read by hand rather than with a flag package, for the reason
// rho-lint and rho-dbg read their own: both `-flag value` and `--flag=value` arrive,
// from an editor's launch configuration as much as from a person.
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
	"time"

	"github.com/rho/rho-bench/src/rhobench"

	rholog "github.com/rho/rho-log/src/logger"
)

// log is this component's own logger, so a run's account of itself is identifiable
// in a log several components write to.
//
// Deliberately not the target's output: that belongs to rho-monitor, and a
// benchmark log going through here would be somebody else's data formatted twice.
var log = rholog.Default("rho-bench")

// The exit codes, which a pipeline reads.
//
// Three, because "the firmware got slower" and "this run could not happen" are
// different failures, and a pipeline told the second as the first has its gate
// switched off.
const (
	// exitClean means nothing broke.
	exitClean = 0
	// exitRegressed means a budget, a threshold or a limit broke.
	exitRegressed = 1
	// exitUsage means the command was asked for something that cannot be done.
	exitUsage = 2
	// exitCannotMeasure means there was nothing to measure.
	exitCannotMeasure = 3
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return bench(args)
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return exitClean
	case "check":
		return check(args[1:])
	}
	return bench(args)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rho bench [project] [options]     measure what this project benchmarks

options:
  --binary PATH         the benchmark program to run
  --filter GLOB         measure only the benchmarks whose name matches
  --target NAME         host, or a board to measure on
  --iterations N        timed runs per benchmark, over the project's declaration
  --warmup N            runs discarded first, over the project's declaration
  --diff REF            compare against the baseline as a git ref carries it
  --write-baseline      record this run's figures as the baseline
  --no-footprint        skip the static figure, which is rho-mem's
  --json                write the run as a document rather than a table
  --color / --no-color  colour the change verdicts
  --timeout SECONDS     how long the program may take
  -o, --out PATH        write the report here rather than to standard output

  rho bench check [options]    the same measurement, failing on a budget or a
                               regression over the declared limit`)
}

// options is what a run was asked for.
type options struct {
	projectRoot string
	binary      string
	filter      string
	target      string
	iterations  int
	warmup      int
	diffRef     string
	writeBase   bool
	footprint   bool
	asJSON      bool
	color       bool
	timeout     time.Duration
	outPath     string
	enforcing   bool
}

func bench(args []string) int { return measure(args, false) }
func check(args []string) int { return measure(args, true) }

// measure is the run.
func measure(args []string, enforcing bool) int {
	opts := parseArgs(args)
	opts.enforcing = enforcing

	config := rhobench.ReadSettings(opts.projectRoot)
	if opts.iterations > 0 {
		config.Iterations = opts.iterations
	}
	if opts.warmup > 0 {
		config.Warmup = opts.warmup
	}
	target := opts.target
	if target == "" {
		target = "host"
		if len(config.Targets) > 0 {
			target = config.Targets[0]
		}
	}

	note := func(message string) {
		log.Info(message)
		if !opts.asJSON {
			fmt.Fprintln(os.Stderr, "rho bench:", message)
		}
	}

	// What to run. A project that has not said, and has no conventional binary
	// either, is reported rather than run and found to be nothing.
	binary := opts.binary
	if binary == "" {
		binary = conventionalBinary(opts.projectRoot)
	}
	if binary == "" {
		note("no benchmark program: build one, or say which with --binary")
		return exitCannotMeasure
	}

	execution, err := rhobench.RunBenchmark(rhobench.RunOptions{
		Binary:     binary,
		Target:     target,
		Filter:     opts.filter,
		Iterations: config.Iterations,
		Warmup:     config.Warmup,
		Timeout:    opts.timeout,
		WorkDir:    opts.projectRoot,
	})
	if err != nil {
		note(err.Error())
		return exitCannotMeasure
	}

	// What the numbers are *for*, which is not what was asked for.
	//
	// The harness knows where it ran; the caller only knows where they wanted it to
	// run. Taking the caller's word for it would put a board's name on a host's
	// cycles — and a baseline recorded for that board would then be compared against
	// them, which is a number with no meaning. The harness's own answer wins, and a
	// disagreement is said rather than resolved in favour of the request.
	measured := execution.Report.Target
	switch {
	case measured != "" && target != "" && measured != target:
		note(fmt.Sprintf("the benchmark ran on %s and this run asked for %s; "+
			"reporting its numbers as %s, because that is where they were measured",
			measured, target, measured))
	case measured == "":
		measured = target
	}

	results := execution.Results(rhobench.RunOptions{Filter: opts.filter})
	if len(results) == 0 {
		note(fmt.Sprintf("%s reported %d benchmarks and none of them matched %q",
			filepath.Base(binary), len(execution.Report.Benchmarks), opts.filter))
		return exitCannotMeasure
	}

	summary := rhobench.Summary{
		Results:     results,
		Target:      measured,
		Duration:    execution.Duration.Nanoseconds(),
		Diagnostics: execution.Diagnostics,
	}

	// The static figure, from the component that owns it.
	if opts.footprint {
		footprint, notes := rhobench.ReadFootprint(rhobench.FootprintOptions{
			ProjectRoot: opts.projectRoot,
			Target:      target,
		})
		summary.Footprint = footprint
		summary.Diagnostics = append(summary.Diagnostics, notes...)
	}

	// The baseline, from a ref or from the file a previous run recorded.
	var baseline *rhobench.Baseline
	baselinePath := config.BaselinePath(opts.projectRoot)
	switch {
	case opts.diffRef != "":
		from, err := rhobench.BaselineFromGit(opts.projectRoot, opts.diffRef, baselinePath)
		if err != nil {
			note(err.Error())
			return exitCannotMeasure
		}
		baseline = from
	default:
		if read, err := rhobench.ReadBaseline(baselinePath); err == nil {
			baseline = read
		}
	}
	if baseline != nil && baseline.Target != "" && measured != "" && baseline.Target != measured {
		// A host's cycles and a board's cycles are different units. Comparing them
		// produces a number, and the number means nothing.
		note(fmt.Sprintf("the baseline is for %s and this run is for %s; not comparing",
			baseline.Target, measured))
		baseline = nil
	}
	if baseline != nil {
		summary.Baseline = baseline
		summary.Changes = rhobench.Changed(results, baseline)
		summary.Regressions = rhobench.Regressions(summary.Changes, config.Budgets)
	}
	for _, result := range results {
		summary.BudgetFailures = append(summary.BudgetFailures,
			rhobench.OverBudget(result, config.Budgets)...)
	}

	out := os.Stdout
	if opts.outPath != "" {
		file, err := os.Create(resolve(opts.projectRoot, opts.outPath))
		if err != nil {
			note(err.Error())
			return exitUsage
		}
		defer file.Close()
		out = file
	}
	if opts.asJSON {
		if err := rhobench.WriteJSON(out, summary); err != nil {
			note(err.Error())
			return exitUsage
		}
	} else {
		rhobench.WriteReport(out, summary, rhobench.Options{
			Target: measured, Color: opts.color && isTerminal(os.Stdout),
			Baseline: baseline, Width: rhobench.DefaultWidth,
		})
	}

	if opts.writeBase {
		recorded := rhobench.Recorded(results, measured, binary)
		if err := rhobench.WriteBaseline(baselinePath, recorded); err != nil {
			note(err.Error())
			return exitUsage
		}
		if !opts.asJSON {
			fmt.Fprintf(out, "\nBaseline recorded: %s\n", baselinePath)
		}
	}

	log.Info("benchmark run finished",
		"target", measured, "requested", target, "binary", binary,
		"benchmarks", len(results), "regressions", len(summary.Regressions),
		"failures", len(summary.BudgetFailures))

	if !opts.enforcing {
		// A report does not fail a build whatever it found; `check` is what does.
		return exitClean
	}
	if len(summary.Regressions) > 0 || len(summary.BudgetFailures) > 0 {
		return exitRegressed
	}
	return exitClean
}

// conventionalBinary is where a project's benchmark program is, when it did not say.
//
// A small list rather than a scan of `build/`: a build tree holds object files, a
// CMake cache and one intermediate directory per board, and looking for a program in
// all of it finds the one from whichever board was built last — which is the wrong
// answer whenever a project builds for several.
func conventionalBinary(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, "build", "bench", "bench"),
		filepath.Join(projectRoot, "build", "bench", "bench.exe"),
		filepath.Join(projectRoot, "build", "output", "bench"),
		filepath.Join(projectRoot, "build", "output", "bench.exe"),
		filepath.Join(projectRoot, "bench"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// parseArgs reads a run's arguments, in both the spellings that arrive.
func parseArgs(args []string) options {
	opts := options{color: true, footprint: true}
	rest := []string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		trimmed := strings.TrimLeft(arg, "-")
		name, value, hasValue := trimmed, "", false
		if split := strings.Index(trimmed, "="); split >= 0 {
			name, value, hasValue = trimmed[:split], trimmed[split+1:], true
		}
		take := func() (string, bool) {
			if hasValue {
				return value, true
			}
			if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
				index++
				return args[index], true
			}
			return "", false
		}
		switch name {
		case "binary":
			if v, ok := take(); ok {
				opts.binary = v
			}
		case "filter":
			if v, ok := take(); ok {
				opts.filter = v
			}
		case "target":
			if v, ok := take(); ok {
				opts.target = v
			}
		case "iterations":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
					opts.iterations = parsed
				}
			}
		case "warmup":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
					opts.warmup = parsed
				}
			}
		case "diff":
			if v, ok := take(); ok {
				opts.diffRef = v
			}
		case "out", "o", "report":
			if v, ok := take(); ok {
				opts.outPath = v
			}
		case "timeout":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
					opts.timeout = time.Duration(parsed) * time.Second
				}
			}
		case "write-baseline":
			opts.writeBase = !isFalse(value, hasValue)
		case "footprint":
			opts.footprint = !isFalse(value, hasValue)
		case "no-footprint":
			opts.footprint = false
		case "json":
			opts.asJSON = !isFalse(value, hasValue)
		case "color", "colour":
			opts.color = !isFalse(value, hasValue)
		case "no-color", "no-colour":
			opts.color = false
		case "h", "help":
			usage()
			os.Exit(exitClean)
		default:
			fmt.Fprintf(os.Stderr, "rho bench: unknown option %q\n", arg)
			fmt.Fprintln(os.Stderr, "rho bench --help lists what this takes")
		}
	}
	if len(rest) > 0 {
		opts.projectRoot = rest[0]
	}
	if opts.projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.projectRoot = cwd
		}
	}
	if absolute, err := filepath.Abs(opts.projectRoot); err == nil {
		opts.projectRoot = absolute
	}
	return opts
}

// resolve is a path a command line gave, against the project.
func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

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

// isTerminal is whether standard output is a terminal, which is what decides whether
// colour is written at all.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
